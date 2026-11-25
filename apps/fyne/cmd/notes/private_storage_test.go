// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"math"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type lifecycleNotesRepository struct {
	api.NotesData
	api.NotesViewsData
	api.NotesExportData
	scope                           storage.Scope
	count                           int
	pendingErr, drainErr, resumeErr error
	paused, closed                  bool
}

func (r *lifecycleNotesRepository) PrivateStorageScope() storage.Scope { return r.scope }

func (r *lifecycleNotesRepository) Pending(context.Context) (offline.PendingWork, error) {
	return offline.PendingWork{Scope: r.scope, Count: r.count}, r.pendingErr
}

func (r *lifecycleNotesRepository) Drain(context.Context) error { r.paused = true; return r.drainErr }

func (r *lifecycleNotesRepository) Resume() error {
	if r.resumeErr != nil {
		return r.resumeErr
	}
	r.paused = false
	return nil
}

func (r *lifecycleNotesRepository) Close() { r.closed = true }

func (r *lifecycleNotesRepository) Initialize(context.Context) error { return nil }

func (r *lifecycleNotesRepository) IdentityCutover(records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	return nil, errors.New("fixture is lifecycle only")
}

func notesLifecycleFixture(t *testing.T) (*notepadsRepository, *lifecycleNotesRepository) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	local, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	backend := &lifecycleNotesRepository{scope: scope}
	owner := &notepadsRepository{notesRepository: backend, local: local, life: t.Context()}
	t.Cleanup(owner.Close)
	return owner, backend
}

func TestNotepadsOwnerResumesRepositoryBeforeRearmingPresenter(t *testing.T) {
	owner, backend := notesLifecycleFixture(t)
	if err := owner.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	owner.local.OnResume(func() {
		calls++
		if backend.paused {
			t.Error("presenter resumed before repository")
		}
	})
	if err := owner.Resume(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("presenter was not rearmed", calls)
	}
	if err := owner.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	backend.resumeErr = errors.New("repository still paused")
	if err := owner.Resume(); !errors.Is(err, backend.resumeErr) {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("failed repository resume rearmed presenter", calls)
	}
	if _, done, err := owner.local.Start(); err == nil {
		done()
		t.Fatal("failed resume admitted local editor work")
	}
}

func TestNotepadsOwnerDrainsLocalSourcesBeforeRepositoryAndRetainsFailedRetirement(t *testing.T) {
	owner, backend := notesLifecycleFixture(t)
	_, done, err := owner.local.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(done)
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	if err := owner.Drain(expired); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if backend.paused || backend.closed {
		t.Fatal("repository retired before local work joined")
	}
	done()
	backend.drainErr = errors.New("repository retained")
	if err := owner.Drain(t.Context()); !errors.Is(err, backend.drainErr) {
		t.Fatal(err)
	}
	if backend.closed {
		t.Fatal("failed drain closed source owner")
	}
	if _, done, err := owner.local.Start(); err == nil {
		done()
		t.Fatal("failed drain reopened capture/editor admission")
	}
}

func TestNotepadsOwnerCountsDisjointJournalAndDraftSourcesExactly(t *testing.T) {
	owner, backend := notesLifecycleFixture(t)
	backend.count = 2
	if err := owner.local.Write(t.Context(), notas.EspacioBorradores, "capture", "retained"); err != nil {
		t.Fatal(err)
	}
	pending, err := owner.Pending(t.Context())
	if err != nil || pending.Scope != backend.scope || pending.Count != 3 {
		t.Fatal(pending, err)
	}
	backend.count = math.MaxInt
	if _, err := owner.Pending(t.Context()); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatal("overflow count became permission to close", err)
	}
	backend.count = 0
	backend.scope.Subject = "bob"
	if _, err := owner.Pending(t.Context()); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatal("foreign pending count became permission to close", err)
	}
}

func TestNotepadsPlanRejectsMissingResourcesBeforeConstruction(t *testing.T) {
	if _, err := notesPlan(t.Context(), nil, nil); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatal(err)
	}
	if _, err := buildNotesPlan(t.Context(), nil, nil, nil, nil, nil); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatal(err)
	}
}
