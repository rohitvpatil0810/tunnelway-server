package logger

import (
	"log/slog"
	"os"
)

func Logger() *slog.Logger {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	})

	logger := slog.New(jsonHandler)
	return logger
}
