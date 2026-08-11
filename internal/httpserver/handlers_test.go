package httpserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	cfg := config.Config{
		Addr:      "127.0.0.1:0",
		Port:      "0",
		DataDir:   dir,
		AuthToken: token,
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

func TestFavoriteMergeMode(t *testing.T) {
	h := newTestServer(t, "pass")
	code, _ := doJSON(t, h, http.MethodPost, "/favorite", "pass", map[string]interface{}{
		"favorite": map[string]interface{}{
			"history": []int{1, 2},
			"book":    []int{3},
		},
	})
	if code != 200 {
		t.Fatalf("seed code=%d", code)
	}

	reqBody := map[string]interface{}{
		"mode": "merge",
		"favorite": map[string]interface{}{
			"history": []int{2, 9},
			"book":    []int{4},
		},
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/favorite", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer pass")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("merge code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out["history"].(float64) < 3 {
		t.Fatalf("expected union history>=3 got %#v", out)
	}
	if out["book"].(float64) < 2 {
		t.Fatalf("expected union book>=2 got %#v", out)
	}

	code, syncOut := doJSON(t, h, http.MethodGet, "/sync", "pass", nil)
	if code != 200 {
		t.Fatalf("sync %d", code)
	}
	fav := syncOut["favorite"].(map[string]interface{})
	hist := fav["history"].([]interface{})
	seen := map[float64]bool{}
	for _, v := range hist {
		seen[v.(float64)] = true
	}
	for _, id := range []float64{1, 2, 9} {
		if !seen[id] {
			t.Fatalf("missing %v in %#v", id, hist)
		}
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

func TestMethodNotAllowedAndNotFound(t *testing.T) {
	h := newTestServer(t, "pass")
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/health", 405},
		{http.MethodPost, "/ping", 405},
		{http.MethodPost, "/sync", 405},
		{http.MethodDelete, "/progress", 405},
		{http.MethodGet, "/favorite", 405},
		{http.MethodGet, "/progress", 400},
		{http.MethodGet, "/progress?tmdb=missing", 404},
	}
	for _, tc := range cases {
		code, _ := doJSON(t, h, tc.method, tc.path, "pass", nil)
		if code != tc.want {
			t.Fatalf("%s %s -> %d want %d", tc.method, tc.path, code, tc.want)
		}
	}
}

func TestFavoriteValidationAndEmptyAuth(t *testing.T) {
	h := newTestServer(t, "pass")
	code, _ := doJSON(t, h, http.MethodPost, "/favorite", "pass", map[string]interface{}{
		"favorite": nil,
	})
	if code != 400 {
		t.Fatalf("code=%d", code)
	}

	empty := newTestServer(t, "")
	code, out := doJSON(t, empty, http.MethodGet, "/ping", "anything", nil)
	if code != 500 {
		t.Fatalf("%d %#v", code, out)
	}
}

func TestProgressMissingTMDB(t *testing.T) {
	h := newTestServer(t, "pass")
	code, _ := doJSON(t, h, http.MethodPost, "/progress", "pass", map[string]interface{}{
		"time":    1,
		"percent": 1,
	})
	if code != 400 {
		t.Fatalf("code=%d", code)
	}
}
