// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type liveIngressSendBarrier struct {
	after            bool
	skip             int32
	seen             atomic.Int32
	entered, release chan struct{}
	result           chan int
	once             sync.Once
}

func newLiveIngressSendBarrier(after bool, skip int32) *liveIngressSendBarrier {
	return &liveIngressSendBarrier{after: after, skip: skip, entered: make(chan struct{}), release: make(chan struct{}), result: make(chan int, 1)}
}
func (b *liveIngressSendBarrier) allow() { b.once.Do(func() { close(b.release) }) }
func (b *liveIngressSendBarrier) handle(w http.ResponseWriter, r *http.Request, forward *httputil.ReverseProxy) bool {
	if b.seen.Add(1) != b.skip+1 {
		return false
	}
	response := httptest.NewRecorder()
	if b.after {
		forward.ServeHTTP(response, r)
		b.result <- response.Code
	}
	close(b.entered)
	select {
	case <-b.release:
	case <-r.Context().Done():
		return true
	}
	if !b.after {
		forward.ServeHTTP(response, r)
		b.result <- response.Code
	}
	for key, values := range response.Header() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.Code)
	_, _ = io.Copy(w, response.Body)
	return true
}

type liveHandoffSource struct {
	*HTTPSource
	afterFence   func(contract.HandoffView) error
	afterInspect func(context.Context, contract.TaskState) error
}

func (s liveHandoffSource) Inspect(ctx context.Context, id string) (contract.TaskState, error) {
	state, err := s.HTTPSource.Inspect(ctx, id)
	if err != nil {
		return contract.TaskState{}, err
	}
	if s.afterInspect != nil {
		if err := s.afterInspect(ctx, state); err != nil {
			return contract.TaskState{}, err
		}
	}
	return state, nil
}

func (s liveHandoffSource) Handoff(ctx context.Context, request contract.ControlRequest, authority matrixproof.Authority) (contract.HandoffView, error) {
	view, err := s.HTTPSource.Handoff(ctx, request, authority)
	if err != nil {
		return contract.HandoffView{}, err
	}
	if request.Handoff.Phase == "begin" && view.Status == "readable" && s.afterFence != nil {
		if err := s.afterFence(view); err != nil {
			return contract.HandoffView{}, err
		}
	}
	return view, nil
}

