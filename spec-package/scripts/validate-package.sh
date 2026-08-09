#!/usr/bin/env bash
set -euo pipefail

package_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="$(cd "$package_root/.." && pwd)"

ruby "$package_root/scripts/validate-package.rb"

validation_tmp="$(mktemp -d)"
validation_db="$validation_tmp/contracts.sqlite"

sqlite3 "$validation_db" < "$package_root/db/0001_core.sql"
sqlite3 "$validation_db" < "$package_root/db/0002_search_fts.sql"
sqlite3 "$validation_db" < "$package_root/db/0003_seed_core_topics.sql"

integrity="$(sqlite3 "$validation_db" 'PRAGMA integrity_check;')"
if [[ "$integrity" != "ok" ]]; then
  echo "ERROR: SQLite integrity_check=$integrity" >&2
  exit 1
fi

foreign_key_rows="$(sqlite3 "$validation_db" 'PRAGMA foreign_key_check;')"
if [[ -n "$foreign_key_rows" ]]; then
  echo "ERROR: SQLite foreign_key_check returned rows" >&2
  echo "$foreign_key_rows" >&2
  exit 1
fi

migrations="$(sqlite3 "$validation_db" 'SELECT group_concat(version, ",") FROM schema_migrations ORDER BY version;')"
if [[ "$migrations" != "1,2,3" ]]; then
  echo "ERROR: migration versions=$migrations" >&2
  exit 1
fi

sqlite3 "$validation_db" <<'SQL'
PRAGMA foreign_keys = ON;
INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES
  ('user_a','User A','Asia/Shanghai','2026-08-09T00:00:00Z','2026-08-09T00:00:00Z'),
  ('user_b','User B','Asia/Shanghai','2026-08-09T00:00:00Z','2026-08-09T00:00:00Z');
INSERT INTO feeds(feed_id,title,updated_at) VALUES ('feed_a','Example Feed','2026-08-09T00:00:00Z');
INSERT INTO entries(entry_id,feed_id,kind,title,media_json,published_at,content_hash,created_at,updated_at)
VALUES ('entry_a','feed_a','article','Claude Code local workflow','[]','2026-08-09T00:00:00Z','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','2026-08-09T00:00:00Z','2026-08-09T00:00:00Z');
INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES ('user_a','entry_a','2026-08-09T00:00:00Z');
INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,sort_order,created_at,updated_at)
VALUES ('topic_a','user_a','AI','ai','core',10,'2026-08-09T00:00:00Z','2026-08-09T00:00:00Z');
INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at)
VALUES ('user_a','entry_a','topic_a',0.95,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','2026-08-09T00:00:00Z');
INSERT INTO entry_search(entry_id,user_id,title,translation,content,source,topics,tags)
VALUES ('entry_a','user_a','Claude Code local workflow','Claude Code 本地工作流','safe agent execution','Example Feed','AI Agent','coding');
SQL

fts_matches="$(sqlite3 "$validation_db" "SELECT count(*) FROM entry_search WHERE entry_search MATCH 'Claude';")"
if [[ "$fts_matches" != "1" ]]; then
  echo "ERROR: FTS trigram contract fixture did not match" >&2
  exit 1
fi

if sqlite3 "$validation_db" <<'SQL' >/dev/null 2>&1
INSERT INTO ai_provider_configs(provider_id,base_url,model,fingerprint,enabled,updated_at)
VALUES ('openai','https://127.0.0.1:9999','model-a',NULL,1,'2026-08-09T00:00:00Z');
SQL
then
  echo "ERROR: database allowed a non-preset AI provider endpoint" >&2
  exit 1
fi

if sqlite3 "$validation_db" <<'SQL' >/dev/null 2>&1
INSERT INTO ai_provider_configs(provider_id,base_url,model,fingerprint,enabled,updated_at)
VALUES ('openai','https://api.openai.com/v1','model-a',NULL,1,'2026-08-09T00:00:00Z');
INSERT INTO ai_provider_configs(provider_id,base_url,model,fingerprint,enabled,updated_at)
VALUES ('anthropic','https://api.anthropic.com','model-b',NULL,1,'2026-08-09T00:00:00Z');
SQL
then
  echo "ERROR: database allowed two enabled AI providers" >&2
  exit 1
fi

if sqlite3 "$validation_db" <<'SQL' >/dev/null 2>&1
PRAGMA foreign_keys = ON;
INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES ('user_b','entry_a','2026-08-09T00:00:00Z');
INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at)
VALUES ('user_b','entry_a','topic_a',0.90,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','2026-08-09T00:00:00Z');
SQL
then
  echo "ERROR: database allowed cross-account topic assignment" >&2
  exit 1
fi

python3 /Users/mingrui/.codex/skills/frontend-spec/scripts/validate_spec.py \
  "$repo_root/2026-08-09-tantan-frontend-spec.md" --domain frontend --stage final
python3 /Users/mingrui/.codex/skills/backend-spec/scripts/validate_spec.py \
  "$repo_root/2026-08-09-tantan-backend-spec.md" --domain backend --stage final

echo "PASS: spec package SQL migrations, FTS, invariants and final spec gates"
