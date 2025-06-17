// dsklog package is just a simple wrapper around logrus
package dsklog

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Global logger instance
var Dlogger *logrus.Logger

// InitializeDlogger initializes or resets the global logger (Dlogger)
func InitializeDlogger(logFile string) {
	Dlogger = logrus.New()

	// #nosec G304
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logrus.Fatalf("Failed to open log file: %v", err)
	}

	// Set the logger output to the log file
	Dlogger.Out = file
	Dlogger.SetLevel(logrus.DebugLevel)
	// Set the log format (can be JSON or TextFormatter)
	Dlogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}
