package dsklog

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// Test initialization of the global logger for testing (Dlogger)
func TestGlobalLoggerInitialization(t *testing.T) {
	t.Setenv(logLevelEnvVar, "debug")

	logPath := filepath.Join(t.TempDir(), "test.log")
	InitializeDlogger(logPath)
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

func TestSetLevel(t *testing.T) {
	t.Setenv(logLevelEnvVar, "")

	logPath := filepath.Join(t.TempDir(), "test.log")
	InitializeDlogger(logPath)

	if err := SetLevel("error"); err != nil {
		t.Fatalf("SetLevel returned error: %v", err)
	}

	if Dlogger.GetLevel() != logrus.ErrorLevel {
		t.Fatalf("Expected log level to be Error, got %v", Dlogger.GetLevel())
	}

	if err := SetLevel("invalid"); err == nil {
		t.Fatalf("SetLevel should fail for invalid level")
	}

	if Dlogger.GetLevel() != logrus.ErrorLevel {
		t.Fatalf("Log level should remain Error after invalid SetLevel attempt, got %v", Dlogger.GetLevel())
	}
}
