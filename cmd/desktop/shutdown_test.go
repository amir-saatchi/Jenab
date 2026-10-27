package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestShutdownDeadline(t *testing.T) {
	var order []string
	start := time.Now()
	err := Shutdown{
		Refuse: []func(){func() { order = append(order, "refuse") }},
		Cancel: func() { order = append(order, "cancel") },
		Wait: []func(context.Context) error{func(ctx context.Context) error {
			<-ctx.Done() // a turn that never ends
			order = append(order, "wait")
			return ctx.Err()
		}},
		Close: func(ctx context.Context) error {
			if dl, ok := ctx.Deadline(); !ok || time.Until(dl) < ShutdownTimeout-WaitTimeout-time.Second {
				t.Errorf("close has until %v", dl)
			}
			order = append(order, "close")
			return nil
		},
	}.Run()
	if !errors.Is(err, context.DeadlineExceeded) || !slices.Equal(order, []string{"refuse", "cancel", "wait", "close"}) {
		t.Fatalf("err %v, order %v", err, order)
	}
	if took := time.Since(start); took < WaitTimeout || took > WaitTimeout+time.Second {
		t.Errorf("took %v", took)
	}
}

// A shutdown stuck past its deadline exits the app.
func TestShutdownWatchdog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		exited := make(chan time.Duration, 1)
		start := time.Now()
		done := make(chan error)
		go func() {
			done <- Shutdown{
				Close: func(context.Context) error { <-release; return nil }, // ignores its deadline
				Exit:  func() { exited <- time.Since(start); close(release) },
			}.Run()
		}()
		if took := <-exited; took != ShutdownTimeout+exitGrace {
			t.Errorf("exited after %v", took)
		}
		<-done
	})
	// A shutdown in time doesn't exit.
	synctest.Test(t, func(t *testing.T) {
		Shutdown{Exit: func() { t.Error("exited") }}.Run()
		time.Sleep(time.Minute)
	})
}
