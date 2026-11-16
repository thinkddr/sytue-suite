// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BootstrapSlot contains opaque prepared SDK bytes. Sending/Ambiguous means
// reconcile first; reading an existing slot never authorizes blind publication.
type BootstrapSlot struct {
	Kind      resource.BootstrapKind      `json:"kind"`
	Candidate resource.BootstrapCandidate `json:"candidate"`
	SHA256    string                      `json:"sha256"`
	State     string                      `json:"state"`
	Reference *storage.Reference          `json:"reference,omitempty"`
}
type BootstrapState struct {
	PlanID     string `json:"plan_id"`
	PlanSHA256 string `json:"plan_sha256"`
	State      string `json:"state"`
}
type bootstrapRow struct {
	plan        resource.BootstrapPlan
	hash, state string
	generation  int64
}

func (r *Relay) checkBootstrap(ctx context.Context, p resource.VerifiedBootstrapPlan) error {
	v := p.Value()
	if r.dependencies.CheckBootstrapSource == nil || v.CreatorScope.Subject != r.subject || (r.dependencies.RequirePrivateRules && !v.Validation.PrivateRules) {
		return resource.ErrRevoked
	}
	if err := r.scoped(ctx, v.Source.Resource); err != nil {
		return err
	}
	return p.RequireCurrentRosterCapture(ctx)
}

func (r *Relay) readBootstrap(ctx context.Context, tx pgx.Tx, key resource.Resource, lock bool) (bootstrapRow, error) {
	q := `SELECT plan,plan_sha256,state,source_epoch::text,source_seq,acl_version,session_generation,room_id,epoch,creator_user,creator_device,creator_root,session_binding_sha256 FROM resource_e2e_bootstraps WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4)AND owner_sub=$5`
	if lock {
		q += " FOR UPDATE"
	}
	var out bootstrapRow
	var raw []byte
	var sourceEpoch, room, epoch, user, device, root, binding string
	var seq, acl int64
	args := append(resourceKey(key), r.subject)
	err := tx.QueryRow(ctx, q, args...).Scan(&raw, &out.hash, &out.state, &sourceEpoch, &seq, &acl, &out.generation, &room, &epoch, &user, &device, &root, &binding)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out.plan) != nil || !out.plan.Valid() {
		return out, resource.ErrInvalid
	}
	hash, err := resource.BootstrapPlanSHA256(out.plan)
	sourceSeq, _ := resource.Sequence(seq)
	version, _ := resource.Sequence(acl)
	if err != nil || hash != out.hash || out.plan.Source.Resource != key || out.plan.Source.OwnerScope.Subject != r.subject || out.plan.Source.SourceEpoch != sourceEpoch || out.plan.Source.SourceSeq != sourceSeq || out.plan.Source.ACLVersion != version || out.plan.RoomID != room || out.plan.Epoch != epoch || out.plan.Creator.UserID != user || out.plan.Creator.DeviceID != device || out.plan.Creator.RootKey != root || out.plan.SessionBindingSHA256 != binding {
		return out, resource.ErrInvalid
	}
	return out, nil
}

func (r *Relay) lockedBootstrap(ctx context.Context, tx pgx.Tx, p resource.VerifiedBootstrapPlan) (bootstrapRow, error) {
	if err := r.dependencies.CheckBootstrapSource(ctx, tx, p.Value().Source); err != nil {
		return bootstrapRow{}, err
	}
	row, err := r.readBootstrap(ctx, tx, p.Value().Source.Resource, true)
	if err != nil {
		return row, err
	}
	if row.hash != p.SHA256() || row.plan != p.Value() {
		return row, resource.ErrConflict
	}
	return row, nil
}

// BeginBootstrap persists plan/evidence before any SDK wire can be admitted.
// Concurrent identical requests return the original plan; another plan cannot
// silently replace the frozen source, actor, recipient, roster or room.
func (r *Relay) BeginBootstrap(ctx context.Context, p resource.VerifiedBootstrapPlan) (BootstrapState, error) {
	var result BootstrapState
	if err := r.checkBootstrap(ctx, p); err != nil {
		return result, err
	}
	capture, err := resource.CaptureBootstrapPlan(ctx, p)
	if err != nil {
		return result, err
	}
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = r.beginBootstrapTx(ctx, tx, p, capture)
		return err
	})
	if err != nil {
		return BootstrapState{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapState{}, err
	}
	return result, nil
}

