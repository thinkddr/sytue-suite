// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// CheckBootstrapPeer resolves permission only from ORIGINAL normal-RLS rows
// under the same document/freeze lock. Collective ACLs require a current ID
// membership authority and deliberately cannot be inferred from roster JSON.
func (s *ResourceBootstrapSource) CheckBootstrapPeer(ctx context.Context, expected resource.BootstrapSourceSnapshot, input resource.BootstrapRosterInput) error {
	if s == nil || s.store == nil || expected.OwnerScope != s.scope || input.Member.Scope.Tenant != s.scope.Tenant || input.Member.Scope.Product != s.scope.Product {
		return resource.ErrInvalid
	}
	if _, err := resource.BootstrapRosterInputBytes(expected.Resource, input); err != nil {
		return err
	}
	return s.store.tenantTx(ctx, func(tx pgx.Tx) error {
		if err := s.CheckBootstrapSourceTx(ctx, tx, expected); err != nil {
			return err
		}
		return s.CheckBootstrapPeerRightsTx(ctx, tx, expected, input)
	})
}

// CheckBootstrapPeerRightsTx checks the exact signed leaf against the ORIGINAL
// live ACL. The encompassing operation must first CheckBootstrapSourceTx in this
// same transaction, preserving the source -> capture lock order. No ID membership
// is inferred from the roster's tenant, subject, role or device enrollment.
func (s *ResourceBootstrapSource) CheckBootstrapPeerRightsTx(ctx context.Context, tx pgx.Tx, expected resource.BootstrapSourceSnapshot, input resource.BootstrapRosterInput) error {
	if s == nil || s.store == nil || expected.OwnerScope != s.scope || input.Member.Scope.Tenant != s.scope.Tenant || input.Member.Scope.Product != s.scope.Product {
		return resource.ErrInvalid
	}
	if _, err := resource.BootstrapRosterInputBytes(expected.Resource, input); err != nil {
		return err
	}
	permissions, err := listPermissionsInTx(ctx, tx, expected.Resource.ID)
	if err != nil {
		return err
	}
	// Collective ACLs deliberately remain fail-closed without a fresh ID proof.
	// The shared reader already removes expired/revoked via_link and blocked rows.
	role, allowed := model.ResolveRole(&model.Document{OwnerSub: expected.OwnerScope.Subject}, permissions, model.Subject{Sub: input.Member.Scope.Subject})
	if !allowed || input.Member.Role != string(role) {
		return resource.ErrRevoked
	}
	for _, action := range input.Member.Actions {
		if !role.AllowsResourceAction(action) {
			return resource.ErrRevoked
		}
	}
	return nil
}
