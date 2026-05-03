package manifest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func Read(path string) ([]Entry, error) {
	file, err := os.Open(path) // #nosec G304 -- caller controls manifest path intentionally
	if err != nil {
		return nil, fmt.Errorf("open manifest %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	entries := make([]Entry, 0)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, fmt.Errorf("decode manifest line %d: %w", line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return entries, nil
}
