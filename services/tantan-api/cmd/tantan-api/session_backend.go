package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
)

type sqliteSessionBackend struct {
	store *storage.Store
}

func newSQLiteSessionBackend(store *storage.Store) (*sqliteSessionBackend, error) {
	if store == nil {
		return nil, errors.New("SQLite session storage is required")
	}
	return &sqliteSessionBackend{store: store}, nil
}

func (backend *sqliteSessionBackend) SaveSession(ctx context.Context, record session.Record) error {
	if backend == nil || backend.store == nil || len(record.IDHash) != 64 || strings.TrimSpace(record.User.ID) == "" || strings.TrimSpace(record.User.Name) == "" || record.ExpiresAt.IsZero() || record.LastSeen.IsZero() || record.CreatedAt.IsZero() {
		return errors.New("valid SQLite session record is required")
	}
	return backend.store.Write(ctx, func(transaction *sql.Tx) error {
		var avatar any
		if record.User.Image != nil {
			avatar = *record.User.Image
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO accounts(user_id,name,avatar,timezone,created_at,updated_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(user_id) DO UPDATE SET
  name=excluded.name,
  avatar=excluded.avatar,
  timezone=excluded.timezone,
  updated_at=excluded.updated_at`, record.User.ID, strings.TrimSpace(record.User.Name), avatar, record.Timezone, record.CreatedAt.UTC().Format(time.RFC3339Nano), record.LastSeen.UTC().Format(time.RFC3339Nano)); err != nil {
			return errors.New("save session account failed")
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO local_sessions(id_hash,user_id,expires_at,last_seen_at,created_at)
VALUES(?,?,?,?,?)
ON CONFLICT(id_hash) DO UPDATE SET
  user_id=excluded.user_id,
  expires_at=excluded.expires_at,
  last_seen_at=excluded.last_seen_at`, record.IDHash, record.User.ID, record.ExpiresAt.UTC().Format(time.RFC3339Nano), record.LastSeen.UTC().Format(time.RFC3339Nano), record.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return errors.New("save local session failed")
		}
		return nil
	})
}

func (backend *sqliteSessionBackend) FindSession(ctx context.Context, idHash string) (session.Record, bool, error) {
	if backend == nil || backend.store == nil || len(idHash) != 64 {
		return session.Record{}, false, errors.New("valid local session hash is required")
	}
	var record session.Record
	var avatar sql.NullString
	var expiresAt, lastSeen, createdAt string
	err := backend.store.DB().QueryRowContext(ctx, `
SELECT s.id_hash,a.user_id,a.name,a.avatar,a.timezone,s.expires_at,s.last_seen_at,s.created_at
FROM local_sessions s JOIN accounts a ON a.user_id=s.user_id
WHERE s.id_hash=?`, idHash).Scan(&record.IDHash, &record.User.ID, &record.User.Name, &avatar, &record.Timezone, &expiresAt, &lastSeen, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Record{}, false, nil
	}
	if err != nil {
		return session.Record{}, false, errors.New("read local session failed")
	}
	if avatar.Valid {
		record.User.Image = &avatar.String
	}
	if record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return session.Record{}, false, errors.New("local session expiry is invalid")
	}
	if record.LastSeen, err = time.Parse(time.RFC3339Nano, lastSeen); err != nil {
		return session.Record{}, false, errors.New("local session timestamp is invalid")
	}
	if record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return session.Record{}, false, errors.New("local session creation is invalid")
	}
	return record, true, nil
}

func (backend *sqliteSessionBackend) DeleteSession(ctx context.Context, idHash string) error {
	if backend == nil || backend.store == nil || len(idHash) != 64 {
		return errors.New("valid local session hash is required")
	}
	return backend.store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, "DELETE FROM local_sessions WHERE id_hash=?", idHash)
		if err != nil {
			return errors.New("delete local session failed")
		}
		return nil
	})
}
