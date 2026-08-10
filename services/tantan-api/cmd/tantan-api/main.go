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
	"strconv"
	"strings"
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
	DataDir             string
	StaticDir           string
	ListenAddress       string
	PublicOrigin        string
	TrustedProxyCIDRs   []string
	SingleUser          bool
	SingleUserAccessID  string
	CloudflareContainer bool
	GatewaySecret       string
	MasterKeyFile       string
	MasterKeyBase64     string
	GeminiAPIKeyFile    string
	GeminiAPIKey        string
	FoloAPIURL          string
	FoloWebURL          string
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
	return runWithOptions(logger, serveOptions{
		DataDir:             configuredDataDirectory(),
		StaticDir:           os.Getenv("TANTAN_STATIC_DIR"),
		ListenAddress:       configuredListenAddress(),
		PublicOrigin:        configuredPublicOrigin(),
		TrustedProxyCIDRs:   splitConfiguredList(os.Getenv("TANTAN_TRUSTED_PROXY_CIDRS")),
		SingleUser:          configuredSingleUser(),
		SingleUserAccessID:  strings.TrimSpace(os.Getenv("TANTAN_OWNER_ACCESS_ID")),
		CloudflareContainer: configuredCloudflareContainer(),
		GatewaySecret:       os.Getenv("TANTAN_GATEWAY_SECRET"),
		MasterKeyFile:       os.Getenv("TANTAN_MASTER_KEY_FILE"),
		MasterKeyBase64:     os.Getenv("TANTAN_MASTER_KEY_B64"),
		GeminiAPIKeyFile:    os.Getenv("TANTAN_GEMINI_API_KEY_FILE"),
		GeminiAPIKey:        os.Getenv("TANTAN_GEMINI_API_KEY"),
		FoloAPIURL:          os.Getenv("TANTAN_FOLO_API_URL"),
		FoloWebURL:          os.Getenv("TANTAN_FOLO_WEB_URL"),
	})
}

