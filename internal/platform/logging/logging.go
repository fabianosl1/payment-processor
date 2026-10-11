package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/huncoding/payment-processor-course/internal/platform/config"
)

func New(service string) *slog.Logger {
	return NewWithWriter(service, os.Stdout)
}

func NewWithWriter(service string, w io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     levelFromEnv(),
		AddSource: true,
	})

	logger := slog.New(handler).With("service", service)
	slog.SetDefault(logger)
	return logger
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(config.String("LOG_LEVEL", "")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
