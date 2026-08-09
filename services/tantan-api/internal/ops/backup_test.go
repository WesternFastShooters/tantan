package ops_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

func seedBackupFixture(t *testing.T, store *storage.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	hash := strings.Repeat("d", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_1','Backup User','Asia/Shanghai',?,?)", args: []any{now, now}},
			{query: "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES('feed_1','Backup Source',0,?)", args: []any{now}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_1','feed_1','article','Backup Entry','[]',?,?,?,?)", args: []any{now, hash, now, now}},
			{query: "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_1','entry_1',?)", args: []any{now}},
			{query: "INSERT INTO home_filters(filter_id,user_id,prompt,normalized_json,status,created_at,updated_at) VALUES('filter_1','user_1','Backup Prompt','{}','active',?,?)", args: []any{now, now}},
			{query: "INSERT INTO daily_queues(queue_id,user_id,local_date,filter_key,timezone,status,version,generated_at,created_at,updated_at) VALUES('queue_1','user_1','2026-08-09','filter_1','Asia/Shanghai','ready',1,?,?,?)", args: []any{now, now, now}},
			{query: "INSERT INTO daily_queue_items(queue_id,entry_id,rank,score,score_json,state,added_at,updated_at) VALUES('queue_1','entry_1',1,10,'{}','unread',?,?)", args: []any{now, now}},
		}
		for _, statement := range statements {
			if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBackupNeverOverwritesAndRestoreValidatesBeforeAtomicReplace(t *testing.T) {
	ctx := context.Background()
	source, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	seedBackupFixture(t, source)
	backupDirectory := t.TempDir()
	existing := filepath.Join(backupDirectory, "existing.sqlite")
	if err := os.WriteFile(existing, []byte("do-not-overwrite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Backup(ctx, source, existing); !errors.Is(err, ops.ErrDestinationExists) {
		t.Fatalf("existing backup error=%v", err)
	}
	contents, err := os.ReadFile(existing)
	if err != nil || string(contents) != "do-not-overwrite" {
		t.Fatalf("existing target changed: %q err=%v", contents, err)
	}
	backupPath := filepath.Join(backupDirectory, "tantan.sqlite")
	backup, err := ops.Backup(ctx, source, backupPath)
	if err != nil || backup.Path != backupPath || backup.Integrity != "ok" || backup.RowCounts["home_filters"] != 1 || backup.RowCounts["daily_queue_items"] != 1 {
		t.Fatalf("backup=%#v err=%v", backup, err)
	}

	restoreDirectory := t.TempDir()
	destination, err := storage.Open(ctx, storage.Config{DataDir: restoreDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := ops.Restore(ctx, backupPath, restoreDirectory)
	if err != nil || restored.Integrity != "ok" || restored.RowCounts["home_filters"] != 1 || restored.RowCounts["daily_queue_items"] != 1 || restored.RecoveryPath == "" {
		t.Fatalf("restore=%#v err=%v", restored, err)
	}
	reopened, err := storage.Open(ctx, storage.Config{DataDir: restoreDirectory})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var prompt, queueID string
	if err := reopened.DB().QueryRowContext(ctx, "SELECT prompt FROM home_filters WHERE filter_id='filter_1'").Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DB().QueryRowContext(ctx, "SELECT queue_id FROM daily_queues WHERE queue_id='queue_1'").Scan(&queueID); err != nil {
		t.Fatal(err)
	}
	if prompt != "Backup Prompt" || queueID != "queue_1" {
		t.Fatalf("restored prompt=%q queue=%q", prompt, queueID)
	}

	corrupt := filepath.Join(backupDirectory, "corrupt.sqlite")
	if err := os.WriteFile(corrupt, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Restore(ctx, corrupt, restoreDirectory); err == nil {
		t.Fatal("corrupt restore was accepted")
	}
	if err := reopened.DB().QueryRowContext(ctx, "SELECT prompt FROM home_filters WHERE filter_id='filter_1'").Scan(&prompt); err != nil || prompt != "Backup Prompt" {
		t.Fatalf("failed restore changed destination prompt=%q err=%v", prompt, err)
	}
}

func TestDailyBackupIsIdempotentAndKeepsExactlySevenVerifiedFiles(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedBackupFixture(t, store)
	directory := filepath.Join(t.TempDir(), "backups")
	start := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	for day := 0; day < 9; day++ {
		result, err := ops.CreateDailyBackup(ctx, store, directory, start.AddDate(0, 0, day))
		if err != nil || !result.Created || result.Integrity != "ok" {
			t.Fatalf("day=%d result=%#v err=%v", day, result, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 || entries[0].Name() != "tantan-2026-08-03.sqlite" || entries[6].Name() != "tantan-2026-08-09.sqlite" {
		t.Fatalf("daily backups=%v", entries)
	}
	repeated, err := ops.CreateDailyBackup(ctx, store, directory, start.AddDate(0, 0, 8))
	if err != nil || repeated.Created || repeated.Integrity != "ok" {
		t.Fatalf("repeated=%#v err=%v", repeated, err)
	}
}
