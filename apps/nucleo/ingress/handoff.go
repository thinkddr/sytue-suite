// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type HandoffSource interface {
	Inspect(context.Context, string) (contract.TaskState, error)
	Enroll(context.Context, contract.KeyPlan, storage.PublicationTarget, matrixproof.Authority) error
	Handoff(context.Context, contract.ControlRequest, matrixproof.Authority) (contract.HandoffView, error)
	OpenHandoff(context.Context, contract.ControlRequest, contract.Task, matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error)
}
type HandoffExecutor interface {
	// ReconcileHandoff reads complete authenticated domain history without an A
	// claim or publication permit. Returning nil asserts a complete no-match.
	ReconcileHandoff(context.Context, contract.Task, contract.Assignment, []byte, io.ReaderAt, int64) (*storage.Reference, error)
}
type ingressPlanRecovery interface {
	RecoverIngressKeyPlan(context.Context, contract.Task) (contract.KeyPlan, error)
}

type handoffIntent struct {
	State        contract.TaskState       `json:"state"`
	Begin        contract.ControlRequest  `json:"begin"`
	Resolve      *contract.ControlRequest `json:"resolve,omitempty"`
	View         *contract.HandoffView    `json:"view,omitempty"`
	Cipher       *offline.StagedFile      `json:"cipher,omitempty"`
	Acknowledged bool                     `json:"acknowledged,omitempty"`
	Rejected     []rejectedHandoff        `json:"rejected,omitempty"`
}

func (h handoffIntent) validate(cfg Config, work intent) error {
	if err := h.validateBinding(cfg, work); err != nil {
		return err
	}
	for _, rejected := range h.Rejected {
		if err := rejected.validate(cfg, work); err != nil {
			return err
		}
	}
	if err := h.validateResolution(); err != nil {
		return err
	}
	if err := h.validateView(work); err != nil {
		return err
	}
	return h.validateProgress(work)
}

func (h handoffIntent) validateBinding(cfg Config, work intent) error {
	if h.State.Validate(cfg.Scope, cfg.Epoch, work.ID) != nil || h.State.Assignment == nil || h.Begin.Validate(cfg.Scope, cfg.Epoch) != nil || h.Begin.Kind != "task.handoff" || h.Begin.Handoff == nil || h.Begin.Handoff.Phase != "begin" {
		return contract.ErrContract
	}
	b := h.Begin.Handoff
	if h.Begin.Target != work.Target || b.TaskID != work.ID || b.Token != work.Token || b.CipherSHA256 != h.State.Task.Cipher.SHA256 || !b.Previous.Same(*h.State.Assignment) {
		return contract.ErrClaim
	}
	return nil
}

func (w *Worker) prepareHandoff(ctx context.Context, work *intent) error {
	source, ok := w.cfg.Source.(HandoffSource)
	if !ok {
		return nil
	}
	state, err := source.Inspect(ctx, work.ID)
	if err != nil {
		return err
	}
	if state.Validate(w.cfg.Scope, w.cfg.Epoch, work.ID) != nil {
		return contract.ErrContract
	}
	if state.Assignment == nil {
		return nil
	}
	if state.Assignment.Target.DeviceID == work.Target.DeviceID {
		return contract.ErrClaim
	}
	// Expiry admits inspection of a possible handoff, never publication. Avoid
	// retaining a renewable snapshot before the owning device has gone idle.
	if state.Receipt == nil && state.Assignment.ExpiresAt.After(time.Now()) {
		return contract.ErrClaim
	}
	if _, ok := w.cfg.Executor.(HandoffExecutor); !ok {
		return contract.ErrContract
	}
	request := contract.ControlRequest{Version: 1, Scope: w.cfg.Scope, Epoch: w.cfg.Epoch, Kind: "task.handoff", Target: work.Target, Handoff: &contract.HandoffRequest{ID: uuid.NewString(), Phase: "begin", TaskID: work.ID, CipherSHA256: state.Task.Cipher.SHA256, Previous: *state.Assignment, Token: work.Token}}
	work.Handoff = &handoffIntent{State: state, Begin: request}
	return nil
}

func (w *Worker) resumeHandoff(ctx context.Context, work *intent) error {
	h := work.Handoff
	if h.Acknowledged {
		return w.cleanupHandoff(work)
	}
	if work.Claim != nil {
		return w.resume(ctx, work)
	}
	source, ok := w.cfg.Source.(HandoffSource)
	if !ok {
		return contract.ErrContract
	}
	if err := w.prepareHandoffRead(ctx, source, work); err != nil {
		return err
	}
	view, err := source.Handoff(ctx, *h.Resolve, work.Authority)
	if err != nil {
		return err
	}
	if view.Validate(*h.Resolve, work.Authority) != nil {
		return contract.ErrClaim
	}
	switch view.Status {
	case "acknowledged":
		h.View = &view
		h.Acknowledged = true
		if err := w.save(work); err != nil {
			return err
		}
		return w.cleanupHandoff(work)
	case "claimed":
		work.Claim = view.Claim
		work.Cipher = h.Cipher
		if err := w.save(work); err != nil {
			return err
		}
		return w.resume(ctx, work)
	default:
		return contract.ErrClaim
	}
}

func (w *Worker) captureHandoff(ctx context.Context, source HandoffSource, work *intent) error {
	h := work.Handoff
	if h.Cipher != nil {
		return nil
	}
	row, input, err := source.OpenHandoff(ctx, h.Begin, h.State.Task, work.Authority)
	if input != nil {
		defer input.Close()
	}
	if err != nil {
		return err
	}
	if input == nil || row != h.State.Task.Cipher {
		return contract.ErrContract
	}
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	file, err := w.cfg.Journal.StageFile(ctx, w.owner(), storage.PrivateIngressStagingNamespace, input, row.Size)
	if err != nil {
		return err
	}
	if file.SizeBytes != row.Size || file.SHA256 != row.SHA256 {
		_ = w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, file)
		return contract.ErrContract
	}
	h.Cipher = &file
	if err := w.save(work); err != nil {
		_ = w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, file)
		return err
	}
	return nil
}

