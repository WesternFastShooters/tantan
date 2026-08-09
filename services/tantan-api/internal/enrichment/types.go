package enrichment

import (
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

type Config struct {
	Store         *storage.Store
	Settings      *ai.SettingsService
	Generator     ai.Generator
	Topics        *topic.Service
	Now           func() time.Time
	PromptVersion string
}

type EnsureRequest struct {
	UserID   string
	EntryID  string
	Language string
	Fields   []string
}

type Accepted struct {
	JobID string
}

type Result struct {
	State     string
	Data      *ai.EnrichmentV1
	ErrorCode string
}

type jobPayload struct {
	UserID        string   `json:"userId"`
	EntryID       string   `json:"entryId"`
	Language      string   `json:"language"`
	Fields        []string `json:"fields"`
	ProviderFP    string   `json:"providerFp"`
	ContentHash   string   `json:"contentHash"`
	PromptVersion string   `json:"promptVersion"`
}
