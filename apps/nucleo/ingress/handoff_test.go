// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type handoffWorkerDevice struct {
	*workerDevice
	plan contract.KeyPlan
}

func (d handoffWorkerDevice) RecoverIngressKeyPlan(context.Context, contract.Task) (contract.KeyPlan, error) {
	return d.plan, nil
}

type handoffWorkerSource struct {
	*workerSource
	state            contract.TaskState
	fence            *contract.DeviceFence
	granted          *contract.Claim
	digest           string
	loseResolve      bool
	checkHandoffWAL  func(contract.ControlRequest)
	begins, resolves int
}

func (s *handoffWorkerSource) Inspect(context.Context, string) (contract.TaskState, error) {
	return s.state, nil
}

func (s *handoffWorkerSource) Enroll(_ context.Context, plan contract.KeyPlan, _ storage.PublicationTarget, authority matrixproof.Authority) error {
	if plan != s.state.Plan || authority.Generation != s.authorityGeneration() {
		return contract.ErrRevoked
	}
	return nil
}

func (s *handoffWorkerSource) OpenHandoff(_ context.Context, _ contract.ControlRequest, task contract.Task, _ matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error) {
	s.opens++
	return task.Cipher, io.NopCloser(bytes.NewReader(s.cipher)), nil
}

func (s *handoffWorkerSource) Handoff(_ context.Context, request contract.ControlRequest, authority matrixproof.Authority) (contract.HandoffView, error) {
	if s.checkHandoffWAL != nil {
		s.checkHandoffWAL(request)
	}
	if authority.Generation != s.authorityGeneration() {
		return contract.HandoffView{}, contract.ErrRevoked
	}
	begin, err := request.BeginHandoffRequest()
	if err != nil {
		return contract.HandoffView{}, err
	}
	digest, err := matrixproof.RequestDigest(begin)
	if err != nil {
		return contract.HandoffView{}, err
	}
	if s.digest != "" && s.digest != digest {
		return contract.HandoffView{}, contract.ErrClaim
	}
	s.digest = digest
	if s.fence == nil {
		now := time.Now()
		s.fence = &contract.DeviceFence{Version: 1, Scope: request.Scope, Epoch: request.Epoch, ID: request.Handoff.ID, TaskID: s.task.ID, CipherSHA256: s.task.Cipher.SHA256, Previous: *s.state.Assignment, Target: request.Target, Authority: authority, RequestSHA256: digest, Status: "confirmed", ConfirmedAt: &now}
	}
	view := contract.HandoffView{Version: 1, ID: request.Handoff.ID, Task: s.task, Previous: *s.state.Assignment, Target: request.Target, Status: "readable", Fence: s.fence}
	if request.Handoff.Phase == "begin" {
		s.begins++
		return view, nil
	}
	s.resolves++
	if request.Handoff.Resolution == "committed" {
		view.Status = "acknowledged"
		view.Receipt = request.Handoff.Receipt
	} else {
		if s.granted == nil {
			s.granted = &contract.Claim{Version: 1, Task: s.task, Target: request.Target, Token: request.Handoff.Token, Generation: s.state.Assignment.Generation + 1, ExpiresAt: time.Now().Add(time.Minute)}
		}
		view.Status = "claimed"
		view.Claim = s.granted
	}
	if s.loseResolve {
		s.loseResolve = false
		return contract.HandoffView{}, io.ErrUnexpectedEOF
	}
	return view, view.Validate(request, authority)
}

func (s *handoffWorkerSource) Claim(_ context.Context, _ storage.Scope, _, _ string, target storage.PublicationTarget, token string, authority matrixproof.Authority) (contract.Claim, error) {
	s.claims++
	if s.granted == nil || s.granted.Target != target || s.granted.Token != token || authority.Generation != s.authorityGeneration() {
		return contract.Claim{}, contract.ErrClaim
	}
	return *s.granted, nil
}

type handoffWorkerExecutor struct {
	*workerExecutor
	reads int
}