func runWithOptions(logger *slog.Logger, options serveOptions) error {
	if err := localhttp.ValidateRuntimeListenAddr(options.ListenAddress, options.CloudflareContainer); err != nil {
		return err
	}
	if options.CloudflareContainer {
		if !options.SingleUser || len(options.GatewaySecret) < 32 || len(options.GatewaySecret) > 256 || strings.ContainsAny(options.GatewaySecret, "\r\n\x00") || !strings.HasPrefix(options.PublicOrigin, "https://") {
			return errors.New("Cloudflare container mode requires HTTPS, single-user mode and a private gateway secret")
		}
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
	var foloSecrets session.SecretStore
	var foloMasterKey []byte
	if options.MasterKeyFile != "" && options.MasterKeyBase64 != "" {
		return errors.New("configure exactly one server master key source")
	}
	if options.MasterKeyFile != "" {
		foloMasterKey, err = loadMasterKeyFile(options.MasterKeyFile)
		if err != nil {
			return err
		}
		defer clear(foloMasterKey)
	} else if options.MasterKeyBase64 != "" {
		foloMasterKey, err = loadMasterKeyEnvironment(options.MasterKeyBase64)
		if err != nil {
			return err
		}
		defer clear(foloMasterKey)
	} else {
		foloSecrets, err = session.NewKeyringSecretStore(session.FoloSessionService)
		if err != nil {
			return err
		}
	}
	var aiSecrets keyring.Store
	if options.GeminiAPIKeyFile != "" && options.GeminiAPIKey != "" {
		return errors.New("configure exactly one Gemini API key source")
	}
	if options.GeminiAPIKeyFile != "" {
		apiKey, keyErr := loadGeminiAPIKeyFile(options.GeminiAPIKeyFile)
		if keyErr != nil {
			return keyErr
		}
		defer clear(apiKey)
		aiSecrets, err = newServerAISecretStore(apiKey)
		if err != nil {
			return err
		}
	} else if options.GeminiAPIKey != "" {
		apiKey, keyErr := loadGeminiAPIKeyEnvironment(options.GeminiAPIKey)
		if keyErr != nil {
			return keyErr
		}
		defer clear(apiKey)
		aiSecrets, err = newServerAISecretStore(apiKey)
		if err != nil {
			return err
		}
	} else {
		aiSecrets, err = keyring.NewAIProviderStore()
		if err != nil {
			return err
		}
	}
	var probeSecrets ops.Keychain
	var cursorSecrets ops.Keychain
	if len(foloMasterKey) == 32 {
		probeSecrets = newEphemeralSecretStore()
		cursorSecrets, err = newDerivedCursorSecretStore(foloMasterKey)
		if err != nil {
			return err
		}
	} else {
		probeSecrets, err = keyring.NewOSStore(readinessKeychainService)
		if err != nil {
			return err
		}
		cursorSecrets, err = keyring.NewOSStore("tantan.cursor.signing")
		if err != nil {
			return err
		}
	}
	application, err := newApplication(context.Background(), applicationConfig{DataDir: options.DataDir, StaticDir: options.StaticDir, PublicOrigin: options.PublicOrigin, TrustedProxyCIDRs: options.TrustedProxyCIDRs, SingleUser: options.SingleUser, SingleUserAccessID: options.SingleUserAccessID, GatewaySecret: options.GatewaySecret, Upstream: upstream, FoloWebURL: foloWebURL, Client: client, FoloSecrets: foloSecrets, FoloMasterKey: foloMasterKey, AISecrets: aiSecrets, ProbeKeychain: probeSecrets, CursorSecrets: cursorSecrets, Logger: logger, Now: time.Now, Version: buildVersion, StartWorkers: true})
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
	var trustedProxyCIDRs string
	singleUserDefault := configuredSingleUser()
	flags.StringVar(&options.DataDir, "data-dir", configuredDataDirectory(), "local Tantan data directory")
	flags.StringVar(&options.StaticDir, "static-dir", os.Getenv("TANTAN_STATIC_DIR"), "absolute Mobile Web build directory")
	flags.StringVar(&options.ListenAddress, "listen-address", configuredListenAddress(), "fixed loopback address")
	flags.StringVar(&options.PublicOrigin, "public-origin", configuredPublicOrigin(), "public HTTPS origin")
	flags.StringVar(&trustedProxyCIDRs, "trusted-proxy-cidrs", os.Getenv("TANTAN_TRUSTED_PROXY_CIDRS"), "comma-separated trusted reverse proxy CIDRs")
	flags.BoolVar(&options.SingleUser, "single-user", singleUserDefault, "enable trusted-proxy single-user browser sessions")
	flags.StringVar(&options.SingleUserAccessID, "single-user-access-id", strings.TrimSpace(os.Getenv("TANTAN_OWNER_ACCESS_ID")), "trusted reverse proxy owner identity")
	options.CloudflareContainer = configuredCloudflareContainer()
	options.GatewaySecret = os.Getenv("TANTAN_GATEWAY_SECRET")
	flags.StringVar(&options.MasterKeyFile, "master-key-file", os.Getenv("TANTAN_MASTER_KEY_FILE"), "path to the server session master key")
	options.MasterKeyBase64 = os.Getenv("TANTAN_MASTER_KEY_B64")
	flags.StringVar(&options.GeminiAPIKeyFile, "gemini-api-key-file", os.Getenv("TANTAN_GEMINI_API_KEY_FILE"), "path to the server Gemini API key")
	options.GeminiAPIKey = os.Getenv("TANTAN_GEMINI_API_KEY")
	flags.StringVar(&options.FoloAPIURL, "folo-api-url", os.Getenv("TANTAN_FOLO_API_URL"), "built-in Folo API URL")
	flags.StringVar(&options.FoloWebURL, "folo-web-url", os.Getenv("TANTAN_FOLO_WEB_URL"), "built-in Folo web URL")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || options.DataDir == "" || (options.SingleUser && options.SingleUserAccessID == "") {
		return serveOptions{}, errors.New("invalid serve arguments")
	}
	options.TrustedProxyCIDRs = splitConfiguredList(trustedProxyCIDRs)
	return options, nil
}

func configuredPublicOrigin() string {
	if value := strings.TrimSpace(os.Getenv("TANTAN_PUBLIC_ORIGIN")); value != "" {
		return value
	}
	return "http://127.0.0.1:3000"
}

func configuredSingleUser() bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("TANTAN_SINGLE_USER")))
	return err == nil && value
}

func configuredCloudflareContainer() bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("TANTAN_CLOUDFLARE_CONTAINER")))
	return err == nil && value
}

func splitConfiguredList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
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
