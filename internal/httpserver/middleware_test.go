package httpserver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kotopheiop/lampa-sync-server/internal/config"
	"github.com/kotopheiop/lampa-sync-server/internal/store"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		real   string
		xff    string
		want   string
	}{
		{name: "remote", remote: "10.0.0.5:12345", want: "10.0.0.5"},
		{name: "x-real-ip wins", remote: "10.0.0.5:1", real: "1.2.3.4", xff: "9.9.9.9", want: "1.2.3.4"},
		{name: "xff first", remote: "10.0.0.5:1", xff: " 8.8.8.8 , 1.1.1.1", want: "8.8.8.8"},
		{name: "x-real with port", remote: "10.0.0.5:1", real: "1.2.3.4:443", want: "1.2.3.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.RemoteAddr = tc.remote
			if tc.real != "" {
				req.Header.Set("X-Real-IP", tc.real)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(req); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestAuthFailLogLines(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Addr: "127.0.0.1:0", Port: "0", DataDir: dir, AuthToken: "secret"}
	h := New(cfg, st).Handler()

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.10:5555"
	req.Header.Set("X-Real-IP", "198.51.100.7")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("code=%d", rr.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "AUTH_FAIL ip=198.51.100.7 method=GET path=/ping reason=missing_token") {
		t.Fatalf("missing AUTH_FAIL missing_token:\n%s", out)
	}
	if !strings.Contains(out, "ip=198.51.100.7 -> 401") {
		t.Fatalf("access log missing ip:\n%s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatal("token leaked into logs")
	}

	buf.Reset()
	req = httptest.NewRequest(http.MethodGet, "/sync", nil)
	req.RemoteAddr = "203.0.113.10:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	req.Header.Set("Authorization", "Bearer wrong-pass")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("code=%d", rr.Code)
	}
	out = buf.String()
	if !strings.Contains(out, "AUTH_FAIL ip=203.0.113.50 method=GET path=/sync reason=invalid_token") {
		t.Fatalf("missing AUTH_FAIL invalid_token:\n%s", out)
	}
	if strings.Contains(out, "wrong-pass") {
		t.Fatal("token leaked into logs")
	}
}
