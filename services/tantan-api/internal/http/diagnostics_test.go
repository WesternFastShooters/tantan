package http_test

import (
	"context"
	"database/sql"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
)

func TestDiagnosticsReturnsOnlyAggregatesForCurrentSession(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	const canary = "diagnostics-CANARY-private-content"
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_1','Diag User','Asia/Shanghai',?,?)", timestamp, timestamp); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO sync_state(user_id,state,scope,total,processed,failed,error_code,updated_at) VALUES('user_1','failed','entries',10,7,3,'FOLO_UNAVAILABLE',?)", timestamp); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO jobs(job_id,user_id,kind,dedupe_key,state,payload_json,next_run_at,error_code,created_at,updated_at) VALUES('job_1','user_1','sync','diag','failed','{}',?,'FOLO_UNAVAILABLE',?,?)", timestamp, timestamp, timestamp); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES('feed_1','Diag Source',0,?)", timestamp); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_1','feed_1','article','Diag',?,'[]',?,'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',?,?)", canary, timestamp, timestamp, timestamp); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := localhttp.NewDiagnosticsHandler(localhttp.DiagnosticsConfig{DB: store.DB(), DatabasePath: store.Path(), Version: "test-version", DeniedFoloRoutes: func() uint64 { return 7 }, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/tantan/v1/diagnostics", nil)
	record := session.Record{User: session.User{ID: "user_1", Name: "Diag User"}, Timezone: "Asia/Shanghai"}
	request = request.WithContext(session.WithRecord(request.Context(), record))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK || strings.Contains(response.Body.String(), canary) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != "test-version" || body["deniedFoloRoutes"] != float64(7) {
		t.Fatalf("diagnostics=%v", body)
	}
	jobs := body["jobs"].(map[string]any)
	syncStatus := body["sync"].(map[string]any)
	if jobs["failed"] != float64(1) || syncStatus["state"] != "failed" || syncStatus["error"].(map[string]any)["code"] != "FOLO_UNAVAILABLE" {
		t.Fatalf("jobs=%v sync=%v", jobs, syncStatus)
	}
}