func (r *Relay) beginBootstrapTx(ctx context.Context, tx pgx.Tx, p resource.VerifiedBootstrapPlan, capture resource.CapturedBootstrapPlan) (BootstrapState, error) {
	value := capture.Value()
	key := value.Source.Resource
	if !capture.Valid() || value != p.Value() || capture.SHA256() != p.SHA256() {
		return BootstrapState{}, resource.ErrInvalid
	}
	generation, err := p.SessionBinding().Generation.Int64()
	if err != nil {
		return BootstrapState{}, err
	}
	seq, _ := value.Source.SourceSeq.Int64()
	acl, _ := value.Source.ACLVersion.Int64()
	if err := r.dependencies.CheckBootstrapSource(ctx, tx, value.Source); err != nil {
		return BootstrapState{}, err
	}
	if row, readErr := r.readBootstrap(ctx, tx, key, true); readErr == nil {
		if row.hash != p.SHA256() || row.plan != p.Value() {
			return BootstrapState{}, resource.ErrConflict
		}
		if err := storeResourceEvidence(ctx, tx, key, capture); err != nil {
			return BootstrapState{}, err
		}
		return BootstrapState{PlanID: row.plan.ID, PlanSHA256: row.hash, State: row.state}, nil
	} else if !errors.Is(readErr, pgx.ErrNoRows) {
		return BootstrapState{}, readErr
	}
	_, err = tx.Exec(ctx, `INSERT INTO resource_e2e_bootstraps(tenant,product,kind,resource_id,plan_id,room_id,epoch,source_epoch,source_seq,acl_version,owner_sub,creator_user,creator_device,creator_root,session_generation,session_binding_sha256,plan_sha256,plan,signed_plan,evidence_sha256,evidence)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)ON CONFLICT DO NOTHING`, key.Tenant, key.Product, key.Kind, key.ID, value.ID, value.RoomID, value.Epoch, value.Source.SourceEpoch, seq, acl, r.subject, value.Creator.UserID, value.Creator.DeviceID, value.Creator.RootKey, generation, value.SessionBindingSHA256, capture.SHA256(), capture.Bytes(), capture.SignedBytes(), capture.EvidenceSHA256(), capture.EvidenceBytes())
	if err != nil {
		return BootstrapState{}, err
	}
	row, err := r.lockedBootstrap(ctx, tx, p)
	if err != nil {
		return BootstrapState{}, err
	}
	if err := storeResourceEvidence(ctx, tx, key, capture); err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{PlanID: row.plan.ID, PlanSHA256: row.hash, State: row.state}, nil
}

func (r *Relay) bootstrapSlot(ctx context.Context, tx pgx.Tx, row bootstrapRow, kind resource.BootstrapKind, lock bool) (BootstrapSlot, error) {
	var result BootstrapSlot
	result.Kind = kind
	q := `SELECT signed_publication,encrypted_event,session_certificate,candidate_sha256,state,room_id,event_id FROM resource_e2e_bootstrap_slots WHERE(tenant,product,kind,resource_id,plan_id,bootstrap_kind)=($1,$2,$3,$4,$5,$6)`
	if lock {
		q += " FOR UPDATE"
	}
	args := append(resourceKey(row.plan.Source.Resource), row.plan.ID, kind)
	var proof []byte
	var room, event *string
	err := tx.QueryRow(ctx, q, args...).Scan(&proof, &result.Candidate.EncryptedEvent, &result.Candidate.SessionCertificate, &result.SHA256, &result.State, &room, &event)
	if err != nil {
		return result, err
	}
	if json.Unmarshal(proof, &result.Candidate.Proof) != nil {
		return result, resource.ErrInvalid
	}
	hash, err := resource.BootstrapCandidateSHA256(result.Candidate)
	if err != nil || hash != result.SHA256 || result.Candidate.Proof.PlanSHA256 != row.hash || result.Candidate.Proof.Kind != kind {
		return result, resource.ErrInvalid
	}
	if result.State == "accepted" {
		if room == nil || event == nil {
			return result, resource.ErrInvalid
		}
		ref := storage.Reference{RoomID: *room, EventID: *event}
		if !ref.Valid() || ref.RoomID != row.plan.RoomID {
			return result, resource.ErrInvalid
		}
		result.Reference = &ref
	}
	return result, nil
}

