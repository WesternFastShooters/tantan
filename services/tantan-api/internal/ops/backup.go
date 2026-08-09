package ops

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"tantan.local/tantan-api/internal/storage"
)

const maximumDatabaseBytes int64 = 5 * 1024 * 1024 * 1024

var (
	ErrDestinationExists = errors.New("backup destination already exists")
	ErrServiceRunning    = errors.New("restore requires the service to be stopped")
)

var auditedTables = []string{
	"accounts",
	"local_sessions",
	"secret_records",
	"auth_token_replays",
	"feeds",
	"entries",
	"account_entries",
	"entry_enrichments",
	"topics",
	"entry_topics",
	"home_filters",
	"daily_queues",
	"daily_queue_items",
	"recommendation_events",
	"recommendation_blocks",
	"ai_provider_configs_v1",
	"jobs",
	"sync_state",
	"entry_search",
}

var dailyBackupNamePattern = regexp.MustCompile(`^tantan-[0-9]{4}-[0-9]{2}-[0-9]{2}(?:-v[0-9]{6})?\.sqlite$`)

type DatabaseInspection struct {
	Integrity     string         `json:"integrity"`
	SchemaVersion int            `json:"schemaVersion"`
	SizeBytes     int64          `json:"sizeBytes"`
	Checksum      string         `json:"checksum"`
	RowCounts     map[string]int `json:"rowCounts"`
}

type BackupResult struct {
	Path string `json:"path"`
	DatabaseInspection
}

type RestoreResult struct {
	Path         string `json:"path"`
	RecoveryPath string `json:"recoveryPath,omitempty"`
	DatabaseInspection
}

type DailyBackupResult struct {
	Created bool `json:"created"`
	BackupResult
}

func CreateDailyBackup(ctx context.Context, store *storage.Store, directory string, now time.Time) (DailyBackupResult, error) {
	if store == nil || directory == "" || now.IsZero() {
		return DailyBackupResult{}, errors.New("daily backup storage, directory and time are required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return DailyBackupResult{}, errors.New("resolve daily backup directory failed")
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return DailyBackupResult{}, errors.New("create daily backup directory failed")
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return DailyBackupResult{}, errors.New("daily backup directory must not be a symlink")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return DailyBackupResult{}, errors.New("secure daily backup directory failed")
	}
	var schemaVersion int
	if err := store.DB().QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&schemaVersion); err != nil || schemaVersion < 1 {
		return DailyBackupResult{}, errors.New("read daily backup schema version failed")
	}
	path := filepath.Join(absolute, fmt.Sprintf("tantan-%s-v%06d.sqlite", now.Format(time.DateOnly), schemaVersion))
	if _, err := os.Lstat(path); err == nil {
		inspection, inspectErr := InspectDatabase(ctx, path)
		if inspectErr != nil {
			return DailyBackupResult{}, inspectErr
		}
		if err := pruneDailyBackups(absolute); err != nil {
			return DailyBackupResult{}, err
		}
		return DailyBackupResult{Created: false, BackupResult: BackupResult{Path: path, DatabaseInspection: inspection}}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return DailyBackupResult{}, errors.New("inspect daily backup failed")
	}
	backup, err := Backup(ctx, store, path)
	if err != nil {
		return DailyBackupResult{}, err
	}
	if err := pruneDailyBackups(absolute); err != nil {
		return DailyBackupResult{}, err
	}
	return DailyBackupResult{Created: true, BackupResult: backup}, nil
}

func pruneDailyBackups(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return errors.New("list daily backups failed")
	}
	retained := make([]string, 0, len(entries))
	for _, entry := range entries {
		if dailyBackupNamePattern.MatchString(entry.Name()) {
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("daily backup must be a regular file")
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("daily backup must be a regular file")
			}
			retained = append(retained, entry.Name())
		}
	}
	sort.Strings(retained)
	for _, name := range retained[:max(0, len(retained)-7)] {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return errors.New("remove expired daily backup failed")
		}
	}
	return syncDirectory(directory)
}

