package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const version = "2.0.0"

var favoriteKeys = []string{
	"card", "like", "watch", "book", "history",
	"look", "viewed", "scheduled", "continued", "thrown",
}

type server struct {
	addr         string
	dataDir      string
	authToken    string
	progressPath string
	favoritePath string
	pluginPaths  []string
	mu           sync.Mutex
}

func main() {
	loadDotEnv(".env")

	port := getenv("PORT", "3000")
	dataDir := getenv("DATA_DIR", ".")
	if dataDir == "" {
		dataDir = "."
	}
	dataDir, _ = filepath.Abs(dataDir)

	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	cwd, _ := os.Getwd()

	s := &server{
		addr:         "0.0.0.0:" + port,
		dataDir:      dataDir,
		authToken:    strings.TrimSpace(os.Getenv("SYNC_PASSWORD")),
		progressPath: filepath.Join(dataDir, "progress.json"),
		favoritePath: filepath.Join(dataDir, "favorite.json"),
		pluginPaths: []string{
			filepath.Join(cwd, "public", "plugin.js"),
			filepath.Join(exeDir, "public", "plugin.js"),
			filepath.Join(cwd, "plugin.js"),
			filepath.Join(exeDir, "plugin.js"),
			filepath.Join(cwd, "..", "plugin.js"),
		},
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if _, err := s.readProgress(); err != nil {
		log.Fatal("progress init:", err)
	}
	if _, err := s.readFavorite(); err != nil {
		log.Fatal("favorite init:", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ping", s.withAuth(s.handlePing))
	mux.HandleFunc("/sync", s.withAuth(s.handleSync))
	mux.HandleFunc("/progress", s.withAuth(s.handleProgress))
	mux.HandleFunc("/favorite", s.withAuth(s.handleFavorite))
	mux.HandleFunc("/plugin.js", s.handlePlugin)

	handler := s.cors(s.logRequests(mux))

	log.Printf("Lampa Sync server on http://0.0.0.0:%s", port)
	log.Printf("Health: http://127.0.0.1:%s/health", port)
	log.Printf("Plugin: http://127.0.0.1:%s/plugin.js", port)
	log.Printf("Data dir: %s", dataDir)
	if s.authToken == "" {
		log.Printf("WARN: SYNC_PASSWORD is empty — set it in .env")
	}
	log.Printf("Data files ready")

	if err := http.ListenAndServe(s.addr, handler); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, ngrok-skip-browser-warning")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/plugin.js" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s -> %d (%dms)",
			time.Now().UTC().Format(time.RFC3339Nano),
			r.Method, r.URL.RequestURI(), rw.code, time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Unauthorized: Missing Authorization Bearer token",
			})
			return
		}
		if s.authToken == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Server: SYNC_PASSWORD not set in .env",
			})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token != s.authToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Unauthorized: Invalid token",
			})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	progress, err := s.readProgressUnlocked()
	if err != nil {
		log.Println("Error in /ping:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Internal server error"})
		return
	}
	favorite, err := s.readFavoriteUnlocked()
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
		"history":   len(asSlice(favorite["history"])),
		"book":      len(asSlice(favorite["book"])),
		"version":   version,
	})
}

