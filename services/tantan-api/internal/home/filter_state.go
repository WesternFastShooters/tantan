package home

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"tantan.local/tantan-api/internal/recommendation"
)

func (service *Service) resolveFilter(ctx context.Context, userID, requestedID string) (string, *recommendation.FilterSpecV1, error) {
	var filterID string
	var normalized string
	var err error
	if requestedID != "" {
		err = service.store.DB().QueryRowContext(ctx, `
SELECT filter_id,normalized_json FROM home_filters
WHERE user_id=? AND filter_id=? AND status='active'`, userID, requestedID).Scan(&filterID, &normalized)
	} else {
		err = service.store.DB().QueryRowContext(ctx, `
SELECT filter_id,normalized_json FROM home_filters
WHERE user_id=? AND status='active'`, userID).Scan(&filterID, &normalized)
	}
	if errors.Is(err, sql.ErrNoRows) && requestedID == "" {
		return defaultFilterKey, nil, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, errors.New("active home filter was not found")
	}
	if err != nil {
		return "", nil, fmt.Errorf("read active home filter: %w", err)
	}
	spec, _, err := recommendation.ValidateFilterSpec([]byte(normalized))
	if err != nil {
		return "", nil, errors.New("active home filter is invalid")
	}
	return filterID, &spec, nil
}

func (service *Service) validateTopic(ctx context.Context, userID, topicID string) error {
	if topicID == "recommend" {
		return nil
	}
	var exists int
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id=? AND topic_id=?", userID, topicID).Scan(&exists); err != nil {
		return fmt.Errorf("validate home topic: %w", err)
	}
	if exists != 1 {
		return errors.New("home topic was not found")
	}
	return nil
}
