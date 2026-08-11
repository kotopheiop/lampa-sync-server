package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kotopheiop/lampa-sync-server/internal/config"
)

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=fromfile\nBAR=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "fromenv")
	_ = os.Unsetenv("BAR")

	config.LoadDotEnv(path)
	if got := os.Getenv("FOO"); got != "fromenv" {
		t.Fatalf("FOO=%q, want fromenv", got)
	}
	if got := os.Getenv("BAR"); got != "bar" {
		t.Fatalf("BAR=%q, want bar", got)
	}
}

func TestLoadDotEnvQuoted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("Q=\"hello world\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("Q")
	config.LoadDotEnv(path)
	if got := os.Getenv("Q"); got != "hello world" {
		t.Fatalf("Q=%q", got)
	}
}

func TestLoadUsesDataDirAndPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORT", "3123")
	t.Setenv("DATA_DIR", dir)
	t.Setenv("SYNC_PASSWORD", "secret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "3123" {
		t.Fatalf("Port=%q", cfg.Port)
	}
	if cfg.Addr != "0.0.0.0:3123" {
		t.Fatalf("Addr=%q", cfg.Addr)
	}
	if cfg.DataDir != dir {
		t.Fatalf("DataDir=%q want %q", cfg.DataDir, dir)
	}
	if cfg.AuthToken != "secret" {
		t.Fatalf("AuthToken=%q", cfg.AuthToken)
	}
}

func TestLoadDefaults(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("DATA_DIR")
	_ = os.Unsetenv("SYNC_PASSWORD")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "3000" {
		t.Fatalf("Port=%q", cfg.Port)
	}
	if cfg.AuthToken != "" {
		t.Fatalf("AuthToken=%q", cfg.AuthToken)
	}
}

func TestLoadDotEnvSkipsJunk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\nNOEQUALS\n=noval\nOK=1\nS='single'\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("OK")
	_ = os.Unsetenv("S")
	config.LoadDotEnv(path)
	if os.Getenv("OK") != "1" || os.Getenv("S") != "single" {
		t.Fatalf("OK=%q S=%q", os.Getenv("OK"), os.Getenv("S"))
	}
}
