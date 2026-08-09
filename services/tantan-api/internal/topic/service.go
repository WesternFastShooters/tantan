package topic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/storage"
)

var ErrVersionConflict = errors.New("topic version conflict")

type Service struct {
	store   *storage.Store
	indexer *search.Indexer
	now     func() time.Time
}

type Item struct {
	ID          string
	Name        string
	Kind        string
	Fixed       bool
	Pinned      bool
	Hidden      bool
	UnreadCount int
}

type ListResponse struct {
	Version        int64
	TopicsRevision int64
	ActiveFilterID *string
	Topics         []Item
}

type Operation struct {
	Op           string
	TopicID      string
	AfterTopicID *string
}

type topicRow struct {
	ID          string
	Name        string
	Kind        string
	Pinned      bool
	Hidden      bool
	SortOrder   int
	StableUntil *time.Time
	UpdatedAt   time.Time
	UnreadCount int
}

func NewService(store *storage.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, indexer: search.NewIndexer(store), now: now}
}

func (service *Service) EnsureCore(ctx context.Context, userID string) error {
	if service == nil || service.store == nil || strings.TrimSpace(userID) == "" {
		return errors.New("topic storage and user are required")
	}
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		return service.EnsureCoreTx(ctx, transaction, userID)
	})
}

func (service *Service) EnsureCoreTx(ctx context.Context, transaction *sql.Tx, userID string) error {
	if service == nil || transaction == nil || strings.TrimSpace(userID) == "" {
		return errors.New("topic transaction and user are required")
	}
	rows, err := transaction.QueryContext(ctx, "SELECT slug,name,sort_order FROM core_topic_templates ORDER BY sort_order")
	if err != nil {
		return fmt.Errorf("read core topic templates: %w", err)
	}
	type template struct {
		slug      string
		name      string
		sortOrder int
	}
	var templates []template
	for rows.Next() {
		var item template
		if err := rows.Scan(&item.slug, &item.name, &item.sortOrder); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan core topic template: %w", err)
		}
		templates = append(templates, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate core topic templates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close core topic templates: %w", err)
	}
	now := service.now().UTC().Format(time.RFC3339Nano)
	changed := false
	for _, item := range templates {
		normalized := NormalizeName(item.name)
		result, err := transaction.ExecContext(ctx, `
INSERT OR IGNORE INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,created_at,updated_at)
VALUES(?,?,?,?, 'core',0,0,?,?,?)`, CoreID(userID, item.slug), userID, item.name, normalized, item.sortOrder, now, now)
		if err != nil {
			return fmt.Errorf("seed core topic: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect core topic seed: %w", err)
		}
		changed = changed || inserted > 0
	}
	if changed {
		if err := service.bumpTopicsRevisionTx(ctx, transaction, userID, now); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) EnsureDynamic(ctx context.Context, userID, name string) (Item, error) {
	if service == nil || service.store == nil || strings.TrimSpace(userID) == "" {
		return Item{}, errors.New("topic storage and user are required")
	}
	var result Item
	err := service.store.Write(ctx, func(transaction *sql.Tx) error {
		var err error
		result, err = service.EnsureGeneratedTx(ctx, transaction, userID, name, "dynamic")
		return err
	})
	if err != nil {
		return Item{}, err
	}
	return result, nil
}

func (service *Service) EnsureGeneratedTx(ctx context.Context, transaction *sql.Tx, userID, name, kind string) (Item, error) {
	if service == nil || transaction == nil || strings.TrimSpace(userID) == "" || (kind != "dynamic" && kind != "filter") {
		return Item{}, errors.New("valid generated topic transaction is required")
	}
	display := displayName(name)
	normalized := NormalizeName(display)
	if count := utf8.RuneCountInString(display); count < 1 || count > 20 || normalized == "" {
		return Item{}, errors.New("generated topic name must contain 1 to 20 characters")
	}
	if err := service.EnsureCoreTx(ctx, transaction, userID); err != nil {
		return Item{}, err
	}
	var sortOrder int
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(sort_order),0)+10 FROM topics WHERE user_id=?", userID).Scan(&sortOrder); err != nil {
		return Item{}, fmt.Errorf("choose generated topic order: %w", err)
	}
	now := service.now().UTC()
	var stableUntil any
	if kind == "dynamic" {
		stableUntil = now.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
	}
	timestamp := now.Format(time.RFC3339Nano)
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,stable_until,created_at,updated_at)
VALUES(?,?,?,?,?,0,0,?,?,?,?)
ON CONFLICT(user_id,normalized_name) DO UPDATE SET
  stable_until=CASE
    WHEN topics.kind='dynamic' AND excluded.kind='dynamic'
      AND (topics.stable_until IS NULL OR topics.stable_until<excluded.stable_until)
    THEN excluded.stable_until
    ELSE topics.stable_until
  END,
  updated_at=CASE WHEN topics.kind IN ('dynamic','filter') THEN excluded.updated_at ELSE topics.updated_at END`,
		generatedID(userID, kind, normalized), userID, display, normalized, kind, sortOrder, stableUntil, timestamp, timestamp); err != nil {
		return Item{}, fmt.Errorf("merge generated topic: %w", err)
	}
	if err := service.bumpTopicsRevisionTx(ctx, transaction, userID, timestamp); err != nil {
		return Item{}, err
	}
	var result Item
	var pinned int
	var hidden int
	if err := transaction.QueryRowContext(ctx, `