func Backup(ctx context.Context, store *storage.Store, output string) (BackupResult, error) {
	if store == nil || store.DB() == nil {
		return BackupResult{}, errors.New("backup storage is required")
	}
	outputPath, err := secureAbsentDestination(output)
	if err != nil {
		return BackupResult{}, err
	}
	directory := filepath.Dir(outputPath)
	temporary, err := os.CreateTemp(directory, ".tantan-backup-*.sqlite")
	if err != nil {
		return BackupResult{}, errors.New("create backup temporary path failed")
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return BackupResult{}, errors.New("close backup temporary path failed")
	}
	if err := os.Remove(temporaryPath); err != nil {
		return BackupResult{}, errors.New("prepare backup temporary path failed")
	}
	defer os.Remove(temporaryPath)
	if _, err := store.DB().ExecContext(ctx, "VACUUM INTO ?", temporaryPath); err != nil {
		return BackupResult{}, errors.New("create consistent SQLite backup failed")
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return BackupResult{}, errors.New("secure SQLite backup failed")
	}
	inspection, err := InspectDatabase(ctx, temporaryPath)
	if err != nil {
		return BackupResult{}, err
	}
	if err := os.Link(temporaryPath, outputPath); err != nil {
		if _, statErr := os.Lstat(outputPath); statErr == nil {
			return BackupResult{}, ErrDestinationExists
		}
		return BackupResult{}, errors.New("publish SQLite backup failed")
	}
	if err := syncDirectory(directory); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Path: outputPath, DatabaseInspection: inspection}, nil
}

func Restore(ctx context.Context, backupPath, dataDirectory string) (RestoreResult, error) {
	sourcePath, err := secureRegularFile(backupPath)
	if err != nil {
		return RestoreResult{}, err
	}
	sourceInspection, err := InspectDatabase(ctx, sourcePath)
	if err != nil {
		return RestoreResult{}, err
	}
	directory, err := filepath.Abs(dataDirectory)
	if err != nil || dataDirectory == "" {
		return RestoreResult{}, errors.New("valid restore data directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return RestoreResult{}, errors.New("create restore data directory failed")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return RestoreResult{}, errors.New("secure restore data directory failed")
	}
	destination := filepath.Join(directory, "tantan.sqlite")
	if sourcePath == destination {
		return RestoreResult{}, errors.New("restore source and destination must differ")
	}
	for _, sidecar := range []string{destination + "-wal", destination + "-shm"} {
		if _, err := os.Lstat(sidecar); err == nil {
			return RestoreResult{}, ErrServiceRunning
		} else if !errors.Is(err, os.ErrNotExist) {
			return RestoreResult{}, errors.New("inspect restore sidecar failed")
		}
	}
	temporary, err := os.CreateTemp(directory, ".tantan-restore-*.sqlite")
	if err != nil {
		return RestoreResult{}, errors.New("create restore temporary file failed")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return RestoreResult{}, errors.New("secure restore temporary file failed")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		_ = temporary.Close()
		return RestoreResult{}, errors.New("open restore source failed")
	}
	copied, copyErr := io.Copy(temporary, io.LimitReader(source, maximumDatabaseBytes+1))
	closeSourceErr := source.Close()
	syncErr := temporary.Sync()
	closeTemporaryErr := temporary.Close()
	if copyErr != nil || closeSourceErr != nil || syncErr != nil || closeTemporaryErr != nil || copied > maximumDatabaseBytes {
		return RestoreResult{}, errors.New("copy restore database failed")
	}
	temporaryInspection, err := InspectDatabase(ctx, temporaryPath)
	if err != nil {
		return RestoreResult{}, err
	}
	if temporaryInspection.Checksum != sourceInspection.Checksum || !sameRowCounts(temporaryInspection.RowCounts, sourceInspection.RowCounts) {
		return RestoreResult{}, errors.New("restore copy verification failed")
	}
	recoveryPath := ""
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return RestoreResult{}, errors.New("restore destination must be a regular database file")
		}
		recoveryPath = filepath.Join(directory, fmt.Sprintf("tantan.sqlite.pre-restore-%d", time.Now().UTC().UnixNano()))
		if err := os.Link(destination, recoveryPath); err != nil {
			return RestoreResult{}, errors.New("create pre-restore recovery copy failed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return RestoreResult{}, errors.New("inspect restore destination failed")
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		if recoveryPath != "" {
			_ = os.Remove(recoveryPath)
		}
		return RestoreResult{}, errors.New("atomically replace restore destination failed")
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return RestoreResult{}, errors.New("secure restored database failed")
	}
	if err := syncDirectory(directory); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Path: destination, RecoveryPath: recoveryPath, DatabaseInspection: temporaryInspection}, nil
}

