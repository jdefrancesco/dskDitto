package dsklog

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Dlogger *logrus.Logger

// InitializeDlogger initializes or resets the global logger (Dlogger)
func InitializeDlogger(logFile string, logLevel logrus.Level) {
	// Create a new logger instance
	Dlogger = logrus.New()

	// Open or create the log file
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logrus.Fatalf("Failed to open log file: %v", err)
	}

	// Set the logger output to the log file
	Dlogger.Out = file

	// Set the log level from the passed argument
	Dlogger.SetLevel(logLevel)

	// Set the log format (can be JSON or TextFormatter)
	Dlogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}

// init runs when the package is imported, providing a default global logger (Dlogger)
func init() {
	InitializeDlogger("app.log", logrus.InfoLevel)
}