SELECT topic_id,name,kind,pinned,hidden
FROM topics WHERE user_id=? AND normalized_name=?`, userID, normalized).Scan(&result.ID, &result.Name, &result.Kind, &pinned, &hidden); err != nil {
		return Item{}, fmt.Errorf("read generated topic: %w", err)
	}
	result.Fixed = result.Kind == "core"
	result.Pinned = pinned == 1
	result.Hidden = hidden == 1
	return result, nil
}

func (service *Service) List(ctx context.Context, userID string) (ListResponse, error) {
	if service == nil || service.store == nil || strings.TrimSpace(userID) == "" {
		return ListResponse{}, errors.New("topic storage and user are required")
	}
	var topicsRevision int64
	if err := service.store.DB().QueryRowContext(ctx, "SELECT topics_revision FROM accounts WHERE user_id=?", userID).Scan(&topicsRevision); err != nil {
		return ListResponse{}, fmt.Errorf("read topics revision: %w", err)
	}
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT
  t.topic_id,t.name,t.kind,t.pinned,t.hidden,t.sort_order,t.stable_until,t.updated_at,
  COUNT(ae.entry_id) FILTER (WHERE ae.read_at IS NULL)
FROM topics t
LEFT JOIN entry_topics et ON et.user_id=t.user_id AND et.topic_id=t.topic_id
LEFT JOIN entries e ON e.entry_id=et.entry_id AND e.content_hash=et.content_hash
LEFT JOIN account_entries ae ON ae.user_id=et.user_id AND ae.entry_id=e.entry_id
WHERE t.user_id=?
GROUP BY t.topic_id
ORDER BY t.pinned DESC,t.sort_order,t.topic_id`, userID)
	if err != nil {
		return ListResponse{}, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	var records []topicRow
	for rows.Next() {
		var record topicRow
		var pinned int
		var hidden int
		var stableUntil sql.NullString
		var updatedAt string
		if err := rows.Scan(&record.ID, &record.Name, &record.Kind, &pinned, &hidden, &record.SortOrder, &stableUntil, &updatedAt, &record.UnreadCount); err != nil {
			return ListResponse{}, fmt.Errorf("scan topic: %w", err)
		}
		record.Pinned = pinned == 1
		record.Hidden = hidden == 1
		if record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return ListResponse{}, errors.New("topic contains an invalid timestamp")
		}
		if stableUntil.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, stableUntil.String)
			if err != nil {
				return ListResponse{}, errors.New("topic contains an invalid stable timestamp")
			}
			record.StableUntil = &parsed
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return ListResponse{}, fmt.Errorf("iterate topics: %w", err)
	}
	var totalUnread int
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM account_entries WHERE user_id=? AND read_at IS NULL", userID).Scan(&totalUnread); err != nil {
		return ListResponse{}, fmt.Errorf("count unread entries: %w", err)
	}
	result := ListResponse{Version: versionOf(records), TopicsRevision: topicsRevision, Topics: []Item{{ID: "recommend", Name: "推荐", Kind: "virtual", Fixed: true, UnreadCount: totalUnread}}}
	var activeFilter sql.NullString
	err = service.store.DB().QueryRowContext(ctx, "SELECT filter_id FROM home_filters WHERE user_id=? AND status='active'", userID).Scan(&activeFilter)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ListResponse{}, fmt.Errorf("read active filter: %w", err)
	}
	if activeFilter.Valid {
		result.ActiveFilterID = &activeFilter.String
	}
	now := service.now().UTC()
	for _, record := range records {
		if record.Kind == "dynamic" && !record.Pinned && record.UnreadCount < 3 && (record.StableUntil == nil || !record.StableUntil.After(now)) {
			continue
		}
		result.Topics = append(result.Topics, Item{
			ID:          record.ID,
			Name:        record.Name,
			Kind:        record.Kind,
			Fixed:       record.Kind == "core",
			Pinned:      record.Pinned,
			Hidden:      record.Hidden,
			UnreadCount: record.UnreadCount,
		})
	}
	return result, nil
}

