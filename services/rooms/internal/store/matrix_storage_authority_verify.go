// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// VerifyMatrixStorageAction is a fresh authority check, never a reusable lease.
// The domain supplies its retained expected nonce/transcript and atomically
// consumes that nonce with its own queue mutation. No Matrix admin key leaves
// Rooms; only public verification metadata is returned.
func (s *Store) VerifyMatrixStorageAction(ctx context.Context, subject, product string, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	var result matrixproof.Authorization
	// Una bóveda post-cuántica (ADR 0087) no tiene autoridad en Matrix: se comprueba
	// contra la clave que Sytue ID tiene registrada. Las dos vías nunca se mezclan.
	if vaultproof.Signed(proof) {
		scope, err := cutoverScope(ctx, subject, product)
		if err != nil {
			return result, err
		}
		return s.verifyVaultStorageAction(ctx, scope, expected, proof)
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return result, domain.ErrMatrixUnavailable
	}
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		result, err = verifyMatrixStorageActionTx(ctx, tx, scope, expected, proof, directory)
		return err
	})
	return result, err
}

func verifyMatrixStorageActionTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, expected matrixproof.Action, proof matrixproof.SignedAction, directory matrixIngressDirectory) (matrixproof.Authorization, error) {
	if expected.Scope != scope || proof.Action != expected {
		return matrixproof.Authorization{}, domain.ErrForbidden
	}
	authority, err := matrixAuthorityTx(ctx, tx, scope)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := authority.RequireActive(scope, expected.Generation); err != nil {
		return matrixproof.Authorization{}, err
	}
	// Tras un traspaso a la bóveda, ninguna prueba de Matrix del ámbito vale.
	if err := sinTraspasoTx(ctx, tx, scope, matrixproof.ErrUnverified); err != nil {
		return matrixproof.Authorization{}, err
	}
	if expected.RootKey != authority.RootKey {
		return matrixproof.Authorization{}, matrixproof.ErrUnverified
	}
	if err := matrixAuthorityOwnsTarget(ctx, tx, scope, expected.Target); err != nil {
		return matrixproof.Authorization{}, err
	}
	origin, err := directory.IngressDeviceOrigin(ctx, scope, expected.Target)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if origin.UserID.String() != authority.UserID || origin.Scope != scope {
		return matrixproof.Authorization{}, matrixproof.ErrUnverified
	}
	device, err := matrixproof.Verify(ctx, origin, id.Ed25519(authority.RootKey))
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := device.VerifyAction(ctx, proof, expected, time.Now()); err != nil {
		return matrixproof.Authorization{}, err
	}
	signing, err := device.SigningKey(scope, expected.Target)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	result := matrixproof.Authorization{Version: 1, Authority: authority, Target: expected.Target, SigningKey: signing, Nonce: expected.Nonce, RequestSHA256: expected.RequestSHA256}
	return result, nil
}