func (w *Worker) reconcileHandoff(ctx context.Context, work *intent) error {
	h := work.Handoff
	executor, ok := w.cfg.Executor.(HandoffExecutor)
	if !ok || h.Cipher == nil {
		return contract.ErrContract
	}
	// This sibling context can only read. Even an accidental executor Send is
	// denied by the common SDK immediately before encryption and publication.
	readCtx, err := w.cfg.Device.RequireAuthorizedStoragePublication(ctx, w.cfg.Scope, work.Target, storage.PublicationPolicy(func(context.Context) error { return contract.ErrClaim }))
	if err != nil {
		return err
	}
	cipher, err := w.cfg.Journal.OpenStagedFile(readCtx, w.owner(), storage.PrivateIngressStagingNamespace, *h.Cipher)
	if err != nil {
		return err
	}
	defer cipher.Close()
	metadata, body, err := w.cfg.Device.OpenIngressBody(readCtx, h.State.Task, cipher)
	if err != nil {
		return err
	}
	defer clear(metadata)
	accepted, err := executor.ReconcileHandoff(readCtx, h.State.Task, *h.State.Assignment, metadata, body, body.Size())
	if err != nil {
		return err
	}
	resolve := h.Begin
	request := *resolve.Handoff
	request.Phase = "resolve"
	if accepted == nil {
		request.Resolution = "absent"
		request.LeaseSeconds = 300
	} else {
		if !accepted.Valid() || accepted.RoomID != request.Previous.Target.RoomID {
			return contract.ErrReceipt
		}
		request.Resolution = "committed"
		request.Receipt = &contract.Receipt{Version: 1, Scope: w.cfg.Scope, Epoch: w.cfg.Epoch, TaskID: work.ID, CipherSHA256: h.State.Task.Cipher.SHA256, ClaimGeneration: request.Previous.Generation, Commit: *accepted}
	}
	resolve.Handoff = &request
	if resolve.Validate(w.cfg.Scope, w.cfg.Epoch) != nil {
		return contract.ErrContract
	}
	h.Resolve = &resolve
	return w.save(work)
}

func (w *Worker) cleanupHandoff(work *intent) error {
	if work.Handoff.Cipher != nil {
		if err := w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, *work.Handoff.Cipher); err != nil {
			return err
		}
	}
	return w.cfg.Journal.Delete(w.owner(), storage.PrivateIngressNamespace, work.ID)
}

func (h handoffIntent) validateResolution() error {
	if h.Resolve != nil {
		begin, err := h.Resolve.BeginHandoffRequest()
		if err != nil {
			return err
		}
		expected, err := matrixproof.RequestDigest(h.Begin)
		if err != nil {
			return err
		}
		actual, err := matrixproof.RequestDigest(begin)
		if err != nil || actual != expected || h.Resolve.Handoff.Phase != "resolve" {
			return contract.ErrClaim
		}
	}
	return nil
}

func (h handoffIntent) validateView(work intent) error {
	if h.View != nil {
		request := h.Begin
		if h.View.Status == "claimed" || h.View.Status == "acknowledged" {
			if h.Resolve == nil {
				return contract.ErrClaim
			}
			request = *h.Resolve
		}
		if h.View.Validate(request, work.Authority) != nil {
			return contract.ErrClaim
		}
	}
	return nil
}

func (h handoffIntent) validateProgress(work intent) error {
	if work.Claim != nil && (h.Resolve == nil || h.Resolve.Handoff.Resolution != "absent" || work.Claim.Task != h.State.Task || work.Claim.Generation != h.State.Assignment.Generation+1) {
		return contract.ErrClaim
	}
	if h.Cipher != nil && (h.Cipher.SizeBytes != h.State.Task.Cipher.Size || h.Cipher.SHA256 != h.State.Task.Cipher.SHA256) {
		return contract.ErrContract
	}
	if h.Acknowledged && (h.View == nil || h.View.Status != "acknowledged" || h.Cipher == nil) {
		return contract.ErrReceipt
	}
	return nil
}

func (w *Worker) prepareHandoffRead(ctx context.Context, source HandoffSource, work *intent) error {
	h := work.Handoff
	if h.View != nil && h.View.Status == "not_started" {
		return w.refreshRejectedHandoff(ctx, source, work)
	}
	recovery, ok := w.cfg.Device.(ingressPlanRecovery)
	if !ok {
		return contract.ErrContract
	}
	plan, err := recovery.RecoverIngressKeyPlan(ctx, h.State.Task)
	if err != nil {
		return err
	}
	if plan != h.State.Plan {
		return contract.ErrRevoked
	}
	if err := source.Enroll(ctx, plan, work.Target, work.Authority); err != nil {
		return err
	}
	if h.Resolve == nil {
		view, err := source.Handoff(ctx, h.Begin, work.Authority)
		if err != nil {
			return err
		}
		if view.Validate(h.Begin, work.Authority) != nil {
			return contract.ErrClaim
		}
		h.View = &view
		if err := w.save(work); err != nil {
			return err
		}
		if view.Status == "not_started" {
			return w.refreshRejectedHandoff(ctx, source, work)
		}
		if view.Status != "readable" {
			return contract.ErrClaim
		}
		if err := w.captureHandoff(ctx, source, work); err != nil {
			return err
		}
		if err := w.reconcileHandoff(ctx, work); err != nil {
			return err
		}
	}
	return nil
}
