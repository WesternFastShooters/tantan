package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tantan.local/tantan-api/internal/session"
)

type SessionBackend struct {
	store *Store
}

func NewSessionBackend(store *Store) *SessionBackend {
	return &SessionBackend{store: store}
}

func (backend *SessionBackend) SaveSession(ctx context.Context, record session.Record) error {
	if backend == nil || backend.store == nil {
		return errors.New("session storage is unavailable")
	}
	return backend.store.Write(ctx, func(transaction *sql.Tx) error {
		now := record.LastSeen.UTC().Format(time.RFC3339Nano)
		if now == "0001-01-01T00:00:00Z" {
			now = time.Now().UTC().Format(time.RFC3339Nano)
		}
		timezone := record.Timezone
		if timezone == "" {
			timezone = "Asia/Shanghai"
		}
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
  updated_at=excluded.updated_at`, record.User.ID, record.User.Name, avatar, timezone, now, now); err != nil {
			return fmt.Errorf("upsert session account: %w", err)
		}
		createdAt := record.CreatedAt.UTC().Format(time.RFC3339Nano)
		if record.CreatedAt.IsZero() {
			createdAt = now
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO local_sessions(id_hash,user_id,expires_at,last_seen_at,created_at)
VALUES(?,?,?,?,?)
ON CONFLICT(id_hash) DO UPDATE SET
  user_id=excluded.user_id,
  expires_at=excluded.expires_at,
  last_seen_at=excluded.last_seen_at`, record.IDHash, record.User.ID, record.ExpiresAt.UTC().Format(time.RFC3339Nano), now, createdAt); err != nil {
			return fmt.Errorf("upsert local session: %w", err)
		}
		return nil
	})
}

func (backend *SessionBackend) FindSession(ctx context.Context, idHash string) (session.Record, bool, error) {
	if backend == nil || backend.store == nil {
		return session.Record{}, false, errors.New("session storage is unavailable")
	}
	var record session.Record
	var avatar sql.NullString
	var expiresAt string
	var lastSeenAt string
	var createdAt string
	err := backend.store.database.QueryRowContext(ctx, `
SELECT s.id_hash,a.user_id,a.name,a.avatar,a.timezone,s.expires_at,s.last_seen_at,s.created_at
FROM local_sessions s
JOIN accounts a ON a.user_id=s.user_id
WHERE s.id_hash=?`, idHash).Scan(
		&record.IDHash,
		&record.User.ID,
		&record.User.Name,
		&avatar,
		&record.Timezone,
		&expiresAt,
		&lastSeenAt,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Record{}, false, nil
	}
	if err != nil {
		return session.Record{}, false, fmt.Errorf("find local session: %w", err)
	}
	if avatar.Valid {
		record.User.Image = &avatar.String
	}
	for value, target := range map[string]*time.Time{
		expiresAt:  &record.ExpiresAt,
		lastSeenAt: &record.LastSeen,
		createdAt:  &record.CreatedAt,
	} {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return session.Record{}, false, errors.New("local session contains an invalid timestamp")
		}
		*target = parsed
	}
	return record, true, nil
}

func (backend *SessionBackend) DeleteSession(ctx context.Context, idHash string) error {
	if backend == nil || backend.store == nil {
		return errors.New("session storage is unavailable")
	}
	return backend.store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM local_sessions WHERE id_hash = ?", idHash); err != nil {
			return fmt.Errorf("delete local session: %w", err)
		}
		return nil
	})
}
