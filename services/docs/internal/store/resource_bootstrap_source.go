// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ResourceBootstrapSource is fixed to the authenticated CURRENT owner scope by
// product composition. It reads only the ORIGINAL frozen Office document and
// lifecycle rows. The server neither reconstructs content nor pretends the final
// epoch exists before the baseline and checkpoint have actually been accepted.
type ResourceBootstrapSource struct {
	store *Store
	scope storage.Scope
}

func NewResourceBootstrapSource(store *Store, current storage.Scope) (*ResourceBootstrapSource, error) {
	_, office := model.ProductKind(current.Product)
	if store == nil || store.pool == nil || !current.Valid() || !office {
		return nil, resource.ErrInvalid
	}
	return &ResourceBootstrapSource{store: store, scope: current}, nil
}

var _ resource.OriginalBootstrapSource = (*ResourceBootstrapSource)(nil)

func (s *ResourceBootstrapSource) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	var result resource.BootstrapSourceSnapshot
	if s == nil || s.store == nil {
		return result, resource.ErrInvalid
	}
	err := s.store.tenantTx(ctx, func(tx pgx.Tx) error { var err error; result, err = s.bootstrapSourceTx(ctx, tx, key); return err })
	return result, err
}

// This is the SAME original source read, used inside the relay's metadata write
// transaction. No SDK/HTTP call or tenant substitution occurs while holding it.
func (s *ResourceBootstrapSource) bootstrapSourceTx(ctx context.Context, tx pgx.Tx, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	var result resource.BootstrapSourceSnapshot
	id, kind, err := s.validateBootstrapKey(ctx, key)
	if err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, s.scope.Subject); err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `SELECT public.docs_private_document_lock($1,false)`, id); err != nil {
		return result, err
	}
	state, err := pg.SharePrivateStorageState(ctx, tx, key.Product+"_private_storage_states", key.Tenant, s.scope.Subject)
	if err != nil {
		return result, err
	}
	// Legacy or already retired sources cannot silently become a new genesis.
	if state.Mode != "frozen" || state.Epoch == "" {
		return result, resource.ErrRevoked
	}
	return s.readBootstrapDocumentSnapshot(ctx, tx, key, id, kind, state.Epoch)
}

func (s *ResourceBootstrapSource) validateBootstrapKey(ctx context.Context, key resource.Resource) (uuid.UUID, model.Kind, error) {
	if s == nil || s.store == nil {
		return uuid.Nil, "", resource.ErrInvalid
	}
	tenant, err := tenancy.From(ctx)
	id, parseErr := uuid.Parse(key.ID)
	kind, office := model.ProductKind(s.scope.Product)
	if err != nil || !office || parseErr != nil || id == uuid.Nil || id.String() != key.ID || !key.Valid() || tenant.Slug != key.Tenant || key.Tenant != s.scope.Tenant || key.Product != s.scope.Product || key.Kind != string(kind) {
		return uuid.Nil, "", resource.ErrInvalid
	}
	return id, kind, nil
}

func (s *ResourceBootstrapSource) readBootstrapDocumentSnapshot(ctx context.Context, tx pgx.Tx, key resource.Resource, id uuid.UUID, kind model.Kind, epoch string) (resource.BootstrapSourceSnapshot, error) {
	var result resource.BootstrapSourceSnapshot
	var sequence, version int64
	var owner string
	err := tx.QueryRow(ctx, `SELECT d.owner_sub,d.seq,COALESCE(f.version,0) FROM documents d LEFT JOIN doc_call_access_fences f ON f.document_id=d.id AND f.tenant=d.tenant WHERE d.tenant=$1 AND d.id=$2 AND d.kind::text=$3 AND d.owner_sub=$4 AND d.deleted_at IS NULL`, key.Tenant, id, string(kind), s.scope.Subject).Scan(&owner, &sequence, &version)
	if isNoRows(err) {
		return result, resource.ErrRevoked
	}
	if err != nil {
		return result, err
	}
	if owner != s.scope.Subject {
		return result, resource.ErrRevoked
	}
	result = resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: s.scope, History: resource.HistoryFull, SourceEpoch: epoch}
	result.SourceSeq, err = resource.Sequence(sequence)
	if err != nil {
		return result, err
	}
	result.ACLVersion, err = resource.Sequence(version)
	if err != nil {
		return result, err
	}
	if !result.Valid() {
		return result, resource.ErrInvalid
	}
	return result, nil
}

func (s *ResourceBootstrapSource) CheckBootstrapSource(ctx context.Context, expected resource.BootstrapSourceSnapshot) error {
	actual, err := s.BootstrapSource(ctx, expected.Resource)
	if err != nil {
		return err
	}
	if actual != expected {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func (s *ResourceBootstrapSource) CheckBootstrapSourceTx(ctx context.Context, tx pgx.Tx, expected resource.BootstrapSourceSnapshot) error {
	actual, err := s.bootstrapSourceTx(ctx, tx, expected.Resource)
	if err != nil {
		return err
	}
	if actual != expected {
		return resource.ErrRevoked
	}
	return ctx.Err()
}
