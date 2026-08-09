package http

import (
	"context"
	"database/sql"
	"errors"
	"math"
	stdhttp "net/http"
	"os"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/session"
)

type DiagnosticsConfig struct {
	DB               *sql.DB
	DatabasePath     string
	Version          string
	DeniedFoloRoutes func() uint64
	Now              func() time.Time
}

type diagnosticsHandler struct {
	database         *sql.DB
	databasePath     string
	version          string
	deniedFoloRoutes func() uint64
	now              func() time.Time
}

type diagnosticsResponse struct {
	Version          string              `json:"version"`
	Database         diagnosticsDatabase `json:"database"`
	Jobs             map[string]int      `json:"jobs"`
	Sync             syncStatusResponse  `json:"sync"`
	DeniedFoloRoutes int64               `json:"deniedFoloRoutes"`
}

type diagnosticsDatabase struct {
	SchemaVersion int    `json:"schemaVersion"`
	SizeBytes     int64  `json:"sizeBytes"`
	Integrity     string `json:"integrity"`
}

type syncStatusResponse struct {
	State     string            `json:"state"`
	Scope     *string           `json:"scope"`
	Counts    syncCounts        `json:"counts"`
	Error     *diagnosticsError `json:"error"`
	UpdatedAt string            `json:"updatedAt"`
}

type syncCounts struct {
	Processed int `json:"processed"`
	Total     int `json:"total"`
	Failed    int `json:"failed"`
}

type diagnosticsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewDiagnosticsHandler(config DiagnosticsConfig) (stdhttp.Handler, error) {
	config.Version = strings.TrimSpace(config.Version)
	if config.DB == nil || config.DatabasePath == "" || len(config.Version) < 1 || len(config.Version) > 64 {
		return nil, errors.New("diagnostics database, path and version are required")
	}
	if config.DeniedFoloRoutes == nil {
		config.DeniedFoloRoutes = func() uint64 { return 0 }
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &diagnosticsHandler{database: config.DB, databasePath: config.DatabasePath, version: config.Version, deniedFoloRoutes: config.DeniedFoloRoutes, now: config.Now}, nil
}

func (handler *diagnosticsHandler) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if request.Method != stdhttp.MethodGet {
		writer.WriteHeader(stdhttp.StatusMethodNotAllowed)
		return
	}
	record, ok := session.FromContext(request.Context())
	if !ok {
		writeError(writer, request.Header.Get("X-Request-Id"), stdhttp.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	response, err := handler.snapshot(request.Context(), record.User.ID)
	if err != nil {
		writeError(writer, request.Header.Get("X-Request-Id"), stdhttp.StatusInternalServerError, "LOCAL_STORAGE_ERROR", "本地诊断数据暂不可用")
		return
	}
	writeJSON(writer, stdhttp.StatusOK, response)
}

func (handler *diagnosticsHandler) snapshot(ctx context.Context, userID string) (diagnosticsResponse, error) {
	database, err := handler.databaseSnapshot(ctx)
	if err != nil {
		return diagnosticsResponse{}, err
	}
	jobs := map[string]int{"queued": 0, "running": 0, "succeeded": 0, "failed": 0, "cancelled": 0}
	rows, err := handler.database.QueryContext(ctx, "SELECT state,COUNT(*) FROM jobs WHERE user_id=? GROUP BY state", userID)
	if err != nil {
		return diagnosticsResponse{}, err
	}
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			_ = rows.Close()
			return diagnosticsResponse{}, err
		}
		if _, expected := jobs[state]; expected {
			jobs[state] = count
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return diagnosticsResponse{}, err
	}
	if err := rows.Close(); err != nil {
		return diagnosticsResponse{}, err
	}
	syncStatus, err := readSyncStatus(ctx, handler.database, userID, handler.now().UTC())
	if err != nil {
		return diagnosticsResponse{}, err
	}
	denied := handler.deniedFoloRoutes()
	if denied > math.MaxInt64 {
		denied = math.MaxInt64
	}
	return diagnosticsResponse{Version: handler.version, Database: database, Jobs: jobs, Sync: syncStatus, DeniedFoloRoutes: int64(denied)}, nil
}

func (handler *diagnosticsHandler) databaseSnapshot(ctx context.Context) (diagnosticsDatabase, error) {
	var schemaVersion int
	if err := handler.database.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&schemaVersion); err != nil {
		return diagnosticsDatabase{}, err
	}
	integrity := "unknown"
	var checked string
	if err := handler.database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&checked); err != nil {
		integrity = "error"
	} else if checked == "ok" {
		integrity = "ok"
	} else {
		integrity = "error"
	}
	info, err := os.Stat(handler.databasePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
		return diagnosticsDatabase{}, errors.New("database file is unavailable")
	}
	return diagnosticsDatabase{SchemaVersion: schemaVersion, SizeBytes: info.Size(), Integrity: integrity}, nil
}

func readSyncStatus(ctx context.Context, database *sql.DB, userID string, now time.Time) (syncStatusResponse, error) {
	result := syncStatusResponse{State: "idle", Counts: syncCounts{}, UpdatedAt: now.Format(time.RFC3339Nano)}
	var scope sql.NullString
	var errorCode sql.NullString
	err := database.QueryRowContext(ctx, `
SELECT state,scope,processed,total,failed,error_code,updated_at
FROM sync_state WHERE user_id=?`, userID).Scan(&result.State, &scope, &result.Counts.Processed, &result.Counts.Total, &result.Counts.Failed, &errorCode, &result.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return syncStatusResponse{}, err
	}
	if _, err := time.Parse(time.RFC3339Nano, result.UpdatedAt); err != nil {
		return syncStatusResponse{}, errors.New("sync status timestamp is invalid")
	}
	if scope.Valid {
		result.Scope = &scope.String
	}
	if errorCode.Valid {
		result.Error = &diagnosticsError{Code: publicErrorCode(errorCode.String), Message: "同步任务未完成，请稍后重试"}
	}
	return result, nil
}

func publicErrorCode(code string) string {
	allowed := map[string]bool{
		"FOLO_UNAVAILABLE":        true,
		"FOLO_RATE_LIMITED":       true,
		"AI_NOT_CONFIGURED":       true,
		"AI_PROVIDER_UNAVAILABLE": true,
		"AI_OUTPUT_INVALID":       true,
		"LOCAL_STORAGE_ERROR":     true,
		"VALIDATION_ERROR":        true,
		"SERVICE_NOT_READY":       true,
	}
	if allowed[code] {
		return code
	}
	return "LOCAL_STORAGE_ERROR"
}
