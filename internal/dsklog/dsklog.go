// dsklog package is just a simple wrapper around logrus
package dsklog

import (
	"errors"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Global logger instance
var Dlogger *logrus.Logger

// Set log-level via env variable.
const logLevelEnvVar = "DSKDITTO_LOG_LEVEL"

// InitializeDlogger initializes or resets the global logger (Dlogger)
func InitializeDlogger(logFile string) {
	Dlogger = logrus.New()

	// #nosec G304
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logrus.Fatalf("Failed to open log file: %v", err)
	}

	// Set the logger output to the log file
	Dlogger.Out = file
	// Set the log format (can be JSON or TextFormatter)
	Dlogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	level := resolveLevel(os.Getenv(logLevelEnvVar))
	Dlogger.SetLevel(level)
}

// SetLevel allows callers to adjust the global logger level at runtime.
func SetLevel(level string) error {
	if Dlogger == nil {
		return errors.New("logger not initialized")
	}

	parsed, err := parseLevel(level)
	if err != nil {
		return err
	}

	Dlogger.SetLevel(parsed)
	return nil
}

// resolveLevel calls parseLevel; handling any potential error cases. It helps keep
// the InitializeDlogger a bit cleaner.
func resolveLevel(raw string) logrus.Level {
	parsed, err := parseLevel(raw)
	if err != nil {
		return logrus.InfoLevel
	}
	return parsed
}

// parseLevel processes the environment variable string. If no string is found or the log
// level isn't valid we return INFO.
func parseLevel(raw string) (logrus.Level, error) {
	// No env variable set. Default to INFO.
	if strings.TrimSpace(raw) == "" {
		return logrus.InfoLevel, nil
	}

	level, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return logrus.InfoLevel, err
	}
	return level, nil
}