func InspectDatabase(ctx context.Context, path string) (DatabaseInspection, error) {
	securePath, err := secureRegularFile(path)
	if err != nil {
		return DatabaseInspection{}, err
	}
	info, err := os.Stat(securePath)
	if err != nil || info.Size() > maximumDatabaseBytes {
		return DatabaseInspection{}, errors.New("database file exceeds the supported size")
	}
	dsn := (&url.URL{Scheme: "file", Path: securePath}).String() + "?mode=ro&_foreign_keys=on"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return DatabaseInspection{}, errors.New("open database inspection failed")
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return DatabaseInspection{}, errors.New("database inspection failed")
	}
	var integrity string
	if err := database.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return DatabaseInspection{}, errors.New("database integrity check failed")
	}
	foreignKeyRows, err := database.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return DatabaseInspection{}, errors.New("database foreign-key check failed")
	}
	if foreignKeyRows.Next() {
		_ = foreignKeyRows.Close()
		return DatabaseInspection{}, errors.New("database foreign-key check failed")
	}
	if err := foreignKeyRows.Err(); err != nil {
		_ = foreignKeyRows.Close()
		return DatabaseInspection{}, errors.New("database foreign-key check failed")
	}
	if err := foreignKeyRows.Close(); err != nil {
		return DatabaseInspection{}, errors.New("database foreign-key check failed")
	}
	if err := CheckMigrations(ctx, database); err != nil {
		return DatabaseInspection{}, errors.New("database migration check failed")
	}
	var schemaVersion int
	if err := database.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&schemaVersion); err != nil {
		return DatabaseInspection{}, errors.New("read database schema version failed")
	}
	rowCounts := make(map[string]int, len(auditedTables))
	for _, table := range auditedTables {
		var count int
		if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return DatabaseInspection{}, errors.New("read database row counts failed")
		}
		rowCounts[table] = count
	}
	checksum, err := fileChecksum(securePath)
	if err != nil {
		return DatabaseInspection{}, err
	}
	return DatabaseInspection{Integrity: integrity, SchemaVersion: schemaVersion, SizeBytes: info.Size(), Checksum: checksum, RowCounts: rowCounts}, nil
}

func secureAbsentDestination(path string) (string, error) {
	if path == "" {
		return "", errors.New("explicit backup destination is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("resolve backup destination failed")
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("inspect backup destination failed")
	}
	directoryInfo, err := os.Lstat(filepath.Dir(absolute))
	if err != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return "", errors.New("backup destination directory must already exist and not be a symlink")
	}
	return absolute, nil
}

func secureRegularFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("resolve database path failed")
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("database path must be a regular file")
	}
	return absolute, nil
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("open database checksum failed")
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", errors.New("read database checksum failed")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameRowCounts(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for table, count := range left {
		if right[table] != count {
			return false
		}
	}
	return true
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.New("open database directory for sync failed")
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return errors.New("sync database directory failed")
	}
	return nil
}
