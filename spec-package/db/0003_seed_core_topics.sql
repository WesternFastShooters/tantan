PRAGMA foreign_keys = ON;

BEGIN IMMEDIATE;

CREATE TABLE core_topic_templates (
  slug TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 40),
  sort_order INTEGER NOT NULL UNIQUE CHECK (sort_order >= 0),
  created_at TEXT NOT NULL
) STRICT;

INSERT INTO core_topic_templates(slug, name, sort_order, created_at) VALUES
  ('ai', 'AI', 10, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ('web3', 'Web3', 20, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ('3d', '3D', 30, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ('politics', '时事政治', 40, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ('frontend', '前端', 50, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ('agent', 'Agent', 60, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

INSERT INTO schema_migrations(version, checksum, applied_at)
VALUES (3, 'spec-package-1.0.0-0003-core-topic-templates', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

COMMIT;
