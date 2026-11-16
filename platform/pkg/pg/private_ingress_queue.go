// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// PrivateIngressTransaction resolves scope/epoch from the domain's verified
// identity and state before invoking fn. Only public key enrollment may run
// before a legacy freeze; delivery operations require that exact frozen epoch.
type (
	PrivateIngressTransaction func(context.Context, storage.Scope, string, string, func(pgx.Tx, AuthorizedPrivateIngressConfig) error) error
	PrivateIngressBodyOpener  func(context.Context, storage.Scope, string) (io.ReadCloser, error)
)

type PrivateIngressQueue struct {
	Transaction PrivateIngressTransaction
	Body        PrivateIngressBodyOpener
}

func (q PrivateIngressQueue) Challenge(ctx context.Context, scope storage.Scope, request ingress.ControlRequest) (matrixproof.Action, error) {
	var result matrixproof.Action
	if q.Transaction == nil || request.Validate(scope, request.Epoch) != nil {
		return result, ingress.ErrContract
	}
	err := q.Transaction(ctx, scope, request.Epoch, request.Kind, func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		var err error
		result, err = ChallengePrivateIngress(ctx, tx, cfg, request)
		return err
	})
	return result, err
}

func (q PrivateIngressQueue) Apply(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction) (ingress.ControlResult, error) {
	var result ingress.ControlResult
	if q.Transaction == nil || request.Validate(scope, request.Epoch) != nil {
		return result, ingress.ErrContract
	}
	if request.Kind == "task.handoff" {
		return q.applyHandoff(ctx, scope, request, proof)
	}
	err := q.Transaction(ctx, scope, request.Epoch, request.Kind, func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		var err error
		result, err = ApplyPrivateIngressAction(ctx, tx, cfg, request, proof)
		return err
	})
	return result, err
}

func (q PrivateIngressQueue) Pending(ctx context.Context, scope storage.Scope, epoch, after string, limit int) (ingress.PendingPage, error) {
	var result ingress.PendingPage
	if q.Transaction == nil || !scope.Valid() || !storage.MigrationEpoch(epoch) {
		return result, ingress.ErrContract
	}
	err := q.Transaction(ctx, scope, epoch, "pending", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		if cfg.Authority == nil {
			return ingress.ErrRevoked
		}
		authority, err := cfg.Authority.ReadStorageAuthority(ctx, scope)
		if err != nil {
			return err
		}
		if authority.RequireActive(scope, authority.Generation) != nil {
			return ingress.ErrRevoked
		}
		result, err = ListPrivateIngressPending(ctx, tx, cfg.PrivateIngressConfig, after, limit)
		return err
	})
	return result, err
}

func (q PrivateIngressQueue) Revalidate(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction, claim ingress.Claim) (matrixproof.Authorization, error) {
	var result matrixproof.Authorization
	if q.Transaction == nil || request.Validate(scope, request.Epoch) != nil {
		return result, ingress.ErrContract
	}
	err := q.Transaction(ctx, scope, request.Epoch, "task.claim", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		var err error
		result, err = RevalidatePrivateIngressPublication(ctx, tx, cfg, request, proof, claim)
		return err
	})
	return result, err
}

func (q PrivateIngressQueue) Open(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction, claim ingress.Claim) (storage.StreamSourceRow, io.ReadCloser, error) {
	if q.Transaction == nil || q.Body == nil || request.Validate(scope, request.Epoch) != nil {
		return storage.StreamSourceRow{}, nil, ingress.ErrContract
	}
	var blob string
	err := q.Transaction(ctx, scope, request.Epoch, "task.claim", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		if _, err := RevalidatePrivateIngressPublication(ctx, tx, cfg, request, proof, claim); err != nil {
			return err
		}
		stored, err := readPrivateIngressDelivery(ctx, tx, cfg.PrivateIngressConfig, claim.Task.ID, false)
		if err != nil {
			return err
		}
		blob = stored.BlobID
		return nil
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	body, err := q.Body(ctx, scope, blob)
	return claim.Task.Cipher, body, err
}
