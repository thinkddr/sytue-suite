// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

type BootstrapPeerTransactionSource interface {
	CheckBootstrapSourceTx(context.Context, pgx.Tx, resource.BootstrapSourceSnapshot) error
	CheckBootstrapPeerRightsTx(context.Context, pgx.Tx, resource.BootstrapSourceSnapshot, resource.BootstrapRosterInput) error
}

// Only this request's already locked immutable capture is read here. There is
// no provider/HTTP callback in SQL and no page-level permission cache.
type bootstrapPeerTransactionPages struct {
	tx       pgx.Tx
	metadata resource.BootstrapPeerMetadata
}

func (p bootstrapPeerTransactionPages) BootstrapRosterPage(ctx context.Context, offset resource.Decimal) (resource.BootstrapRosterPage, error) {
	index, err := offset.Int64()
	if err != nil {
		return resource.BootstrapRosterPage{}, resource.ErrInvalid
	}
	plan := p.metadata.Plan()
	args := append(resourceKey(plan.Source.Resource), plan.ID, index)
	var raw []byte
	err = p.tx.QueryRow(ctx, `SELECT value FROM resource_e2e_roster_pages WHERE(tenant,product,kind,resource_id,plan_id,input_offset)=($1,$2,$3,$4,$5,$6)`, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return resource.BootstrapRosterPage{}, resource.ErrPending
	}
	if err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	return resource.DecodeBootstrapRosterPage(plan.Source.Resource, raw)
}

// CurrentPeerEvidenceOperation verifies immutable signatures/commitments once,
// then fences CURRENT owner, B and ORIGINAL SQL at PRE/POST. Its one-use request
// witnesses are consumed internally and never returned/stored as permission.
// All network I/O is outside SQL. Future SessionProvider.Revalidate stays live.
func (s *BootstrapCaptureStore) CurrentPeerEvidenceOperation(ctx context.Context, query resource.BootstrapPeerQuery, source BootstrapPeerTransactionSource, authority *SessionAuthorityClient) (resource.SessionEvidence, error) {
	var zero resource.SessionEvidence
	provider, metadata, err := s.verifyPeerOperationRequest(ctx, query, source, authority)
	if err != nil {
		return zero, err
	}
	input, err := s.selectPeerRosterInput(ctx, source, metadata)
	if err != nil {
		return zero, err
	}
	before, _, err := authority.resolveCurrentEvidence(ctx, metadata.Scope(), metadata.DeviceID())
	if err != nil {
		return zero, err
	}
	if err := metadata.CheckPeerSnapshot(input, before); err != nil {
		return zero, err
	}
	if err := s.reverifyPeerRosterInput(ctx, source, metadata, input); err != nil {
		return zero, err
	}
	ownerAfter, err := provider.Resolve(ctx, query.Plan.CreatorScope, query.Plan.Creator.DeviceID)
	if err != nil {
		return zero, err
	}
	if err := metadata.CheckOwnerSnapshot(ownerAfter); err != nil {
		return zero, err
	}
	after, evidence, err := authority.resolveCurrentEvidence(ctx, metadata.Scope(), metadata.DeviceID())
	if err != nil {
		return zero, err
	}
	if before.Binding() != after.Binding() || before.Device().Identity() != after.Device().Identity() {
		return zero, resource.ErrRevoked
	}
	if err := metadata.CheckPeerSnapshot(input, after); err != nil {
		return zero, err
	}
	return evidence, nil
}

func (s *BootstrapCaptureStore) verifyPeerOperationRequest(ctx context.Context, query resource.BootstrapPeerQuery, source BootstrapPeerTransactionSource, authority *SessionAuthorityClient) (*resource.SessionProvider, resource.BootstrapPeerMetadata, error) {
	if s == nil || s.relay == nil || source == nil || authority == nil || !query.Valid() || query.Plan.CreatorScope.Subject != s.relay.subject {
		return nil, resource.BootstrapPeerMetadata{}, resource.ErrInvalid
	}
	if err := s.relay.scoped(ctx, query.Plan.Source.Resource); err != nil {
		return nil, resource.BootstrapPeerMetadata{}, err
	}
	provider, err := authority.Provider()
	if err != nil {
		return nil, resource.BootstrapPeerMetadata{}, err
	}
	owner, err := provider.Resolve(ctx, query.Plan.CreatorScope, query.Plan.Creator.DeviceID)
	if err != nil {
		return nil, resource.BootstrapPeerMetadata{}, err
	}
	metadata, err := resource.VerifyBootstrapPeerMetadata(ctx, query, owner)
	return provider, metadata, err
}

func (s *BootstrapCaptureStore) selectPeerRosterInput(ctx context.Context, source BootstrapPeerTransactionSource, metadata resource.BootstrapPeerMetadata) (resource.BootstrapRosterInput, error) {
	var input resource.BootstrapRosterInput
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if err := source.CheckBootstrapSourceTx(ctx, tx, metadata.Plan().Source); err != nil {
			return err
		}
		if _, err := s.read(ctx, tx, metadata, true); err != nil {
			return err
		}
		var err error
		input, err = metadata.SelectPeer(ctx, bootstrapPeerTransactionPages{tx: tx, metadata: metadata})
		if err != nil {
			return err
		}
		return source.CheckBootstrapPeerRightsTx(ctx, tx, metadata.Plan().Source, input)
	})
	return input, err
}

func (s *BootstrapCaptureStore) reverifyPeerRosterInput(ctx context.Context, source BootstrapPeerTransactionSource, metadata resource.BootstrapPeerMetadata, input resource.BootstrapRosterInput) error {
	return s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if err := source.CheckBootstrapSourceTx(ctx, tx, metadata.Plan().Source); err != nil {
			return err
		}
		if err := source.CheckBootstrapPeerRightsTx(ctx, tx, metadata.Plan().Source, input); err != nil {
			return err
		}
		_, err := s.read(ctx, tx, metadata, true)
		return err
	})
}
