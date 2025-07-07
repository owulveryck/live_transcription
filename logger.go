package main

import (
	"log/slog"
	"os"
	"strings"
)

// Global logger instance
var logger *slog.Logger

// initLogger initializes the structured logger based on configuration
func initLogger() {
	// Get log level from environment variable, default to INFO
	logLevel := os.Getenv("LOG_LEVEL")
	var level slog.Level
	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default to INFO
	}

	// Get log format from environment variable, default to JSON
	logFormat := os.Getenv("LOG_FORMAT")
	var handler slog.Handler
	switch strings.ToUpper(logFormat) {
	case "TEXT":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	case "JSON", "":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)
}