// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type bootstrapRetainedCaptureSource struct {
	store   *BootstrapCaptureStore
	current resource.VerifiedSession
	key     resource.Resource
	plan    string
}

// HistoricalSource authenticates the CURRENT original owner. It exposes only
// public retained SDK evidence, never a live A, epoch key or publication permit.
func (s *BootstrapCaptureStore) HistoricalSource(ctx context.Context, current resource.VerifiedSession, key resource.Resource, plan string) (resource.TrustedBootstrapCaptureSource, error) {
	view := bootstrapRetainedCaptureSource{store: s, current: current, key: key, plan: plan}
	if err := view.check(ctx); err != nil {
		return nil, err
	}
	if _, err := view.RetainedBootstrapCapture(ctx, key, plan); err != nil {
		return nil, err
	}
	return view, nil
}

func (v bootstrapRetainedCaptureSource) check(ctx context.Context) error {
	if v.store == nil || v.store.relay == nil || v.current.Scope() != (storage.Scope{Tenant: v.key.Tenant, Subject: v.store.relay.subject, Product: v.key.Product}) {
		return resource.ErrRevoked
	}
	if err := v.store.relay.scoped(ctx, v.key); err != nil {
		return err
	}
	return v.current.Revalidate(ctx)
}

func (s *BootstrapCaptureStore) readRetainedCapture(ctx context.Context, tx pgx.Tx, current resource.VerifiedSession, key resource.Resource, plan string, lock bool) (resource.RetainedBootstrapCapture, error) {
	var out resource.RetainedBootstrapCapture
	q := `SELECT state,source_epoch::text,source_seq,acl_version,creator_user,creator_device,creator_root,session_generation,session_binding_sha256,room_id,plan_sha256,manifest_sha256,evidence_sha256,evidence,creator_signing_key,creator_encryption_key,root_generation FROM resource_e2e_roster_captures WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6`
	if lock {
		q += " FOR UPDATE"
	}
	args := append(resourceKey(key), plan, s.relay.subject)
	var epoch, user, device, root string
	var seq, acl, generation int64
	var evidenceSHA, signing, encryption *string
	var rootGeneration *int64
	err := tx.QueryRow(ctx, q, args...).Scan(&out.State, &epoch, &seq, &acl, &user, &device, &root, &generation, &out.SessionBindingSHA256, &out.RoomID, &out.PlanSHA256, &out.ManifestSHA256, &evidenceSHA, &out.Evidence, &signing, &encryption, &rootGeneration)
	if err != nil {
		return out, err
	}
	if evidenceSHA == nil || signing == nil || encryption == nil || rootGeneration == nil {
		return resource.RetainedBootstrapCapture{}, resource.ErrRevoked
	}
	out.Resource = key
	out.PlanID = plan
	out.EvidenceSHA256 = *evidenceSHA
	out.SessionGeneration, _ = resource.Sequence(generation)
	out.RootGeneration, _ = resource.Sequence(*rootGeneration)
	sourceSeq, _ := resource.Sequence(seq)
	version, _ := resource.Sequence(acl)
	scope := storage.Scope{Tenant: key.Tenant, Subject: s.relay.subject, Product: key.Product}
	out.Source = resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: scope, SourceEpoch: epoch, SourceSeq: sourceSeq, ACLVersion: version, History: resource.HistoryFull}
	out.Creator = resource.Actor{UserID: user, DeviceID: device, RootKey: root}
	out.OwnerPin = matrixproof.Identity{Scope: scope, UserID: id.UserID(user), DeviceID: id.DeviceID(device), RootKey: id.Ed25519(root), SigningKey: id.Ed25519(*signing), EncryptionKey: id.Curve25519(*encryption)}
	// Owner authority is the stable account/root/product, not the historical
	// actor's device or leaf. A different root requires explicit reset policy.
	if current.Scope() != scope || current.Actor().UserID != user || current.Actor().RootKey != root || current.Binding().RootGeneration != out.RootGeneration {
		return resource.RetainedBootstrapCapture{}, resource.ErrRevoked
	}
	return out, nil
}