func (r *Relay) PendingBootstrap(ctx context.Context, p resource.VerifiedBootstrapPlan, kind resource.BootstrapKind) (BootstrapSlot, error) {
	var result BootstrapSlot
	if err := r.checkBootstrap(ctx, p); err != nil {
		return result, err
	}
	err := r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.lockedBootstrap(ctx, tx, p)
		if err != nil {
			return err
		}
		result, err = r.bootstrapSlot(ctx, tx, row, kind, false)
		return err
	})
	if err != nil {
		return BootstrapSlot{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapSlot{}, err
	}
	return result, nil
}

// StageBootstrap accepts the shared verifier token and exact original bytes.
// The SQL slot precedes admission/transport. A retry cannot pick a new SDK txn.
func (r *Relay) StageBootstrap(ctx context.Context, p resource.VerifiedBootstrapPlan, c resource.BootstrapCandidate, verified resource.VerifiedBootstrapCandidate) (BootstrapSlot, error) {
	var result BootstrapSlot
	if err := r.checkBootstrap(ctx, p); err != nil {
		return result, err
	}
	hash, err := resource.BootstrapCandidateSHA256(c)
	if err != nil || hash != verified.SHA256() || c.Proof.BootstrapPublication != verified.Publication() || c.Proof.PlanSHA256 != p.SHA256() {
		return result, resource.ErrInvalid
	}
	raw, err := json.Marshal(c.Proof)
	if err != nil {
		return result, err
	}
	ctxBytes, err := resource.BootstrapContextBytes(verified.Context())
	if err != nil {
		return result, err
	}
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.lockedBootstrap(ctx, tx, p)
		if err != nil {
			return err
		}
		if row.state != "preparing" {
			return resource.ErrRevoked
		}
		existing, err := r.bootstrapSlot(ctx, tx, row, c.Proof.Kind, true)
		if err == nil {
			if existing.SHA256 != hash {
				return resource.ErrConflict
			}
			result = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		k := row.plan.Source.Resource
		_, err = tx.Exec(ctx, `INSERT INTO resource_e2e_bootstrap_slots(tenant,product,kind,resource_id,plan_id,bootstrap_kind,attempt_id,candidate_sha256,context_sha256,context,outer_cipher_sha256,signed_publication,session_certificate,encrypted_event,state)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'prepared')`, k.Tenant, k.Product, k.Kind, k.ID, row.plan.ID, c.Proof.Kind, c.Proof.TransactionID, hash, c.Proof.ContextSHA256, ctxBytes, c.Proof.OuterCipherSHA256, raw, c.SessionCertificate, c.EncryptedEvent)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO resource_e2e_public_evidence(tenant,product,sha256,value)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, k.Tenant, k.Product, c.Proof.ContextSHA256, ctxBytes); err != nil {
			return err
		}
		result, err = r.bootstrapSlot(ctx, tx, row, c.Proof.Kind, false)
		return err
	})
	if err != nil {
		return BootstrapSlot{}, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapSlot{}, err
	}
	return result, nil
}

// AcquireBootstrap claims Prepared inside the row lock. A second dispatcher
// always gets fresh=false for Sending/Ambiguous/Accepted, including lost replies.
func (r *Relay) AcquireBootstrap(ctx context.Context, p resource.VerifiedBootstrapPlan, kind resource.BootstrapKind) (BootstrapSlot, bool, error) {
	var result BootstrapSlot
	fresh := false
	if err := r.checkBootstrap(ctx, p); err != nil {
		return result, false, err
	}
	err := r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.lockedBootstrap(ctx, tx, p)
		if err != nil {
			return err
		}
		result, err = r.bootstrapSlot(ctx, tx, row, kind, true)
		if err != nil {
			return err
		}
		if result.State != "prepared" {
			return nil
		}
		if row.state != "preparing" {
			return resource.ErrRevoked
		}
		_, err = tx.Exec(ctx, `UPDATE resource_e2e_bootstrap_slots SET state='sending' WHERE(tenant,product,kind,resource_id,plan_id,bootstrap_kind)=($1,$2,$3,$4,$5,$6)AND state='prepared'`, row.plan.Source.Resource.Tenant, row.plan.Source.Resource.Product, row.plan.Source.Resource.Kind, row.plan.Source.Resource.ID, row.plan.ID, kind)
		if err != nil {
			return err
		}
		result.State = "sending"
		fresh = true
		return nil
	})
	if err != nil {
		return BootstrapSlot{}, false, err
	}
	if err := p.Revalidate(ctx); err != nil {
		return BootstrapSlot{}, false, err
	}
	return result, fresh, nil
}