func TestIngressLiveDeviceHandoffFixture(t *testing.T) {
	f := newLiveIngressFixture(t)
	mode := os.Getenv("SYTUE_INGRESS_HANDOFF_CASE")
	if mode != "absent" && mode != "committed" && mode != "renewal" {
		t.Fatal("missing explicit handoff proof case")
	}
	journalA, err := offline.OpenWithSecrets(t.TempDir(), f.device.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	workerA, err := New(Config{Context: f.ctx, Scope: f.vector.Scope, Epoch: f.vector.Epoch, Journal: journalA, Lease: f.lease, Source: f.source, Device: f.device.Client, Executor: liveSDKExecutor{device: f.device.Client, journal: journalA, scope: f.vector.Scope}})
	if err != nil {
		t.Fatal(err)
	}
	defer workerA.Close()
	gate := newLiveIngressSendBarrier(mode == "committed", map[bool]int32{true: 1, false: 0}[mode == "committed"])
	t.Cleanup(gate.allow)
	f.device.Barrier.Store(gate)
	doneA := make(chan error, 1)
	go func() { doneA <- workerA.Process(f.ctx, f.task.ID) }()
	select {
	case <-gate.entered:
	case err := <-doneA:
		t.Fatal("A did not reach the real in-flight SDK request", err)
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	if err := expireLiveIngressAssignment(f.ctx, false); err != nil {
		t.Fatal(err)
	}
	fresh := openLiveIngressDevice(f.ctx, t, f.vector, "LIVE_HANDOFF_B", f.device.Recovery)
	if err := fresh.Client.RestoreBackup(f.ctx); err != nil {
		t.Fatal(err)
	}
	journalB, err := offline.OpenWithSecrets(t.TempDir(), fresh.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := journalB.RecordIDs(workerA.owner(), storage.PrivateIngressNamespace)
	if err != nil || len(ids) != 0 {
		t.Fatal("B inherited the original WAL", err)
	}
	sourceConfig := f.sourceConfig
	sourceConfig.Signer = fresh.Client
	sourceConfig.Lease = liveIngressLease(t)
	sourceB, err := NewHTTPSource(sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	var lateAStatus int
	wrapped := liveHandoffSource{HTTPSource: sourceB, afterFence: func(view contract.HandoffView) error {
		if view.Status != "readable" || view.Claim != nil || view.Fence == nil || view.Fence.Status != "confirmed" {
			return contract.ErrClaim
		}
		gate.allow()
		select {
		case lateAStatus = <-gate.result:
		case <-f.ctx.Done():
			return f.ctx.Err()
		}
		if mode != "committed" && lateAStatus != http.StatusUnauthorized {
			return errors.New("Synapse did not reject the already prepared late A request")
		}
		if mode == "committed" && lateAStatus != http.StatusOK {
			return errors.New("A index was not actually accepted before fencing")
		}
		select {
		case err := <-doneA:
			if err == nil {
				return errors.New("fenced A still finished its queue operation")
			}
		case <-f.ctx.Done():
			return f.ctx.Err()
		}
		return nil
	}}
	if mode == "renewal" {
		var changed bool
		wrapped.afterInspect = func(ctx context.Context, _ contract.TaskState) error {
			if changed {
				return nil
			}
			changed = true
			return expireLiveIngressAssignment(ctx, true)
		}
	}
	workerB, err := New(Config{Context: f.ctx, Scope: f.vector.Scope, Epoch: f.vector.Epoch, Journal: journalB, Lease: sourceConfig.Lease, Source: wrapped, Device: fresh.Client, Executor: liveSDKExecutor{device: fresh.Client, journal: journalB, scope: f.vector.Scope}})
	if err != nil {
		t.Fatal(err)
	}
	defer workerB.Close()
	if mode == "renewal" {
		if err := workerB.Process(f.ctx, f.task.ID); err == nil {
			t.Fatal("lost rejection response was not exercised")
		}
		if fresh.Sends.Load() != 0 {
			t.Fatal("unknown begin response reached publication")
		}
		if err := workerB.Process(f.ctx, f.task.ID); !errors.Is(err, contract.ErrClaim) {
			t.Fatal("explicit refusal did not retain a fresh attempt", err)
		}
		var retained intent
		found, err := journalB.Read(workerB.owner(), storage.PrivateIngressNamespace, f.task.ID, &retained)
		if err != nil || !found || retained.Handoff == nil || len(retained.Handoff.Rejected) != 1 || retained.Handoff.Rejected[0].Begin.Handoff.ID == retained.Handoff.Begin.Handoff.ID {
			t.Fatal("lost rejection discarded or reused its refused intent", err)
		}
		if fresh.Sends.Load() != 0 {
			t.Fatal("refusal refresh published before a real fence")
		}
	}
	if err := workerB.Process(f.ctx, f.task.ID); err != nil {
		t.Fatal("actual SDK handoff failed", err)
	}
	expectedB := int32(2)
	if mode == "committed" {
		expectedB = 0
	}
	if fresh.Sends.Load() != expectedB || fresh.KeyWrites.Load() != 0 {
		t.Fatal("B replaced keys or republished an accepted A task", fresh.Sends.Load(), fresh.KeyWrites.Load())
	}
	catalog, err := storage.LoadCatalog(f.ctx, fresh.Client, f.vector.Scope)
	if err != nil || len(catalog.Heads) != 1 {
		t.Fatal("handoff did not finish with exactly one SDK head", err)
	}
	head := catalog.Heads[0]
	if head.Commit.Content == nil {
		t.Fatal("handoff lost its content record")
	}
	record, err := fresh.Client.ReadStorageRecord(f.ctx, head.Commit.Content.Reference, head.ObjectID, "content")
	if err != nil {
		t.Fatal(err)
	}
	var original struct{ Metadata, Body []byte }
	if json.Unmarshal(record.Payload, &original) != nil || !bytes.Equal(original.Metadata, []byte(`{"future":9007199254740993,"subject":"PRIVATE-LIVE-METADATA-MARKER"}`)) || !bytes.Equal(original.Body, []byte("From: synthetic@example.invalid\r\nSubject: PRIVATE-LIVE-MIME-MARKER\r\n\r\nOriginal MIME / 漢字 é\r\n")) {
		t.Fatal("handoff changed original metadata or MIME bytes")
	}
	if pending, err := workerB.Pending(f.ctx); err != nil || pending.Count != 0 {
		t.Fatal("B did not durably ACK its reconciled result", err)
	}
	if pending, err := workerA.Pending(f.ctx); err != nil || pending.Count != 1 {
		t.Fatal("A original journal was deleted", err)
	}
	t.Log("INGRESS_LIVE_DEVICE_HANDOFF_PASS; fresh verified SDK B with empty WAL and recovered SSSS; real individual A fence; late request settled before grant; one SDK head; readiness remains disabled pending production domain execution")
}

func expireLiveIngressAssignment(ctx context.Context, renew bool) error {
	path := "/fixture/expire"
	if renew {
		path += "?renew=1"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv("SYTUE_INGRESS_DEVICE_API_URL")+path, http.NoBody)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer synthetic-live-device-session")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("disposable assignment transition failed")
	}
	return nil
}
