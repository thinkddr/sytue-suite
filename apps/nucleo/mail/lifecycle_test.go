// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func leasedMailFixture(t *testing.T) (*fixture, *atomic.Int32) {
	t.Helper()
	f := setup(t, true)
	cfg := f.repo.cfg
	f.repo.Close()
	active := &atomic.Int32{}
	cfg.WorkLease = func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if active.Add(1) != 1 {
			active.Add(-1)
			return nil, offline.ErrWorkLease
		}
		return func() { active.Add(-1) }, nil
	}
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.repo = repo
	t.Cleanup(repo.Close)
	f.open(t)
	return f, active
}

func TestMailDrainClosesMIMEAndRetainsDurableWorkWithoutClosingSDK(t *testing.T) {
	f, active := leasedMailFixture(t)
	body, err := f.repo.Raw(t.Context(), "message")
	if err != nil {
		t.Fatal(err)
	}
	if active.Load() != 1 {
		t.Fatal("MIME reader did not retain its session permission", active.Load())
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	if err := f.repo.cfg.Journal.Write(owner, ingestNamespace, "retained", map[string]string{"original": "opaque encrypted journal"}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if active.Load() != 0 {
		t.Fatal("drained reader kept its session permission")
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatal("drained reader remained readable", err)
	}
	if _, err := f.repo.Mailboxes(t.Context()); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("paused repository admitted new work", err)
	}
	pending, err := f.repo.Pending(t.Context())
	if err != nil || pending.Scope != testScope || pending.Count != 1 {
		t.Fatal("drain lost original durable work", pending, err)
	}
	if err := f.repo.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Mailboxes(t.Context()); err != nil {
		t.Fatal("aborted shutdown could not resume", err)
	}
	if len(f.crypto.records) == 0 {
		t.Fatal("drain closed or discarded borrowed SDK history")
	}
}

type delayedMailDownload struct {
	*cryptoFixture
	enabled          bool
	canceled, finish chan struct{}
}

func (c *delayedMailDownload) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return request, err
	}
	if !c.enabled || request.SizeBytes <= 1<<20 {
		return c.cryptoFixture.DownloadStorageFile(ctx, gateway, ref, object, revision, out)
	}
	<-ctx.Done()
	close(c.canceled)
	<-c.finish
	return request, ctx.Err()
}

func TestMailDrainRetainsStreamLeaseUntilCanceledSDKDownloadActuallyFinishes(t *testing.T) {
	f, active := leasedMailFixture(t)
	crypto := &delayedMailDownload{cryptoFixture: f.crypto, canceled: make(chan struct{}), finish: make(chan struct{})}
	finish := sync.OnceFunc(func() { close(crypto.finish) })
	defer finish()
	replaceTestCrypto(t, f.repo, crypto)
	raw := bytes.Repeat([]byte("private MIME stream\r\n"), 60000)
	metadata, err := json.Marshal(map[string]any{"id": "streamed", "mailbox_id": "inbox", "size": len(raw), "thread_id": "streamed", "received_at": "2026-10-30T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.StoreMessage(t.Context(), "stream_operation", metadata, bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatal(err)
	}
	crypto.enabled = true
	body, err := f.repo.Raw(t.Context(), "streamed")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	drained := make(chan error, 1)
	go func() { drained <- f.repo.Drain(t.Context()) }()
	select {
	case <-crypto.canceled:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if active.Load() != 1 {
		t.Fatal("cancellation released the stream permission before SDK cleanup", active.Load())
	}
	select {
	case err := <-drained:
		t.Fatal("drain finished while the SDK request remained active", err)
	default:
	}
	finish()
	select {
	case err := <-drained:
		if err != nil {
			t.Fatal(err)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if active.Load() != 0 {
		t.Fatal("completed SDK cleanup retained a session permission")
	}
}

func TestMailPendingForScopeCountsDeferredEnrollmentAndOnePhysicalJournalInventory(t *testing.T) {
	f := setup(t, true)
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	journal := f.repo.cfg.Journal
	for _, namespace := range []string{writesNamespace, importNamespace, ingestNamespace, storage.PendingUploadsNamespace, storage.PrivateIngressNamespace, storage.PrivateIngressEnrollmentNamespace} {
		if err := journal.Write(owner, namespace, "pending", map[string]bool{"retained": true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"intent", "effect"} {
		if err := journal.Write(owner, storage.IdentityJournalNamespace("mail"), id, map[string]bool{"retained": true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := journal.Write(owner, storage.PrivateIngressEnrollmentHistoryNamespace, "confirmed", map[string]bool{"retained": true}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Write("another-owner", writesNamespace, "pending", map[string]bool{"retained": true}); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingForScope(t.Context(), testScope, journal)
	if err != nil || pending.Scope != testScope || pending.Count != 7 {
		t.Fatal("deferred holder lost work or counted one physical journal through several wrappers", pending, err)
	}
	if _, err := PendingForScope(t.Context(), testScope, nil); !errors.Is(err, offline.ErrEncryptedStore) {
		t.Fatal("unknown journal became an empty account", err)
	}
}

type publicationValueKey struct{}

func TestMailNativePublicationBindingUsesTheOriginalAdmittedCallerAndRetainsItsLease(t *testing.T) {
	f, active := leasedMailFixture(t)
	seen := 0
	f.repo.cfg.BindPublicationContext = func(ctx context.Context) (context.Context, error) {
		seen++
		if active.Load() != 1 || ctx.Value(publicationValueKey{}) != "original caller" {
			t.Fatal("publication binding replaced the original scoped operation/lease", active.Load())
		}
		return context.WithValue(ctx, struct{ Admission bool }{true}, "bound private admission"), nil
	}
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), publicationValueKey{}, "original caller"))
	ctx, done, err := f.repo.start(parent)
	if err != nil {
		t.Fatal(err)
	}
	if seen != 1 || active.Load() != 1 || ctx.Value(struct{ Admission bool }{true}) != "bound private admission" {
		t.Fatal("admitted operation lost the native publication context", seen, active.Load())
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("publication binding lost caller cancellation")
	}
	done()
	if active.Load() != 0 {
		t.Fatal("publication binding leaked the actual native lease")
	}
	for _, bind := range []func(context.Context) (context.Context, error){
		func(context.Context) (context.Context, error) { return nil, nil }, //nolint:nilnil // test double: deliberately returns a nil context to prove the binder rejects it
		func(context.Context) (context.Context, error) { return nil, ErrRecord },
	} {
		f.repo.cfg.BindPublicationContext = bind
		if _, done, err := f.repo.start(t.Context()); err == nil || done != nil || active.Load() != 0 {
			t.Fatal("failed native binding admitted work or leaked a lease", err, active.Load())
		}
	}
}
