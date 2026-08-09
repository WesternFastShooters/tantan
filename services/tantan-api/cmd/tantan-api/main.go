package main

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	stdhttp "net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/folo"
	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/session"
)

const defaultListenAddress = "127.0.0.1:3000"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("tantan_api_stopped", slog.String("errorCode", "SERVICE_NOT_READY"), slog.String("reason", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	listenAddress := os.Getenv("TANTAN_LISTEN_ADDR")
	if listenAddress == "" {
		listenAddress = defaultListenAddress
	}
	if err := localhttp.ValidateListenAddr(listenAddress); err != nil {
		return err
	}

	upstream, err := resolveServerURL(os.Getenv("TANTAN_FOLO_API_URL"), "https://api.folo.is", "api.folo.is")
	if err != nil {
		return err
	}
	foloWebURL, err := resolveServerURL(os.Getenv("TANTAN_FOLO_WEB_URL"), "https://app.folo.is", "app.folo.is")
	if err != nil {
		return err
	}
	client := &stdhttp.Client{Timeout: 60 * time.Second}
	policy, err := folo.LoadPolicy()
	if err != nil {
		return err
	}
	sessions := session.NewStore(time.Now)
	secrets, err := session.NewKeyringSecretStore(session.FoloSessionService)
	if err != nil {
		return err
	}
	foloAuth, err := folo.NewAuthClient(upstream, client)
	if err != nil {
		return err
	}
	bridge, err := auth.NewBridge(auth.Config{
		FoloWebURL:  foloWebURL.String(),
		CallbackURL: "http://127.0.0.1:3000/auth/folo/callback",
		Logger:      logger,
		Sessions:    sessions,
		Secrets:     secrets,
		Folo:        foloAuth,
	})
	if err != nil {
		return err
	}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstream,
		Client:   client,
		Secrets:  secrets,
		Logger:   logger,
	})
	if err != nil {
		return err
	}
	handler := localhttp.NewRouter(localhttp.RouterConfig{
		Auth:     bridge,
		Proxy:    proxy,
		Sessions: sessions,
		Logger:   logger,
	})
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	server := &stdhttp.Server{
		Addr:              listenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       70 * time.Second,
		WriteTimeout:      70 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 * 1024,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		contextWithTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(contextWithTimeout)
	}()
	logger.Info("tantan_api_started", slog.String("listenAddress", listenAddress))
	if err := server.Serve(listener); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		return err
	}
	return nil
}

func resolveServerURL(raw, defaultValue, officialHost string) (*url.URL, error) {
	if raw == "" {
		raw = defaultValue
	}
	value, err := url.Parse(raw)
	if err != nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return nil, errors.New("invalid Folo server URL")
	}
	if value.Scheme == "https" && value.Host == officialHost {
		value.Path = ""
		return value, nil
	}
	if value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost") {
		value.Path = ""
		return value, nil
	}
	return nil, errors.New("Folo server URL must use the built-in host or a loopback test server")
}
