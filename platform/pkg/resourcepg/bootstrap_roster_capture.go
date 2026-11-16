// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapCaptureStore is a restricted view of the SAME relay before an epoch
// exists. It exposes no publication, routing, device, key or capability methods.
type BootstrapCaptureStore struct{ relay *Relay }

func NewBootstrapCaptureStore(dependencies Dependencies, owner string) (*BootstrapCaptureStore, error) {
	if dependencies.TenantTx == nil || dependencies.ValidateResource == nil || dependencies.CheckBootstrapSource == nil || owner == "" {
		return nil, resource.ErrInvalid
	}
	return &BootstrapCaptureStore{relay: &Relay{dependencies: dependencies, subject: owner}}, nil
}

type BootstrapCaptureStatus struct {
	PlanID         string           `json:"plan_id"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	MemberCount    resource.Decimal `json:"member_count"`
	ReceivedCount  resource.Decimal `json:"received_count"`
	State          string           `json:"state"`
}

func (s *BootstrapCaptureStore) check(ctx context.Context, p resource.VerifiedBootstrapRosterManifest) error {
	if s == nil || s.relay == nil || p.Plan().CreatorScope.Subject != s.relay.subject {
		return resource.ErrRevoked
	}
	if err := s.relay.scoped(ctx, p.Plan().Source.Resource); err != nil {
		return err
	}
	return p.Revalidate(ctx)
}

type bootstrapCaptureMetadata interface {
	Plan() resource.BootstrapPlan
	Value() resource.BootstrapRosterManifest
	SHA256() string
	PublicPlanBytes() []byte
	PublicManifestBytes() []byte
	SessionBinding() resource.SessionBinding
}

func (s *BootstrapCaptureStore) read(ctx context.Context, tx pgx.Tx, p bootstrapCaptureMetadata, lock bool) (BootstrapCaptureStatus, error) {
	var out BootstrapCaptureStatus
	key := p.Plan().Source.Resource
	q := `SELECT plan_id::text,manifest_sha256,member_count,received_count,state,plan,signed_plan,manifest,source_epoch::text,source_seq,acl_version,session_generation,creator_user,creator_device,creator_root,session_binding_sha256,room_id FROM resource_e2e_roster_captures WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6`
	if lock {
		q += " FOR UPDATE"
	}
	args := append(resourceKey(key), p.Plan().ID, s.relay.subject)
	var count, received, seq, acl, generation int64
	var unsigned, signed, manifest []byte
	var epoch, user, device, root, binding, room string
	err := tx.QueryRow(ctx, q, args...).Scan(&out.PlanID, &out.ManifestSHA256, &count, &received, &out.State, &unsigned, &signed, &manifest, &epoch, &seq, &acl, &generation, &user, &device, &root, &binding, &room)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, resource.ErrPending
	}
	if err != nil {
		return out, err
	}
	out.MemberCount, _ = resource.Sequence(count)
	out.ReceivedCount, _ = resource.Sequence(received)
	if !matchesCaptureMetadata(p, out, unsigned, signed, manifest, epoch, seq, acl, generation, user, device, root, binding, room) {
		return BootstrapCaptureStatus{}, resource.ErrConflict
	}
	switch out.State {
	case "cancelled":
		return BootstrapCaptureStatus{}, resource.ErrRevoked
	case "pending", "complete":
		return out, nil
	default:
		return BootstrapCaptureStatus{}, resource.ErrInvalid
	}
}

func matchesCaptureMetadata(p bootstrapCaptureMetadata, out BootstrapCaptureStatus, unsigned, signed, manifest []byte, epoch string, seq, acl, generation int64, user, device, root, binding, room string) bool {
	plan := p.Plan()
	expected, _ := matrixproof.CanonicalBytes(plan)
	actualSeq, _ := plan.Source.SourceSeq.Int64()
	actualACL, _ := plan.Source.ACLVersion.Int64()
	actualGeneration, _ := p.SessionBinding().Generation.Int64()
	return out.ManifestSHA256 == p.SHA256() && out.MemberCount == p.Value().MemberCount && bytes.Equal(expected, unsigned) && bytes.Equal(signed, p.PublicPlanBytes()) && bytes.Equal(manifest, p.PublicManifestBytes()) && epoch == plan.Source.SourceEpoch && seq == actualSeq && acl == actualACL && generation == actualGeneration && user == plan.Creator.UserID && device == plan.Creator.DeviceID && root == plan.Creator.RootKey && binding == plan.SessionBindingSHA256 && room == plan.RoomID
}

func (s *BootstrapCaptureStore) locked(ctx context.Context, tx pgx.Tx, p resource.VerifiedBootstrapRosterManifest) (BootstrapCaptureStatus, error) {
	if err := s.relay.dependencies.CheckBootstrapSource(ctx, tx, p.Plan().Source); err != nil {
		return BootstrapCaptureStatus{}, err
	}
	return s.read(ctx, tx, p, true)
}

// Begin retains the signed count/digest and ORIGINAL independent source fields.
// It never calls BeginBootstrap or registers a room from an incomplete roster.
func (s *BootstrapCaptureStore) Begin(ctx context.Context, p resource.VerifiedBootstrapRosterManifest) (BootstrapCaptureStatus, error) {
	var out BootstrapCaptureStatus
	if err := s.check(ctx, p); err != nil {
		return out, err
	}
	captured, captureErr := resource.CaptureBootstrapRosterEvidence(ctx, p)
	if captureErr != nil {
		return out, captureErr
	}
	pin := captured.OwnerPin()
	rootGeneration, _ := captured.SessionBinding().RootGeneration.Int64()
	plan := p.Plan()
	key := plan.Source.Resource
	raw, err := matrixproof.CanonicalBytes(plan)
	if err != nil {
		return out, err
	}
	count, _ := p.Value().MemberCount.Int64()
	seq, _ := plan.Source.SourceSeq.Int64()
	acl, _ := plan.Source.ACLVersion.Int64()
	generation, _ := p.SessionBinding().Generation.Int64()
	err = s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if err := s.relay.dependencies.CheckBootstrapSource(ctx, tx, plan.Source); err != nil {
			return err
		}
		existing, readErr := s.read(ctx, tx, p, true)
		if readErr == nil {
			out = existing
			return nil
		}
		if !errors.Is(readErr, resource.ErrPending) {
			return readErr
		}
		_, err := tx.Exec(ctx, `INSERT INTO resource_e2e_roster_captures(tenant,product,kind,resource_id,plan_id,owner_sub,room_id,source_epoch,source_seq,acl_version,session_generation,creator_user,creator_device,creator_root,session_binding_sha256,plan,plan_sha256,signed_plan,manifest,manifest_sha256,member_count,stream_sha256,evidence_sha256,evidence,creator_signing_key,creator_encryption_key,root_generation)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)ON CONFLICT DO NOTHING`, key.Tenant, key.Product, key.Kind, key.ID, plan.ID, s.relay.subject, plan.RoomID, plan.Source.SourceEpoch, seq, acl, generation, plan.Creator.UserID, plan.Creator.DeviceID, plan.Creator.RootKey, plan.SessionBindingSHA256, raw, p.Value().PlanSHA256, p.PublicPlanBytes(), p.PublicManifestBytes(), p.SHA256(), count, p.Value().OrderedInputsSHA256, captured.SHA256(), captured.Bytes(), pin.SigningKey.String(), pin.EncryptionKey.String(), rootGeneration)
		if err != nil {
			return err
		}
		if err := storeResourceEvidence(ctx, tx, key, captured); err != nil {
			return err
		}
		out, err = s.read(ctx, tx, p, true)
		if errors.Is(err, resource.ErrPending) {
			return resource.ErrConflict
		}
		return err
	})
	if err != nil {
		return BootstrapCaptureStatus{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapCaptureStatus{}, err
	}
	return out, nil
}

func (s *BootstrapCaptureStore) Status(ctx context.Context, p resource.VerifiedBootstrapRosterManifest) (BootstrapCaptureStatus, error) {
	var out BootstrapCaptureStatus
	if err := s.check(ctx, p); err != nil {
		return out, err
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.locked(ctx, tx, p); return err })
	if err != nil {
		return BootstrapCaptureStatus{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapCaptureStatus{}, err
	}
	return out, nil
}

// Append is idempotent for exact immutable bytes. Only a fresh contiguous page
// can advance SQL progress, with GLOBAL order checked under the same row lock.
func (s *BootstrapCaptureStore) Append(ctx context.Context, p resource.VerifiedBootstrapRosterManifest, page resource.BootstrapRosterPage) (BootstrapCaptureStatus, error) {
	var out BootstrapCaptureStatus
	if err := s.check(ctx, p); err != nil {
		return out, err
	}
	raw, err := resource.BootstrapRosterPageBytes(p.Plan().Source.Resource, page)
	if err != nil {
		return out, err
	}
	first, last, err := resource.BootstrapRosterPageOrderKeys(p.Plan().Source.Resource, page)
	if err != nil {
		return out, err
	}
	sha, err := resource.CipherJSONDigest(raw)
	if err != nil {
		return out, err
	}
	offset, _ := page.Offset.Int64()
	key := p.Plan().Source.Resource
	err = s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.locked(ctx, tx, p)
		if err != nil {
			return err
		}
		var saved []byte
		args := append(resourceKey(key), p.Plan().ID, offset)
		err = tx.QueryRow(ctx, `SELECT value FROM resource_e2e_roster_pages WHERE(tenant,product,kind,resource_id,plan_id,input_offset)=($1,$2,$3,$4,$5,$6)`, args...).Scan(&saved)
		if err == nil {
			if !bytes.Equal(saved, raw) {
				return resource.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if out.State != "pending" || page.Offset != out.ReceivedCount {
			return resource.ErrConflict
		}
		total, _ := out.MemberCount.Int64()
		if offset > total || int64(len(page.Inputs)) > total-offset {
			return resource.ErrInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO resource_e2e_roster_pages(tenant,product,kind,resource_id,plan_id,input_offset,input_count,value,sha256,first_key,last_key)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, key.Tenant, key.Product, key.Kind, key.ID, p.Plan().ID, offset, len(page.Inputs), raw, sha, first, last)
		if err != nil {
			return err
		}
		out, err = s.read(ctx, tx, p, false)
		return err
	})
	if err != nil {
		return BootstrapCaptureStatus{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapCaptureStatus{}, err
	}
	return out, nil
}

type bootstrapStoredRosterPages struct {
	store   *BootstrapCaptureStore
	pending resource.VerifiedBootstrapRosterManifest
}

func (s *BootstrapCaptureStore) Pages(p resource.VerifiedBootstrapRosterManifest) resource.BootstrapRosterPageSource {
	return bootstrapStoredRosterPages{store: s, pending: p}
}

func (p bootstrapStoredRosterPages) BootstrapRosterPage(ctx context.Context, offset resource.Decimal) (resource.BootstrapRosterPage, error) {
	if err := p.store.check(ctx, p.pending); err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	index, err := offset.Int64()
	if err != nil {
		return resource.BootstrapRosterPage{}, resource.ErrInvalid
	}
	var raw []byte
	key := p.pending.Plan().Source.Resource
	err = p.store.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := p.store.locked(ctx, tx, p.pending); err != nil {
			return err
		}
		args := append(resourceKey(key), p.pending.Plan().ID, index)
		err := tx.QueryRow(ctx, `SELECT value FROM resource_e2e_roster_pages WHERE(tenant,product,kind,resource_id,plan_id,input_offset)=($1,$2,$3,$4,$5,$6)`, args...).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return resource.ErrPending
		}
		return err
	})
	if err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	page, err := resource.DecodeBootstrapRosterPage(key, raw)
	if err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	if err := p.pending.Revalidate(ctx); err != nil {
		return resource.BootstrapRosterPage{}, err
	}
	return page, nil
}

// Cancel retains partial evidence. There is no HTTP/SDK publication in this
// phase, so it cannot discard an accepted or ambiguous message/epoch hold.
func (s *BootstrapCaptureStore) Cancel(ctx context.Context, p resource.VerifiedBootstrapRosterManifest) error {
	if err := s.check(ctx, p); err != nil {
		return err
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		status, err := s.locked(ctx, tx, p)
		if err != nil {
			return err
		}
		if status.State != "pending" {
			return resource.ErrConflict
		}
		args := append(resourceKey(p.Plan().Source.Resource), p.Plan().ID, s.relay.subject)
		_, err = tx.Exec(ctx, `UPDATE resource_e2e_roster_captures SET state='cancelled',finished_at=now()WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6 AND state='pending'`, args...)
		return err
	})
	if err != nil {
		return err
	}
	return p.Revalidate(ctx)
}

// Finalize REVERIFIES every CURRENT recipient and the original frozen source.
// The SAME complete VerifiedPlan/evidence and completed capture commit in ONE
// transaction. This prepares a bootstrap; it grants no room/key or origin send.
func (s *BootstrapCaptureStore) Finalize(ctx context.Context, p resource.VerifiedBootstrapRosterManifest, provider *resource.SessionProvider) (resource.VerifiedBootstrapPlan, error) {
	if err := s.check(ctx, p); err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	verified, err := resource.CompleteBootstrapRosterCapture(ctx, p, s.Pages(p), provider)
	if err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	if err := verified.RequireCurrentRosterCapture(ctx); err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	if err := s.relay.checkBootstrap(ctx, verified); err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	capture, err := resource.CaptureBootstrapPlan(ctx, verified)
	if err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	err = s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		status, err := s.locked(ctx, tx, p)
		if err != nil {
			return err
		}
		if status.ReceivedCount != status.MemberCount {
			return resource.ErrPending
		}
		if _, err := s.relay.beginBootstrapTx(ctx, tx, verified, capture); err != nil {
			return err
		}
		if status.State == "complete" {
			return nil
		}
		args := append(resourceKey(p.Plan().Source.Resource), p.Plan().ID, s.relay.subject)
		_, err = tx.Exec(ctx, `UPDATE resource_e2e_roster_captures SET state='complete',finished_at=now()WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6 AND state='pending'`, args...)
		return err
	})
	if err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	if err := verified.Revalidate(ctx); err != nil {
		return resource.VerifiedBootstrapPlan{}, err
	}
	return verified, nil
}
