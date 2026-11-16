// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateDriveDrainCancelsWorkAndReleasesItsLeaseWithoutDeletingJournal(t *testing.T) {
	f := newCatalogTransferFixture(t)
	var held atomic.Int32
	cfg := f.target.cfg
	cfg.WorkLease = func(context.Context) (func(), error) {
		held.Add(1)
		return func() { held.Add(-1) }, nil
	}
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = cfg.Journal.Write(repo.owner(), migrationNamespace, "begin_fixture", beginWork{Scope: readerScope}); err != nil {
		t.Fatal(err)
	}
	ctx, done, err := repo.start(t.Context())
	if err != nil || held.Load() != 1 {
		t.Fatal("work did not acquire its session lease", err)
	}
	finished := make(chan error, 1)
	go func() { finished <- repo.Drain(t.Context()) }()
	<-ctx.Done()
	select {
	case <-finished:
		t.Fatal("drain returned before the caller finished its cleanup")
	default:
	}
	done()
	if err = <-finished; err != nil || held.Load() != 0 {
		t.Fatal("drain did not release the lease", err)
	}
	if _, _, err = repo.start(t.Context()); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("new work admitted during logout", err)
	}
	pending, err := repo.Pending(t.Context())
	if err != nil || pending.Scope != readerScope || pending.Count != 1 {
		t.Fatal("drain lost durable work", pending, err)
	}
	if err = repo.Resume(); err != nil {
		t.Fatal(err)
	}
	_, done, err = repo.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done()
	repo.Close()
	if err = repo.Resume(); !errors.Is(err, offline.ErrWorkClosed) {
		t.Fatal("closed repository resumed", err)
	}
	if pending, err = repo.Pending(t.Context()); err != nil || pending.Count != 1 {
		t.Fatal("close erased its journal", pending, err)
	}
}

func TestPrivateDrivePendingGroupsDomainBodiesAndSDKPublications(t *testing.T) {
	f := newCatalogTransferFixture(t)
	work := rowWork{V: 1, Scope: readerScope, Epoch: readerEpoch, SourceKind: "file", SourceID: "fixture", Bodies: map[string]*fileWork{"binary": {Revision: "body-revision"}}}
	if err := f.journal.Write(f.target.owner(), migrationNamespace, "row_fixture", work); err != nil {
		t.Fatal(err)
	}
	job := "body_" + digest([]byte(readerEpoch+"\x00"+bodyID("file", "fixture", "binary")+"\x00body-revision"))
	for _, id := range []string{job, job + "_publication"} {
		if err := f.journal.Write(f.target.owner(), storage.PendingUploadsNamespace, id, map[string]string{"synthetic": "retained"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.journal.Write(f.target.owner(), migrationNamespace, "mutation_fixture", rowWork{V: 1, Scope: readerScope}); err != nil {
		t.Fatal(err)
	}
	if err := f.journal.Write(f.target.owner(), identityHistoryNamespace, "plan", map[string]bool{"complete": true}); err != nil {
		t.Fatal(err)
	}
	if err := f.journal.Write("other-owner", migrationNamespace, "row_other", work); err != nil {
		t.Fatal(err)
	}
	pending, err := f.target.Pending(t.Context())
	if err != nil || pending.Count != 2 || pending.Scope != readerScope {
		t.Fatal("internal file fragments inflated the logical count", pending, err)
	}
	if err = f.journal.Write(f.target.owner(), migrationNamespace, "mutation_broken", map[string]string{"scope": "invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.target.Pending(t.Context()); err == nil {
		t.Fatal("invalid retained work was treated as a successful count")
	}
}

func TestPrivateDriveLeaseFailurePrecedesCryptoAndSource(t *testing.T) {
	f := newCatalogTransferFixture(t)
	cfg := f.target.cfg
	failure := errors.New("synthetic persisted session changed")
	cfg.WorkLease = func(context.Context) (func(), error) { return nil, failure }
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Initialize(t.Context()); !errors.Is(err, failure) || f.next.writes != 0 {
		t.Fatal("revoked session reached the private source", err)
	}
	f.assertNoPlaintextSource(t)
}
