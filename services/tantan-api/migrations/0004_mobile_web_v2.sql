PRAGMA foreign_keys = ON;

BEGIN IMMEDIATE;

ALTER TABLE local_sessions ADD COLUMN secret_ref TEXT;
ALTER TABLE local_sessions ADD COLUMN csrf_hash TEXT NOT NULL DEFAULT '0000000000000000000000000000000000000000000000000000000000000000'
  CHECK (length(csrf_hash) = 64);

CREATE TABLE secret_records (
  secret_ref TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  secret_kind TEXT NOT NULL CHECK (secret_kind = 'folo_session'),
  key_version INTEGER NOT NULL CHECK (key_version >= 1),
  nonce BLOB NOT NULL CHECK (length(nonce) = 12),
  ciphertext BLOB NOT NULL CHECK (length(ciphertext) >= 16),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (owner_id, secret_kind)
) STRICT;

CREATE INDEX idx_secret_records_key_version ON secret_records(key_version);

CREATE TABLE auth_token_replays (
  token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_auth_token_replays_expires_at ON auth_token_replays(expires_at);

ALTER TABLE accounts ADD COLUMN topics_revision INTEGER NOT NULL DEFAULT 1
  CHECK (topics_revision >= 1);
ALTER TABLE accounts ADD COLUMN email TEXT
  CHECK (email IS NULL OR length(email) BETWEEN 3 AND 320);

ALTER TABLE home_filters ADD COLUMN revision INTEGER NOT NULL DEFAULT 1
  CHECK (revision >= 1);

ALTER TABLE daily_queues ADD COLUMN generation TEXT;
ALTER TABLE daily_queues ADD COLUMN topic_id TEXT NOT NULL DEFAULT 'recommend';
ALTER TABLE daily_queues ADD COLUMN filter_id TEXT;

UPDATE daily_queues
SET generation = queue_id || '-v' || version
WHERE generation IS NULL;

CREATE UNIQUE INDEX idx_daily_queues_generation
  ON daily_queues(user_id, local_date, topic_id, ifnull(filter_id, ''), generation);
CREATE INDEX idx_daily_queues_scope
  ON daily_queues(user_id, local_date, topic_id, filter_id, status, version);

ALTER TABLE ai_provider_configs RENAME TO ai_provider_configs_v1;

INSERT INTO schema_migrations(version, checksum, applied_at)
VALUES (4, 'spec-package-2.0.0-0004-mobile-web-v2', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

COMMIT;
