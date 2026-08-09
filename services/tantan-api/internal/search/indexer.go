package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tantan.local/tantan-api/internal/storage"
)

type Indexer struct {
	store *storage.Store
}

func NewIndexer(store *storage.Store) *Indexer {
	return &Indexer{store: store}
}

func (indexer *Indexer) Refresh(ctx context.Context, userID string, entryIDs []string) error {
	if indexer == nil || indexer.store == nil {
		return errors.New("search index storage is unavailable")
	}
	return indexer.store.Write(ctx, func(transaction *sql.Tx) error {
		return indexer.RefreshTx(ctx, transaction, userID, entryIDs)
	})
}

func (indexer *Indexer) RefreshTx(ctx context.Context, transaction *sql.Tx, userID string, entryIDs []string) error {
	if transaction == nil || userID == "" {
		return errors.New("search index transaction and user are required")
	}
	if len(entryIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(entryIDs))
	for _, entryID := range entryIDs {
		if entryID == "" {
			return errors.New("search index entry ID is required")
		}
		if _, duplicate := seen[entryID]; duplicate {
			return errors.New("search index entry IDs must be unique")
		}
		seen[entryID] = struct{}{}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(entryIDs)), ",")
	deleteArguments := make([]any, 0, len(entryIDs)+1)
	deleteArguments = append(deleteArguments, userID)
	for _, entryID := range entryIDs {
		deleteArguments = append(deleteArguments, entryID)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM entry_search WHERE user_id=? AND entry_id IN ("+placeholders+")", deleteArguments...); err != nil {
		return fmt.Errorf("delete old search rows: %w", err)
	}
	insertArguments := make([]any, 0, len(entryIDs)+1)
	insertArguments = append(insertArguments, userID)
	for _, entryID := range entryIDs {
		insertArguments = append(insertArguments, entryID)
	}
	statement := `
INSERT INTO entry_search(entry_id,user_id,title,translation,content,source,topics,tags)
SELECT
  e.entry_id,
  ae.user_id,
  e.title,
  COALESCE((
    SELECT group_concat(trim(COALESCE(en.translated_title,'') || ' ' || COALESCE(en.translated_content,'')), ' ')
    FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready'
  ), ''),
  trim(COALESCE(e.description,'') || ' ' || COALESCE(e.content,'') || ' ' || COALESCE(e.author,'')),
  COALESCE(f.title, ''),
  COALESCE((
    SELECT group_concat(t.name, ' ')
    FROM entry_topics et
    JOIN topics t ON t.topic_id=et.topic_id AND t.user_id=et.user_id
    WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id
  ), ''),
  COALESCE((
    SELECT group_concat(CAST(tag.value AS TEXT), ' ')
    FROM entry_enrichments en, json_each(en.tags_json) tag
    WHERE en.entry_id=e.entry_id AND en.state='ready'
  ), '')
FROM entries e
JOIN account_entries ae ON ae.entry_id=e.entry_id AND ae.user_id=?
LEFT JOIN feeds f ON f.feed_id=e.feed_id
	WHERE e.entry_id IN (` + placeholders + `)`
	if _, err := transaction.ExecContext(ctx, statement, insertArguments...); err != nil {
		return fmt.Errorf("insert search rows: %w", err)
	}
	return nil
}

func (indexer *Indexer) Status(ctx context.Context, userID string) (string, error) {
	if indexer == nil || indexer.store == nil {
		return "degraded", errors.New("search index storage is unavailable")
	}
	var factCount int
	var indexCount int
	if err := indexer.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM account_entries WHERE user_id = ?", userID).Scan(&factCount); err != nil {
		return "degraded", fmt.Errorf("count searchable entries: %w", err)
	}
	if err := indexer.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entry_search WHERE user_id = ?", userID).Scan(&indexCount); err != nil {
		return "degraded", fmt.Errorf("count search index: %w", err)
	}
	if indexCount > factCount {
		return "degraded", nil
	}
	if indexCount < factCount {
		return "building", nil
	}
	var mismatch int
	if err := indexer.store.DB().QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT entry_id FROM account_entries WHERE user_id=?
  EXCEPT
  SELECT entry_id FROM entry_search WHERE user_id=?
)`, userID, userID).Scan(&mismatch); err != nil {
		return "degraded", fmt.Errorf("verify search index: %w", err)
	}
	if mismatch != 0 {
		return "degraded", nil
	}
	return "ready", nil
}
