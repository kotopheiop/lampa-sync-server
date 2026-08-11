package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store persists progress and favorite JSON files with a process-wide mutex.
type Store struct {
	dataDir      string
	progressPath string
	favoritePath string
	mu           sync.Mutex
}

// New creates a Store rooted at dataDir and ensures the directory exists.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dataDir:      dataDir,
		progressPath: filepath.Join(dataDir, "progress.json"),
		favoritePath: filepath.Join(dataDir, "favorite.json"),
	}, nil
}

// Init loads (or creates) both data files.
func (s *Store) Init() error {
	if _, err := s.ReadProgress(); err != nil {
		return fmt.Errorf("progress: %w", err)
	}
	if _, err := s.ReadFavorite(); err != nil {
		return fmt.Errorf("favorite: %w", err)
	}
	return nil
}

// ReadProgress returns the full progress map.
func (s *Store) ReadProgress() (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readProgressUnlocked()
}

// WriteProgress replaces the progress map on disk.
func (s *Store) WriteProgress(data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeProgressUnlocked(data)
}

// ReadFavorite returns the normalized favorite object.
func (s *Store) ReadFavorite() (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readFavoriteUnlocked()
}

// WriteFavorite normalizes and persists favorite.
func (s *Store) WriteFavorite(favorite map[string]interface{}) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeFavoriteUnlocked(favorite)
}

// WithLock runs fn while holding the store mutex (for multi-step updates).
func (s *Store) WithLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn()
}

// ReadProgressUnlocked must be called under WithLock / existing lock.
func (s *Store) ReadProgressUnlocked() (map[string]interface{}, error) {
	return s.readProgressUnlocked()
}

// WriteProgressUnlocked must be called under WithLock / existing lock.
func (s *Store) WriteProgressUnlocked(data map[string]interface{}) error {
	return s.writeProgressUnlocked(data)
}

// ReadFavoriteUnlocked must be called under WithLock / existing lock.
func (s *Store) ReadFavoriteUnlocked() (map[string]interface{}, error) {
	return s.readFavoriteUnlocked()
}

// WriteFavoriteUnlocked must be called under WithLock / existing lock.
func (s *Store) WriteFavoriteUnlocked(favorite map[string]interface{}) (map[string]interface{}, error) {
	return s.writeFavoriteUnlocked(favorite)
}

func (s *Store) readProgressUnlocked() (map[string]interface{}, error) {
	return readJSONObject(s.progressPath, map[string]interface{}{})
}

func (s *Store) writeProgressUnlocked(data map[string]interface{}) error {
	return writeJSONFile(s.progressPath, data)
}

func (s *Store) readFavoriteUnlocked() (map[string]interface{}, error) {
	data, err := os.ReadFile(s.favoritePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			progress, err := s.readProgressUnlocked()
			if err != nil {
				return nil, err
			}
			merged := EmptyFavorite()
			for _, raw := range progress {
				rec, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if fav, ok := rec["favorite"].(map[string]interface{}); ok {
					merged = MergeFavorite(merged, fav)
					delete(rec, "favorite")
				}
			}
			merged["updated"] = time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := s.writeFavoriteUnlocked(merged); err != nil {
				return nil, err
			}
			if err := s.writeProgressUnlocked(progress); err != nil {
				return nil, err
			}
			return merged, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return EmptyFavorite(), nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return NormalizeFavoriteObject(parsed), nil
}

func (s *Store) writeFavoriteUnlocked(favorite map[string]interface{}) (map[string]interface{}, error) {
	normalized := NormalizeFavoriteObject(favorite)
	if normalized["updated"] == nil || normalized["updated"] == "" {
		normalized["updated"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := writeJSONFile(s.favoritePath, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func readJSONObject(path string, fallback map[string]interface{}) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := writeJSONFile(path, fallback); err != nil {
				return nil, err
			}
			return cloneMap(fallback), nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cloneMap(fallback), nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		backup := fmt.Sprintf("%s.backup.%d", path, time.Now().UnixMilli())
		_ = os.Rename(path, backup)
		log.Println("Invalid JSON, backup:", backup)
		if err := writeJSONFile(path, fallback); err != nil {
			return nil, err
		}
		return cloneMap(fallback), nil
	}
	return parsed, nil
}

func writeJSONFile(path string, v interface{}) error {
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