func (e *handoffWorkerExecutor) ReconcileHandoff(ctx context.Context, _ contract.Task, _ contract.Assignment, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	policy, ok := ctx.Value(workerPolicyKey{}).(matrix.StoragePublicationPolicy)
	if !ok || !errors.Is(policy(ctx), contract.ErrClaim) {
		return nil, errors.New("handoff reconciliation was allowed to publish")
	}
	e.reads++
	raw, err := io.ReadAll(io.NewSectionReader(source, 0, size))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	if !bytes.Equal(raw, e.body) || !bytes.Equal(metadata, e.metadata) {
		return nil, contract.ErrContract
	}
	return e.accepted, nil
}

func configureWorkerHandoff(t *testing.T, f *workerFixture) (*handoffWorkerSource, *handoffWorkerExecutor) {
	t.Helper()
	public := f.device.identity.Recipient().String()
	plan := contract.KeyPlan{Version: 1, Scope: f.cfg.Scope, Epoch: f.cfg.Epoch, Generation: 1, KeyID: contract.PublicFingerprint(public), Profile: contract.Profile, Recipient: public}
	previous := contract.Assignment{Target: storage.PublicationTarget{RoomID: "!old:fixture", DeviceID: "DEVICE_A"}, Generation: 1, SigningKey: strings.Repeat("A", 43), ExpiresAt: time.Now().Add(-time.Minute)}
	source := &handoffWorkerSource{workerSource: f.source, state: contract.TaskState{Version: 1, Task: f.source.task, Plan: plan, Assignment: &previous}}
	executor := &handoffWorkerExecutor{workerExecutor: f.executor}
	f.cfg.Source = source
	f.cfg.Executor = executor
	f.cfg.Device = handoffWorkerDevice{workerDevice: f.device, plan: plan}
	return source, executor
}

func TestWorkerHandoffRetainsSignedResolutionAcrossLostResponseAndRestart(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "accepted-A"}[committed], func(t *testing.T) {
			f := newWorkerFixture(t)
			source, executor := configureWorkerHandoff(t, f)
			if committed {
				ref := storage.Reference{RoomID: source.state.Assignment.Target.RoomID, EventID: "$accepted_by_A"}
				executor.accepted = &ref
			}
			source.loseResolve = true
			w := f.open(t)
			source.checkHandoffWAL = func(request contract.ControlRequest) {
				var retained intent
				found, err := w.cfg.Journal.Read(w.owner(), storage.PrivateIngressNamespace, request.Handoff.TaskID, &retained)
				if err != nil || !found || retained.Handoff == nil {
					t.Fatal("handoff preceded durable encrypted intent", err)
				}
				if request.Handoff.Phase == "resolve" && (retained.Handoff.Resolve == nil || retained.Handoff.Cipher == nil) {
					t.Fatal("resolve preceded retained reconciliation")
				}
			}
			if err := w.Process(t.Context(), source.task.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("lost resolve response was not exercised", err)
			}
			if executor.reads != 1 || executor.executions != 0 || source.claims != 0 || source.acks != 0 {
				t.Fatal("read-only handoff executed before grant")
			}
			f.checkCipherOnly(t)
			w.Close()
			w = f.open(t)
			if err := w.Process(t.Context(), source.task.ID); err != nil {
				t.Fatal("retained handoff could not resume", err)
			}
			if executor.reads != 1 || source.opens != 1 || source.begins != 1 || source.resolves != 2 {
				t.Fatal("retry replaced retained reconciliation", executor.reads, source.opens, source.begins, source.resolves)
			}
			if committed && (executor.executions != 0 || source.claims != 0 || source.acks != 0) {
				t.Fatal("accepted A result caused B publication")
			}
			if !committed && (executor.executions != 1 || source.claims != 1 || source.acks != 1) {
				t.Fatal("absent A result did not publish exactly once under B grant")
			}
			pending, err := w.Pending(t.Context())
			if err != nil || pending.Count != 0 {
				t.Fatal("acknowledged handoff WAL was not cleaned", err)
			}
			f.checkCipherOnly(t)
		})
	}
}
