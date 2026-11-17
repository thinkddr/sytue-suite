// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"testing"
	"time"

	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type activeLeaseHandoffSource struct {
	*handoffWorkerSource
	calls int
}

func (s *activeLeaseHandoffSource) Handoff(context.Context, contract.ControlRequest, matrixproof.Authority) (contract.HandoffView, error) {
	s.calls++
	return contract.HandoffView{}, contract.ErrClaim
}

func TestWorkerHandoffDoesNotCaptureRenewableAssignmentOrBeginWhileAIsActive(t *testing.T) {
	f := newWorkerFixture(t)
	source, _ := configureWorkerHandoff(t, f)
	source.state.Assignment.ExpiresAt = time.Now().Add(time.Minute)
	active := &activeLeaseHandoffSource{handoffWorkerSource: source}
	f.cfg.Source = active
	w := f.open(t)
	if err := w.Process(t.Context(), source.task.ID); !errors.Is(err, contract.ErrClaim) {
		t.Fatal("active assignment was admitted for handoff", err)
	}
	pending, err := w.Pending(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if active.calls != 0 || pending.Count != 0 {
		t.Fatal("active renewable assignment was captured or sent for fencing", active.calls, pending.Count)
	}
}
