package observability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tantan.local/tantan-api/internal/observability"
)

func TestRotatingWriterSecuresFilesCapsRetentionAndRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "tantan.jsonl")
	writer, err := observability.NewRotatingWriter(observability.RotatingWriterConfig{Path: path, MaxBytes: 64, Backups: 2})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		if _, err := writer.Write([]byte(strings.Repeat("x", 40) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("stat %s: %v", candidate, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions=%o", candidate, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("retention kept unexpected backup: %v", err)
	}
	link := filepath.Join(directory, "linked.jsonl")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := observability.NewRotatingWriter(observability.RotatingWriterConfig{Path: link, MaxBytes: 64, Backups: 2}); err == nil {
		t.Fatal("symlink log target was accepted")
	}
}
