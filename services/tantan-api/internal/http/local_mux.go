package http

import (
	stdhttp "net/http"
	"net/url"
)

type PreflightAuthorizer interface {
	AllowsPreflight(method, path string) bool
}

type LocalMux struct {
	mux *stdhttp.ServeMux
}

func NewLocalMux() *LocalMux {
	return &LocalMux{mux: stdhttp.NewServeMux()}
}

func (mux *LocalMux) Handle(method, pattern string, handler stdhttp.Handler) {
	mux.mux.Handle(method+" "+pattern, handler)
}

func (mux *LocalMux) HandleFunc(method, pattern string, handler stdhttp.HandlerFunc) {
	mux.Handle(method, pattern, handler)
}

func (mux *LocalMux) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	mux.mux.ServeHTTP(writer, request)
}

func (mux *LocalMux) AllowsPreflight(method, path string) bool {
	_, pattern := mux.mux.Handler(&stdhttp.Request{Method: method, URL: &url.URL{Path: path}})
	return pattern != ""
}
