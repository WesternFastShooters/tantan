package home

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
)

type candidateRequest struct {
	UserID       string
	Timezone     string
	Spec         *recommendation.FilterSpecV1
	Now          time.Time
	CreatedAfter *time.Time
	ExcludeQueue string
}

type candidateRecord struct {
	Candidate recommendation.Candidate
	Title     string
	Body      string
	Language  string
	Kind      string
	TopicIDs  []string
}

func (service *Service) loadCandidates(ctx context.Context, request candidateRequest) ([]recommendation.Candidate, error) {
	now := request.Now.UTC()
	if now.IsZero() {
		now = service.now().UTC()
	}
	_, _, start, err := calendarWindow(now, request.Timezone, request.Spec)
	if err != nil {
		return nil, err
	}
	statement := `
SELECT
  e.entry_id,e.feed_id,e.published_at,e.title,
  trim(COALESCE(e.description,'') || ' ' || COALESCE(e.content,'')),
  COALESCE(e.language,''),e.kind,
  COALESCE((SELECT en.quality_score FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND en.quality_score IS NOT NULL
    ORDER BY en.updated_at DESC LIMIT 1),5),
  COALESCE((SELECT et.topic_id FROM entry_topics et
    WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id AND et.content_hash=e.content_hash AND et.is_primary=1 LIMIT 1),''),
  COALESCE((SELECT json_group_array(et.topic_id) FROM entry_topics et
    WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id AND et.content_hash=e.content_hash),'[]')
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
JOIN feeds f ON f.feed_id=e.feed_id
WHERE ae.user_id=? AND ae.read_at IS NULL
  AND e.published_at>=? AND e.published_at<?
  AND julianday(e.published_at)>=julianday(?) AND julianday(e.published_at)<=julianday(?)
  AND NOT EXISTS(SELECT 1 FROM recommendation_blocks b WHERE b.user_id=ae.user_id AND b.target_type='entry' AND b.target_id=e.entry_id)
  AND NOT EXISTS(SELECT 1 FROM recommendation_blocks b WHERE b.user_id=ae.user_id AND b.target_type='source' AND b.target_id=e.feed_id)
  AND NOT EXISTS(
    SELECT 1 FROM recommendation_blocks b
    JOIN entry_topics et ON et.user_id=b.user_id AND et.topic_id=b.target_id
    WHERE b.user_id=ae.user_id AND b.target_type='topic' AND et.entry_id=e.entry_id AND et.content_hash=e.content_hash
  )`
	arguments := []any{
		request.UserID,
		start.Add(-time.Second).Format(time.RFC3339Nano),
		now.Add(time.Second).Format(time.RFC3339Nano),
		start.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	}
	if request.CreatedAfter != nil {
		statement += "\n  AND e.created_at>?"
		arguments = append(arguments, request.CreatedAfter.UTC().Format(time.RFC3339Nano))
	}
	if request.ExcludeQueue != "" {
		statement += "\n  AND NOT EXISTS(SELECT 1 FROM daily_queue_items existing WHERE existing.queue_id=? AND existing.entry_id=e.entry_id)"
		arguments = append(arguments, request.ExcludeQueue)
	}
	statement += "\nORDER BY e.published_at DESC,e.entry_id ASC LIMIT 500"
	rows, err := service.store.DB().QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load home candidates: %w", err)
	}
	defer rows.Close()
	records := make([]candidateRecord, 0, 500)
	for rows.Next() {
		var record candidateRecord
		var published string
		var quality sql.NullFloat64
		var topicsJSON string
		if err := rows.Scan(&record.Candidate.EntryID, &record.Candidate.SourceID, &published, &record.Title, &record.Body, &record.Language, &record.Kind, &quality, &record.Candidate.PrimaryTopicID, &topicsJSON); err != nil {
			return nil, fmt.Errorf("scan home candidate: %w", err)
		}
		record.Candidate.PublishedAt, err = time.Parse(time.RFC3339Nano, published)
		if err != nil {
			return nil, errors.New("home candidate has invalid published time")
		}
		if record.Candidate.PublishedAt.Before(start) || record.Candidate.PublishedAt.After(now) {
			continue
		}
		if quality.Valid {
			record.Candidate.Quality = quality.Float64
		} else {
			record.Candidate.Quality = 5
		}
		if err := json.Unmarshal([]byte(topicsJSON), &record.TopicIDs); err != nil {
			return nil, errors.New("home candidate has invalid topics")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate home candidates: %w", err)
	}
	result := make([]recommendation.Candidate, 0, len(records))
	for _, record := range records {
		candidate, ok := applyFilter(record, request.Spec)
		if ok {
			result = append(result, candidate)
		}
	}
	return result, nil
}

func applyFilter(record candidateRecord, spec *recommendation.FilterSpecV1) (recommendation.Candidate, bool) {
	candidate := record.Candidate
	if spec == nil {
		return candidate, true
	}
	topics := stringSet(record.TopicIDs)
	if len(spec.IncludeTopics) > 0 && !setIntersects(topics, spec.IncludeTopics) {
		return recommendation.Candidate{}, false
	}
	if setIntersects(topics, spec.ExcludeTopics) || containsString(spec.ExcludeSources, candidate.SourceID) {
		return recommendation.Candidate{}, false
	}
	if len(spec.IncludeSources) > 0 && !containsString(spec.IncludeSources, candidate.SourceID) {
		return recommendation.Candidate{}, false
	}
	if len(spec.Languages) > 0 && !containsString(spec.Languages, record.Language) {
		return recommendation.Candidate{}, false
	}
	if len(spec.ContentStyles) > 0 && !containsString(spec.ContentStyles, record.Kind) {
		return recommendation.Candidate{}, false
	}
	haystack := strings.ToLower(record.Title + " " + record.Body)
	if matchesAny(haystack, spec.NegativeTerms) {
		return recommendation.Candidate{}, false
	}
	if len(spec.IncludeTerms) > 0 && !matchesAny(haystack, spec.IncludeTerms) {
		return recommendation.Candidate{}, false
	}
	matched := 0
	total := 0
	for _, values := range []struct {
		count bool
		match bool
	}{
		{count: len(spec.IncludeTopics) > 0, match: setIntersects(topics, spec.IncludeTopics)},
		{count: len(spec.IncludeSources) > 0, match: containsString(spec.IncludeSources, candidate.SourceID)},
		{count: len(spec.IncludeTerms) > 0, match: matchesAny(haystack, spec.IncludeTerms)},
	} {
		if values.count {
			total++
			if values.match {
				matched++
			}
		}
	}
	if total > 0 {
		candidate.FilterMatch = 30 * float64(matched) / float64(total)
	}
	weights := spec.Weights
	candidate.FilterWeights = &weights
	return candidate, true
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func setIntersects(set map[string]struct{}, values []string) bool {
	for _, value := range values {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func matchesAny(haystack string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(haystack, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
