package logging

import (
	"log"
	"log/slog"
	"os"
)

// NewJSONLogger создает JSON-логгер для вывода в stdout.
func NewJSONLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// NewJSONStdLogger создает стандартный логгер, который пишет JSON через slog handler.
func NewJSONStdLogger() *log.Logger {
	return slog.NewLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}), slog.LevelInfo)
}