func (v bootstrapRetainedCaptureSource) RetainedBootstrapCapture(ctx context.Context, key resource.Resource, plan string) (resource.RetainedBootstrapCapture, error) {
	var out resource.RetainedBootstrapCapture
	if key != v.key || plan != v.plan {
		return out, resource.ErrRevoked
	}
	if err := v.check(ctx); err != nil {
		return out, err
	}
	err := v.store.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = v.store.readRetainedCapture(ctx, tx, v.current, key, plan, false)
		if err != nil {
			return err
		}
		return v.store.relay.dependencies.CheckBootstrapSource(ctx, tx, out.Source)
	})
	if err != nil {
		return resource.RetainedBootstrapCapture{}, err
	}
	if err := v.current.Revalidate(ctx); err != nil {
		return resource.RetainedBootstrapCapture{}, err
	}
	return out, nil
}

func (v bootstrapRetainedCaptureSource) PublicEvidence(ctx context.Context, key resource.Resource, sha string) (json.RawMessage, error) {
	if key != v.key {
		return nil, resource.ErrRevoked
	}
	if err := v.check(ctx); err != nil {
		return nil, err
	}
	var raw []byte
	err := v.store.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		held, err := v.store.readRetainedCapture(ctx, tx, v.current, key, v.plan, false)
		if err != nil {
			return err
		}
		if err := v.store.relay.dependencies.CheckBootstrapSource(ctx, tx, held.Source); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT value FROM resource_e2e_public_evidence WHERE tenant=$1 AND product=$2 AND sha256=$3`, key.Tenant, key.Product, sha).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	if err := v.current.Revalidate(ctx); err != nil {
		return nil, err
	}
	return raw, nil
}

// CancelRecovered is strictly PRE-epoch metadata cancellation by current B.
// Existing bootstrap (even zero slots), source epoch/journal or accepted work
// requires a separate fenced handoff; absence/404/elapsed time is no authority.
func (s *BootstrapCaptureStore) CancelRecovered(ctx context.Context, c resource.VerifiedBootstrapCaptureCancellation) error {
	if s == nil || s.relay == nil {
		return resource.ErrRevoked
	}
	q := c.Value()
	if q.Scope.Subject != s.relay.subject {
		return resource.ErrRevoked
	}
	if err := s.relay.scoped(ctx, q.Resource); err != nil {
		return err
	}
	if err := c.Revalidate(ctx); err != nil {
		return err
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		// Freeze/source locks precede the capture lock, matching Finalize.
		if err := s.relay.dependencies.CheckBootstrapSource(ctx, tx, q.Source); err != nil {
			return err
		}
		var state, manifestSHA, evidenceSHA, user, device, root, signing, encryption string
		var oldSourceEpoch string
		var seq, acl int64
		args := append(resourceKey(q.Resource), q.PlanID, s.relay.subject)
		if err := tx.QueryRow(ctx, `SELECT state,manifest_sha256,evidence_sha256,creator_user,creator_device,creator_root,creator_signing_key,creator_encryption_key,source_epoch::text,source_seq,acl_version FROM resource_e2e_roster_captures WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6 FOR UPDATE`, args...).Scan(&state, &manifestSHA, &evidenceSHA, &user, &device, &root, &signing, &encryption, &oldSourceEpoch, &seq, &acl); err != nil {
			return err
		}
		sourceSeq, _ := q.Source.SourceSeq.Int64()
		sourceACL, _ := q.Source.ACLVersion.Int64()
		pin := matrixproof.Identity{Scope: q.Scope, UserID: id.UserID(user), DeviceID: id.DeviceID(device), RootKey: id.Ed25519(root), SigningKey: id.Ed25519(signing), EncryptionKey: id.Curve25519(encryption)}
		if state != "pending" || manifestSHA != q.ManifestSHA256 || evidenceSHA != c.OriginalEvidenceSHA256() || pin != c.OriginalOwnerPin() || oldSourceEpoch != q.Source.SourceEpoch || seq != sourceSeq || acl != sourceACL {
			return resource.ErrRevoked
		}
		var published bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM resource_e2e_bootstraps WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4))OR EXISTS(SELECT 1 FROM resource_e2e_bootstrap_slots WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4))OR EXISTS(SELECT 1 FROM resource_e2e_resources WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4))OR EXISTS(SELECT 1 FROM resource_e2e_candidates WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4))OR EXISTS(SELECT 1 FROM resource_e2e_receipts WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4))`, resourceKey(q.Resource)...).Scan(&published); err != nil {
			return err
		}
		if published {
			return resource.ErrRevoked
		}
		_, err := tx.Exec(ctx, `UPDATE resource_e2e_roster_captures SET state='cancelled',finished_at=now()WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6 AND state='pending'`, args...)
		return err
	})
	if err != nil {
		return err
	}
	return c.Revalidate(ctx)
}
