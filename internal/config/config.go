package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds runtime settings for the sync server.
type Config struct {
	Addr        string
	Port        string
	DataDir     string
	AuthToken   string
	PluginPaths []string
}

// Load reads environment (and optional .env file) into Config.
func Load() (Config, error) {
	LoadDotEnv(".env")

	port := getenv("PORT", "3000")
	dataDir := getenv("DATA_DIR", ".")
	if dataDir == "" {
		dataDir = "."
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, err
	}

	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	cwd, _ := os.Getwd()

	return Config{
		Addr:      "0.0.0.0:" + port,
		Port:      port,
		DataDir:   abs,
		AuthToken: strings.TrimSpace(os.Getenv("SYNC_PASSWORD")),
		PluginPaths: []string{
			filepath.Join(cwd, "public", "plugin.js"),
			filepath.Join(exeDir, "public", "plugin.js"),
			filepath.Join(cwd, "plugin.js"),
			filepath.Join(exeDir, "plugin.js"),
			filepath.Join(cwd, "..", "plugin.js"),
		},
	}, nil
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// LoadDotEnv loads KEY=VALUE pairs from path into the environment
// without overriding already-set variables.
func LoadDotEnv(path string) {
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
