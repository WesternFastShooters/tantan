package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

func TestBackupCommandRequiresExplicitNonExistingOutputAndEmitsJSON(t *testing.T) {
	ctx := context.Background()
	dataDirectory := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "backup.sqlite")
	buffer := &bytes.Buffer{}
	handled, err := runManagementCommand(ctx, []string{"backup", "--data-dir", dataDirectory, "--output", output}, buffer)
	if err != nil || !handled || buffer.Len() == 0 {
		t.Fatalf("handled=%v output=%q err=%v", handled, buffer.String(), err)
	}
	buffer.Reset()
	if handled, err := runManagementCommand(ctx, []string{"backup", "--data-dir", dataDirectory, "--output", output}, buffer); !handled || !errors.Is(err, ops.ErrDestinationExists) {
		t.Fatalf("repeated handled=%v err=%v", handled, err)
	}
	if handled, err := runManagementCommand(ctx, []string{"backup", "--data-dir", dataDirectory}, buffer); !handled || err == nil {
		t.Fatalf("missing output handled=%v err=%v", handled, err)
	}
	if handled, err := runManagementCommand(ctx, []string{"unknown"}, buffer); handled || err != nil {
		t.Fatalf("unknown handled=%v err=%v", handled, err)
	}
}
