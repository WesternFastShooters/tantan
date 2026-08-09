PRAGMA foreign_keys = ON;

BEGIN IMMEDIATE;

CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY,
  checksum TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL
) STRICT;

CREATE TABLE accounts (
  user_id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
  avatar TEXT,
  timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai' CHECK (length(timezone) BETWEEN 1 AND 64),
  last_success_sync_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE local_sessions (
  id_hash TEXT PRIMARY KEY CHECK (length(id_hash) = 64),
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  created_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_local_sessions_expires_at ON local_sessions(expires_at);
CREATE INDEX idx_local_sessions_user_id ON local_sessions(user_id);

CREATE TABLE feeds (
  feed_id TEXT PRIMARY KEY,
  title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
  url TEXT,
  image TEXT,
  view INTEGER NOT NULL DEFAULT 0 CHECK (view IN (0, 1)),
  updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE entries (
  entry_id TEXT PRIMARY KEY,
  feed_id TEXT REFERENCES feeds(feed_id) ON DELETE SET NULL,
  kind TEXT NOT NULL CHECK (kind IN ('article', 'post', 'image', 'video')),
  title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
  description TEXT,
  content TEXT,
  author TEXT,
  url TEXT,
  language TEXT,
  media_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(media_json)),
  published_at TEXT NOT NULL,
  content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_entries_published ON entries(published_at DESC, entry_id ASC);
CREATE INDEX idx_entries_feed_published ON entries(feed_id, published_at DESC);
CREATE INDEX idx_entries_content_hash ON entries(content_hash);

CREATE TABLE account_entries (
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
  read_at TEXT,
  collected_at TEXT,
  last_seen_at TEXT NOT NULL,
  PRIMARY KEY (user_id, entry_id)
) STRICT;

CREATE INDEX idx_account_entries_unread ON account_entries(user_id, read_at, last_seen_at DESC);
CREATE INDEX idx_account_entries_collected ON account_entries(user_id, collected_at DESC) WHERE collected_at IS NOT NULL;

CREATE TABLE entry_enrichments (
  entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
  provider_fp TEXT NOT NULL CHECK (length(provider_fp) = 12),
  language TEXT NOT NULL CHECK (length(language) BETWEEN 2 AND 16),
  state TEXT NOT NULL CHECK (state IN ('queued', 'processing', 'ready', 'failed', 'stale')),
  translated_title TEXT,
  translated_content TEXT,
  summary_text TEXT,
  key_points_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(key_points_json)),
  tags_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(tags_json)),
  quality_score REAL CHECK (quality_score IS NULL OR quality_score BETWEEN 0 AND 15),
  content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
  prompt_version TEXT NOT NULL,
  error_code TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (entry_id, provider_fp, language)
) STRICT;

CREATE INDEX idx_entry_enrichments_state ON entry_enrichments(state, updated_at);
CREATE INDEX idx_entry_enrichments_content_hash ON entry_enrichments(entry_id, content_hash);

CREATE TABLE topics (
  topic_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
  normalized_name TEXT NOT NULL CHECK (length(normalized_name) BETWEEN 1 AND 80),
  kind TEXT NOT NULL CHECK (kind IN ('core', 'dynamic', 'filter')),
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
  hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
  sort_order INTEGER NOT NULL,
  stable_until TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (user_id, normalized_name),
  UNIQUE (user_id, topic_id),
  UNIQUE (user_id, sort_order)
) STRICT;

CREATE INDEX idx_topics_visible_order ON topics(user_id, hidden, pinned DESC, sort_order);

CREATE TABLE entry_topics (
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
  topic_id TEXT NOT NULL REFERENCES topics(topic_id) ON DELETE CASCADE,
  confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
  content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
  created_at TEXT NOT NULL,
  FOREIGN KEY (user_id, topic_id) REFERENCES topics(user_id, topic_id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, entry_id, topic_id)
) STRICT;

CREATE INDEX idx_entry_topics_topic_entry ON entry_topics(user_id, topic_id, entry_id);
CREATE UNIQUE INDEX idx_entry_topics_one_primary ON entry_topics(user_id, entry_id) WHERE is_primary = 1;

CREATE TABLE home_filters (
  filter_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  prompt TEXT NOT NULL CHECK (length(prompt) BETWEEN 1 AND 300),
  normalized_json TEXT NOT NULL CHECK (json_valid(normalized_json)),
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_home_filters_one_active ON home_filters(user_id) WHERE status = 'active';
CREATE INDEX idx_home_filters_history ON home_filters(user_id, created_at DESC);

CREATE TABLE daily_queues (
  queue_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  local_date TEXT NOT NULL CHECK (length(local_date) = 10),
  filter_key TEXT NOT NULL,
  timezone TEXT NOT NULL CHECK (length(timezone) BETWEEN 1 AND 64),
  target_size INTEGER NOT NULL DEFAULT 50 CHECK (target_size = 50),
  hard_limit INTEGER NOT NULL DEFAULT 60 CHECK (hard_limit = 60),
  status TEXT NOT NULL CHECK (status IN ('building', 'ready', 'failed', 'superseded')),
  version INTEGER NOT NULL CHECK (version >= 1),
  generated_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (user_id, local_date, filter_key, version)
) STRICT;

CREATE UNIQUE INDEX idx_daily_queues_one_ready ON daily_queues(user_id, local_date, filter_key) WHERE status = 'ready';
CREATE INDEX idx_daily_queues_retention ON daily_queues(local_date, status);

CREATE TABLE daily_queue_items (
  queue_id TEXT NOT NULL REFERENCES daily_queues(queue_id) ON DELETE CASCADE,
  entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
  rank INTEGER NOT NULL CHECK (rank >= 1 AND rank <= 60),
  score REAL NOT NULL,
  score_json TEXT NOT NULL CHECK (json_valid(score_json)),
  state TEXT NOT NULL CHECK (state IN ('unread', 'read', 'removed')),
  added_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (queue_id, entry_id),
  UNIQUE (queue_id, rank)
) STRICT;

CREATE INDEX idx_daily_queue_items_page ON daily_queue_items(queue_id, state, rank);

CREATE TABLE recommendation_events (
  event_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('not_interested', 'block_source', 'block_topic', 'undo')),
  topic_id TEXT,
  source_id TEXT,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 128),
  created_at TEXT NOT NULL,
  UNIQUE (user_id, idempotency_key)
) STRICT;

