package dsklog

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"
)

// Test initialization of the global logger for testing (Dlogger)
func TestGlobalLoggerInitialization(t *testing.T) {
	InitializeDlogger("test.log", logrus.DebugLevel)
	// defer os.Remove("test.log") // Clean up the test log file
	Dlogger.Debug("Test message")

	// Ensure logger is not nil
	if Dlogger == nil {
		t.Fatal("Dlogger is not initialized")
	}

	// Verify the log level
	if Dlogger.GetLevel() != logrus.DebugLevel {
		t.Fatalf("Expected log level to be Debug, got %v", Dlogger.GetLevel())
	}

	// Test logging output to a buffer instead of file
	var buf bytes.Buffer
	Dlogger.Out = &buf

	// Log something
	Dlogger.Debug("Test message")

	// Check the buffer for the logged message
	if !bytes.Contains(buf.Bytes(), []byte("Test message")) {
		t.Errorf("Expected log message not found in buffer")
	}
}
