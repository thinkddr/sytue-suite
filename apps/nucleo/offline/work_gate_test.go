// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWorkGateDrainCancelsWaitsAndRejectsNewJobs(t *testing.T) {
	gate := NewWorkGate(t.Context())
	defer gate.Close()
	job, done, err := gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	drained := make(chan error, 1)
	go func() { drained <- gate.Drain(t.Context()) }()
	select {
	case <-job.Done():
	case <-time.After(time.Second):
		t.Fatal("drain did not cancel active work")
	}
	select {
	case err := <-drained:
		t.Fatal("drain returned before cleanup", err)
	default:
	}
	if _, cleanup, err := gate.Start(t.Context()); !errors.Is(err, ErrWorkPaused) {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("admitted new job during drain", err)
	}
	if err := gate.Resume(); !errors.Is(err, ErrWorkPaused) {
		t.Fatal("resumed before active cleanup", err)
	}
	done()
	done()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if err := gate.Resume(); err != nil {
		t.Fatal(err)
	}
	_, finish, err := gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	finish()
}

func TestWorkGateTimeoutRetainsPauseAndCloseCannotResume(t *testing.T) {
	gate := NewWorkGate(t.Context())
	_, done, err := gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithCancel(t.Context())
	cancel()
	if err := gate.Drain(deadline); !errors.Is(err, context.Canceled) {
		t.Fatal("drain ignored caller cancellation", err)
	}
	if _, cleanup, err := gate.Start(t.Context()); !errors.Is(err, ErrWorkPaused) {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("timeout resumed admission", err)
	}
	done()
	gate.Close()
	if err := gate.Resume(); !errors.Is(err, ErrWorkClosed) {
		t.Fatal("closed gate resumed", err)
	}
	if _, _, err := gate.Start(t.Context()); !errors.Is(err, ErrWorkClosed) {
		t.Fatal("closed gate admitted job", err)
	}
}

func TestWorkGateConcurrentAdmissionAndDrain(t *testing.T) {
	gate := NewWorkGate(t.Context())
	defer gate.Close()
	var jobs sync.WaitGroup
	for range 64 {
		jobs.Go(func() {
			ctx, done, err := gate.Start(t.Context())
			if errors.Is(err, ErrWorkPaused) {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			defer done()
			<-ctx.Done()
		})
	}
	if err := gate.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs.Wait()
}

func TestWorkGateLeaseDrainCancelsAcquisitionAndWaitsForRelease(t *testing.T) {
	started := make(chan struct{})
	acquiring := make(chan error, 1)
	gate := NewWorkGateWithLease(t.Context(), func(ctx context.Context) (func(), error) { close(started); <-ctx.Done(); return nil, ctx.Err() })
	defer gate.Close()
	go func() { _, _, err := gate.Start(t.Context()); acquiring <- err }()
	<-started
	if err := gate.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-acquiring; !errors.Is(err, context.Canceled) {
		t.Fatal("drain did not cancel lease acquisition", err)
	}
	var releases int
	active := NewWorkGateWithLease(t.Context(), func(context.Context) (func(), error) { return func() { releases++ }, nil })
	defer active.Close()
	_, done, err := active.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done()
	done()
	if releases != 1 {
		t.Fatal("lease released more than once", releases)
	}
}

type (
	boundWorkCallerKey  struct{}
	boundWorkAdapterKey struct{}
)

func TestWorkGateStartBoundKeepsOriginalCallerLeaseAndCleansEveryFailure(t *testing.T) {
	active := 0
	gate := NewWorkGateWithLease(t.Context(), func(context.Context) (func(), error) { active++; return func() { active-- }, nil })
	defer gate.Close()
	caller, cancel := context.WithCancel(context.WithValue(t.Context(), boundWorkCallerKey{}, "original"))
	defer cancel()
	called := 0
	job, done, err := gate.StartBound(caller, func(ctx context.Context) (context.Context, error) {
		called++
		if active != 1 || ctx.Value(boundWorkCallerKey{}) != "original" {
			t.Fatal("binder did not receive actual admitted lease/caller")
		}
		return context.WithValue(ctx, boundWorkAdapterKey{}, "adapter"), nil
	})
	if err != nil || called != 1 || active != 1 || job.Value(boundWorkAdapterKey{}) != "adapter" || job.Value(boundWorkCallerKey{}) != "original" {
		t.Fatal("binding discarded original admitted context", err)
	}
	cancel()
	if !errors.Is(job.Err(), context.Canceled) {
		t.Fatal("bound job lost caller cancellation")
	}
	done()
	done()
	if active != 0 {
		t.Fatal("bound job leaked or released lease twice", active)
	}
	for _, fault := range []string{"nil", "error", "discarded-caller", "cancelled", "panic"} {
		t.Run(fault, func(t *testing.T) {
			sentinel := errors.New("artificial binding failure")
			panicked := false
			bind := func(ctx context.Context) (context.Context, error) {
				switch fault {
				case "nil":
					return nil, nil //nolint:nilnil // test double: the "nil" fault deliberately returns (nil, nil) to prove the gate rejects it
				case "error":
					return nil, sentinel
				case "discarded-caller":
					return t.Context(), nil
				case "cancelled":
					next, cancel := context.WithCancel(ctx)
					cancel()
					return next, nil
				case "panic":
					panic(sentinel)
				}
				return ctx, nil
			}
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				result, finish, err := gate.StartBound(t.Context(), bind)
				if err == nil || result != nil || finish != nil {
					t.Fatal("invalid binding admitted work", fault)
				}
				if fault == "error" && !errors.Is(err, sentinel) {
					t.Fatal("binding error replaced")
				}
			}()
			if panicked != (fault == "panic") || active != 0 {
				t.Fatal("failed binding retained actual lease", fault, active)
			}
			deadline, stop := context.WithTimeout(t.Context(), time.Second)
			defer stop()
			if err := gate.Drain(deadline); err != nil {
				t.Fatal("failed binding remained active", err)
			}
			if err := gate.Resume(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
