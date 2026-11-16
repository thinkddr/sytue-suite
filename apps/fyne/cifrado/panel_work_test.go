// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"

	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func panelWorkFixture(t *testing.T) (*ApplicationSession, *PanelWorkers) {
	t.Helper()
	f, ctx, _, _ := factoryFixture(t)
	session, err := f.Capture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	work, err := newPanelWorkers(session.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	return session, work
}

func TestPanelWorkersKeySetupHoldsSharedLeaseAndDrainWaitsCleanup(t *testing.T) {
	session, work := panelWorkFixture(t)
	ctx, done, err := work.start(session.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	short, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	if release, err := session.witness.Exclusivo(short); err == nil {
		release()
		t.Fatal("key setup did not retain its shared lease")
	}
	cancel()
	drained := make(chan error, 1)
	go func() { drained <- work.Drain(t.Context()) }()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("key setup ignored drain cancellation")
	}
	select {
	case <-drained:
		t.Fatal("drain passed active key cleanup")
	default:
	}
	done()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	release, err := session.witness.Exclusivo(t.Context())
	if err != nil {
		t.Fatal("drain did not release key setup lease", err)
	}
	release()
	if _, _, err := work.start(session.Context(), false); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("drained worker admitted another key write", err)
	}
	if err := work.Resume(); err != nil {
		t.Fatal(err)
	}
	_, done, err = work.start(session.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestPanelWorkersSyncCanBeJoinedWithoutHoldingIndefiniteSharedLease(t *testing.T) {
	session, work := panelWorkFixture(t)
	ctx, done, err := work.start(session.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	release, err := session.witness.Exclusivo(t.Context())
	if err != nil {
		t.Fatal("idle sync held a permanent shared lease", err)
	}
	release()
	drained := make(chan error, 1)
	go func() { drained <- work.Drain(t.Context()) }()
	<-ctx.Done()
	select {
	case <-drained:
		t.Fatal("drain did not join sync cleanup")
	default:
	}
	done()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
}

func TestPanelWorkersPendingUsesSDKRecoveryStateAndKeepsReadErrors(t *testing.T) {
	_, work := panelWorkFixture(t)
	client := new(mx.Client)
	work.retain(client)
	pending := true
	failure := error(nil)
	work.status = func(context.Context, *mx.Client) (mx.VerificationStatus, error) {
		return mx.VerificationStatus{RecoverySetupPending: pending}, failure
	}
	if err := work.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, err := work.Pending(t.Context())
	if err != nil || state.Count != 1 || state.Scope != work.PrivateStorageScope() {
		t.Fatal("unacknowledged SDK recovery became empty pending", state, err)
	}
	failure = mx.ErrSecureStore
	if _, err := work.Pending(t.Context()); !errors.Is(err, mx.ErrSecureStore) {
		t.Fatal("unreadable key setup was treated as clean", err)
	}
	failure = nil
	pending = false
	if state, err := work.Pending(t.Context()); err != nil || state.Count != 0 {
		t.Fatal("SDK recovery acknowledgment was not recognized", state, err)
	}
	work.Close()
	// The work owner is separate from the borrowed SDK key adapter.
	if !work.clients[client] {
		t.Fatal("worker teardown destroyed a borrowed key adapter")
	}
}

func TestPanelWorkerTicketOutlivesSuccessfulRequestButNotDrainedGeneration(t *testing.T) {
	session, work := panelWorkFixture(t)
	panel := &Panel{ctx: session.Context(), generation: 1}
	panel.workers.Store(work)
	result := make(chan context.Context, 1)
	panel.runWork(session.Context(), 1, false, func(ctx context.Context, _ *PanelWorkers) { result <- ctx })
	job := <-result
	deadline := time.Now().Add(3 * time.Second)
	for job.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if job.Err() == nil {
		t.Fatal("request lifetime did not end")
	}
	life, generation, worker, valid := panel.callback(job)
	if !valid || life != session.Context() || generation != 1 || worker != work {
		t.Fatal("normal request completion hid a valid SAS callback")
	}
	if err := work.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := work.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, _, _, valid := panel.callback(job); valid {
		t.Fatal("aborted shutdown reactivated an old queued key callback")
	}
}

func TestPanelWorkersRegistersBeforeFirstKeyRequestAndUnregistersOnlyOnce(t *testing.T) {
	f, ctx, _, _ := factoryFixture(t)
	var registered, removed atomic.Int32
	f.RegisterWork = func(_ offline.PrivateWork, _ auth.TestigoTrabajo) (func(), error) {
		registered.Add(1)
		return func() { removed.Add(1) }, nil
	}
	session, err := f.Capture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	work, err := newPanelWorkers(session.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 1 {
		t.Fatal("device setup was not registered before first key request")
	}
	work.Close()
	work.Close()
	if removed.Load() != 1 {
		t.Fatal("panel work removed a different or later registration")
	}
}
