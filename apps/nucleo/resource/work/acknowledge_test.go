// SPDX-License-Identifier: Apache-2.0
package work_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	resourcework "git.sytue.eu/sytue/one/apps/nucleo/resource/work"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

var errInterrupted = errors.New("artificial journal interruption")

// A scoped host-port contract fixture only. Production native encryption and
// actual Matrix GET/decrypt are exercised by the native SDK integration. The
// inner hybrid envelope below is real maintained age in native AND Go/Wasm.
type journalPort struct {
	rows                    map[string][]byte
	mu                      sync.Mutex
	failHistory, failDelete bool
	historyWritten          bool
}

func (p *journalPort) Revalidate(ctx context.Context) error { return ctx.Err() }
func (p *journalPort) Lock(ctx context.Context, _ string) (func(), error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p.mu.Lock()
	return p.mu.Unlock, nil
}

func (p *journalPort) Read(ctx context.Context, key string) ([]byte, bool, error) {
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	v, ok := p.rows[key]
	return bytes.Clone(v), ok, nil
}

func (p *journalPort) Write(ctx context.Context, key string, raw []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var record offline.Record
	if json.Unmarshal(raw, &record) != nil {
		return errInterrupted
	}
	var h resource.AcknowledgedPublicationWork
	history := json.Unmarshal(record.Value, &h) == nil && h.Valid()
	if history && p.failHistory {
		return errInterrupted
	}
	p.rows[key] = bytes.Clone(raw)
	if history {
		p.historyWritten = true
	}
	return nil
}

func (p *journalPort) Delete(ctx context.Context, key string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !p.historyWritten {
		return errors.New("pending deletion preceded durable history")
	}
	if p.failDelete {
		p.failDelete = false
		return errInterrupted
	}
	delete(p.rows, key)
	return nil
}

