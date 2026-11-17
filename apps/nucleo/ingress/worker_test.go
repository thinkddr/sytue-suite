// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type workerPolicyKey struct{}

type workerDevice struct {
	binding  storage.Binding
	identity *age.X25519Identity
	revoked  bool
}

func (d *workerDevice) PrepareStorage(context.Context, storage.Scope) (storage.Binding, error) {
	return d.binding, nil
}

func (d *workerDevice) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if d.revoked || target != d.binding.Target {
		return contract.ErrRevoked
	}
	return nil
}

func (d *workerDevice) RequireRecoverableIngress(ctx context.Context, _ storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	return ctx, d.CheckStoragePublicationTarget(target)
}

func (d *workerDevice) RequireAuthorizedStoragePublication(ctx context.Context, _ storage.Scope, target storage.PublicationTarget, policy matrix.StoragePublicationPolicy) (context.Context, error) {
	if policy == nil {
		return nil, contract.ErrRevoked
	}
	if err := d.CheckStoragePublicationTarget(target); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, workerPolicyKey{}, policy), nil
}

func (d *workerDevice) OpenIngressBody(ctx context.Context, task contract.Task, reader io.ReaderAt) ([]byte, *io.SectionReader, error) {
	return agecipher.Open(ctx, reader, task, d.identity)
}

type workerSource struct {
	task                               contract.Task
	cipher                             []byte
	target                             storage.PublicationTarget
	token                              string
	opens, claims, acks                int
	loseACK                            bool
	acknowledged, rejectCompletedClaim bool
	generation                         int64
	revalidations                      int
	checkWAL                           func()
}

func (s *workerSource) authorityGeneration() int64 {
	if s.generation == 0 {
		return 1
	}
	return s.generation
}

func (s *workerSource) Authority(_ context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	return matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "fixture", RootKey: strings.Repeat("A", 43), Generation: s.authorityGeneration(), Status: "active"}, nil
}

func (s *workerSource) Revalidate(_ context.Context, _ contract.Claim, authority matrixproof.Authority) error {
	s.revalidations++
	if authority.Generation != s.authorityGeneration() {
		return contract.ErrRevoked
	}
	return nil
}

