// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func scopedCatalogFixture(t *testing.T) (*Repository, *atomic.Int32, *atomic.Int32, []byte) {
	t.Helper()
	r, _, work := acceptedLocalAcknowledgmentFixture(t)
	r.gate.Close()
	active, acquired := &atomic.Int32{}, &atomic.Int32{}
	r.gate = offline.NewWorkGateWithLease(t.Context(), func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !active.CompareAndSwap(0, 1) {
			return nil, errors.New("non-reentrant lease acquired twice")
		}
		acquired.Add(1)
		return func() { active.Store(0) }, nil
	})
	return r, active, acquired, bytes.Clone(work.JSON)
}

func TestProjectCatalogUsesOneLeaseAndRevokesCatalogAndBuffers(t *testing.T) {
	r, active, acquired, expected := scopedCatalogFixture(t)
	var escaped Catalog
	var entry Entry
	var returned []byte
	err := r.ProjectCatalog(t.Context(), func(ctx context.Context, catalog Catalog) error {
		escaped = catalog
		entries, err := catalog.Entries()
		if err != nil || len(entries) != 1 {
			t.Fatal("projection did not capture the catalog", err)
		}
		entry = entries[0]
		returned, err = catalog.ReadJSON(ctx, entry)
		if err != nil || !bytes.Equal(returned, expected) || active.Load() != 1 {
			t.Fatal("projection did not retain the one lease", err)
		}
		again, err := catalog.ReadJSON(ctx, entry)
		if err != nil || !bytes.Equal(again, expected) || active.Load() != 1 {
			t.Fatal("second read reacquired the work lease", err)
		}
		return nil
	})
	if err != nil || active.Load() != 0 || acquired.Load() != 1 {
		t.Fatal("projection reacquired or retained its lease", err, acquired.Load())
	}
	if !bytes.Equal(returned, make([]byte, len(returned))) {
		t.Fatal("projection retained plaintext JSON after return")
	}
	if _, err := escaped.Entries(); !errors.Is(err, ErrCatalogExpired) {
		t.Fatal("expired catalog returned entries", err)
	}
	if raw, err := escaped.ReadJSON(t.Context(), entry); raw != nil || !errors.Is(err, ErrCatalogExpired) {
		t.Fatal("expired catalog returned bytes", err)
	}
}

