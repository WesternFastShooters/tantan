package sync_test

import (
	"strings"
	"testing"

	syncer "tantan.local/tantan-api/internal/sync"
)

func TestContentStreamRejectsOversizedLinesAndCountsUntrustedRows(t *testing.T) {
	contents, missing, invalid, err := syncer.ParseContentStream(strings.NewReader("{invalid}\n{\"id\":\"unknown\",\"content\":\"x\"}\n{\"id\":\"entry_1\",\"content\":\"ok\"}\n{\"id\":\"entry_1\",\"content\":\"duplicate\"}\n"), []string{"entry_1", "entry_2"})
	if err != nil {
		t.Fatalf("parse partial content: %v", err)
	}
	if contents["entry_1"] != "ok" || len(contents) != 1 || len(missing) != 1 || missing[0] != "entry_2" || invalid != 3 {
		t.Fatalf("contents=%v missing=%v invalid=%d", contents, missing, invalid)
	}

	oversized := strings.Repeat("x", 8*1024*1024+1)
	if _, _, _, err := syncer.ParseContentStream(strings.NewReader(oversized), []string{"entry_1"}); err == nil {
		t.Fatal("oversized NDJSON line was accepted")
	}
}
