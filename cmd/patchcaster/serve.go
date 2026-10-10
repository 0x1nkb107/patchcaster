package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/lmittmann/tint"

	"github.com/patchcaster-app/patchcaster/internal/deliver"
	"github.com/patchcaster-app/patchcaster/internal/ingest"
	"github.com/patchcaster-app/patchcaster/internal/mattermost"
	"github.com/patchcaster-app/patchcaster/internal/metrics"
	"github.com/patchcaster-app/patchcaster/internal/ops"
	"github.com/patchcaster-app/patchcaster/internal/routing"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Run serves until ctx ends. It wires every component and owns the shutdown
// choreography (SYSREQ-023): on SIGTERM readiness flips false, the public
// listener stops accepting, and the worker gets the drain budget to finish
// what is in flight. The deployment's terminationGracePeriodSeconds must
// exceed --drain-timeout.
func (c *ServeCmd) Run(ctx context.Context) error {
	log := newLogger(c.LogLevel)

	routes, err := routing.Load(c.RoutingFile)
	if err != nil {
		return err
	}

	db, err := openDB(c.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("reach database: %w", err)
	}
	if err := migrate(ctx, db, log); err != nil {
		return err
	}

	st := store.New(db)
	m := metrics.New()
	probe := ops.NewServer(st, m)

	renderer, err := deliver.NewRenderer()
	if err != nil {
		return err
	}
	secrets := make([][]byte, len(c.SigningSecrets))
	for i, s := range c.SigningSecrets {
		secrets[i] = []byte(s)
	}
	handler := &ingest.Handler{
		Secrets: secrets,
		Store:   st,
		Routes:  routes,
		MaxAge:  c.EventMaxAge,
		Now:     time.Now,
		Log:     log,
		Metrics: m,
	}
	worker := deliver.NewWorker(
		st,
		mattermost.New(c.MattermostURL, c.MattermostToken),
		renderer, m, log,
		deliver.Config{Poll: time.Second, Lease: c.ClaimLease},
		nil,
	)

	// Bind before announcing readiness: a port conflict must fail the start,
	// not arrive as a late listener error.
	pubLn, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("bind public listener %s: %w", c.Addr, err)
	}
	intLn, err := net.Listen("tcp", c.InternalAddr)
	if err != nil {
		_ = pubLn.Close()
		return fmt.Errorf("bind internal listener %s: %w", c.InternalAddr, err)
	}

	pubCtx, cancelPub := context.WithCancel(ctx)
	wrkCtx, cancelWrk := context.WithCancel(ctx)
	intCtx, cancelInt := context.WithCancel(ctx)
	// Belt for early-return paths; the shutdown choreography below cancels
	// these deliberately, and repeated cancel is a no-op.
	defer cancelPub()
	defer cancelWrk()
	defer cancelInt()

	// One buffered slot per goroutine so late returns never block on a
	// channel nobody drains during shutdown.
	errc := make(chan error, 4)
	workerDone := make(chan struct{})
	go func() { errc <- ops.ListenAndServe(intCtx, intLn, probe.Mux(), c.DrainTimeout) }()
	go func() { errc <- ops.ListenAndServe(pubCtx, pubLn, handler.Mux(), c.DrainTimeout) }()
	go func() {
		err := worker.Run(wrkCtx)
		close(workerDone)
		errc <- err
	}()
	go func() { errc <- pruneLoop(wrkCtx, st, c.SeenRetention, log) }()

	probe.MarkReady(true)
	log.Info("serving", "public", c.Addr, "internal", c.InternalAddr)

	var errs []error
	select {
	case err := <-errc:
		// Component failure at startup: tear everything down at once.
		record(err, &errs)
		cancelPub()
		cancelWrk()
		cancelInt()
		recordAll(collect(errc, 3), &errs)

	case <-ctx.Done():
		probe.MarkReady(false)
		log.Info("shutdown signaled", "drain_budget", c.DrainTimeout)
		cancelPub() // webhooks stop being accepted immediately
		worker.StopAfterCurrentJob()

		select {
		case <-workerDone:
			log.Info("worker finished within drain budget")
		case <-time.After(c.DrainTimeout):
			log.Warn("drain budget exhausted; releasing worker")
			cancelWrk()
			<-workerDone
		}
		cancelInt()
		recordAll(collect(errc, 4), &errs)
	}

	probe.MarkReady(false)
	return errors.Join(errs...)
}

// pruneLoop keeps the dedupe table bounded: seen rows older than the
// retention horizon no longer protect anything (SYSREQ-014).
func pruneLoop(ctx context.Context, st *store.Postgres, retention time.Duration, log *slog.Logger) error {
	prune := func() {
		if err := st.PruneSeen(ctx, time.Now().Add(-retention)); err != nil {
			log.Error("prune seen webhooks failed", "err", err)
		}
	}
	prune()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			prune()
		}
	}
}

func collect(errc chan error, n int) []error {
	out := make([]error, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, <-errc)
	}
	return out
}

func record(err error, errs *[]error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ops.ErrStopped) {
		return
	}
	*errs = append(*errs, err)
}

func recordAll(errs []error, into *[]error) {
	for _, err := range errs {
		record(err, into)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	h := tint.NewTextHandler(os.Stderr, &tint.Options{
		Level:      lvl,
		NoColor:    true, // log output is piped in production; color buys nothing
		TimeFormat: time.RFC3339,
	})
	return slog.New(h)
}