func (s *workerSource) Claim(_ context.Context, scope storage.Scope, epoch, id string, target storage.PublicationTarget, token string, authority matrixproof.Authority) (contract.Claim, error) {
	if s.checkWAL != nil {
		s.checkWAL()
	}
	s.claims++
	if s.acknowledged && s.rejectCompletedClaim {
		return contract.Claim{}, contract.ErrReceipt
	}
	if authority.Generation != s.authorityGeneration() || scope != s.task.Scope || epoch != s.task.Epoch || id != s.task.ID || (s.token != "" && (s.token != token || s.target != target)) {
		return contract.Claim{}, contract.ErrClaim
	}
	s.target, s.token = target, token
	return contract.Claim{Version: 1, Task: s.task, Target: target, Token: token, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (s *workerSource) Open(_ context.Context, claim contract.Claim, _ matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error) {
	s.opens++
	return claim.Task.Cipher, io.NopCloser(bytes.NewReader(s.cipher)), nil
}

func (s *workerSource) Acknowledge(_ context.Context, claim contract.Claim, receipt contract.Receipt, _ matrixproof.Authority) (contract.Receipt, error) {
	s.acks++
	if receipt.Validate(claim) != nil {
		return contract.Receipt{}, contract.ErrReceipt
	}
	s.acknowledged = true
	if s.loseACK {
		s.loseACK = false
		return contract.Receipt{}, io.ErrUnexpectedEOF
	}
	return receipt, nil
}

func TestWorkerExpiredLostAcknowledgmentReconcilesWithoutClaimingCompletedTask(t *testing.T) {
	f := newWorkerFixture(t)
	f.source.loseACK = true
	f.source.rejectCompletedClaim = true
	worker := f.open(t)
	if err := worker.Process(t.Context(), f.source.task.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	var work intent
	found, err := worker.cfg.Journal.Read(worker.owner(), storage.PrivateIngressNamespace, f.source.task.ID, &work)
	if err != nil || !found || work.Commit == nil || work.Claim == nil {
		t.Fatal("lost ACK did not retain original committed delivery", err)
	}
	work.Claim.ExpiresAt = time.Now().Add(-time.Minute)
	if err := worker.save(&work); err != nil {
		t.Fatal(err)
	}
	before := f.source.claims
	worker.Close()
	worker = f.open(t)
	if err := worker.Process(t.Context(), f.source.task.ID); err != nil {
		t.Fatal("expired completed receipt could not recover", err)
	}
	if f.source.claims != before || f.executor.executions != 1 || f.source.acks != 2 {
		t.Fatal("lost completed ACK caused claim or republication", f.source.claims, before, f.executor.executions, f.source.acks)
	}
}

type workerExecutor struct {
	metadata, body []byte
	accepted       *storage.Reference
	executions     int
	loseCommit     bool
	beforePolicy   func()
}

func (e *workerExecutor) Reconcile(ctx context.Context, _ contract.Claim, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.NewSectionReader(source, 0, size))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	if !bytes.Equal(metadata, e.metadata) || !bytes.Equal(raw, e.body) {
		return nil, contract.ErrContract
	}
	return e.accepted, nil
}

func (e *workerExecutor) Execute(ctx context.Context, claim contract.Claim, _ []byte, _ io.ReaderAt, _ int64) (storage.Reference, error) {
	if e.beforePolicy != nil {
		e.beforePolicy()
	}
	policy, ok := ctx.Value(workerPolicyKey{}).(matrix.StoragePublicationPolicy)
	if !ok {
		return storage.Reference{}, contract.ErrRevoked
	}
	if err := policy(ctx); err != nil {
		return storage.Reference{}, err
	}
	e.executions++
	ref := storage.Reference{RoomID: claim.Target.RoomID, EventID: "$accepted"}
	e.accepted = &ref
	if e.loseCommit {
		e.loseCommit = false
		return storage.Reference{}, io.ErrUnexpectedEOF
	}
	return ref, nil
}

type workerFixture struct {
	cfg      Config
	path     string
	secrets  *authtest.Secrets
	source   *workerSource
	executor *workerExecutor
	device   *workerDevice
}

func newWorkerFixture(t *testing.T) *workerFixture {
	t.Helper()
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	key, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	public := key.Recipient().String()
	plan := contract.KeyPlan{Version: 1, Scope: scope, Epoch: epoch, Generation: 1, KeyID: contract.PublicFingerprint(public), Profile: contract.Profile, Recipient: public}
	metadata := []byte(`{"id":"message","mailbox_id":"inbox","future":9007199254740993}`)
	body := bytes.Repeat([]byte("private original MIME / 漢字 é\r\n"), 20000)
	var ciphertext bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &ciphertext, plan, "delivery", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	secrets := &authtest.Secrets{}
	path := t.TempDir()
	journal, err := offline.OpenWithSecrets(path, secrets)
	if err != nil {
		t.Fatal(err)
	}
	device := &workerDevice{identity: key, binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!inbox:fixture", DeviceID: "OWNER"}, Rooms: []string{"!inbox:fixture"}}}
	source := &workerSource{task: task, cipher: ciphertext.Bytes()}
	executor := &workerExecutor{metadata: metadata, body: body}
	cfg := Config{Context: t.Context(), Scope: scope, Epoch: epoch, Journal: journal, Lease: func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }, Source: source, Device: device, Executor: executor}
	return &workerFixture{cfg: cfg, path: path, secrets: secrets, source: source, executor: executor, device: device}
}

func (f *workerFixture) open(t *testing.T) *Worker {
	t.Helper()
	journal, err := offline.OpenWithSecrets(f.path, f.secrets)
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.cfg
	cfg.Journal = journal
	worker, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Close)
	return worker
}

func (f *workerFixture) checkCipherOnly(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir(f.path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		defer clear(raw)
		if bytes.Contains(raw, []byte("private original MIME")) || bytes.Contains(raw, []byte("mailbox_id")) {
			t.Fatal("plaintext reached local disk")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkerRestartReconcilesAcceptedCommitAndLostACKWithoutRepublishing(t *testing.T) {
	for _, fault := range []string{"lost-commit", "lost-ack"} {
		t.Run(fault, func(t *testing.T) {
			f := newWorkerFixture(t)
			w := f.open(t)
			if fault == "lost-commit" {
				f.executor.loseCommit = true
			} else {
				f.source.loseACK = true
			}
			f.source.checkWAL = func() {
				var retained intent
				found, err := w.cfg.Journal.Read(w.owner(), storage.PrivateIngressNamespace, "delivery", &retained)
				if err != nil || !found || retained.Target != f.device.binding.Target || retained.Token == "" {
					t.Fatal("claim preceded durable pinned WAL", err)
				}
			}
			if err := w.Process(t.Context(), "delivery"); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("fault not exercised", err)
			}
			pending, err := w.Pending(t.Context())
			if err != nil || pending.Count != 1 {
				t.Fatal("failed work disappeared", pending, err)
			}
			f.checkCipherOnly(t)
			w.Close()
			w = f.open(t)
			if err := w.Process(t.Context(), "delivery"); err != nil {
				t.Fatal(err)
			}
			pending, err = w.Pending(t.Context())
			if err != nil || pending.Count != 0 {
				t.Fatal("acknowledged work retained", pending, err)
			}
			if f.executor.executions != 1 || f.source.opens != 1 {
				t.Fatal("restart republished or redownloaded accepted source", f.executor.executions, f.source.opens)
			}
			f.checkCipherOnly(t)
		})
	}
}

func TestWorkerChangedDeviceOrMissingLeaseCannotConsumeRetainedInput(t *testing.T) {
	f := newWorkerFixture(t)
	w := f.open(t)
	f.executor.loseCommit = true
	if err := w.Process(t.Context(), "delivery"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	w.Close()
	before := f.source.claims
	f.device.binding.Target.DeviceID = "OTHER_DEVICE"
	w = f.open(t)
	if err := w.Process(t.Context(), "delivery"); !errors.Is(err, contract.ErrRevoked) {
		t.Fatal("new device resumed old claim", err)
	}
	if f.source.claims != before || f.executor.executions != 1 {
		t.Fatal("changed target reached remote source")
	}
	f.cfg.Lease = nil
	if _, err := New(f.cfg); !errors.Is(err, contract.ErrContract) {
		t.Fatal("missing session lease accepted", err)
	}
	pending, err := w.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("fenced work removed", pending, err)
	}
}

func TestWorkerRetainsOriginalAuthorityGenerationAcrossRestart(t *testing.T) {
	f := newWorkerFixture(t)
	f.source.loseACK = true
	w := f.open(t)
	if err := w.Process(t.Context(), "delivery"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	w.Close()
	f.source.generation = 2
	w = f.open(t)
	defer w.Close()
	before := f.source.acks
	if err := w.Process(t.Context(), "delivery"); !errors.Is(err, contract.ErrRevoked) {
		t.Fatal("old WAL silently adopted a new authority generation", err)
	}
	if f.source.acks != before || f.executor.executions != 1 {
		t.Fatal("revoked work published or acknowledged")
	}
	pending, err := w.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("revoked encrypted work was removed", pending, err)
	}
	f.checkCipherOnly(t)
}

func TestWorkerPolicyRevalidatesOriginalAuthorityAtPublication(t *testing.T) {
	f := newWorkerFixture(t)
	f.executor.beforePolicy = func() { f.source.generation = 2 }
	w := f.open(t)
	defer w.Close()
	if err := w.Process(t.Context(), "delivery"); !errors.Is(err, contract.ErrRevoked) {
		t.Fatal("revocation after preparation reached publication", err)
	}
	if f.executor.executions != 0 || f.source.acks != 0 || f.source.revalidations < 2 {
		t.Fatal("publication bypassed live original-generation callback")
	}
	f.checkCipherOnly(t)
}
