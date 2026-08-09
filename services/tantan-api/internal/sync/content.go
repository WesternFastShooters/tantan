package sync

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	maxContentStreamBytes = 50 * 1024 * 1024
	maxContentLineBytes   = 8 * 1024 * 1024
)

func ParseContentStream(reader io.Reader, requestedIDs []string) (map[string]string, []string, int, error) {
	if reader == nil || len(requestedIDs) < 1 || len(requestedIDs) > 50 {
		return nil, nil, 0, errors.New("content stream requires a reader and 1 to 50 entry IDs")
	}
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		if id == "" {
			return nil, nil, 0, errors.New("content stream entry ID is required")
		}
		requested[id] = struct{}{}
	}
	limited := io.LimitReader(reader, maxContentStreamBytes+1)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), maxContentLineBytes)
	contents := make(map[string]string, len(requestedIDs))
	invalid := 0
	totalBytes := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		totalBytes += len(line) + 1
		if totalBytes > maxContentStreamBytes {
			return nil, nil, invalid, errors.New("Folo content stream exceeds 50 MiB")
		}
		var item struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			invalid++
			continue
		}
		if _, ok := requested[item.ID]; !ok || item.ID == "" {
			invalid++
			continue
		}
		if _, duplicate := contents[item.ID]; duplicate {
			invalid++
			continue
		}
		contents[item.ID] = item.Content
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, invalid, fmt.Errorf("read Folo content stream: %w", err)
	}
	missing := make([]string, 0, len(requestedIDs)-len(contents))
	for _, id := range requestedIDs {
		if _, ok := contents[id]; !ok {
			missing = append(missing, id)
		}
	}
	return contents, missing, invalid, nil
}
