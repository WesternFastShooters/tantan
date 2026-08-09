package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"log/slog"
	"net"
	stdhttp "net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/keyring"
	"tantan.local/tantan-api/internal/observability"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/session"
)

const defaultListenAddress = "127.0.0.1:3000"

var buildVersion = "dev"

type serveOptions struct {
	DataDir       string
	ListenAddress string
	FoloAPIURL    string
	FoloWebURL    string
}

func main() {
	ctx := context.Background()
	handled, err := runManagementCommand(ctx, os.Args[1:], os.Stdout)
	if handled {
		if err != nil {
			managementErrorLogger().Error("tantan_command_failed", slog.String("errorCode", managementErrorCode(err)))
			os.Exit(1)
		}
		return
	}
	options, err := parseServeOptions(os.Args[1:])
	if err != nil {
		managementErrorLogger().Error("tantan_api_stopped", slog.String("errorCode", "VALIDATION_ERROR"))
		os.Exit(1)
	}
	logger, logWriter, err := runtimeLogger(options.DataDir)
	if err != nil {
		managementErrorLogger().Error("tantan_api_stopped", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
		os.Exit(1)
	}
	defer logWriter.Close()
	if err := runWithOptions(logger, options); err != nil {
		logger.Error("tantan_api_stopped", slog.String("errorCode", "SERVICE_NOT_READY"))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	return runWithOptions(logger, serveOptions{DataDir: configuredDataDirectory(), ListenAddress: configuredListenAddress(), FoloAPIURL: os.Getenv("TANTAN_FOLO_API_URL"), FoloWebURL: os.Getenv("TANTAN_FOLO_WEB_URL")})
}

func runWithOptions(logger *slog.Logger, options serveOptions) error {
	if err := localhttp.ValidateListenAddr(options.ListenAddress); err != nil {
		return err
	}
	upstream, err := resolveServerURL(options.FoloAPIURL, "https://api.folo.is", "api.folo.is")
	if err != nil {
		return err
	}
	foloWebURL, err := resolveServerURL(options.FoloWebURL, "https://app.folo.is", "app.folo.is")
	if err != nil {
		return err
	}
	client := &stdhttp.Client{Timeout: 60 * time.Second}
	foloSecrets, err := session.NewKeyringSecretStore(session.FoloSessionService)
	if err != nil {
		return err
	}
	aiSecrets, err := keyring.NewAIProviderStore()
	if err != nil {
		return err
	}
	probeSecrets, err := keyring.NewOSStore(readinessKeychainService)
	if err != nil {
		return err
	}
	cursorSecrets, err := keyring.NewOSStore("tantan.cursor.signing")
	if err != nil {
		return err
	}
	application, err := newApplication(context.Background(), applicationConfig{DataDir: options.DataDir, Upstream: upstream, FoloWebURL: foloWebURL, Client: client, FoloSecrets: foloSecrets, AISecrets: aiSecrets, ProbeKeychain: probeSecrets, CursorSecrets: cursorSecrets, Logger: logger, Now: time.Now, Version: buildVersion, StartWorkers: true})
	if err != nil {
		return err
	}
	defer application.Close()
	listener, err := net.Listen("tcp", options.ListenAddress)
	if err != nil {
		return err
	}
	server := &stdhttp.Server{
		Addr:              options.ListenAddress,
		Handler:           application.Handler,
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
	logger.Info("tantan_api_started", slog.String("listenAddress", options.ListenAddress))
	if err := server.Serve(listener); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		return err
	}
	return nil
}

func parseServeOptions(arguments []string) (serveOptions, error) {
	if len(arguments) > 0 && arguments[0] == "serve" {
		arguments = arguments[1:]
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	options := serveOptions{}
	flags.StringVar(&options.DataDir, "data-dir", configuredDataDirectory(), "local Tantan data directory")
	flags.StringVar(&options.ListenAddress, "listen-address", configuredListenAddress(), "fixed loopback address")
	flags.StringVar(&options.FoloAPIURL, "folo-api-url", os.Getenv("TANTAN_FOLO_API_URL"), "built-in Folo API URL")
	flags.StringVar(&options.FoloWebURL, "folo-web-url", os.Getenv("TANTAN_FOLO_WEB_URL"), "built-in Folo web URL")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || options.DataDir == "" {
		return serveOptions{}, errors.New("invalid serve arguments")
	}
	return options, nil
}

func runtimeLogger(dataDirectory string) (*slog.Logger, *observability.RotatingWriter, error) {
	writer, err := observability.NewRotatingWriter(observability.RotatingWriterConfig{Path: filepath.Join(dataDirectory, "logs", "tantan.jsonl"), MaxBytes: 10 * 1024 * 1024, Backups: 5})
	if err != nil {
		return nil, nil, err
	}
	output := io.MultiWriter(os.Stdout, writer)
	return slog.New(slog.NewJSONHandler(output, nil)), writer, nil
}

func managementErrorLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil))
}

func managementErrorCode(err error) string {
	switch {
	case errors.Is(err, errDoctorChecksFailed):
		return "SERVICE_NOT_READY"
	case errors.Is(err, ops.ErrDestinationExists):
		return "BACKUP_DESTINATION_EXISTS"
	case errors.Is(err, ops.ErrServiceRunning):
		return "SERVICE_RUNNING"
	default:
		return "LOCAL_STORAGE_ERROR"
	}
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
