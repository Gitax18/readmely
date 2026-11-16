// Package logging builds the structured logger shared by every Go process.
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger that tags every line with the service name.
// Code, prompts and secrets must never be passed to it.
func New(w io.Writer, level slog.Level, service string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service)
}
