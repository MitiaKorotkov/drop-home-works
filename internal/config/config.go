package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
)

const (
	envHTTPAddr    = "HTTP_ADDR"
	envDatabaseURL = "DATABASE_URL"
	envStorageDir  = "STORAGE_DIR"
	envLogFormat   = "LOG_FORMAT"
	envLogLevel    = "LOG_LEVEL"
)

type Config struct {
	HTTPAddress string
	DatabaseURL string
	StorageDir  string
	LogFormat   string
	LogLevel    slog.Level
}

func checkStorageDir(dir string) error {
	if dir == "" {
		return errors.New("empty storage directory")
	}

	fileInfo, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		return errors.New("not a directory")
	}

	probeFile, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}

	probeFile.Close()
	err = os.Remove(probeFile.Name())
	return err
}

func checkDataBaseURL(url string) error {
	if url == "" {
		return errors.New("empty database url")
	}

	return nil
}

func checkLogFormat(format string) error {
	if format != "text" && format != "json" {
		return fmt.Errorf("unknown format %q, want text or json", format)
	}

	return nil
}

func Load(lookup func(string) (string, bool)) (Config, error) {
	get := func(name, defaultVal string) string {
		if val, exists := lookup(name); exists && val != "" {
			return val
		}

		return defaultVal
	}

	config := Config{
		HTTPAddress: get(envHTTPAddr, ":8080"),
		DatabaseURL: get(envDatabaseURL, ""),
		StorageDir:  get(envStorageDir, ""),
		LogFormat:   get(envLogFormat, "text"),
	}

	var errs []error

	if _, _, err := net.SplitHostPort(config.HTTPAddress); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envHTTPAddr, err))
	}
	if err := checkDataBaseURL(config.DatabaseURL); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envDatabaseURL, err))
	}
	if err := checkStorageDir(config.StorageDir); err != nil {
		errs = append(errs, fmt.Errorf("%s %q: %w", envStorageDir, config.StorageDir, err))
	}
	if err := checkLogFormat(config.LogFormat); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envLogFormat, err))
	}
	if err := config.LogLevel.UnmarshalText([]byte(get(envLogLevel, "info"))); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envLogLevel, err))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}

	return config, nil
}