// MarkBootstrapAmbiguous retains an existing hold after an uncertain network
// result. It neither releases it nor requires the original SDK producer online.
func (r *Relay) MarkBootstrapAmbiguous(ctx context.Context, key resource.Resource, planID, hash string, kind resource.BootstrapKind) error {
	if err := r.scoped(ctx, key); err != nil {
		return err
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.readBootstrap(ctx, tx, key, true)
		if err != nil {
			return err
		}
		if row.plan.ID != planID || row.hash != hash {
			return resource.ErrConflict
		}
		slot, err := r.bootstrapSlot(ctx, tx, row, kind, true)
		if err != nil {
			return err
		}
		if slot.State == "accepted" || slot.State == "ambiguous" {
			return nil
		}
		if slot.State != "sending" {
			return resource.ErrPending
		}
		_, err = tx.Exec(ctx, `UPDATE resource_e2e_bootstrap_slots SET state='ambiguous' WHERE(tenant,product,kind,resource_id,plan_id,bootstrap_kind)=($1,$2,$3,$4,$5,$6)AND state='sending'`, key.Tenant, key.Product, key.Kind, key.ID, planID, kind)
		return err
	})
}

// CommitBootstrap is ACK-only for the exact retained slot. The caller supplies
// the shared SDK verified acceptance, not event JSON or an active producer flag.
func (r *Relay) CommitBootstrap(ctx context.Context, key resource.Resource, planID string, ack resource.VerifiedBootstrapAcceptance) error {
	if err := r.scoped(ctx, key); err != nil {
		return err
	}
	if r.dependencies.CheckBootstrapSource == nil {
		return resource.ErrRevoked
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.retainedBootstrapLocked(ctx, tx, key)
		if err != nil {
			return err
		}
		if row.plan.ID != planID || ack.PlanSHA256() != row.hash {
			return resource.ErrConflict
		}
		slot, err := r.bootstrapSlot(ctx, tx, row, ack.Kind(), true)
		if err != nil {
			return err
		}
		if slot.SHA256 != ack.CandidateSHA256() || slot.Candidate.Proof.OuterCipherSHA256 != ack.CipherSHA256() || ack.Reference().RoomID != row.plan.RoomID || !ack.Reference().Valid() {
			return resource.ErrInvalid
		}
		if slot.State == "accepted" {
			if slot.Reference == nil || *slot.Reference != ack.Reference() {
				return resource.ErrFork
			}
			return nil
		}
		if slot.State != "sending" && slot.State != "ambiguous" {
			return resource.ErrPending
		}
		_, err = tx.Exec(ctx, `UPDATE resource_e2e_bootstrap_slots SET state='accepted',room_id=$7,event_id=$8,accepted_at=clock_timestamp() WHERE(tenant,product,kind,resource_id,plan_id,bootstrap_kind)=($1,$2,$3,$4,$5,$6)AND state IN('sending','ambiguous')`, key.Tenant, key.Product, key.Kind, key.ID, planID, ack.Kind(), ack.Reference().RoomID, ack.Reference().EventID)
		return err
	})
}

// FinalizeBootstrap makes the final policy and both exact refs visible in ONE
// transaction. It does not retire or delete any original application source.
func (r *Relay) FinalizeBootstrap(ctx context.Context, activation resource.VerifiedBootstrapActivation) (BootstrapState, error) {
	var result BootstrapState
	signed, hash, err := r.validateBootstrapActivation(ctx, activation)
	if err != nil {
		return result, err
	}
	plan := activation.Plan()
	key := plan.Source.Resource
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = r.finalizeBootstrapTx(ctx, tx, key, plan, activation.PlanSHA256(), signed, hash)
		return err
	})
	if err != nil {
		return BootstrapState{}, err
	}
	if err := activation.Revalidate(ctx); err != nil {
		return BootstrapState{}, err
	}
	return result, nil
}