func (p *journalPort) Keys(ctx context.Context, prefix, after string, limit int) ([]string, string, error) {
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	var keys []string
	for key := range p.rows {
		if strings.HasPrefix(key, prefix) && key > after {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > limit {
		return keys[:limit], keys[limit-1], nil
	}
	return keys, "", nil
}

type activeFence struct{ denied bool }

func (f *activeFence) Revalidate(ctx context.Context, _ resource.VerifiedAccess) error {
	if f.denied {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

type frameReader struct {
	handle   *resource.EpochHandle
	envelope pqc.Envelope
	access   resource.VerifiedAccess
	fence    *activeFence
	reads    int
	after    func()
}

func (r *frameReader) ReadCommitted(ctx context.Context, p resource.VerifiedCommittedRead) ([]byte, error) {
	r.reads++
	out, err := r.handle.OpenCommittedFrame(ctx, p, r.envelope, r.access, r.fence)
	if r.after != nil {
		r.after()
	}
	return out, err
}

type fixture struct {
	cfg        resourcework.Acknowledger
	proof      resource.VerifiedCommittedRead
	row        resource.PublicationWork
	port       *journalPort
	reader     *frameReader
	directory  *resourcetest.SessionSource
	owner, key string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := resourcetest.New(t, false)
	producer, _ := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	writer, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	window, err := resource.CurrentEpoch(f.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := resource.BindOwnedEpoch(window, f.EpochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handle.Close)
	fence := &activeFence{}
	payload := []byte("{\"private\":\"original\",\"seq\":9007199254740993}")
	envelope, err := handle.SealFrame(t.Context(), f.Candidate.Proof.Publication, payload, writer, fence)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := resource.CipherJSONDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.Candidate
	candidate.Proof.InnerCipherSHA256 = hash
	candidate.Proof.Signatures = matrixprooftest.Sign(t, f.Chain.Device, candidate.Proof, f.Chain.Origin.UserID, f.Actor.DeviceID)
	verified, err := resource.VerifyCandidate(t.Context(), candidate, writer, f.Device, nil)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := resource.CaptureReservation(t.Context(), candidate, verified, candidate.Proof.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	source := resourcetest.NewCommittedSource(t, captured, candidate)
	chain := matrixprooftest.NewDevice(t, f.Chain, id.DeviceID("CURRENT_READER_B"))
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, directory := resourcetest.EnrolledSession(t, chain, leaf)
	access, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, current, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	f.Chain.Origin.DeviceActive = false
	proof, err := resource.VerifyCommittedRead(t.Context(), candidate.Proof.Resource, candidate.Proof.OpID, access, fence, source)
	if err != nil {
		t.Fatal(err)
	}
	row := resource.PublicationWork{Version: 1, Binding: producer.Binding(), Publication: candidate.Proof.Publication, Payload: payload, Candidate: &candidate}
	owner, err := f.Scope.JournalOwner()
	if owner != f.Scope.Tenant+"\x00"+f.Scope.Subject+"\x00"+f.Scope.Product {
		t.Fatal("shared owner does not match suite inventory")
	}
	if err != nil {
		t.Fatal(err)
	}
	key, err := resource.PublicationWorkKey(candidate.Proof.Resource, candidate.Proof.OpID)
	if err != nil {
		t.Fatal(err)
	}
	port := &journalPort{rows: map[string][]byte{}}
	journal, err := offline.OpenSealedPort(t.Context(), owner, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Write(owner, storage.ResourcePublicationNamespace, key, row); err != nil {
		t.Fatal(err)
	}
	reader := &frameReader{handle: handle, envelope: envelope, access: access, fence: fence}
	cfg := resourcework.Acknowledger{Journal: journal, Current: current, Access: access, Fence: fence, Reader: reader, Lease: func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }, ValidateLocal: func(ctx context.Context) error { return ctx.Err() }}
	return &fixture{cfg: cfg, proof: proof, row: row, port: port, reader: reader, directory: directory, owner: owner, key: key}
}

func (f *fixture) pending(t *testing.T, want int) {
	t.Helper()
	n, e := f.cfg.Journal.CountOwner(f.owner, storage.ResourcePublicationNamespace)
	if e != nil || n != want {
		t.Fatal("wrong pending count", n, e)
	}
}

func TestPublicationWorkCurrentBAcknowledgesOnlyExactAcceptedIntentAndSurvivesDeleteFailure(t *testing.T) {
	f := newFixture(t)
	f.port.failDelete = true
	if _, err := f.cfg.Acknowledge(t.Context(), f.proof); !errors.Is(err, errInterrupted) {
		t.Fatal("delete interruption was hidden", err)
	}
	f.pending(t, 1)
	if !f.port.historyWritten {
		t.Fatal("pending retired without durable history")
	}
	journal, err := offline.OpenSealedPort(t.Context(), f.owner, f.port)
	if err != nil {
		t.Fatal(err)
	}
	f.cfg.Journal = journal
	if ok, err := f.cfg.Acknowledge(t.Context(), f.proof); err != nil || !ok {
		t.Fatal("restart failed to reconcile original accepted work", ok, err)
	}
	f.pending(t, 0)
	if ok, err := f.cfg.Acknowledge(t.Context(), f.proof); err != nil || !ok || f.reader.reads != 3 {
		t.Fatal("receipt retry skipped original SDK/inner readback", ok, err, f.reader.reads)
	}
	if n, err := journal.CountOwner(f.owner, storage.ResourcePublicationHistoryNamespace); err != nil || n != 1 {
		t.Fatal("exact completion history was lost", n, err)
	}
}

func TestPublicationWorkNeverDropsUnverifiedUnopenedOrChangedIntent(t *testing.T) {
	for _, fault := range []string{"scope", "candidate", "candidate-signature", "payload", "history-write", "local-session", "lease-denied", "revoked-after-read", "zero-proof", "retired-reader"} {
		t.Run(fault, func(t *testing.T) {
			f := newFixture(t)
			proof := f.proof
			switch fault {
			case "scope":
				f.row.Binding.Scope.Subject = "other"
			case "candidate":
				f.row.Candidate.Proof.InnerCipherSHA256 = strings.Repeat("a", 64)
			case "candidate-signature":
				raw, err := json.Marshal(f.row.Candidate)
				if err != nil {
					t.Fatal(err)
				}
				var candidate resource.Candidate
				if json.Unmarshal(raw, &candidate) != nil {
					t.Fatal("candidate clone")
				}
				for _, keys := range candidate.Proof.Signatures {
					for key, sig := range keys {
						if sig[0] == 'A' {
							keys[key] = "B" + sig[1:]
						} else {
							keys[key] = "A" + sig[1:]
						}
					}
				}
				f.row.Candidate = &candidate
			case "payload":
				f.row.Payload = []byte("changed private bytes")
			case "history-write":
				f.port.failHistory = true
			case "lease-denied":
				f.cfg.Lease = func(context.Context) (func(), error) { return nil, resource.ErrRevoked }
			case "local-session":
				f.cfg.ValidateLocal = func(context.Context) error { return resource.ErrRevoked }
			case "revoked-after-read":
				f.reader.after = func() { f.reader.fence.denied = true }
			case "zero-proof":
				proof = resource.VerifiedCommittedRead{}
			case "retired-reader":
				f.directory.Retire()
			}
			if err := f.cfg.Journal.Write(f.owner, storage.ResourcePublicationNamespace, f.key, f.row); err != nil {
				t.Fatal(err)
			}
			if ok, err := f.cfg.Acknowledge(t.Context(), proof); err == nil || ok {
				t.Fatal("unverified/changed work was acknowledged", fault)
			}
			f.pending(t, 1)
			if f.port.historyWritten {
				t.Fatal("failed verification persisted success history", fault)
			}
		})
	}
}
