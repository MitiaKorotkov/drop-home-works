package main

import (
	"io"
	"log/slog"
)

func newLogger(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	handler := slog.NewTextHandler(w, opts)
	if format == "json" {
		handler = (*slog.TextHandler)(slog.NewJSONHandler(w, opts))
	}

	return slog.New(handler)
}