func TestProjectCatalogRejectsChangedEntriesAndCanceledReads(t *testing.T) {
	r, _, _, _ := scopedCatalogFixture(t)
	err := r.ProjectCatalog(t.Context(), func(ctx context.Context, catalog Catalog) error {
		entries, err := catalog.Entries()
		if err != nil || len(entries) != 1 {
			t.Fatal(err)
		}
		wrong := entries[0]
		wrong.ID = "foreign"
		if raw, err := catalog.ReadJSON(ctx, wrong); raw != nil || !errors.Is(err, ErrSource) {
			t.Fatal("foreign entry was read", err)
		}
		entries[0].Version.Commit.Metadata[0] ^= 1
		if raw, err := catalog.ReadJSON(ctx, entries[0]); raw != nil || !errors.Is(err, ErrSource) {
			t.Fatal("changed revision was read", err)
		}
		entries, err = catalog.Entries()
		if err != nil {
			t.Fatal(err)
		}
		canceled, stop := context.WithCancel(ctx)
		stop()
		if raw, err := catalog.ReadJSON(canceled, entries[0]); raw != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("canceled read returned bytes", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectCatalogKeepsCapturedEntriesAcrossHeadChange(t *testing.T) {
	r, _, _, expected := scopedCatalogFixture(t)
	err := r.ProjectCatalog(t.Context(), func(ctx context.Context, catalog Catalog) error {
		entries, err := catalog.Entries()
		if err != nil || len(entries) != 1 {
			return errors.New("missing initial entry")
		}
		r.mu.Lock()
		newer := cloneVersion(r.heads[entries[0].Version.ObjectID][0])
		newer.RevisionID = "newer_index"
		r.heads[newer.ObjectID] = []storage.ObjectVersion{newer}
		r.mu.Unlock()
		again, err := catalog.Entries()
		if err != nil || len(again) != 1 || again[0].Version.RevisionID != entries[0].Version.RevisionID {
			return errors.New("projection mixed catalog heads")
		}
		raw, err := catalog.ReadJSON(ctx, entries[0])
		if err != nil || !bytes.Equal(raw, expected) {
			return errors.New("captured version changed during projection")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectCatalogReleasesLeaseOnErrorAndPanic(t *testing.T) {
	r, active, acquired, _ := scopedCatalogFixture(t)
	failure := errors.New("projection failed")
	if err := r.ProjectCatalog(t.Context(), func(context.Context, Catalog) error { return failure }); !errors.Is(err, failure) || active.Load() != 0 {
		t.Fatal("projection error leaked lease", err)
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = r.ProjectCatalog(t.Context(), func(context.Context, Catalog) error { panic("projection panic") })
	}()
	if recovered != "projection panic" || active.Load() != 0 || acquired.Load() != 2 {
		t.Fatal("projection panic leaked lease", recovered)
	}
}

type scopedBlockedCrypto struct {
	Crypto
	started chan struct{}
}

func (p *scopedBlockedCrypto) ReadStorageRecord(ctx context.Context, _ storage.Reference, _, _ string) (storage.Record, error) {
	close(p.started)
	<-ctx.Done()
	return storage.Record{}, ctx.Err()
}

func TestProjectCatalogCancelsEscapedReadBeforeReleasingLease(t *testing.T) {
	r, active, _, _ := scopedCatalogFixture(t)
	blocked := &scopedBlockedCrypto{Crypto: r.cfg.Crypto, started: make(chan struct{})}
	r.cfg.Crypto = blocked
	result := make(chan error, 1)
	var escaped Catalog
	err := r.ProjectCatalog(t.Context(), func(_ context.Context, catalog Catalog) error {
		escaped = catalog
		entries, err := catalog.Entries()
		if err != nil {
			return err
		}
		go func() {
			_, err := catalog.ReadJSON(context.Background(), entries[0])
			result <- err
		}()
		select {
		case <-blocked.started:
			return nil
		case <-time.After(5 * time.Second):
			return errors.New("escaped read did not start")
		}
	})
	if err != nil || active.Load() != 0 {
		t.Fatal("projection did not release its lease", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("escaped read ignored projection lifetime", err)
	}
	if _, err := escaped.Entries(); !errors.Is(err, ErrCatalogExpired) {
		t.Fatal("catalog remained usable after return", err)
	}
}

func TestProjectCatalogCloseCancelsBlockedReadAndWaitsForLease(t *testing.T) {
	r, active, _, _ := scopedCatalogFixture(t)
	blocked := &scopedBlockedCrypto{Crypto: r.cfg.Crypto, started: make(chan struct{})}
	r.cfg.Crypto = blocked
	result := make(chan error, 1)
	go func() {
		result <- r.ProjectCatalog(t.Context(), func(ctx context.Context, catalog Catalog) error {
			entries, err := catalog.Entries()
			if err != nil {
				return err
			}
			_, err = catalog.ReadJSON(ctx, entries[0])
			return err
		})
	}()
	select {
	case <-blocked.started:
	case <-time.After(5 * time.Second):
		t.Fatal("catalog read did not start")
	}
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrClosed) {
			t.Fatal("close did not cancel the projection", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projection did not stop after close")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not drain the projection")
	}
	if active.Load() != 0 {
		t.Fatal("close retained the lease")
	}
}

func TestProjectCatalogCallerCancellationInterruptsBlockedRead(t *testing.T) {
	r, active, _, _ := scopedCatalogFixture(t)
	blocked := &scopedBlockedCrypto{Crypto: r.cfg.Crypto, started: make(chan struct{})}
	r.cfg.Crypto = blocked
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- r.ProjectCatalog(ctx, func(ctx context.Context, catalog Catalog) error {
			entries, err := catalog.Entries()
			if err != nil {
				return err
			}
			raw, err := catalog.ReadJSON(ctx, entries[0])
			if raw != nil {
				return errors.New("canceled read returned bytes")
			}
			return err
		})
	}()
	select {
	case <-blocked.started:
	case <-time.After(5 * time.Second):
		t.Fatal("catalog read did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("caller cancellation was not propagated", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projection ignored caller cancellation")
	}
	if active.Load() != 0 {
		t.Fatal("canceled projection retained the lease")
	}
}

func TestProjectCatalogDrainCancelsReadAndBlocksAdmission(t *testing.T) {
	r, active, _, _ := scopedCatalogFixture(t)
	blocked := &scopedBlockedCrypto{Crypto: r.cfg.Crypto, started: make(chan struct{})}
	r.cfg.Crypto = blocked
	result := make(chan error, 1)
	go func() {
		result <- r.ProjectCatalog(t.Context(), func(ctx context.Context, catalog Catalog) error {
			entries, err := catalog.Entries()
			if err != nil {
				return err
			}
			_, err = catalog.ReadJSON(ctx, entries[0])
			return err
		})
	}()
	select {
	case <-blocked.started:
	case <-time.After(5 * time.Second):
		t.Fatal("catalog read did not start")
	}
	drained := make(chan error, 1)
	go func() { drained <- r.Drain(t.Context()) }()
	select {
	case err := <-drained:
		if err != nil {
			t.Fatal("drain failed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not wait for cancellation")
	}
	if err := <-result; !errors.Is(err, context.Canceled) || active.Load() != 0 {
		t.Fatal("drain did not stop the projection", err)
	}
	if err := r.ProjectCatalog(t.Context(), func(context.Context, Catalog) error { return nil }); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("drain admitted another projection", err)
	}
}

func TestProjectCatalogCloseWaitsForCallbackThatIgnoresCancellation(t *testing.T) {
	r, active, _, _ := scopedCatalogFixture(t)
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- r.ProjectCatalog(t.Context(), func(ctx context.Context, _ Catalog) error {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
			return nil
		})
	}()
	<-started
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not cancel the callback")
	}
	select {
	case <-closed:
		t.Fatal("close skipped an admitted callback")
	default:
	}
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("closed projection returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projection did not finish")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not finish")
	}
	if active.Load() != 0 {
		t.Fatal("callback retained lease")
	}
}
