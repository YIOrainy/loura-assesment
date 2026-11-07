package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, yaml string) string {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewParsesFileOverDefaults(t *testing.T) {
	cfg, err := New(writeConfig(t, "log_level: debug\npostgres_dsn: postgres://x\nretry_backoff: 2s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != slog.LevelDebug || cfg.RetryBackoff != 2*time.Second || cfg.Workers != defaults().Workers {
		t.Fatalf("got %+v", cfg)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	for _, yaml := range []string{
		"log_level: verbose\npostgres_dsn: postgres://x\n",
		"workers: 0\npostgres_dsn: postgres://x\n",
		"max_attempts: 50\npostgres_dsn: postgres://x\n",
	} {
		if _, err := New(writeConfig(t, yaml)); err == nil {
			t.Errorf("config %q accepted", yaml)
		}
	}
}
