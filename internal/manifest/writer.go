package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Writer struct {
	path    string
	tmpPath string
	file    *os.File
	enc     *json.Encoder
	closed  bool
}

func NewWriter(path string) (*Writer, error) {
	if path == "" {
		return nil, errors.New("manifest path is empty")
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("resolve manifest path %s: %w", path, err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create manifest directory %s: %w", dir, err)
	}

	tmpPath := absPath + ".tmp"
	file, err := openCreateTruncate(tmpPath, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open temp manifest %s: %w", tmpPath, err)
	}

	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)

	return &Writer{
		path:    absPath,
		tmpPath: tmpPath,
		file:    file,
		enc:     enc,
	}, nil
}

func (w *Writer) WriteEntry(entry Entry) error {
	if w == nil {
		return errors.New("manifest writer is nil")
	}
	if w.closed {
		return errors.New("manifest writer is closed")
	}
	if entry.Version == 0 {
		entry.Version = ManifestVersion
	}
	if err := w.enc.Encode(entry); err != nil {
		return fmt.Errorf("encode manifest entry: %w", err)
	}
	return nil
}

func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	if w.closed {
		return nil
	}
	w.closed = true

	var closeErr error
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			closeErr = fmt.Errorf("sync temp manifest %s: %w", w.tmpPath, err)
		}
		if err := w.file.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close temp manifest %s: %w", w.tmpPath, err)
		}
	}
	if closeErr != nil {
		_ = os.Remove(w.tmpPath)
		return closeErr
	}

	if err := os.Rename(w.tmpPath, w.path); err != nil {
		_ = os.Remove(w.tmpPath)
		return fmt.Errorf("rename manifest %s -> %s: %w", w.tmpPath, w.path, err)
	}

	if err := syncDir(filepath.Dir(w.path)); err != nil {
		return fmt.Errorf("sync manifest directory %s: %w", filepath.Dir(w.path), err)
	}
	return nil
}

func (w *Writer) Abort() error {
	if w == nil {
		return nil
	}
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			_ = os.Remove(w.tmpPath)
			return err
		}
	}
	return os.Remove(w.tmpPath)
}
