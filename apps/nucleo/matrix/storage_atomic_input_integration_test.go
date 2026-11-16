//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	storageatomic "git.sytue.eu/sytue/one/apps/nucleo/storage/atomic"
)

// Real SDK/Synapse/Go hybrid age and native sealed store. This proves readback
// first into index intent data, then the composed atomic publisher and restart.
// The receipt readback uses the reopened same device A. Finalizing/cleanup,
// a different device B, and application activation are separate.
func TestNativeAtomicInputContentRequiresExactOriginalSDKReadback(t *testing.T) {
	endpoint := os.Getenv("SYTUE_MATRIX_ATOMIC_SYNAPSE_URL")
	if endpoint == "" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	upstream, err := url.Parse(endpoint)
	if err != nil || upstream.Scheme != "http" || upstream.User != nil || upstream.Port() == "" || (upstream.Hostname() != "127.0.0.1" && upstream.Hostname() != "localhost" && upstream.Hostname() != "::1") || (upstream.Path != "" && upstream.Path != "/") || upstream.RawPath != "" || upstream.RawQuery != "" || upstream.ForceQuery || upstream.Fragment != "" {
		t.Fatal("disposable loopback Synapse URL with explicit port required")
	}
	reverse := httputil.NewSingleHostReverseProxy(upstream)
	var sends atomic.Int64
	var rejectSends atomic.Bool
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/send/") {
			sends.Add(1)
			if rejectSends.Load() {
				http.Error(w, "read-only restart attempted send", http.StatusConflict)
				return
			}
		}
		reverse.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	client := device.Client
	defer func() { _ = client.Close() }()
	scope := client.storageScope()
	target, err := client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"unknown":9007199254740993,"private":"actual captured source"}`)
	sum := sha256.Sum256(payload)
	request := storage.FileRequest{ObjectID: "actual_input_object", RevisionID: "actual_input_revision", SizeBytes: int64(len(payload)), Metadata: []byte(`{}`)}
	journalDir, secrets := t.TempDir(), new(authtest.Secrets)
	journal, err := offline.OpenWithSecrets(journalDir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []storageatomic.Input{{ObjectID: request.ObjectID, State: "live", Metadata: []byte(`{"index":9007199254740995}`), Content: &storageatomic.ContentInput{Kind: "record", Request: request, SHA256: hex.EncodeToString(sum[:]), Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(payload)), nil }}}, {ObjectID: "actual_deleted_object", State: "deleted", Metadata: []byte(`{"deleted":true}`)}}
	plan, err := storageatomic.CaptureInputs(ctx, journal, scope, "actual_readback_intent", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := storageatomic.PublishInputContents(ctx, plan, client, nil)
	if err != nil || len(proofs) != 2 {
		t.Fatal("actual SDK content stage failed", err)
	}
	first, second := proofs[0], proofs[1]
	mutation, err := first.Mutation(ctx)
	if err != nil || mutation.Commit.Content == nil {
		t.Fatal("actual SDK returned no content proof", err)
	}
	ref := mutation.Commit.Content.Reference
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Mutation(ctx); err == nil {
		t.Fatal("retired actual SDK proof became valid capture data")
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := storageatomic.PublishInputContents(ctx, plan, client, nil)
	if err != nil || len(recovered) != 2 {
		t.Fatal("actual SDK restart could not read checkpointed content", err)
	}
	exact, err := recovered[0].Mutation(ctx)
	if err != nil || exact.Commit.Content == nil || exact.Commit.Content.Reference != ref {
		t.Fatal("content retry changed accepted physical reference", err)
	}
	first, second = recovered[0], recovered[1]
	work, err := storageatomic.CaptureVerified(ctx, plan, []storageatomic.VerifiedInputContent{first, second})
	if err != nil || work.Count != 2 {
		t.Fatal("SDK readback could not capture exact prepared-index intents", err)
	}
	member, err := storageatomic.ReadMember(ctx, journal, work, 0)
	if err != nil || member.Mutation.Commit.Content == nil || member.Mutation.Commit.Content.Reference != ref || member.Mutation.PayloadSHA256 != hex.EncodeToString(sum[:]) || !bytes.Equal(member.Mutation.Commit.Metadata, inputs[0].Metadata) {
		t.Fatal("SDK readback lost exact body/index metadata", err)
	}
	catalog, err := storage.LoadAtomicCatalog(ctx, client, scope)
	if err != nil || len(catalog.Entries) != 0 || len(catalog.Heads) != 0 {
		t.Fatal("local capture published application indices", err)
	}
	published, err := storageatomic.PublishInputBatch(ctx, plan, client, nil)
	if err != nil || !published.Reference.Valid() {
		t.Fatal("actual SDK full batch publication failed", err)
	}
	complete, err := storage.LoadAtomicCatalog(ctx, client, scope)
	if err != nil || len(complete.History) != 2 || len(complete.Batches) != 1 || len(complete.Pages) != 1 {
		t.Fatal("actual SDK full batch projection incomplete", err)
	}
	actual, err := client.ReadStorageRecord(ctx, ref, request.ObjectID, "content")
	if err != nil || !bytes.Equal(actual.Payload, payload) {
		t.Fatal("full batch changed original private bytes", err)
	}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := storageatomic.CaptureFinalizationReceipt(ctx, plan, client, nil, lease); err != nil {
		t.Fatal("actual SDK receipt capture", err)
	}
	beforeSends := sends.Load()
	rejectSends.Store(true)
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := storageatomic.PublishInputBatch(ctx, plan, client, nil)
	if err != nil || restored.Reference != published.Reference || sends.Load() != beforeSends {
		t.Fatal("actual SDK restart resent or changed accepted batch", err)
	}
	reopenedJournal, err := offline.OpenWithSecrets(journalDir, secrets)
	if err != nil {
		t.Fatal("reopen encrypted journal", err)
	}
	// The autonomous checker receives no InputPlan or staged-source handle.
	// Reopening the same SQLite/key store remains device A, not a new device B.
	plan = storageatomic.InputPlan{}
	if err := storageatomic.CheckFinalizationReceipt(ctx, reopenedJournal, scope, "actual_readback_intent", client, nil, lease); err != nil {
		t.Fatal("actual SDK receipt readback without InputPlan", err)
	}
	for _, scenario := range []string{"content", "binding"} {
		altered := &atomicReceiptReadFault{RecordStore: client, scenario: scenario, content: ref}
		if err := storageatomic.CheckFinalizationReceipt(ctx, reopenedJournal, scope, "actual_readback_intent", altered, nil, lease); err == nil {
			t.Fatal("actual SDK receipt accepted injected change", scenario)
		}
		if altered.faults == 0 {
			t.Fatal("actual SDK fixture did not inject requested fault", scenario)
		}
	}
	if sends.Load() != beforeSends {
		t.Fatal("receipt verification attempted a new publication")
	}
	if count, err := journal.CountOwner(scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product, storage.PrivateJournalNamespaces(scope.Product)...); err != nil || count == 0 {
		t.Fatal("accepted publisher erased finalization work", count, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Mutation(ctx); err == nil {
		t.Fatal("retired actual SDK proof became valid capture data")
	}
	t.Log("actual SDK/Synapse: exact accepted batch and encrypted receipt; same-device SDK SQLite/key and sealed file journal reopen; readback without InputPlan; changed content/binding rejected; no new sends; pending retained; no ACK or cleanup")
}

// Fault injection wraps the real SDK; it is not a second device or a provider.
type atomicReceiptReadFault struct {
	storage.RecordStore
	scenario string
	content  storage.Reference
	faults   int
}

func (f *atomicReceiptReadFault) StorageBindings(ctx context.Context) (storage.Binding, error) {
	binding, err := f.RecordStore.StorageBindings(ctx)
	if err == nil && f.scenario == "binding" {
		f.faults++
		binding.Scope.Subject += "-changed"
	}
	return binding, err
}

func (f *atomicReceiptReadFault) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, err := f.RecordStore.ReadStorageRecord(ctx, ref, object, role)
	if err == nil && f.scenario == "content" && ref == f.content && role == "content" {
		f.faults++
		record.Payload = []byte(`{"tampered":true}`)
	}
	return record, err
}
