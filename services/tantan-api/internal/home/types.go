package home

import (
	"time"

	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/storage"
)

type Config struct {
	Store     *storage.Store
	CursorKey []byte
	Now       func() time.Time
}

type Query struct {
	UserID   string
	TopicID  string
	FilterID string
	Timezone string
	Limit    int
	Cursor   string
}

type PlanRequest struct {
	UserID    string
	Timezone  string
	FilterKey string
	Spec      *recommendation.FilterSpecV1
}

type QueueState struct {
	ID                  string `json:"id"`
	Version             int    `json:"version"`
	Generation          string `json:"generation"`
	Total               int    `json:"total"`
	Unread              int    `json:"unread"`
	Finished            bool   `json:"finished"`
	CandidateWindowDays int    `json:"candidateWindowDays"`
	GeneratedAt         string `json:"generatedAt"`
}

type Topic struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Source struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Avatar *string `json:"avatar"`
}

type Card struct {
	EntryID     string  `json:"entryId"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Excerpt     *string `json:"excerpt"`
	Cover       *string `json:"cover"`
	Source      Source  `json:"source"`
	PublishedAt string  `json:"publishedAt"`
	Topics      []Topic `json:"topics"`
	Translated  bool    `json:"translated"`
	Rank        int     `json:"-"`
}

type Page struct {
	Items           []Card     `json:"items"`
	NextCursor      *string    `json:"nextCursor"`
	Queue           QueueState `json:"queue"`
	QueueGeneration string     `json:"queueGeneration"`
}

type QueuePlan struct {
	ID          string
	UserID      string
	LocalDate   string
	FilterKey   string
	Timezone    string
	GeneratedAt time.Time
	Items       []recommendation.ScoredCandidate
}
