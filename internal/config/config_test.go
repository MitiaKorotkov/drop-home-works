package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	missing := filepath.Join(dir, "missing")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr []string
	}{
		{
			name: "all set",
			env: map[string]string{
				"HTTP_ADDR":    ":9000",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
				"LOG_FORMAT":   "json",
				"LOG_LEVEL":    "debug",
			},
			want: Config{
				HTTPAddress: ":9000",
				DatabaseURL: "postgres://x",
				StorageDir:  dir,
				LogFormat:   "json",
				LogLevel:    slog.LevelDebug,
			},
		},
		{
			name: "empty httpAddr",
			env: map[string]string{
				"HTTP_ADDR":    "",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
				"LOG_FORMAT":   "text",
				"LOG_LEVEL":    "info",
			},
			want: Config{
				HTTPAddress: ":8080",
				DatabaseURL: "postgres://x",
				StorageDir:  dir,
				LogFormat:   "text",
				LogLevel:    slog.LevelInfo,
			},
		},
		{
			name: "no required fields",
			env:  map[string]string{},
			wantErr: []string{
				"DATABASE_URL",
				"STORAGE_DIR",
			},
		},
		{
			name: "dir is a file",
			env: map[string]string{
				"HTTP_ADDR":    ":8080",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  file,
			},
			wantErr: []string{
				"STORAGE_DIR",
				"not a directory",
			},
		},
		{
			name: "dir does not exist",
			env: map[string]string{
				"HTTP_ADDR":    ":8080",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  missing,
			},
			wantErr: []string{
				"STORAGE_DIR",
			},
		},
		{
			name: "wrong httpAddr",
			env: map[string]string{
				"HTTP_ADDR":    "8080",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
			},
			wantErr: []string{
				"HTTP_ADDR",
			},
		},
		{
			name: "wrong logFormat",
			env: map[string]string{
				"HTTP_ADDR":    "",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
				"LOG_FORMAT":   "xml",
				"LOG_LEVEL":    "info",
			},
			wantErr: []string{
				"LOG_FORMAT",
			},
		},
		{
			name: "wrong logLevel",
			env: map[string]string{
				"HTTP_ADDR":    "",
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
				"LOG_FORMAT":   "text",
				"LOG_LEVEL":    "verbose",
			},
			wantErr: []string{
				"LOG_LEVEL",
			},
		},
		{
			name: "default values",
			env: map[string]string{
				"DATABASE_URL": "postgres://x",
				"STORAGE_DIR":  dir,
			},
			want: Config{
				HTTPAddress: ":8080",
				DatabaseURL: "postgres://x",
				StorageDir:  dir,
				LogFormat:   "text",
				LogLevel:    slog.LevelInfo,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(name string) (string, bool) {
				v, ok := tt.env[name]
				return v, ok
			}

			res, err := Load(lookup)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("should be no errors but had this one: %q", err)
				}
				if res != tt.want {
					t.Fatalf("Load() cfg = %+v, want it to be %+v", res, tt.want)
				}
			} else {
				if err == nil {
					t.Fatalf("Load() error = nil, want error containing %q", tt.wantErr)
				}
				for _, s := range tt.wantErr {
					if !strings.Contains(err.Error(), s) {
						t.Fatalf("Load() error = %q, want it to contain %q", err, s)
					}
				}
			}
		})
	}
}
