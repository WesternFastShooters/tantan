package sync

import (
	"context"
	"io"
	"time"
)

const PageSize = 100

type Mode string

const (
	ModeAuto        Mode = "auto"
	ModeFull        Mode = "full"
	ModeIncremental Mode = "incremental"
)

type Account struct {
	ID       string
	Name     string
	Avatar   *string
	Timezone string
}

type RemoteFeed struct {
	ID    string
	Title string
	URL   string
	Image *string
	View  int
}

type RemoteEntry struct {
	ID          string
	Feed        RemoteFeed
	View        int
	Title       string
	Description string
	Author      string
	URL         string
	Language    string
	MediaJSON   []byte
	PublishedAt time.Time
	Read        bool
	CollectedAt *time.Time
}

type PageRequest struct {
	Limit           int
	PublishedAfter  *time.Time
	PublishedBefore *time.Time
}

type Source interface {
	ListSubscriptions(ctx context.Context, userID string) ([]RemoteFeed, error)
	ListEntries(ctx context.Context, userID string, request PageRequest) ([]RemoteEntry, error)
	StreamContents(ctx context.Context, userID string, entryIDs []string) (io.ReadCloser, error)
}

type Checkpoint struct {
	Mode            Mode       `json:"mode"`
	PublishedAfter  *time.Time `json:"publishedAfter"`
	PublishedBefore *time.Time `json:"publishedBefore"`
	Processed       int        `json:"processed"`
	Failed          int        `json:"failed"`
}

type RunOptions struct {
	Mode Mode
}

type Result struct {
	Processed int
	Failed    int
}
