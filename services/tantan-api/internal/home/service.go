package home

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/storage"
)

const (
	defaultFilterKey = "default"
	initialQueueSize = 50
	queueHardLimit   = 60
)

var homeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

type Service struct {
	store            *storage.Store
	cursorKey        []byte
	now              func() time.Time
	appendMutex      sync.Mutex
	appendWatermarks map[string]queueWatermark
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("home storage is required")
	}
	if len(config.CursorKey) < 32 {
		return nil, errors.New("home cursor key must contain at least 32 bytes")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: config.Store, cursorKey: append([]byte(nil), config.CursorKey...), now: now, appendWatermarks: make(map[string]queueWatermark)}, nil
}

func (service *Service) Get(ctx context.Context, query Query) (Page, error) {
	query.UserID = strings.TrimSpace(query.UserID)
	query.TopicID = strings.TrimSpace(query.TopicID)
	query.FilterID = strings.TrimSpace(query.FilterID)
	query.Timezone = strings.TrimSpace(query.Timezone)
	if query.Timezone == "" {
		query.Timezone = "Asia/Shanghai"
	}
	if !validHomeID(query.UserID) || !validHomeID(query.TopicID) || (query.FilterID != "" && !validHomeID(query.FilterID)) {
		return Page{}, errors.New("valid home user, topic, and filter are required")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 50 {
		return Page{}, errors.New("home limit must be 1 to 50")
	}
	if err := service.validateTopic(ctx, query.UserID, query.TopicID); err != nil {
		return Page{}, err
	}
	filterKey, spec, err := service.resolveFilter(ctx, query.UserID, query.FilterID)
	if err != nil {
		return Page{}, err
	}
	queue, err := service.ensureQueue(ctx, PlanRequest{UserID: query.UserID, Timezone: query.Timezone, FilterKey: filterKey, Spec: spec})
	if err != nil {
		return Page{}, err
	}
	if err := service.reconcileReads(ctx, query.UserID, queue.ID); err != nil {
		return Page{}, err
	}
	queryHash := homeQueryHash(query.UserID, query.TopicID, filterKey, query.Timezone)
	afterRank := 0
	if query.Cursor != "" {
		cursor, err := decodeCursor(service.cursorKey, query.Cursor)
		if err != nil {
			return Page{}, err
		}
		if cursor.QueryHash != queryHash {
			return Page{}, ErrCursorMismatch
		}
		if cursor.Generation != queue.Generation || cursor.QueueID != queue.ID || cursor.QueueVer != queue.Version {
			return Page{}, ErrQueueVersionChanged
		}
		afterRank = cursor.AfterRank
	}
	items, next, state, err := service.readPage(ctx, query.UserID, query.TopicID, queue, query.Limit, afterRank, queryHash)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, NextCursor: next, Queue: state, QueueGeneration: state.Generation}, nil
}

func (service *Service) Rebuild(ctx context.Context, request PlanRequest) (QueueState, error) {
	plan, err := service.Plan(ctx, request)
	if err != nil {
		return QueueState{}, err
	}
	var state QueueState
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		var err error
		state, err = service.ReplaceTx(ctx, transaction, plan)
		return err
	})
	return state, err
}

func (service *Service) Plan(ctx context.Context, request PlanRequest) (QueuePlan, error) {
	request.UserID = strings.TrimSpace(request.UserID)
	request.Timezone = strings.TrimSpace(request.Timezone)
	request.FilterKey = strings.TrimSpace(request.FilterKey)
	if request.FilterKey == "" {
		request.FilterKey = defaultFilterKey
	}
	if !validHomeID(request.UserID) || !validHomeID(request.FilterKey) {
		return QueuePlan{}, errors.New("valid queue user and filter key are required")
	}
	now := service.now().UTC()
	location, localDate, _, err := calendarWindow(now, request.Timezone, request.Spec)
	if err != nil {
		return QueuePlan{}, err
	}
	_ = location
	candidates, err := service.loadCandidates(ctx, candidateRequest{UserID: request.UserID, Timezone: request.Timezone, Spec: request.Spec, Now: now})
	if err != nil {
		return QueuePlan{}, err
	}
	ranked := recommendation.Rank(now, candidates, initialQueueSize)
	queueID, err := newQueueID()
	if err != nil {
		return QueuePlan{}, err
	}
	return QueuePlan{
		ID:          queueID,
		UserID:      request.UserID,
		LocalDate:   localDate,
		FilterKey:   request.FilterKey,
		Timezone:    request.Timezone,
		GeneratedAt: now,
		Items:       ranked,
	}, nil
}

func validHomeID(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && homeIdentifierPattern.MatchString(value)
}

func newQueueID() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		return "", errors.New("create queue ID failed")
	}
	return "queue_" + base64.RawURLEncoding.EncodeToString(buffer), nil
}

func calendarWindow(now time.Time, timezone string, spec *recommendation.FilterSpecV1) (*time.Location, string, time.Time, error) {
	if len(timezone) < 1 || len(timezone) > 64 {
		return nil, "", time.Time{}, errors.New("timezone is invalid")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, "", time.Time{}, errors.New("timezone is invalid")
	}
	local := now.In(location)
	days := 7
	if spec != nil && spec.WindowDays < days {
		days = spec.WindowDays
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -(days - 1)).UTC()
	return location, local.Format(time.DateOnly), start, nil
}