func (service *Service) Patch(ctx context.Context, userID string, version int64, operations []Operation) (ListResponse, error) {
	if len(operations) < 1 || len(operations) > 100 {
		return ListResponse{}, errors.New("topic patch requires 1 to 100 operations")
	}
	current, err := service.List(ctx, userID)
	if err != nil {
		return ListResponse{}, err
	}
	if current.Version != version {
		return ListResponse{}, ErrVersionConflict
	}
	now := service.now().UTC()
	if now.UnixMilli() <= version {
		now = time.UnixMilli(version + 1).UTC()
	}
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		records, err := loadMutableTopics(ctx, transaction, userID)
		if err != nil {
			return err
		}
		for _, operation := range operations {
			if operation.TopicID == "recommend" || strings.TrimSpace(operation.TopicID) == "" {
				return errors.New("recommend topic is immutable")
			}
			index := topicIndex(records, operation.TopicID)
			if index < 0 {
				return errors.New("topic was not found")
			}
			switch operation.Op {
			case "pin":
				records[index].Pinned = true
			case "unpin":
				records[index].Pinned = false
			case "hide":
				records[index].Hidden = true
			case "show":
				records[index].Hidden = false
			case "move":
				target := records[index]
				records = append(records[:index], records[index+1:]...)
				insertAt := 0
				if operation.AfterTopicID != nil {
					afterIndex := topicIndex(records, *operation.AfterTopicID)
					if afterIndex < 0 {
						return errors.New("move anchor topic was not found")
					}
					insertAt = afterIndex + 1
				}
				records = append(records, topicRow{})
				copy(records[insertAt+1:], records[insertAt:])
				records[insertAt] = target
			default:
				return errors.New("unsupported topic operation")
			}
		}
		timestamp := now.Format(time.RFC3339Nano)
		for index, record := range records {
			if _, err := transaction.ExecContext(ctx, "UPDATE topics SET sort_order=?,updated_at=? WHERE user_id=? AND topic_id=?", -(index + 1), timestamp, userID, record.ID); err != nil {
				return fmt.Errorf("stage topic order: %w", err)
			}
		}
		for index, record := range records {
			if _, err := transaction.ExecContext(ctx, "UPDATE topics SET pinned=?,hidden=?,sort_order=?,updated_at=? WHERE user_id=? AND topic_id=?", boolInt(record.Pinned), boolInt(record.Hidden), (index+1)*10, timestamp, userID, record.ID); err != nil {
				return fmt.Errorf("update topic: %w", err)
			}
		}
		if err := service.bumpTopicsRevisionTx(ctx, transaction, userID, timestamp); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ListResponse{}, err
	}
	return service.List(ctx, userID)
}

func (service *Service) ApplyClassification(ctx context.Context, userID, entryID, contentHash string, classification ai.TopicClassificationV1) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		if err := service.ApplyClassificationTx(ctx, transaction, userID, entryID, contentHash, classification); err != nil {
			return err
		}
		return service.indexer.RefreshTx(ctx, transaction, userID, []string{entryID})
	})
}

