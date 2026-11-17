// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
)

func verifyPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	if cfg.Authority == nil || request.Kind != "task.handoff" || request.Validate(cfg.Scope, cfg.Epoch.String()) != nil {
		return matrixproof.Authorization{}, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg.PrivateIngressConfig); err != nil {
		return matrixproof.Authorization{}, err
	}
	retained, err := retainedPrivateIngressAction(ctx, tx, cfg, request, proof)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := requirePrivateIngressDevice(ctx, tx, cfg.PrivateIngressConfig, request.Target); err != nil {
		return matrixproof.Authorization{}, err
	}
	authorization, err := cfg.Authority.VerifyStorageAction(ctx, retained.action, proof)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := authorization.Validate(retained.action); err != nil {
		return matrixproof.Authorization{}, err
	}
	handoff, err := readPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if handoff.view.Status == "not_started" {
		return matrixproof.Authorization{}, ingress.ErrClaim
	}
	return authorization, handoff.requireBinding(request, authorization)
}

// No domain transaction spans Matrix device deletion. The original assignment
// and nonce commit first; an uncertain remote result remains fencing and must
// be resumed with the exact same immutable request, never reset on a timer.
func (q PrivateIngressQueue) applyHandoff(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction) (ingress.ControlResult, error) {
	var result ingress.ControlResult
	var fencer callaccess.StorageDeviceFencer
	var authority matrixproof.Authority
	var completed bool
	initial := request.Handoff.Phase == "begin"
	err := q.Transaction(ctx, scope, request.Epoch, "task.handoff", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		var ok bool
		fencer, ok = cfg.Authority.(callaccess.StorageDeviceFencer)
		if !ok {
			return ingress.ErrRevoked
		}
		if initial {
			var err error
			result, err = ApplyPrivateIngressAction(ctx, tx, cfg, request, proof)
			authority = result.Authorization.Authority
			return err
		}
		authorized, err := verifyPrivateIngressHandoff(ctx, tx, cfg, request, proof)
		authority = authorized.Authority
		if err != nil {
			return err
		}
		handoff, err := readPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request)
		completed = handoff.view.Receipt != nil
		return err
	})
	if err != nil {
		return ingress.ControlResult{}, err
	}
	if initial && (result.Handoff.Status == "readable" || result.Handoff.Status == "not_started") {
		return result, nil
	}
	control := ingress.ControlProof{Request: request, Proof: proof}
	var fence ingress.DeviceFence
	if initial {
		fence, err = fencer.FenceStorageDevice(ctx, control)
	} else if !completed {
		fence, err = fencer.ReadStorageDeviceFence(ctx, control)
	}
	if err != nil {
		return ingress.ControlResult{}, err
	}
	if !completed && (fence.Status != "confirmed" || fence.Validate(request, authority) != nil) {
		return ingress.ControlResult{}, ingress.ErrRevoked
	}
	err = q.Transaction(ctx, scope, request.Epoch, "task.handoff", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		if !completed {
			cfg.handoffFence = &fence
		}
		var err error
		result, err = ApplyPrivateIngressAction(ctx, tx, cfg, request, proof)
		return err
	})
	return result, err
}

// OpenHandoff permits only a confirmed, currently authorized read. It has no
// claim token from A and cannot be used by the publication revalidator.
func (q PrivateIngressQueue) OpenHandoff(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction) (storage.StreamSourceRow, io.ReadCloser, error) {
	if q.Transaction == nil || q.Body == nil || !privateIngressHandoffBodyRequest(scope, request) {
		return storage.StreamSourceRow{}, nil, ingress.ErrContract
	}
	var fencer callaccess.StorageDeviceFencer
	var authority matrixproof.Authority
	var completed bool
	err := q.Transaction(ctx, scope, request.Epoch, "task.handoff", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		var ok bool
		fencer, ok = cfg.Authority.(callaccess.StorageDeviceFencer)
		if !ok {
			return ingress.ErrRevoked
		}
		authorized, err := verifyPrivateIngressHandoff(ctx, tx, cfg, request, proof)
		authority = authorized.Authority
		if err != nil {
			return err
		}
		handoff, err := readPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request)
		completed = handoff.view.Receipt != nil
		return err
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	if !completed {
		fence, err := fencer.ReadStorageDeviceFence(ctx, ingress.ControlProof{Request: request, Proof: proof})
		if err != nil {
			return storage.StreamSourceRow{}, nil, err
		}
		if fence.Status != "confirmed" || fence.Validate(request, authority) != nil {
			return storage.StreamSourceRow{}, nil, ingress.ErrRevoked
		}
	}
	var stored PrivateIngressDelivery
	err = q.Transaction(ctx, scope, request.Epoch, "task.handoff", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		authorized, err := verifyPrivateIngressHandoff(ctx, tx, cfg, request, proof)
		if err != nil {
			return err
		}
		if authorized.Authority != authority {
			return ingress.ErrRevoked
		}
		handoff, err := readPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request)
		if err != nil {
			return err
		}
		if handoff.view.Status != "readable" || (handoff.view.Receipt == nil && (handoff.view.Fence == nil || handoff.view.Fence.Validate(request, authority) != nil)) {
			return ingress.ErrClaim
		}
		stored, err = readPrivateIngressDelivery(ctx, tx, cfg.PrivateIngressConfig, request.Handoff.TaskID, false)
		return err
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	body, err := q.Body(ctx, scope, stored.BlobID)
	return stored.Task.Cipher, body, err
}

func privateIngressHandoffBodyRequest(scope storage.Scope, request ingress.ControlRequest) bool {
	return request.Kind == "task.handoff" && request.Handoff != nil && request.Handoff.Phase == "begin" && request.Validate(scope, request.Epoch) == nil
}
