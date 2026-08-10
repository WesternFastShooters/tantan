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

type sqliteTokenReplayStore struct {
	store *storage.Store
}

func newSQLiteSessionBackend(store *storage.Store) (*sqliteSessionBackend, error) {
	if store == nil {
		return nil, errors.New("SQLite session storage is required")
	}
	return &sqliteSessionBackend{store: store}, nil
}

func newSQLiteTokenReplayStore(store *storage.Store) (*sqliteTokenReplayStore, error) {
	if store == nil {
		return nil, errors.New("SQLite token replay storage is required")
	}
	return &sqliteTokenReplayStore{store: store}, nil
}

func (backend *sqliteSessionBackend) SaveSession(ctx context.Context, record session.Record) error {
	if backend == nil || backend.store == nil || len(record.IDHash) != 64 || len(record.SecretRef) == 0 || len(record.CSRFHash) != 64 || strings.TrimSpace(record.User.ID) == "" || strings.TrimSpace(record.User.Name) == "" || record.ExpiresAt.IsZero() || record.LastSeen.IsZero() || record.CreatedAt.IsZero() {
		return errors.New("valid SQLite session record is required")
	}
	return backend.store.Write(ctx, func(transaction *sql.Tx) error {
		var avatar any
		if record.User.Image != nil {
			avatar = *record.User.Image
		}
		var email any
		if record.User.Email != nil {
			email = *record.User.Email
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO accounts(user_id,name,avatar,email,timezone,created_at,updated_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(user_id) DO UPDATE SET
  name=excluded.name,
  avatar=excluded.avatar,
  email=excluded.email,
  timezone=excluded.timezone,
  updated_at=excluded.updated_at`, record.User.ID, strings.TrimSpace(record.User.Name), avatar, email, record.Timezone, record.CreatedAt.UTC().Format(time.RFC3339Nano), record.LastSeen.UTC().Format(time.RFC3339Nano)); err != nil {
			return errors.New("save session account failed")
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO local_sessions(id_hash,user_id,expires_at,last_seen_at,created_at,secret_ref,csrf_hash)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id_hash) DO UPDATE SET
  user_id=excluded.user_id,
  expires_at=excluded.expires_at,
  last_seen_at=excluded.last_seen_at,
  secret_ref=excluded.secret_ref,
  csrf_hash=excluded.csrf_hash`, record.IDHash, record.User.ID, record.ExpiresAt.UTC().Format(time.RFC3339Nano), record.LastSeen.UTC().Format(time.RFC3339Nano), record.CreatedAt.UTC().Format(time.RFC3339Nano), record.SecretRef, record.CSRFHash); err != nil {
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
	var avatar, email sql.NullString
	var expiresAt, lastSeen, createdAt string
	err := backend.store.DB().QueryRowContext(ctx, `
SELECT s.id_hash,s.secret_ref,s.csrf_hash,a.user_id,a.name,a.avatar,a.email,a.timezone,s.expires_at,s.last_seen_at,s.created_at
FROM local_sessions s JOIN accounts a ON a.user_id=s.user_id
WHERE s.id_hash=?`, idHash).Scan(&record.IDHash, &record.SecretRef, &record.CSRFHash, &record.User.ID, &record.User.Name, &avatar, &email, &record.Timezone, &expiresAt, &lastSeen, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Record{}, false, nil
	}
	if err != nil {
		return session.Record{}, false, errors.New("read local session failed")
	}
	if avatar.Valid {
		record.User.Image = &avatar.String
	}
	if email.Valid {
		record.User.Email = &email.String
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

func (backend *sqliteSessionBackend) FindOwner(ctx context.Context) (session.User, bool, error) {
	if backend == nil || backend.store == nil {
		return session.User{}, false, errors.New("SQLite owner storage is required")
	}
	var count int
	if err := backend.store.DB().QueryRowContext(ctx, "SELECT count(*) FROM accounts").Scan(&count); err != nil {
		return session.User{}, false, errors.New("read single-user owner count failed")
	}
	if count == 0 {
		return session.User{}, false, nil
	}
	if count != 1 {
		return session.User{}, false, errors.New("single-user deployment contains multiple accounts")
	}
	var user session.User
	var avatar, email sql.NullString
	if err := backend.store.DB().QueryRowContext(ctx, `
SELECT user_id,name,avatar,email
FROM accounts
LIMIT 1`).Scan(&user.ID, &user.Name, &avatar, &email); err != nil {
		return session.User{}, false, errors.New("read single-user owner failed")
	}
	if avatar.Valid {
		user.Image = &avatar.String
	}
	if email.Valid {
		user.Email = &email.String
	}
	return user, true, nil
}

func (store *sqliteTokenReplayStore) Reserve(ctx context.Context, tokenHash string, expiresAt time.Time) (bool, error) {
	if store == nil || store.store == nil || len(tokenHash) != 64 || !expiresAt.After(time.Now().UTC().Add(-time.Minute)) {
		return false, errors.New("valid token replay reservation is required")
	}
	reserved := false
	err := store.store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM auth_token_replays WHERE expires_at<=?", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return errors.New("prune token replay records failed")
		}
		result, err := transaction.ExecContext(ctx, `
INSERT INTO auth_token_replays(token_hash,expires_at,created_at)
VALUES(?,?,?)
ON CONFLICT(token_hash) DO NOTHING`, tokenHash, expiresAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return errors.New("reserve token replay record failed")
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return errors.New("inspect token replay reservation failed")
		}
		reserved = rows == 1
		return nil
	})
	return reserved, err
}

func (store *sqliteTokenReplayStore) Release(ctx context.Context, tokenHash string) error {
	if store == nil || store.store == nil || len(tokenHash) != 64 {
		return errors.New("valid token replay hash is required")
	}
	return store.store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM auth_token_replays WHERE token_hash=?", tokenHash); err != nil {
			return errors.New("release token replay record failed")
		}
		return nil
	})
}
