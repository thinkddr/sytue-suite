// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"time"

	"github.com/google/uuid"

	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

type rejectedHandoff struct {
	State contract.TaskState      `json:"state"`
	Begin contract.ControlRequest `json:"begin"`
	View  contract.HandoffView    `json:"view"`
}

func (r rejectedHandoff) validate(cfg Config, work intent) error {
	previous := handoffIntent{State: r.State, Begin: r.Begin, View: &r.View}
	if previous.validateBinding(cfg, work) != nil || r.View.Status != "not_started" || r.View.Task != r.State.Task {
		return contract.ErrContract
	}
	return r.View.Validate(r.Begin, work.Authority)
}

// Only an immutable server refusal can advance to a fresh inspection. The
// original signed begin survives in this encrypted WAL. Timeouts, cancellation
// and uncertain responses never enter this path or erase an earlier request.
func (w *Worker) refreshRejectedHandoff(ctx context.Context, source HandoffSource, work *intent) error {
	h := work.Handoff
	if h.View == nil || h.View.Status != "not_started" || h.View.Validate(h.Begin, work.Authority) != nil || h.Resolve != nil || h.Cipher != nil || work.Claim != nil {
		return contract.ErrClaim
	}
	authority, err := w.cfg.Source.Authority(ctx, w.cfg.Scope)
	if err != nil {
		return err
	}
	if authority != work.Authority {
		return contract.ErrRevoked
	}
	state, err := source.Inspect(ctx, work.ID)
	if err != nil {
		return err
	}
	if state.Validate(w.cfg.Scope, w.cfg.Epoch, work.ID) != nil || state.Task != h.State.Task || state.Plan != h.State.Plan {
		return contract.ErrRevoked
	}
	if !freshHandoffAssignment(state, h.State, work) {
		return contract.ErrClaim
	}
	authority, err = w.cfg.Source.Authority(ctx, w.cfg.Scope)
	if err != nil {
		return err
	}
	if authority != work.Authority {
		return contract.ErrRevoked
	}
	h.Rejected = append(h.Rejected, rejectedHandoff{State: h.State, Begin: h.Begin, View: *h.View})
	h.State = state
	h.Begin = contract.ControlRequest{Version: 1, Scope: w.cfg.Scope, Epoch: w.cfg.Epoch, Kind: "task.handoff", Target: work.Target, Handoff: &contract.HandoffRequest{ID: uuid.NewString(), Phase: "begin", TaskID: work.ID, CipherSHA256: state.Task.Cipher.SHA256, Previous: *state.Assignment, Token: work.Token}}
	h.View = nil
	if err := w.save(work); err != nil {
		return err
	}
	return contract.ErrClaim
}

func freshHandoffAssignment(state, previous contract.TaskState, work *intent) bool {
	if state.Assignment == nil || state.Assignment.Target.DeviceID == work.Target.DeviceID || (state.Receipt == nil && state.Assignment.ExpiresAt.After(time.Now())) {
		return false
	}
	if previous.Assignment == nil || !state.Assignment.Same(*previous.Assignment) {
		return true
	}
	return state.Receipt != nil && (previous.Receipt == nil || *state.Receipt != *previous.Receipt)
}