func (r *Relay) validateBootstrapActivation(ctx context.Context, activation resource.VerifiedBootstrapActivation) (resource.SignedEpoch, string, error) {
	plan := activation.Plan()
	key := plan.Source.Resource
	if r.dependencies.CheckBootstrapSource == nil || plan.CreatorScope.Subject != r.subject || (r.dependencies.RequirePrivateRules && !plan.Validation.PrivateRules) {
		return resource.SignedEpoch{}, "", resource.ErrRevoked
	}
	if err := r.scoped(ctx, key); err != nil {
		return resource.SignedEpoch{}, "", err
	}
	if err := activation.RequireCurrentRosterCapture(ctx); err != nil {
		return resource.SignedEpoch{}, "", err
	}
	var signed resource.SignedEpoch
	if json.Unmarshal(activation.PublicEpochBytes(), &signed) != nil || signed.Epoch != activation.Epoch().Value() {
		return resource.SignedEpoch{}, "", resource.ErrInvalid
	}
	hash, err := resource.EpochDigest(signed.Epoch)
	if err != nil || hash != activation.Epoch().SHA256() {
		return resource.SignedEpoch{}, "", resource.ErrInvalid
	}
	return signed, hash, nil
}

func (r *Relay) finalizeBootstrapTx(ctx context.Context, tx pgx.Tx, key resource.Resource, plan resource.BootstrapPlan, planSHA string, signed resource.SignedEpoch, hash string) (BootstrapState, error) {
	var result BootstrapState
	row, err := r.retainedBootstrapLocked(ctx, tx, key)
	if err != nil {
		return result, err
	}
	if row.plan != plan || row.hash != planSHA {
		return result, resource.ErrConflict
	}
	if row.state == "active" {
		head, err := r.readHead(ctx, tx, key, false)
		if err != nil {
			return result, err
		}
		if head.policy != hash {
			return result, resource.ErrConflict
		}
		return BootstrapState{PlanID: plan.ID, PlanSHA256: row.hash, State: "active"}, nil
	}
	if err := r.verifyAcceptedBootstrapSlots(ctx, tx, row, signed); err != nil {
		return result, err
	}
	head, err := r.insertInitialResource(ctx, tx, signed, hash)
	if err != nil {
		return result, err
	}
	if head.policy != hash || head.membership != signed.MembershipSHA256 || head.epochID != signed.ID || head.generation != 1 || head.position != signed.Cutover || head.baseline != signed.Baseline.Position || head.state != "active" {
		return result, resource.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE resource_e2e_bootstraps SET state='active',activated_at=clock_timestamp() WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4)AND plan_id=$5 AND state='preparing'`, key.Tenant, key.Product, key.Kind, key.ID, plan.ID)
	if err != nil {
		return result, err
	}
	return BootstrapState{PlanID: plan.ID, PlanSHA256: row.hash, State: "active"}, nil
}

func (r *Relay) verifyAcceptedBootstrapSlots(ctx context.Context, tx pgx.Tx, row bootstrapRow, signed resource.SignedEpoch) error {
	for _, kind := range []resource.BootstrapKind{resource.BootstrapBaseline, resource.BootstrapCheckpoint} {
		slot, err := r.bootstrapSlot(ctx, tx, row, kind, true)
		if err != nil {
			return err
		}
		expected := signed.Baseline.Reference
		cipher := signed.Baseline.Position.HeadCipherSHA256
		contextHash := signed.Baseline.ContextSHA256
		certificate := signed.Baseline.SessionCertificateID
		if kind == resource.BootstrapCheckpoint {
			expected = signed.Checkpoint.Reference
			cipher = signed.Checkpoint.CipherSHA256
			contextHash = signed.Checkpoint.ContextSHA256
			certificate = signed.Checkpoint.SessionCertificateID
		}
		if slot.State != "accepted" || slot.Reference == nil || *slot.Reference != expected || slot.Candidate.Proof.OuterCipherSHA256 != cipher || slot.Candidate.Proof.ContextSHA256 != contextHash || slot.Candidate.Proof.SessionCertificateID != certificate {
			return resource.ErrPending
		}
	}
	return nil
}

// Lock ordering is ORIGINAL source first, then phase/slots. A read-only hint
// supplies no authority: its full immutable plan is compared again under lock.
func (r *Relay) retainedBootstrapLocked(ctx context.Context, tx pgx.Tx, key resource.Resource) (bootstrapRow, error) {
	hint, err := r.readBootstrap(ctx, tx, key, false)
	if err != nil {
		return hint, err
	}
	if err := r.dependencies.CheckBootstrapSource(ctx, tx, hint.plan.Source); err != nil {
		return bootstrapRow{}, err
	}
	current, err := r.readBootstrap(ctx, tx, key, true)
	if err != nil {
		return current, err
	}
	if current.plan != hint.plan || current.hash != hint.hash {
		return current, resource.ErrConflict
	}
	return current, nil
}
