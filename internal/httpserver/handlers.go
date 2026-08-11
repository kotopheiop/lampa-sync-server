package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/kotopheiop/lampa-sync-server/internal/config"
	"github.com/kotopheiop/lampa-sync-server/internal/store"
	"github.com/kotopheiop/lampa-sync-server/internal/version"
)

// Server is the HTTP API for Lampa sync.
type Server struct {
	cfg   config.Config
	store *store.Store
}

// New wires config and store into an HTTP server.
func New(cfg config.Config, st *store.Store) *Server {
	return &Server{cfg: cfg, store: st}
}

// Handler returns the root HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ping", s.withAuth(s.handlePing))
	mux.HandleFunc("/sync", s.withAuth(s.handleSync))
	mux.HandleFunc("/progress", s.withAuth(s.handleProgress))
	mux.HandleFunc("/favorite", s.withAuth(s.handleFavorite))
	return s.cors(s.logRequests(mux))
}

// ListenAndServe starts the HTTP server on cfg.Addr.
func (s *Server) ListenAndServe() error {
	log.Printf("Lampa Sync server on http://0.0.0.0:%s", s.cfg.Port)
	log.Printf("Health: http://127.0.0.1:%s/health", s.cfg.Port)
	log.Printf("Data dir: %s", s.cfg.DataDir)
	if s.cfg.AuthToken == "" {
		log.Printf("WARN: SYNC_PASSWORD is empty — set it in .env")
	}
	log.Printf("Data files ready")
	return http.ListenAndServe(s.cfg.Addr, s.Handler())
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	var (
		progress map[string]interface{}
		favorite map[string]interface{}
		err      error
	)
	err = s.store.WithLock(func() error {
		progress, err = s.store.ReadProgressUnlocked()
		if err != nil {
			return err
		}
		favorite, err = s.store.ReadFavoriteUnlocked()
		return err
	})
	if err != nil {
		log.Println("Error in /ping:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"auth":      true,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"records":   len(progress),
		"history":   len(store.AsSlice(favorite["history"])),
		"book":      len(store.AsSlice(favorite["book"])),
		"version":   version.Version,
	})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	var (
		progress map[string]interface{}
		favorite map[string]interface{}
		err      error
	)
	err = s.store.WithLock(func() error {
		progress, err = s.store.ReadProgressUnlocked()
		if err != nil {
			return err
		}
		favorite, err = s.store.ReadFavoriteUnlocked()
		return err
	})
	if err != nil {
		log.Println("Error in /sync:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	summary := store.BuildProgressSummary(progress)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":               true,
		"favorite":         favorite,
		"progress":         summary,
		"records":          len(summary),
		"history":          len(store.AsSlice(favorite["history"])),
		"book":             len(store.AsSlice(favorite["book"])),
		"favorite_updated": favorite["updated"],
		"updated":          time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getProgress(w, r)
	case http.MethodPost:
		s.postProgress(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}
}

func (s *Server) getProgress(w http.ResponseWriter, r *http.Request) {
	tmdb := r.URL.Query().Get("tmdb")
	fileID := r.URL.Query().Get("file_id")
	if tmdb == "" && fileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing tmdb or file_id parameter"})
		return
	}

	var (
		record    map[string]interface{}
		foundTmdb = tmdb
		err       error
	)
	err = s.store.WithLock(func() error {
		progress, e := s.store.ReadProgressUnlocked()
		if e != nil {
			return e
		}
		if fileID != "" && tmdb == "" {
			if key, rec, ok := store.FindProgressByFileID(progress, fileID); ok {
				foundTmdb = key
				record = rec
			}
			return nil
		}
		if raw, ok := progress[tmdb]; ok {
			record, _ = raw.(map[string]interface{})
		}
		return nil
	})
	if err != nil {
		log.Println("Error getting progress:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Progress not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tmdb":         foundTmdb,
		"time":         store.NumOr(record["time"], 0),
		"percent":      store.NumOr(record["percent"], 0),
		"file_mapping": store.MapOrEmpty(record["file_mapping"]),
		"device_id":    record["device_id"],
		"updated":      record["updated"],
	})
}

func (s *Server) postProgress(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid body"})
		return
	}
	var req struct {
		TMDB     interface{} `json:"tmdb"`
		Time     *float64    `json:"time"`
		Percent  *float64    `json:"percent"`
		FileID   interface{} `json:"file_id"`
		DeviceID interface{} `json:"device_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if req.TMDB == nil || fmt.Sprint(req.TMDB) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing tmdb parameter"})
		return
	}
	if req.Time == nil || req.Percent == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid time or percent"})
		return
	}

	tmdbKey := store.StringifyID(req.TMDB)
	var updated map[string]interface{}
	err = s.store.WithLock(func() error {
		progress, e := s.store.ReadProgressUnlocked()
		if e != nil {
			return e
		}
		var existing map[string]interface{}
		if raw, ok := progress[tmdbKey]; ok {
			existing, _ = raw.(map[string]interface{})
		}
		updated = store.ApplyProgressUpdate(existing, req.TMDB, *req.Time, *req.Percent, req.FileID, req.DeviceID)
		progress[tmdbKey] = updated
		return s.store.WriteProgressUnlocked(progress)
	})
	if err != nil {
		log.Println("Error saving progress:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"updated": updated["updated"],
		"time":    updated["time"],
		"percent": updated["percent"],
	})
}

func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid body"})
		return
	}
	var req struct {
		Favorite map[string]interface{} `json:"favorite"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Favorite == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid favorite object"})
		return
	}
	req.Favorite["updated"] = time.Now().UTC().Format(time.RFC3339Nano)

	var saved map[string]interface{}
	err = s.store.WithLock(func() error {
		var e error
		saved, e = s.store.WriteFavoriteUnlocked(req.Favorite)
		return e
	})
	if err != nil {
		log.Println("Error saving favorite:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"updated": saved["updated"],
		"history": len(store.AsSlice(saved["history"])),
		"book":    len(store.AsSlice(saved["book"])),
	})
}