CREATE INDEX idx_recommendation_events_retention ON recommendation_events(created_at);

CREATE TABLE recommendation_blocks (
  user_id TEXT NOT NULL REFERENCES accounts(user_id) ON DELETE CASCADE,
  target_type TEXT NOT NULL CHECK (target_type IN ('entry', 'source', 'topic')),
  target_id TEXT NOT NULL,
  strength REAL NOT NULL DEFAULT 1 CHECK (strength BETWEEN 0 AND 1),
  created_at TEXT NOT NULL,
  PRIMARY KEY (user_id, target_type, target_id)
) STRICT;

CREATE TABLE ai_provider_configs (
  provider_id TEXT PRIMARY KEY CHECK (provider_id IN ('openai', 'anthropic', 'google', 'deepseek', 'openrouter')),
  base_url TEXT NOT NULL CHECK (
    (provider_id = 'openai' AND base_url = 'https://api.openai.com/v1') OR
    (provider_id = 'anthropic' AND base_url = 'https://api.anthropic.com') OR
    (provider_id = 'google' AND base_url = 'https://generativelanguage.googleapis.com') OR
    (provider_id = 'deepseek' AND base_url = 'https://api.deepseek.com') OR
    (provider_id = 'openrouter' AND base_url = 'https://openrouter.ai/api/v1')
  ),
  model TEXT NOT NULL CHECK (length(model) BETWEEN 1 AND 100),
  fingerprint TEXT CHECK (fingerprint IS NULL OR length(fingerprint) = 12),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  updated_at TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_ai_provider_one_enabled ON ai_provider_configs(enabled) WHERE enabled = 1;

CREATE TABLE jobs (
  job_id TEXT PRIMARY KEY,
  user_id TEXT REFERENCES accounts(user_id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('sync', 'content', 'enrich', 'topic', 'queue', 'reconcile')),
  dedupe_key TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
  payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_run_at TEXT NOT NULL,
  lease_until TEXT,
  error_code TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  finished_at TEXT
) STRICT;

CREATE UNIQUE INDEX idx_jobs_active_dedupe ON jobs(kind, dedupe_key) WHERE state IN ('queued', 'running');
CREATE INDEX idx_jobs_claim ON jobs(state, next_run_at, created_at);
CREATE INDEX idx_jobs_retention ON jobs(state, finished_at);

CREATE TABLE sync_state (
  user_id TEXT PRIMARY KEY REFERENCES accounts(user_id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('idle', 'queued', 'running', 'succeeded', 'failed')),
  scope TEXT CHECK (scope IS NULL OR scope IN ('all', 'subscriptions', 'entries', 'reads', 'collections')),
  cursor_json TEXT CHECK (cursor_json IS NULL OR json_valid(cursor_json)),
  total INTEGER NOT NULL DEFAULT 0 CHECK (total >= 0),
  processed INTEGER NOT NULL DEFAULT 0 CHECK (processed >= 0),
  failed INTEGER NOT NULL DEFAULT 0 CHECK (failed >= 0),
  error_code TEXT,
  started_at TEXT,
  updated_at TEXT NOT NULL,
  finished_at TEXT
) STRICT;

INSERT INTO schema_migrations(version, checksum, applied_at)
VALUES (1, 'spec-package-1.0.0-0001-core', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

COMMIT;
