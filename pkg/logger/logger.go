package logger

import (
	"log/slog"
	"os"
)

func Logger() *slog.Logger {
	jsonHandler := slog.NewJSONHandler(os.Stdout, nil)
	logger := slog.New(jsonHandler)
	return logger
}
