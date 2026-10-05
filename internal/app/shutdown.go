package app

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const (
	// ShutdownTimeout is how long quitting may take (Q30, SPEC 2.8). Past
	// it the app exits anyway; the lock files left behind start crash
	// recovery on the next start.
	ShutdownTimeout = 10 * time.Second
	// WaitTimeout bounds step 2: turns and runs ending.
	WaitTimeout = 5 * time.Second
)

// Shutdown is the Q30 order, run by the app root.
type Shutdown struct {
	// Refuse stops new work: the scheduler, new turns (step 1).
	Refuse []func()
	// Cancel cancels the app context, so turns end as with Stop and runs
	// become interrupted (step 2).
	Cancel func()
	// Wait waits for the turns and runs to end, up to WaitTimeout.
	Wait []func(context.Context) error
	// Close drains the writers, checkpoints and closes every project,
	// removing its lock file (steps 3–4), with the rest of the time.
	Close func(context.Context) error
	Log   *slog.Logger
}

// Run runs the steps within ShutdownTimeout and returns what failed.
func (s Shutdown) Run() error {
	start := time.Now()
	log := s.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	for _, fn := range s.Refuse {
		fn()
	}
	if s.Cancel != nil {
		s.Cancel()
	}
	var errs []error
	wctx, wcancel := context.WithTimeout(ctx, WaitTimeout)
	for _, wait := range s.Wait {
		if err := wait(wctx); err != nil {
			errs = append(errs, err)
		}
	}
	wcancel()
	if s.Close != nil {
		if err := s.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	err := errors.Join(errs...)
	if err != nil {
		log.Error("app: shutdown", "took", time.Since(start), "err", err)
	} else {
		log.Info("app: shut down", "took", time.Since(start))
	}
	return err
}