func (service *Service) ApplyClassificationTx(ctx context.Context, transaction *sql.Tx, userID, entryID, contentHash string, classification ai.TopicClassificationV1) error {
	if transaction == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(entryID) == "" || len(contentHash) != 64 || classification.Version != 1 || len(classification.Topics) < 1 || len(classification.Topics) > 5 {
		return errors.New("valid topic classification is required")
	}
	var storedHash string
	if err := transaction.QueryRowContext(ctx, `
SELECT e.content_hash
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, userID, entryID).Scan(&storedHash); err != nil {
		return errors.New("classification entry was not found")
	}
	if storedHash != contentHash {
		return errors.New("classification content hash is stale")
	}
	assignments := append([]ai.TopicClassification(nil), classification.Topics...)
	sort.Slice(assignments, func(left, right int) bool {
		if assignments[left].Confidence == assignments[right].Confidence {
			return assignments[left].TopicID < assignments[right].TopicID
		}
		return assignments[left].Confidence > assignments[right].Confidence
	})
	seen := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if _, duplicate := seen[assignment.TopicID]; duplicate || assignment.Confidence < 0 || assignment.Confidence > 1 || utf8.RuneCountInString(assignment.Reason) < 1 || utf8.RuneCountInString(assignment.Reason) > 200 {
			return errors.New("topic classification is invalid")
		}
		seen[assignment.TopicID] = struct{}{}
		var exists int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id=? AND topic_id=?", userID, assignment.TopicID).Scan(&exists); err != nil || exists != 1 {
			return errors.New("classification referenced an unknown topic")
		}
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM entry_topics WHERE user_id=? AND entry_id=?", userID, entryID); err != nil {
		return fmt.Errorf("replace topic classification: %w", err)
	}
	now := service.now().UTC().Format(time.RFC3339Nano)
	for index, assignment := range assignments {
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at)
VALUES(?,?,?,?,?,?,?)`, userID, entryID, assignment.TopicID, assignment.Confidence, boolInt(index == 0), contentHash, now); err != nil {
			return fmt.Errorf("insert topic classification: %w", err)
		}
	}
	if err := service.bumpTopicsRevisionTx(ctx, transaction, userID, now); err != nil {
		return err
	}
	return nil
}

func (service *Service) bumpTopicsRevisionTx(ctx context.Context, transaction *sql.Tx, userID, timestamp string) error {
	result, err := transaction.ExecContext(ctx, `
UPDATE accounts
SET topics_revision=topics_revision+1,updated_at=?
WHERE user_id=?`, timestamp, userID)
	if err != nil {
		return fmt.Errorf("advance topics revision: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect topics revision: %w", err)
	}
	if updated != 1 {
		return errors.New("topic account was not found")
	}
	return nil
}

func versionOf(records []topicRow) int64 {
	var version int64 = 1
	for _, record := range records {
		if value := record.UpdatedAt.UnixMilli(); value > version {
			version = value
		}
	}
	return version
}

func loadMutableTopics(ctx context.Context, transaction *sql.Tx, userID string) ([]topicRow, error) {
	rows, err := transaction.QueryContext(ctx, "SELECT topic_id,pinned,hidden,sort_order FROM topics WHERE user_id=? ORDER BY sort_order,topic_id", userID)
	if err != nil {
		return nil, fmt.Errorf("read mutable topics: %w", err)
	}
	defer rows.Close()
	var records []topicRow
	for rows.Next() {
		var record topicRow
		var pinned int
		var hidden int
		if err := rows.Scan(&record.ID, &pinned, &hidden, &record.SortOrder); err != nil {
			return nil, fmt.Errorf("scan mutable topic: %w", err)
		}
		record.Pinned = pinned == 1
		record.Hidden = hidden == 1
		records = append(records, record)
	}
	return records, rows.Err()
}

func topicIndex(records []topicRow, topicID string) int {
	for index, record := range records {
		if record.ID == topicID {
			return index
		}
	}
	return -1
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