func (s *server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	progress, err := s.readProgressUnlocked()
	if err != nil {
		log.Println("Error in /sync:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	favorite, err := s.readFavoriteUnlocked()
	if err != nil {
		log.Println("Error in /sync:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	summary := buildProgressSummary(progress)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":               true,
		"favorite":         favorite,
		"progress":         summary,
		"records":          len(summary),
		"history":          len(asSlice(favorite["history"])),
		"book":             len(asSlice(favorite["book"])),
		"favorite_updated": favorite["updated"],
		"updated":          time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *server) handleProgress(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getProgress(w, r)
	case http.MethodPost:
		s.postProgress(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}
}

func (s *server) getProgress(w http.ResponseWriter, r *http.Request) {
	tmdb := r.URL.Query().Get("tmdb")
	fileID := r.URL.Query().Get("file_id")
	if tmdb == "" && fileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing tmdb or file_id parameter"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	progress, err := s.readProgressUnlocked()
	if err != nil {
		log.Println("Error getting progress:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	var record map[string]interface{}
	foundTmdb := tmdb

	if fileID != "" && tmdb == "" {
		for key, raw := range progress {
			rec, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			mapping, _ := rec["file_mapping"].(map[string]interface{})
			if mapping != nil {
				if _, ok := mapping[fileID]; ok {
					foundTmdb = key
					record = rec
					break
				}
			}
		}
	} else if raw, ok := progress[tmdb]; ok {
		record, _ = raw.(map[string]interface{})
	}

	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Progress not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tmdb":         foundTmdb,
		"time":         numOr(record["time"], 0),
		"percent":      numOr(record["percent"], 0),
		"file_mapping": mapOrEmpty(record["file_mapping"]),
		"device_id":    record["device_id"],
		"updated":      record["updated"],
	})
}

func (s *server) postProgress(w http.ResponseWriter, r *http.Request) {
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

	tmdbKey := stringifyID(req.TMDB)
	timeVal := *req.Time
	percentVal := *req.Percent

	s.mu.Lock()
	defer s.mu.Unlock()
	progress, err := s.readProgressUnlocked()
	if err != nil {
		log.Println("Error saving progress:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	var existing map[string]interface{}
	if raw, ok := progress[tmdbKey]; ok {
		existing, _ = raw.(map[string]interface{})
	}

	finalTime := timeVal
	finalPercent := percentVal
	if existing != nil {
		existDevice := stringifyID(existing["device_id"])
		reqDevice := stringifyID(req.DeviceID)
		if reqDevice != "" && existDevice != "" && reqDevice == existDevice {
			finalTime = timeVal
			finalPercent = percentVal
		} else if reqDevice != "" && existDevice != "" && reqDevice != existDevice {
			finalTime = maxFloat(numOr(existing["time"], 0), timeVal)
			finalPercent = maxFloat(numOr(existing["percent"], 0), percentVal)
		} else if upd, ok := existing["updated"].(string); ok && upd != "" {
			if t, err := time.Parse(time.RFC3339Nano, upd); err == nil {
				if time.Since(t) < 5*time.Second {
					finalTime = maxFloat(numOr(existing["time"], 0), timeVal)
					finalPercent = maxFloat(numOr(existing["percent"], 0), percentVal)
				}
			} else if t, err := time.Parse(time.RFC3339, upd); err == nil {
				if time.Since(t) < 5*time.Second {
					finalTime = maxFloat(numOr(existing["time"], 0), timeVal)
					finalPercent = maxFloat(numOr(existing["percent"], 0), percentVal)
				}
			}
		}
	}

	fileMapping := map[string]interface{}{}
	if existing != nil {
		for k, v := range mapOrEmpty(existing["file_mapping"]) {
			fileMapping[k] = v
		}
	}
	if req.FileID != nil && fmt.Sprint(req.FileID) != "" {
		fileMapping[fmt.Sprint(req.FileID)] = req.TMDB
	}

	deviceID := req.DeviceID
	if deviceID == nil || fmt.Sprint(deviceID) == "" {
		if existing != nil {
			deviceID = existing["device_id"]
		} else {
			deviceID = nil
		}
	}

	updated := time.Now().UTC().Format(time.RFC3339Nano)
	progress[tmdbKey] = map[string]interface{}{
		"time":         finalTime,
		"percent":      finalPercent,
		"file_mapping": fileMapping,
		"device_id":    deviceID,
		"updated":      updated,
	}
	if err := s.writeProgressUnlocked(progress); err != nil {
		log.Println("Error saving progress:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"updated": updated,
		"time":    finalTime,
		"percent": finalPercent,
	})
}

func (s *server) handleFavorite(w http.ResponseWriter, r *http.Request) {
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

	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.writeFavoriteUnlocked(req.Favorite)
	if err != nil {
		log.Println("Error saving favorite:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"updated": saved["updated"],
		"history": len(asSlice(saved["history"])),
		"book":    len(asSlice(saved["book"])),
	})
}

func (s *server) handlePlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	for _, p := range s.pluginPaths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			http.ServeFile(w, r, p)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin.js not found"})
}

func (s *server) readProgress() (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readProgressUnlocked()
}

func (s *server) readFavorite() (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readFavoriteUnlocked()
}

func (s *server) readProgressUnlocked() (map[string]interface{}, error) {
	return readJSONObject(s.progressPath, map[string]interface{}{})
}

func (s *server) writeProgressUnlocked(data map[string]interface{}) error {
	return writeJSONFile(s.progressPath, data)
}

func (s *server) readFavoriteUnlocked() (map[string]interface{}, error) {
	data, err := os.ReadFile(s.favoritePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			progress, err := s.readProgressUnlocked()
			if err != nil {
				return nil, err
			}
			merged := emptyFavorite()
			for _, raw := range progress {
				rec, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if fav, ok := rec["favorite"].(map[string]interface{}); ok {
					merged = mergeFavorite(merged, fav)
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
		return emptyFavorite(), nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return normalizeFavoriteObject(parsed), nil
}

func (s *server) writeFavoriteUnlocked(favorite map[string]interface{}) (map[string]interface{}, error) {
	normalized := normalizeFavoriteObject(favorite)
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

func emptyFavorite() map[string]interface{} {
	out := map[string]interface{}{"updated": nil}
	for _, k := range favoriteKeys {
		out[k] = []interface{}{}
	}
	return out
}

func normalizeArray(arr interface{}) []interface{} {
	slice, ok := arr.([]interface{})
	if !ok || slice == nil {
		return []interface{}{}
	}
	out := make([]interface{}, 0, len(slice))
	seen := map[string]struct{}{}
	for _, item := range slice {
		if item == nil {
			continue
		}
		var norm interface{}
		switch v := item.(type) {
		case float64, bool:
			norm = v
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				norm = float64(n)
			} else {
				norm = v
			}
		case map[string]interface{}:
			if id, ok := v["id"]; ok {
				norm = id
			} else if id, ok := v["tmdb_id"]; ok {
				norm = id
			} else {
				norm = v
			}
		default:
			norm = v
		}
		key := fmt.Sprintf("%T:%v", norm, norm)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, norm)
	}
	return out
}

func normalizeFavoriteObject(favorite map[string]interface{}) map[string]interface{} {
	out := emptyFavorite()
	if favorite == nil {
		return out
	}
	for _, key := range favoriteKeys {
		out[key] = normalizeArray(favorite[key])
	}
	if u, ok := favorite["updated"]; ok {
		out["updated"] = u
	}
	return out
}

func mergeFavorite(a, b map[string]interface{}) map[string]interface{} {
	if a == nil {
		return normalizeFavoriteObject(b)
	}
	if b == nil {
		return normalizeFavoriteObject(a)
	}
	merged := emptyFavorite()
	for _, key := range favoriteKeys {
		merged[key] = normalizeArray(append(asSlice(a[key]), asSlice(b[key])...))
	}
	return merged
}

func buildProgressSummary(progress map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for tmdb, raw := range progress {
		rec, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		out[tmdb] = map[string]interface{}{
			"time":         numOr(rec["time"], 0),
			"percent":      numOr(rec["percent"], 0),
			"file_mapping": mapOrEmpty(rec["file_mapping"]),
			"updated":      rec["updated"],
			"device_id":    rec["device_id"],
		}
	}
	return out
}

func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok && s != nil {
		return s
	}
	return []interface{}{}
}

func mapOrEmpty(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok && m != nil {
		return m
	}
	return map[string]interface{}{}
}

func numOr(v interface{}, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			return f
		}
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func stringifyID(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		s := fmt.Sprint(v)
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
