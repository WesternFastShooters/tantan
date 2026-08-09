PRAGMA foreign_keys = ON;

BEGIN IMMEDIATE;

CREATE VIRTUAL TABLE entry_search USING fts5(
  entry_id UNINDEXED,
  user_id UNINDEXED,
  title,
  translation,
  content,
  source,
  topics,
  tags,
  tokenize = 'trigram'
);

INSERT INTO schema_migrations(version, checksum, applied_at)
VALUES (2, 'spec-package-1.0.0-0002-search-fts', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

COMMIT;
