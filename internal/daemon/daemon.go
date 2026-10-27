// Package daemon wires the store, engine, API server and (optionally) the
// Telegram adapter into one foreground process.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	// Named IANA zones must work in distroless/scratch images without a
	// system zoneinfo directory.
	_ "time/tzdata"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/autostart"
	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/engine"
	"github.com/itsgiahung/502Drive/internal/httpapi"
	"github.com/itsgiahung/502Drive/internal/power"
	"github.com/itsgiahung/502Drive/internal/state"
	"github.com/itsgiahung/502Drive/internal/telegram"
)

type Options struct {
	Version string
	Log     *slog.Logger
	// Listener, if set, replaces binding cfg.Listen (tests).
	Listener net.Listener
	// Started is called with the bound address once the API is serving.
	Started func(addr string)
	// DriveEndpoint overrides the Drive API base URL (tests only).
	DriveEndpoint string
	// Provider overrides the Drive provider entirely (tests only).
	Provider drive.Provider
	// RevokeEndpoint overrides Google's token revocation URL (tests only).
	RevokeEndpoint string
	// Maintenance blocks new work: jobs are accepted never; the engine idles.
	Maintenance bool
}

// Run serves until ctx is cancelled, then shuts down in order: stop accepting
// requests, let the engine reach safe points, close state.
func Run(parent context.Context, cfg config.Config, o Options) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, stop := context.WithCancel(parent)
	defer stop()
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	rawToken, err := config.ReadPrivate(cfg.TokenFile)
	if err != nil {
		return fmt.Errorf("api token: %w", err)
	}
	store, err := state.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	svc := &app.Service{Store: store, Version: o.Version, ConfigDir: filepath.Dir(cfg.TokenFile), BaseCtx: ctx,
		Systemd: func() app.SystemdState {
			st := autostart.SystemdStateNow()
			return app.SystemdState{Installed: st.Installed, Enabled: st.Enabled, Unit: st.Unit, Detail: st.Detail}
		}}
	if every, err := cfg.FullReconcileEveryDuration(); err != nil {
		return err
	} else if every > 0 {
		svc.FullReconcileEvery = &every
	} else {
		// Explicitly disabled: a non-nil zero duration says "off".
		zero := time.Duration(0)
		svc.FullReconcileEvery = &zero
	}
	var auth *drive.Auth
	if cfg.OAuthClientFile != "" {
		raw, err := config.ReadPrivate(cfg.OAuthClientFile)
		if err != nil {
			return fmt.Errorf("oauth client file: %w", err)
		}
		oc, err := google.ConfigFromJSON(raw, drive.Scope)
		if err != nil {
			return errors.New("oauth client file is not a valid Google client JSON")
		}
		secrets := filepath.Join(cfg.DataDir, "secrets")
		validate := func(ctx context.Context, access string) error {
			c, err := drive.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: access, TokenType: "Bearer"}), o.DriveEndpoint)
			if err != nil {
				return err
			}
			acc, err := c.Account(ctx)
			if err != nil {
				return errors.New("the new credential was rejected by Google Drive")
			}
			return svc.RememberAccount(ctx, acc.Email)
		}
		if auth, err = drive.NewAuth(secrets, *oc, validate); err != nil {
			return err
		}
		defer auth.Close()
		svc.Auth = auth
	}

	start, maxWorkers, successes := cfg.Concurrency()
	throttle := drive.NewThrottle(drive.ThrottleOptions{Start: start, Max: maxWorkers,
		Successes: successes, ReadPerSecond: cfg.ReadRate()})
	var clientMu sync.Mutex
	var client *drive.Client
	baseProvider := func(ctx context.Context) (drive.Provider, error) {
		if o.Provider != nil {
			return o.Provider, nil
		}
		if auth == nil {
			return nil, drive.ErrNotConnected
		}
		if _, err := auth.Token(); err != nil {
			return nil, err
		}
		clientMu.Lock()
		defer clientMu.Unlock()
		if client == nil {
			c, err := drive.NewClient(context.WithoutCancel(ctx), auth.TokenSource(context.WithoutCancel(ctx)), o.DriveEndpoint)
			if err != nil {
				return nil, err
			}
			client = c
		}
		return client, nil
	}
	// Every Drive call in the daemon shares one account-wide AIMD limit and
	// read budget.
	provider := func(ctx context.Context) (drive.Provider, error) {
		p, err := baseProvider(ctx)
		if err != nil {
			return nil, err
		}
		return drive.Paced(p, throttle), nil
	}
	svc.Provider = provider
	svc.Throttle = func() app.ThrottleStatus {
		snap := throttle.Snapshot()
		return app.ThrottleStatus{WorkersNow: snap.WorkersNow, ThrottledLast5m: snap.ThrottledLast5m, NextRetryAt: snap.NextRetryAt}
	}

	eng := &engine.Engine{Store: store, Provider: provider, Log: log, Throttle: throttle,
		Workers: start, MaxWorkers: maxWorkers, Jobs: cfg.ActiveJobs()}
	svc.Wake = eng.Wake

	// Power/network lifecycle: OS events where available, clock gaps always.
	// The engine only sees a gate; the OS specifics stay in internal/power.
	src, srcErr := power.NewSource(log)
	if srcErr != nil {
		log.Info("power: no OS power notifications; using the clock-gap detector", "err", srcErr)
	}
	monitor := power.NewMonitor(src, power.MonitorOptions{})
	lifeCtx, stopLife := context.WithCancel(context.WithoutCancel(ctx))
	defer stopLife()
	var keeper power.WakeKeeper
	if cfg.KeepAwakeWhileCopying {
		keeper = power.NewWakeKeeper(&power.WakeOptions{Log: log, Reason: "502Drive is copying"})
	}
	life := newLifecycle(lifecycleOptions{
		Store: store,
		Log:   log,
		Probe: func(ctx context.Context) error {
			p, err := provider(ctx)
			if err != nil {
				return err
			}
			_, err = p.Account(ctx)
			return err
		},
		ResetConnections: func() {
			clientMu.Lock()
			if client != nil {
				client.CloseIdleConnections()
			}
			clientMu.Unlock()
		},
		Wake:      eng.Wake,
		Keeper:    keeper,
		KeepAwake: func() bool { return cfg.KeepAwakeWhileCopying },
		RunningJobs: func(ctx context.Context) int {
			counts, err := store.CountJobs(ctx)
			if err != nil {
				return 0
			}
			return counts[state.JobRunning] + counts[state.JobPausing] + counts[state.JobCancelling] + counts[state.JobReconciling]
		},
	})
	eng.Network = life.gateRef()
	eng.OnUnreachable = func(err error) { life.noteUnreachable(lifeCtx, err) }
	svc.Lifecycle = life.view
	// Decide the startup state before the engine may claim anything.
	life.start(lifeCtx)
	// Sync bookkeeping runs after every finished run: the store records the
	// result on the subscription and may hand back a coalesced follow-up run.
	bookkeep := context.WithoutCancel(ctx)
	// jobNotifier is set below when a Telegram bot is configured; it runs in
	// the engine goroutine, which starts after this assignment.
	var jobNotifier func(context.Context, string) error
	eng.AfterRun = func(jobID string) {
		next, err := store.RunFinished(bookkeep, jobID)
		if err != nil {
			log.Error("sync: run bookkeeping failed", "job", jobID, "err", err)
			return
		}
		if next != nil {
			eng.Wake()
		}
		if jobNotifier == nil {
			return
		}
		if nerr := jobNotifier(bookkeep, jobID); nerr != nil {
			log.Warn("telegram: cannot queue a notification", "job", jobID, "err", nerr)
		}
	}
	var workerUp atomic.Bool
	// A terminal copy commits its own state and a durable bookkeeping marker in
	// one transaction; the callback above consumes the marker. Anything left
	// here is a crash between those two writes: replay it before workers start,
	// so the collection state and after-copy contract are not lost.
	if !o.Maintenance {
		if n, err := store.FinishPendingBookkeeping(bookkeep); err != nil {
			log.Error("sync: bookkeeping replay failed", "err", err)
		} else if n > 0 {
			log.Info("sync: replayed finished-job bookkeeping", "jobs", n)
		}
	}
	srv := &httpapi.Server{Svc: svc, TokenHash: httpapi.HashToken(string(rawToken)), Log: log, OAuthPort: cfg.OAuthCallbackPort,
		RevokeEndpoint: o.RevokeEndpoint, WorkerReady: workerUp.Load, Maintenance: o.Maintenance, Done: ctx.Done(), Stop: stop,
		Sleeping: life.suspending}

	// The Telegram adapter runs in the same daemon and calls the same service
	// as the HTTP API. A bot failure never makes the daemon unready.
	var tg *telegram.Telegram
	botState := atomic.Value{}
	botState.Store(telegram.StateDisabled)
	if !o.Maintenance && cfg.TelegramTokenFile != "" {
		botState.Store(telegram.StateStarting)
		rawBotToken, err := config.ReadPrivate(cfg.TelegramTokenFile)
		if err != nil {
			log.Error("telegram disabled: cannot read the token file", "err", err)
			botState.Store(telegram.StateError)
		} else if tg, err = telegram.New(context.WithoutCancel(ctx), telegram.Options{Service: svc,
			Token: strings.TrimSpace(string(rawBotToken)), OwnerID: cfg.TelegramOwnerID, Log: log}); err != nil {
			log.Error("telegram disabled", "err", err)
			botState.Store(telegram.StateError)
			tg = nil
		}
	}
	var notifier *telegram.Notifier
	if tg != nil {
		notifier = telegram.NewNotifier(telegram.NotifierOptions{Service: svc, OwnerID: cfg.TelegramOwnerID})
		jobNotifier = notifier.JobFinished
	}
	svc.BotState = func() string {
		if tg != nil {
			return tg.State()
		}
		if s, ok := botState.Load().(string); ok {
			return s
		}
		return telegram.StateDisabled
	}

	ln := o.Listener
	if ln == nil {
		if ln, err = net.Listen("tcp", cfg.Listen); err != nil {
			return fmt.Errorf("cannot listen on %s: %w", cfg.Listen, err)
		}
	}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}

	engCtx, stopEngine := context.WithCancel(context.WithoutCancel(ctx))
	engDone := make(chan struct{})
	if o.Maintenance {
		close(engDone)
		workerUp.Store(false)
	} else {
		go func() {
			defer close(engDone)
			workerUp.Store(true)
			if err := eng.Run(engCtx); err != nil {
				log.Error("engine stopped", "err", err)
			}
			workerUp.Store(false)
		}()
	}
	// The syncer admits scheduled runs and advances change-feed cursors. In
	// maintenance mode it does not run: nothing would execute the runs.
	syncCtx, stopSync := context.WithCancel(context.WithoutCancel(ctx))
	syncDone := make(chan struct{})
	if o.Maintenance {
		close(syncDone)
	} else {
		syncer := newSyncer(store, provider, eng.Wake, log)
		syncer.applyAfterCopy = svc.ApplyAfterCopySync
		if notifier != nil {
			syncer.notifyError = notifier.SubscriptionError
			syncer.notifyClear = notifier.SubscriptionHealthy
		}
		go func() {
			defer close(syncDone)
			syncer.run(syncCtx)
		}()
	}
	lifeDone := make(chan struct{})
	go func() {
		defer close(lifeDone)
		life.Run(lifeCtx, monitor.Run(lifeCtx))
	}()
	botCtx, stopBot := context.WithCancel(context.WithoutCancel(ctx))
	telegramDone := make(chan struct{})
	if tg != nil {
		go func() {
			defer close(telegramDone)
			tg.Run(botCtx)
		}()
	} else {
		close(telegramDone)
	}

	maintCtx, stopMaint := context.WithCancel(context.WithoutCancel(ctx))
	maintDone := make(chan struct{})
	go func() {
		defer close(maintDone)
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		lastDaily := time.Now()
		for {
			select {
			case <-maintCtx.Done():
				return
			case <-ticker.C:
				_ = store.CheckpointTruncate(maintCtx)
				_ = store.Optimize(maintCtx)
				if time.Since(lastDaily) >= 24*time.Hour {
					lastDaily = time.Now()
					if prefs, err := store.Preferences(maintCtx, "default"); err == nil && prefs.RetentionDays > 0 {
						_, _ = store.Prune(maintCtx, state.PruneOptions{
							OlderThan: time.Duration(prefs.RetentionDays) * 24 * time.Hour,
							DryRun:    false,
						})
					}
				}
			}
		}
	}()

	serveErr := make(chan error, 1)
	go func() { serveErr <- hs.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String())
	if o.Started != nil {
		o.Started(ln.Addr().String())
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-serveErr:
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutCtx)
	stopEngine()
	<-engDone
	stopLife()
	<-lifeDone
	stopSync()
	<-syncDone
	stopBot()
	<-telegramDone
	stopMaint()
	<-maintDone
	_ = store.CheckpointTruncate(shutCtx)
	_ = store.Optimize(shutCtx)
	if errors.Is(runErr, http.ErrServerClosed) {
		runErr = nil
	}
	return runErr
}

// EnsureDir makes sure the data directory exists privately.
func EnsureDir(dir string) error { return os.MkdirAll(dir, 0o700) }
