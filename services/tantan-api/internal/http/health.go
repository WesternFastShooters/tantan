package http

import (
	"context"
	stdhttp "net/http"
	"strings"

	"tantan.local/tantan-api/internal/ops"
)

func defaultHealthHandler() stdhttp.Handler {
	return NewHealthHandler(version, nil)
}

type ReadinessChecker interface {
	Check(context.Context) ops.ReadinessResult
}

func NewHealthHandler(buildVersion string, readiness ReadinessChecker) stdhttp.Handler {
	buildVersion = strings.TrimSpace(buildVersion)
	if buildVersion == "" {
		buildVersion = "dev"
	}
	return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		if request.Method != stdhttp.MethodGet {
			writer.WriteHeader(stdhttp.StatusMethodNotAllowed)
			return
		}
		switch request.URL.Path {
		case "/healthz":
			writeJSON(writer, stdhttp.StatusOK, map[string]string{"status": "ok", "version": buildVersion})
		case "/readyz":
			if readiness == nil {
				writeError(writer, request.Header.Get("X-Request-Id"), stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪", true)
				return
			}
			result := readiness.Check(request.Context())
			if !result.Ready {
				writeError(writer, request.Header.Get("X-Request-Id"), stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "本地存储或系统钥匙串不可用", true)
				return
			}
			writeJSON(writer, stdhttp.StatusOK, result)
		default:
			writer.WriteHeader(stdhttp.StatusNotFound)
		}
	})
}
