package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/enrichment"
	"tantan.local/tantan-api/internal/filter"
	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/home"
	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/keyring"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/secrets"
	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
	syncer "tantan.local/tantan-api/internal/sync"
	"tantan.local/tantan-api/internal/topic"
)

const cursorKeyAccount = "cursor-v1"

type applicationConfig struct {
	DataDir           string
	StaticDir         string
	PublicOrigin      string
	TrustedProxyCIDRs []string
	Upstream          *url.URL
	FoloWebURL        *url.URL
	Client            *stdhttp.Client
	FoloSecrets       session.SecretStore
	FoloMasterKey     []byte
	AISecrets         ai.SecretStore
	ProbeKeychain     ops.Keychain
	CursorSecrets     ops.Keychain
	Logger            *slog.Logger
	Now               func() time.Time
	Version           string
	StartWorkers      bool
}

type application struct {
	Handler stdhttp.Handler
	Store   *storage.Store

	cancel    context.CancelFunc
	workers   sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func newApplication(ctx context.Context, config applicationConfig) (*application, error) {
	if config.DataDir == "" || config.PublicOrigin == "" || config.Upstream == nil || config.FoloWebURL == nil || config.AISecrets == nil || config.ProbeKeychain == nil || config.CursorSecrets == nil || (config.FoloSecrets == nil && len(config.FoloMasterKey) != 32) {
		return nil, errors.New("application data, Folo and Keychain dependencies are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	client := config.Client
	if client == nil {
		client = &stdhttp.Client{Timeout: 60 * time.Second}
	}
	store, err := storage.Open(ctx, storage.Config{DataDir: config.DataDir})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*application, error) {
		_ = store.Close()
		return nil, err
	}
	if _, err := ops.CreateDailyBackup(ctx, store, filepath.Join(config.DataDir, "backups"), config.Now()); err != nil {
		return fail(err)
	}
	foloSecrets := config.FoloSecrets
	if foloSecrets == nil {
		foloSecrets, err = secrets.NewStore(secrets.Config{Store: store, Key: config.FoloMasterKey, Now: config.Now})
		if err != nil {
			return fail(err)
		}
	}
	sessionBackend, err := newSQLiteSessionBackend(store)
	if err != nil {
		return fail(err)
	}
	sessions, err := session.NewStoreWithBackend(config.Now, sessionBackend)
	if err != nil {
		return fail(err)
	}
	replays, err := newSQLiteTokenReplayStore(store)
	if err != nil {
		return fail(err)
	}
	settings, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: config.AISecrets})
	if err != nil {
		return fail(err)
	}
	topics := topic.NewService(store, config.Now)
	cursorKey, err := loadOrCreateCursorKey(ctx, config.CursorSecrets)
	if err != nil {
		return fail(err)
	}
	homeService, err := home.NewService(home.Config{Store: store, CursorKey: cursorKey, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	searchService, err := search.NewService(search.Config{Store: store, CursorKey: cursorKey})
	if err != nil {
		return fail(err)
	}
	filterService, err := filter.NewService(filter.Config{Store: store, Settings: settings, Home: homeService, Topics: topics, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	feedbackService, err := recommendation.NewFeedbackService(recommendation.FeedbackConfig{Store: store, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	enrichmentService, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Topics: topics, Now: config.Now, PromptVersion: ai.DefaultPromptVersion})
	if err != nil {
		return fail(err)
	}
	foloAuth, err := folo.NewAuthClient(config.Upstream, client)
	if err != nil {
		return fail(err)
	}
	bridge, err := auth.NewBridge(auth.Config{PublicOrigin: config.PublicOrigin, FoloWebURL: config.FoloWebURL.String(), Logger: config.Logger, Sessions: sessions, Secrets: foloSecrets, Replays: replays, Folo: foloAuth, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	policy, err := folo.LoadPolicy()
	if err != nil {
		return fail(err)
	}
	proxy, err := folo.NewProxy(folo.ProxyConfig{Policy: policy, Upstream: config.Upstream, PublicOrigin: config.PublicOrigin, Client: client, Secrets: foloSecrets, Logger: config.Logger})
	if err != nil {
		return fail(err)
	}
	readiness, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: config.ProbeKeychain, Timeout: 5 * time.Second})
	if err != nil {
		return fail(err)
	}
	diagnostics, err := localhttp.NewDiagnosticsHandler(localhttp.DiagnosticsConfig{DB: store.DB(), DatabasePath: store.Path(), Version: config.Version, DeniedFoloRoutes: proxy.DeniedCount, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	local, err := newLocalAPI(localAPIConfig{
		Store:      store,
		Home:       homeService,
		Topics:     topics,
		Filter:     filterService,
		Feedback:   feedbackService,
		Search:     searchService,
		Enrichment: enrichmentService,
		AISettings: settings,
		ProviderTester: func(providerContext context.Context) (ai.ConnectionTestResult, error) {
			_, apiKey, credentialErr := settings.Credential(providerContext, ai.DefaultPromptVersion)
			if credentialErr != nil {
				return ai.ConnectionTestResult{}, credentialErr
			}
			return ai.TestConnection(providerContext, apiKey, nil, time.Now)
		},
		Diagnostics: diagnostics,
		Now:         config.Now,
	})
	if err != nil {
		return fail(err)
	}
	var static stdhttp.Handler
	if config.StaticDir != "" {
		static, err = localhttp.NewSPAHandler(config.StaticDir)
		if err != nil {
			return fail(err)
		}
	}
	health := localhttp.NewHealthHandler(config.Version, readiness)
	handler, err := localhttp.NewRouter(localhttp.RouterConfig{PublicOrigin: config.PublicOrigin, TrustedProxyCIDRs: config.TrustedProxyCIDRs, Auth: bridge, Proxy: proxy, Sessions: sessions, Local: local, Health: health, Static: static, Logger: config.Logger})
	if err != nil {
		return fail(err)
	}
	source, err := syncer.NewHTTPSource(syncer.HTTPSourceConfig{Upstream: config.Upstream, Client: client, Token: func(tokenContext context.Context, userID string) (string, error) {
		var sessionHash string
		if err := store.DB().QueryRowContext(tokenContext, "SELECT secret_ref FROM local_sessions WHERE user_id=? ORDER BY last_seen_at DESC LIMIT 1", userID).Scan(&sessionHash); err != nil {
			return "", errors.New("active Folo session was not found")
		}
		return foloSecrets.Get(tokenContext, sessionHash)
	}})
	if err != nil {
		return fail(err)
	}
	syncService, err := syncer.NewService(syncer.Config{Store: store, Source: source, Now: config.Now})
	if err != nil {
		return fail(err)
	}
	workerContext, cancel := context.WithCancel(ctx)
	result := &application{Handler: handler, Store: store, cancel: cancel}
	if config.StartWorkers {
		result.startWorkers(workerContext, readiness, enrichmentService, syncService, config.Now, config.Logger)
	}
	return result, nil
}

func (application *application) Close() error {
	if application == nil {
		return nil
	}
	application.closeOnce.Do(func() {
		if application.cancel != nil {
			application.cancel()
		}
		application.workers.Wait()
		if application.Store != nil {
			application.closeErr = application.Store.Close()
		}
	})
	return application.closeErr
}

func loadOrCreateCursorKey(ctx context.Context, secrets ops.Keychain) ([]byte, error) {
	encoded, err := secrets.Get(ctx, cursorKeyAccount)
	if err == nil {
		key, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) != 32 || base64.RawURLEncoding.EncodeToString(key) != encoded {
			return nil, errors.New("stored cursor key is invalid")
		}
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, session.ErrSecretNotFound) {
		return nil, errors.New("read cursor key from Keychain failed")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, errors.New("generate cursor key failed")
	}
	if err := secrets.Set(ctx, cursorKeyAccount, base64.RawURLEncoding.EncodeToString(key)); err != nil {
		return nil, errors.New("save cursor key to Keychain failed")
	}
	return key, nil
}
