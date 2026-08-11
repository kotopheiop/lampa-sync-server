package httpserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kotopheiop/lampa-sync-server/internal/config"
	"github.com/kotopheiop/lampa-sync-server/internal/httpserver"
	"github.com/kotopheiop/lampa-sync-server/internal/store"
)

func newTestServer(t *testing.T, token string) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, "plugin.js")
	if err := os.WriteFile(plugin, []byte("/* plugin */"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Addr:        "127.0.0.1:0",
		Port:        "0",
		DataDir:     dir,
		AuthToken:   token,
		PluginPaths: []string{plugin},
	}
	return httpserver.New(cfg, st).Handler()
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]interface{}) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

func TestHealth(t *testing.T) {
	h := newTestServer(t, "pass")
	code, out := doJSON(t, h, http.MethodGet, "/health", "", nil)
	if code != 200 || out["status"] != "ok" {
		t.Fatalf("%d %#v", code, out)
	}
}

func TestAuthRequired(t *testing.T) {
	h := newTestServer(t, "pass")
	code, _ := doJSON(t, h, http.MethodGet, "/ping", "", nil)
	if code != 401 {
		t.Fatalf("code=%d", code)
	}
	code, _ = doJSON(t, h, http.MethodGet, "/ping", "wrong", nil)
	if code != 401 {
		t.Fatalf("code=%d", code)
	}
}

func TestPingOK(t *testing.T) {
	h := newTestServer(t, "pass")
	code, out := doJSON(t, h, http.MethodGet, "/ping", "pass", nil)
	if code != 200 || out["ok"] != true || out["auth"] != true {
		t.Fatalf("%d %#v", code, out)
	}
	if _, ok := out["version"]; !ok {
		t.Fatalf("missing version: %#v", out)
	}
}

func TestProgressLifecycle(t *testing.T) {
	h := newTestServer(t, "pass")
	code, out := doJSON(t, h, http.MethodPost, "/progress", "pass", map[string]interface{}{
		"tmdb":      550,
		"time":      100,
		"percent":   10,
		"file_id":   "f1",
		"device_id": "d1",
	})
	if code != 200 || out["success"] != true {
		t.Fatalf("%d %#v", code, out)
	}

	code, out = doJSON(t, h, http.MethodGet, "/progress?tmdb=550", "pass", nil)
	if code != 200 {
		t.Fatalf("%d %#v", code, out)
	}
	if out["time"].(float64) != 100 {
		t.Fatalf("%v", out["time"])
	}

	code, out = doJSON(t, h, http.MethodGet, "/progress?file_id=f1", "pass", nil)
	if code != 200 || out["tmdb"] != "550" {
		t.Fatalf("%d %#v", code, out)
	}

	code, out = doJSON(t, h, http.MethodGet, "/sync", "pass", nil)
	if code != 200 || out["records"].(float64) != 1 {
		t.Fatalf("%d %#v", code, out)
	}
}

func TestFavoritePOST(t *testing.T) {
	h := newTestServer(t, "pass")
	code, out := doJSON(t, h, http.MethodPost, "/favorite", "pass", map[string]interface{}{
		"favorite": map[string]interface{}{
			"history": []int{1, 2, 2},
			"book":    []int{3},
		},
	})
	if code != 200 || out["history"].(float64) != 2 || out["book"].(float64) != 1 {
		t.Fatalf("%d %#v", code, out)
	}
}

func TestPluginJS(t *testing.T) {
	h := newTestServer(t, "pass")
	req := httptest.NewRequest(http.MethodGet, "/plugin.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	if got := rr.Body.String(); got != "/* plugin */" {
		t.Fatalf("%q", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newTestServer(t, "pass")
	req := httptest.NewRequest(http.MethodOptions, "/sync", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal(rr.Header())
	}
}

func TestProgressValidation(t *testing.T) {
	h := newTestServer(t, "pass")
	code, _ := doJSON(t, h, http.MethodPost, "/progress", "pass", map[string]interface{}{
		"tmdb": 1,
	})
	if code != 400 {
		t.Fatalf("code=%d", code)
	}
}
