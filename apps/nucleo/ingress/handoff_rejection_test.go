// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type renewingHandoffSource struct {
	*handoffWorkerSource
	rejected  map[string]contract.HandoffView
	lose      bool
	uncertain bool
	seen      []string
}

func (s *renewingHandoffSource) Inspect(context.Context, string) (contract.TaskState, error) {
	state := s.state
	if state.Assignment != nil {
		assignment := *state.Assignment
		state.Assignment = &assignment
	}
	if state.Receipt != nil {
		receipt := *state.Receipt
		state.Receipt = &receipt
	}
	return state, nil
}

func (s *renewingHandoffSource) Handoff(ctx context.Context, request contract.ControlRequest, authority matrixproof.Authority) (contract.HandoffView, error) {
	if request.Handoff.Phase != "begin" {
		return s.handoffWorkerSource.Handoff(ctx, request, authority)
	}
	s.seen = append(s.seen, request.Handoff.ID)
	if s.uncertain {
		return contract.HandoffView{}, io.ErrUnexpectedEOF
	}
	if retained, ok := s.rejected[request.Handoff.ID]; ok {
		return retained, nil
	}
	if len(s.rejected) == 0 {
		s.state.Assignment.ExpiresAt = time.Now().Add(-30 * time.Second)
		digest, err := matrixproof.RequestDigest(request)
		if err != nil {
			return contract.HandoffView{}, err
		}
		view := contract.HandoffView{Version: 1, ID: request.Handoff.ID, Task: s.task, Previous: request.Handoff.Previous, Target: request.Target, Status: "not_started", Rejection: &contract.HandoffRejection{Version: 1, ID: request.Handoff.ID, RequestSHA256: digest, Authority: authority, Reason: "assignment_changed"}}
		s.rejected[request.Handoff.ID] = view
		if s.lose {
			s.lose = false
			return contract.HandoffView{}, io.ErrUnexpectedEOF
		}
		return view, nil
	}
	return s.handoffWorkerSource.Handoff(ctx, request, authority)
}

func TestWorkerRetainsLostRejectionBeforeCapturingFreshAssignment(t *testing.T) {
	f := newWorkerFixture(t)
	base, executor := configureWorkerHandoff(t, f)
	source := &renewingHandoffSource{handoffWorkerSource: base, rejected: map[string]contract.HandoffView{}, lose: true}
	f.cfg.Source = source
	w := f.open(t)
	if err := w.Process(t.Context(), source.task.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("lost refusal was not exercised", err)
	}
	firstID := source.seen[0]
	w.Close()
	w = f.open(t)
	if err := w.Process(t.Context(), source.task.ID); !errors.Is(err, contract.ErrClaim) {
		t.Fatal("refusal did not retain a fresh attempt", err)
	}
	var retained intent
	found, err := w.cfg.Journal.Read(w.owner(), storage.PrivateIngressNamespace, source.task.ID, &retained)
	if err != nil || !found || retained.Handoff == nil || len(retained.Handoff.Rejected) != 1 || retained.Handoff.Rejected[0].Begin.Handoff.ID != firstID || retained.Handoff.Begin.Handoff.ID == firstID {
		t.Fatal("refresh discarded the rejected signed request", err)
	}
	if source.begins != 0 || source.claims != 0 || executor.executions != 0 || executor.reads != 0 {
		t.Fatal("refusal granted read or publication")
	}
	if len(source.seen) != 2 || source.seen[0] != source.seen[1] {
		t.Fatal("uncertain response changed its original ID")
	}
	if err := w.Process(t.Context(), source.task.ID); err != nil {
		t.Fatal("fresh retained attempt failed", err)
	}
	if executor.executions != 1 || executor.reads != 1 || source.begins != 1 {
		t.Fatal("refresh duplicated execution")
	}
	f.checkCipherOnly(t)
}

func TestWorkerNeverRefreshesAnAmbiguousBegin(t *testing.T) {
	f := newWorkerFixture(t)
	base, _ := configureWorkerHandoff(t, f)
	source := &renewingHandoffSource{handoffWorkerSource: base, uncertain: true}
	f.cfg.Source = source
	w := f.open(t)
	for range 2 {
		if err := w.Process(t.Context(), source.task.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(err)
		}
		w.Close()
		w = f.open(t)
	}
	if len(source.seen) != 2 || source.seen[0] != source.seen[1] {
		t.Fatal("ambiguous begin was reset or replaced")
	}
	var retained intent
	found, err := w.cfg.Journal.Read(w.owner(), storage.PrivateIngressNamespace, source.task.ID, &retained)
	if err != nil || !found || retained.Handoff == nil || retained.Handoff.View != nil || len(retained.Handoff.Rejected) != 0 {
		t.Fatal("ambiguous response became a refusal", err)
	}
}
