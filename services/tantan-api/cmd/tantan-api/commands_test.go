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

func TestMigrateCommandInitializesStorageBeforeReplication(t *testing.T) {
	dataDirectory := t.TempDir()
	handled, err := runManagementCommand(
		context.Background(),
		[]string{"migrate", "--data-dir", dataDirectory},
		&bytes.Buffer{},
	)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	store, err := storage.Open(context.Background(), storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if integrity, err := store.Integrity(context.Background()); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
	if handled, err := runManagementCommand(context.Background(), []string{"migrate", "extra"}, &bytes.Buffer{}); !handled || err == nil {
		t.Fatalf("invalid migrate handled=%v err=%v", handled, err)
	}
}
