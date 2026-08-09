package http

import (
	stdhttp "net/http"
)

func defaultHealthHandler() stdhttp.Handler {
	return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		if request.Method != stdhttp.MethodGet {
			writer.WriteHeader(stdhttp.StatusMethodNotAllowed)
			return
		}
		switch request.URL.Path {
		case "/healthz":
			writeJSON(writer, stdhttp.StatusOK, map[string]string{"status": "ok", "version": version})
		case "/readyz":
			writeError(writer, request.Header.Get("X-Request-Id"), stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪")
		default:
			writer.WriteHeader(stdhttp.StatusNotFound)
		}
	})
}
