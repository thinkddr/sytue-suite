// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Verifier delegates to the ONE SDK/resource authority. The broker
// verifies independent pinned roots, current account/device authority and the
// authenticated subject. SQL metadata or Valid() never constructs these values.
// Network/SDK verification runs OUTSIDE SQL transactions, then the complete
// generation/policy/CAS snapshot is checked again under its durable row lock.
// No callback receives plaintext, epoch private identities or user access tokens.
type Verifier interface {
	Access(context.Context, resource.Resource, resource.Action, json.RawMessage) (resource.VerifiedAccess, error)
	Producer(context.Context, resource.Candidate, resource.VerifiedAccess) (resource.VerifiedPendingPublication, error)
	Complete(context.Context, resource.Candidate, resource.VerifiedPendingPublication) (resource.VerifiedPublication, error)
	VerifyAcceptance(context.Context, resource.Candidate, resource.VerifiedPublication, storage.Reference) (resource.VerifiedAcceptance, error)
	VerifyReservedAcceptance(context.Context, resource.VerifiedReservation, storage.Reference) (resource.VerifiedAcceptance, error)
	VerifyNotAccepted(context.Context, resource.JournalEntry, json.RawMessage) error
	InitialEpoch(context.Context, resource.SignedEpoch) (resource.VerifiedEpoch, error)
	NextEpoch(context.Context, resource.SignedEpoch, resource.VerifiedAccess, resource.Position) (resource.VerifiedEpoch, error)
}

// Relay is bound to one authenticated subject, never the owner vault.
// It is deliberately not installed by a Main until the shared broker and every
// original Office feature use the encrypted client repository.
type Relay struct {
	dependencies Dependencies
	verifier     Verifier
	subject      string
}

var _ resource.RelayStore = (*Relay)(nil)

// Dependencies are fixed by the product composition root. A request can choose
// neither table names, transaction implementation nor a resource validator.
// Notify participates in the same receipt/head transaction, preserving outbox.
type Dependencies struct {
	TenantTx            func(context.Context, func(pgx.Tx) error) error
	ValidateResource    func(context.Context, resource.Resource) error
	Notify              func(context.Context, pgx.Tx, resource.Receipt) error
	RequirePrivateRules bool
	// CheckBootstrapSource reads and locks the ORIGINAL frozen source/ACL in this
	// same transaction. No HTTP or caller-selected storage is permitted here.
	CheckBootstrapSource func(context.Context, pgx.Tx, resource.BootstrapSourceSnapshot) error
}

func New(dependencies Dependencies, verifier Verifier, subject string) (*Relay, error) {
	if dependencies.TenantTx == nil || dependencies.ValidateResource == nil || dependencies.Notify == nil || verifier == nil || subject == "" {
		return nil, resource.ErrInvalid
	}
	return &Relay{dependencies: dependencies, verifier: verifier, subject: subject}, nil
}

type resourceHead struct {
	epoch                                     json.RawMessage
	policy, membership, epochID, rules, state string
	generation                                int64
	privateRules                              bool
	position, baseline                        resource.Position
	hold                                      *string
}

func (r *Relay) scoped(ctx context.Context, value resource.Resource) error {
	tenant, e := tenancy.From(ctx)
	if e != nil {
		return e
	}
	if !value.Valid() || tenant.Slug != value.Tenant {
		return resource.ErrInvalid
	}
	return r.dependencies.ValidateResource(ctx, value)
}

func resourceKey(v resource.Resource) []any { return []any{v.Tenant, v.Product, v.Kind, v.ID} }
func (r *Relay) readHead(ctx context.Context, tx pgx.Tx, key resource.Resource, lock bool) (resourceHead, error) {
	q := `SELECT e.signed_policy,r.policy_sha256,r.membership_sha256,r.epoch,r.generation,r.private_rules,r.rules_version,r.seq,r.head_cipher_sha256,r.baseline_seq,r.baseline_head_cipher_sha256,r.state,r.hold_op_id FROM resource_e2e_resources r JOIN resource_e2e_epochs e USING(tenant,product,kind,resource_id,epoch) WHERE (r.tenant,r.product,r.kind,r.resource_id)=($1,$2,$3,$4)`
	if lock {
		q += " FOR UPDATE OF r"
	}
	var h resourceHead
	var seq, baseline int64
	e := tx.QueryRow(ctx, q, resourceKey(key)...).Scan(&h.epoch, &h.policy, &h.membership, &h.epochID, &h.generation, &h.privateRules, &h.rules, &seq, &h.position.HeadCipherSHA256, &baseline, &h.baseline.HeadCipherSHA256, &h.state, &h.hold)
	if errors.Is(e, pgx.ErrNoRows) {
		return h, resource.ErrRevoked
	}
	if e != nil {
		return h, e
	}
	h.position.Seq, _ = resource.Sequence(seq)
	h.baseline.Seq, _ = resource.Sequence(baseline)
	return h, nil
}

func (h resourceHead) matches(access resource.VerifiedAccess) bool {
	e := access.Epoch().Value()
	g, err := e.Generation.Int64()
	return err == nil && e.Valid() && h.policy == access.Epoch().SHA256() && h.membership == e.MembershipSHA256 && h.epochID == e.ID && h.generation == g && h.privateRules == e.Validation.PrivateRules && h.rules == e.Validation.RulesVersion
}

func (r *Relay) access(ctx context.Context, key resource.Resource, action resource.Action) (resourceHead, resource.VerifiedAccess, error) {
	var h resourceHead
	if err := r.scoped(ctx, key); err != nil {
		return h, resource.VerifiedAccess{}, err
	}
	err := r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error { var e error; h, e = r.readHead(ctx, tx, key, false); return e })
	if err != nil {
		return h, resource.VerifiedAccess{}, err
	}
	if h.state == "revoked" {
		return h, resource.VerifiedAccess{}, resource.ErrRevoked
	}
	a, err := r.verifier.Access(ctx, key, action, h.epoch)
	if err != nil {
		return h, a, err
	}
	a, err = resource.Reauthorize(ctx, a, r.subject, action)
	if err != nil {
		return h, a, err
	}
	if !h.matches(a) || a.Epoch().Value().Resource != key {
		return h, a, resource.ErrRevoked
	}
	return h, a, nil
}

func (r *Relay) locked(ctx context.Context, tx pgx.Tx, key resource.Resource, access resource.VerifiedAccess) (resourceHead, error) {
	h, err := r.readHead(ctx, tx, key, true)
	if err != nil {
		return h, err
	}
	if h.state == "revoked" || !h.matches(access) {
		return h, resource.ErrRevoked
	}
	return h, nil
}

func (r *Relay) load(ctx context.Context, tx pgx.Tx, key resource.Resource, op string, lock bool) (resource.JournalEntry, error) {
	q := `SELECT signed_publication,encrypted_event,session_certificate,validation_certificate,state,attempt_id::text FROM resource_e2e_candidates WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`
	if lock {
		q += " FOR UPDATE"
	}
	args := append(resourceKey(key), op)
	var out resource.JournalEntry
	var proof, certificate []byte
	if err := tx.QueryRow(ctx, q, args...).Scan(&proof, &out.Candidate.EncryptedEvent, &out.Candidate.SessionCertificate, &certificate, &out.State, &out.AttemptID); err != nil {
		return out, err
	}
	if err := json.Unmarshal(proof, &out.Candidate.Proof); err != nil {
		return out, err
	}
	if len(certificate) > 0 {
		if err := json.Unmarshal(certificate, &out.Candidate.Validation); err != nil {
			return out, err
		}
	}
	if out.State == resource.Committed {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT receipt FROM resource_e2e_receipts WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, args...).Scan(&raw); err != nil {
			return out, err
		}
		if err := json.Unmarshal(raw, &out.Receipt); err != nil || out.Receipt == nil || !out.Receipt.Valid() {
			return out, resource.ErrInvalid
		}
	}
	return out, nil
}

func pendingMatches(c resource.Candidate, p resource.VerifiedPendingPublication) bool {
	hash, err := resource.CandidateDigest(c)
	return err == nil && hash == p.CandidateSHA256() && p.Value() == c.Proof.CipherPublication && p.SHA256() != ""
}

func fullMatches(c resource.Candidate, p resource.VerifiedPublication) bool {
	hash, err := resource.CandidateDigest(c)
	return err == nil && hash == p.CandidateSHA256() && p.Value() == c.Proof.CipherPublication && p.SHA256() != ""
}

func (r *Relay) Stage(ctx context.Context, candidate resource.Candidate) (resource.JournalEntry, error) {
	key := candidate.Proof.Resource
	_, access, err := r.access(ctx, key, candidate.Proof.Action)
	if err != nil {
		return resource.JournalEntry{}, err
	}
	pending, state, proof, validation, err := r.prepareCandidateStage(ctx, &candidate, access)
	if err != nil {
		return resource.JournalEntry{}, err
	}
	var out resource.JournalEntry
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = r.stageCandidateTx(ctx, tx, key, candidate, access, pending, state, proof, validation)
		return e
	})
	return out, err
}

func (r *Relay) prepareCandidateStage(ctx context.Context, candidate *resource.Candidate, access resource.VerifiedAccess) (resource.VerifiedPendingPublication, resource.CandidateState, []byte, []byte, error) {
	pending, err := r.verifier.Producer(ctx, *candidate, access)
	if err != nil || !pendingMatches(*candidate, pending) {
		if err == nil {
			err = resource.ErrInvalid
		}
		return resource.VerifiedPendingPublication{}, "", nil, nil, err
	}
	state := resource.Prepared
	full, err := r.verifier.Complete(ctx, *candidate, pending)
	switch {
	case errors.Is(err, resource.ErrValidatorUnavailable):
		state = resource.WaitingValidation
	case err != nil:
		return resource.VerifiedPendingPublication{}, "", nil, nil, err
	case !fullMatches(*candidate, full):
		return resource.VerifiedPendingPublication{}, "", nil, nil, resource.ErrInvalid
	}
	proof, err := json.Marshal(candidate.Proof)
	if err != nil {
		return resource.VerifiedPendingPublication{}, "", nil, nil, err
	}
	var validation []byte
	if state == resource.WaitingValidation {
		candidate.Validation = nil
	}
	if candidate.Validation != nil {
		validation, err = json.Marshal(candidate.Validation)
		if err != nil {
			return resource.VerifiedPendingPublication{}, "", nil, nil, err
		}
	}
	return pending, state, proof, validation, nil
}

func (r *Relay) stageCandidateTx(ctx context.Context, tx pgx.Tx, key resource.Resource, candidate resource.Candidate, access resource.VerifiedAccess, pending resource.VerifiedPendingPublication, state resource.CandidateState, proof, validation []byte) (resource.JournalEntry, error) {
	h, e := r.locked(ctx, tx, key, access)
	if e != nil {
		return resource.JournalEntry{}, e
	}
	if h.state != "active" {
		return resource.JournalEntry{}, resource.ErrRevoked
	}
	existing, e := r.load(ctx, tx, key, candidate.Proof.OpID, true)
	if e == nil {
		digest, e := resource.CandidateDigest(existing.Candidate)
		if e != nil || digest != pending.CandidateSHA256() {
			return resource.JournalEntry{}, resource.ErrConflict
		}
		if existing.State == resource.WaitingValidation && state == resource.Prepared {
			_, e = tx.Exec(ctx, `UPDATE resource_e2e_candidates SET validation_certificate=$6,state='prepared',updated_at=now() WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, candidate.Proof.OpID, validation)
			if e != nil {
				return resource.JournalEntry{}, e
			}
			existing.Candidate.Validation = candidate.Validation
			existing.State = resource.Prepared
		}
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return resource.JournalEntry{}, e
	}
	if h.position != candidate.Proof.ExpectedBase {
		state = resource.Stale
	}
	generation, _ := candidate.Proof.Generation.Int64()
	base, _ := candidate.Proof.ExpectedBase.Seq.Int64()
	count, _ := candidate.Proof.OpCount.Int64()
	_, e = tx.Exec(ctx, `INSERT INTO resource_e2e_candidates(tenant,product,kind,resource_id,op_id,epoch,generation,publication_sha256,candidate_sha256,signed_publication,session_certificate,encrypted_event,validation_certificate,base_seq,base_head_cipher_sha256,op_count,outer_cipher_sha256,state,attempt_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, key.Tenant, key.Product, key.Kind, key.ID, candidate.Proof.OpID, candidate.Proof.Epoch, generation, pending.SHA256(), pending.CandidateSHA256(), proof, candidate.SessionCertificate, candidate.EncryptedEvent, validation, base, candidate.Proof.ExpectedBase.HeadCipherSHA256, count, candidate.Proof.OuterCipherSHA256, state, candidate.Proof.TransactionID)
	if e != nil {
		return resource.JournalEntry{}, e
	}
	return r.load(ctx, tx, key, candidate.Proof.OpID, false)
}

func (r *Relay) Pending(ctx context.Context, key resource.Resource, op string) (resource.JournalEntry, error) {
	_, a, err := r.access(ctx, key, resource.Read)
	if err != nil {
		return resource.JournalEntry{}, err
	}
	var out resource.JournalEntry
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := r.locked(ctx, tx, key, a); e != nil {
			return e
		}
		var e error
		out, e = r.load(ctx, tx, key, op, false)
		return e
	})
	return out, err
}

// AcquireSending persists the producer attempt before the first network call.
// Returning an existing Sending/Ambiguous entry does not authorize an indefinite
// blind resend: the relay must reconcile using its maintained SDK/AS evidence.
func (r *Relay) AcquireSending(ctx context.Context, key resource.Resource, op string) (resource.JournalEntry, error) {
	entry, _, _, err := r.acquireSending(ctx, key, op)
	return entry, err
}

func (r *Relay) acquireSending(ctx context.Context, key resource.Resource, op string) (resource.JournalEntry, resource.VerifiedPublication, bool, error) {
	entry, err := r.Pending(ctx, key, op)
	if err != nil {
		return entry, resource.VerifiedPublication{}, false, err
	}
	_, a, err := r.access(ctx, key, entry.Candidate.Proof.Action)
	if err != nil {
		return entry, resource.VerifiedPublication{}, false, err
	}
	pending, err := r.verifier.Producer(ctx, entry.Candidate, a)
	if err != nil {
		return entry, resource.VerifiedPublication{}, false, err
	}
	full, err := r.verifier.Complete(ctx, entry.Candidate, pending)
	if err != nil {
		return entry, resource.VerifiedPublication{}, false, err
	}
	if !fullMatches(entry.Candidate, full) {
		return entry, resource.VerifiedPublication{}, false, resource.ErrInvalid
	}
	captured, err := resource.CaptureReservation(ctx, entry.Candidate, full, entry.AttemptID)
	if err != nil {
		return entry, resource.VerifiedPublication{}, false, err
	}
	fresh := false
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		entry, fresh, e = r.acquireSendingTx(ctx, tx, key, op, a, full, captured)
		return e
	})
	return entry, full, fresh && err == nil, err
}

func (r *Relay) acquireSendingTx(ctx context.Context, tx pgx.Tx, key resource.Resource, op string, a resource.VerifiedAccess, full resource.VerifiedPublication, captured resource.CapturedReservation) (resource.JournalEntry, bool, error) {
	h, e := r.locked(ctx, tx, key, a)
	if e != nil {
		return resource.JournalEntry{}, false, e
	}
	entry, e := r.load(ctx, tx, key, op, true)
	if e != nil {
		return resource.JournalEntry{}, false, e
	}
	if entry.State == resource.Committed {
		return entry, false, nil
	}
	if entry.State == resource.Sending || entry.State == resource.Ambiguous {
		if h.hold == nil || *h.hold != op {
			return entry, false, resource.ErrFork
		}
		return entry, false, nil
	}
	if h.state != "active" {
		return entry, false, resource.ErrRevoked
	}
	if h.hold != nil || entry.State != resource.Prepared {
		return entry, false, resource.ErrPending
	}
	if h.position != entry.Candidate.Proof.ExpectedBase || !fullMatches(entry.Candidate, full) || captured.Value().AttemptID != entry.AttemptID {
		return entry, false, resource.ErrConflict
	}
	if err := storeResourceEvidence(ctx, tx, key, captured); err != nil {
		return entry, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='sending',reservation=$6,reservation_sha256=$7,updated_at=now() WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, op, captured.Bytes(), captured.SHA256()); err != nil {
		return entry, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_e2e_resources SET hold_op_id=$5,updated_at=now() WHERE (tenant,product,kind,resource_id)=($1,$2,$3,$4)`, key.Tenant, key.Product, key.Kind, key.ID, op); err != nil {
		return entry, false, err
	}
	entry.State = resource.Sending
	return entry, true, nil
}

func (r *Relay) producerOrManager(ctx context.Context, key resource.Resource, candidate resource.Candidate, access resource.VerifiedAccess) (resource.VerifiedAccess, error) {
	if access.Capability().Value().UserID == candidate.Proof.Actor.UserID {
		return access, nil
	}
	_, manager, err := r.access(ctx, key, resource.Manage)
	return manager, err
}

func (r *Relay) MarkAmbiguous(ctx context.Context, entry resource.JournalEntry) error {
	key := entry.Candidate.Proof.Resource
	op := entry.Candidate.Proof.OpID
	snapshot, err := r.Pending(ctx, key, op)
	if err != nil {
		return err
	}
	expected, e := resource.CandidateDigest(entry.Candidate)
	actual, e2 := resource.CandidateDigest(snapshot.Candidate)
	if e != nil || e2 != nil || expected != actual || entry.AttemptID != snapshot.AttemptID {
		return resource.ErrConflict
	}
	_, a, err := r.access(ctx, key, snapshot.Candidate.Proof.Action)
	if err != nil {
		return err
	}
	a, err = r.producerOrManager(ctx, key, snapshot.Candidate, a)
	if err != nil {
		return err
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		h, e := r.locked(ctx, tx, key, a)
		if e != nil {
			return e
		}
		current, e := r.load(ctx, tx, key, op, true)
		if e != nil {
			return e
		}
		if current.AttemptID != entry.AttemptID {
			return resource.ErrConflict
		}
		if current.State == resource.Committed {
			return nil
		}
		if h.hold == nil || *h.hold != op {
			return resource.ErrFork
		}
		if current.State == resource.Ambiguous {
			return nil
		}
		if current.State != resource.Sending {
			return resource.ErrPending
		}
		_, e = tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='ambiguous',updated_at=now() WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, op)
		return e
	})
}

func (r *Relay) Commit(ctx context.Context, entry resource.JournalEntry, reference storage.Reference) (resource.Receipt, error) {
	key := entry.Candidate.Proof.Resource
	op := entry.Candidate.Proof.OpID
	if !reference.Valid() || reference.RoomID != entry.Candidate.Proof.RoomID {
		return resource.Receipt{}, resource.ErrInvalid
	}
	current, done, err := r.prepareCommitCandidate(ctx, key, op, entry, reference)
	if err != nil || done {
		if done && current.Receipt != nil {
			return *current.Receipt, nil
		}
		return resource.Receipt{}, err
	}
	_, a, full, err := r.verifyCommitAcceptance(ctx, key, current, reference)
	if err != nil {
		return resource.Receipt{}, err
	}
	end, err := current.Candidate.Proof.End()
	if err != nil {
		return resource.Receipt{}, err
	}
	receipt := resource.Receipt{Resource: key, Epoch: current.Candidate.Proof.Epoch, OpID: op, PublicationSHA256: full.SHA256(), Reference: reference, Base: current.Candidate.Proof.ExpectedBase, Head: resource.Position{Seq: end, HeadCipherSHA256: current.Candidate.Proof.OuterCipherSHA256}}
	return r.commitReceipt(ctx, a, current, receipt, "")
}

func (r *Relay) prepareCommitCandidate(ctx context.Context, key resource.Resource, op string, entry resource.JournalEntry, reference storage.Reference) (resource.JournalEntry, bool, error) {
	current, err := r.Pending(ctx, key, op)
	if err != nil {
		return resource.JournalEntry{}, false, err
	}
	expected, e := resource.CandidateDigest(entry.Candidate)
	actual, e2 := resource.CandidateDigest(current.Candidate)
	if e != nil || e2 != nil || expected != actual || entry.AttemptID != current.AttemptID {
		return resource.JournalEntry{}, false, resource.ErrConflict
	}
	if current.Receipt != nil {
		if current.Receipt.Reference != reference {
			return resource.JournalEntry{}, false, resource.ErrFork
		}
		return current, true, nil
	}
	if current.State != resource.Sending && current.State != resource.Ambiguous {
		return resource.JournalEntry{}, false, resource.ErrPending
	}
	return current, false, nil
}

func (r *Relay) verifyCommitAcceptance(ctx context.Context, key resource.Resource, current resource.JournalEntry, reference storage.Reference) (resourceHead, resource.VerifiedAccess, resource.VerifiedPublication, error) {
	head, a, err := r.access(ctx, key, current.Candidate.Proof.Action)
	if err != nil {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, err
	}
	pending, err := r.verifier.Producer(ctx, current.Candidate, a)
	if err != nil {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, err
	}
	full, err := r.verifier.Complete(ctx, current.Candidate, pending)
	if err != nil {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, err
	}
	if !fullMatches(current.Candidate, full) {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, resource.ErrInvalid
	}
	accepted, err := r.verifier.VerifyAcceptance(ctx, current.Candidate, full, reference)
	if err != nil {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, err
	}
	if accepted.Reference() != reference || accepted.Resource() != key || accepted.PublicationSHA256() != full.SHA256() || accepted.CipherSHA256() != current.Candidate.Proof.OuterCipherSHA256 {
		return resourceHead{}, resource.VerifiedAccess{}, resource.VerifiedPublication{}, resource.ErrInvalid
	}
	return head, a, full, nil
}

func (r *Relay) commitReceipt(ctx context.Context, a resource.VerifiedAccess, entry resource.JournalEntry, receipt resource.Receipt, reservationSHA string) (resource.Receipt, error) {
	key := receipt.Resource
	op := receipt.OpID
	reference := receipt.Reference
	raw, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		h, e := r.locked(ctx, tx, key, a)
		if e != nil {
			return e
		}
		saved, e := r.load(ctx, tx, key, op, true)
		if e != nil {
			return e
		}
		if saved.Receipt != nil {
			if saved.Receipt.Reference != reference {
				return resource.ErrFork
			}
			receipt = *saved.Receipt
			return nil
		}
		if reservationSHA != "" {
			var capturedSHA string
			if err := tx.QueryRow(ctx, `SELECT reservation_sha256 FROM resource_e2e_candidates WHERE(tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, op).Scan(&capturedSHA); err != nil {
				return err
			}
			if capturedSHA != reservationSHA {
				return resource.ErrConflict
			}
		}
		if saved.AttemptID != entry.AttemptID || h.hold == nil || *h.hold != op || h.position != receipt.Base {
			return resource.ErrConflict
		}
		base, _ := receipt.Base.Seq.Int64()
		seq, _ := receipt.Head.Seq.Int64()
		if _, e = tx.Exec(ctx, `INSERT INTO resource_e2e_receipts(tenant,product,kind,resource_id,op_id,publication_sha256,epoch,base_seq,seq,base_head_cipher_sha256,head_cipher_sha256,room_id,event_id,receipt)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, key.Tenant, key.Product, key.Kind, key.ID, op, receipt.PublicationSHA256, receipt.Epoch, base, seq, receipt.Base.HeadCipherSHA256, receipt.Head.HeadCipherSHA256, reference.RoomID, reference.EventID, raw); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='committed',updated_at=now() WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, op); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE resource_e2e_resources SET seq=$5,head_cipher_sha256=$6,hold_op_id=NULL,updated_at=now() WHERE (tenant,product,kind,resource_id)=($1,$2,$3,$4)`, key.Tenant, key.Product, key.Kind, key.ID, seq, receipt.Head.HeadCipherSHA256); e != nil {
			return e
		}
		return r.dependencies.Notify(ctx, tx, receipt)
	})
	return receipt, err
}

func (r *Relay) Page(ctx context.Context, key resource.Resource, cursor resource.Cursor, limit int) (resource.Page, error) {
	_, a, err := r.access(ctx, key, resource.Read)
	if err != nil {
		return resource.Page{}, err
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var out resource.Page
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		position, seq, e := r.validatePageCursor(ctx, tx, key, a, cursor)
		if e != nil {
			return e
		}
		out, e = readPageRows(ctx, tx, key, position, seq, limit)
		return e
	})
	return out, err
}

func (r *Relay) validatePageCursor(ctx context.Context, tx pgx.Tx, key resource.Resource, a resource.VerifiedAccess, cursor resource.Cursor) (resource.Position, int64, error) {
	h, e := r.locked(ctx, tx, key, a)
	if e != nil {
		return resource.Position{}, 0, e
	}
	position := cursor.Position
	if position.Seq == "" {
		position = a.Capability().Value().From
	}
	if !position.Valid() {
		return resource.Position{}, 0, resource.ErrInvalid
	}
	seq, _ := position.Seq.Int64()
	from, _ := a.Capability().Value().From.Seq.Int64()
	if seq < from {
		return resource.Position{}, 0, resource.ErrRevoked
	}
	if position != h.baseline {
		var head string
		e = tx.QueryRow(ctx, `SELECT head_cipher_sha256 FROM resource_e2e_receipts WHERE (tenant,product,kind,resource_id,seq)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, seq).Scan(&head)
		if e != nil || head != position.HeadCipherSHA256 {
			return resource.Position{}, 0, resource.ErrFork
		}
	}
	return position, seq, nil
}

func readPageRows(ctx context.Context, tx pgx.Tx, key resource.Resource, position resource.Position, seq int64, limit int) (resource.Page, error) {
	var out resource.Page
	rows, e := tx.Query(ctx, `SELECT receipt FROM resource_e2e_receipts WHERE (tenant,product,kind,resource_id)=($1,$2,$3,$4) AND seq>$5 ORDER BY seq LIMIT $6`, key.Tenant, key.Product, key.Kind, key.ID, seq, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	out.Next = resource.Cursor{Position: position}
	for rows.Next() {
		if len(out.Records) == limit {
			out.More = true
			break
		}
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		var receipt resource.Receipt
		if err := json.Unmarshal(raw, &receipt); err != nil || !receipt.Valid() || receipt.Resource != key || receipt.Base != out.Next.Position {
			return out, resource.ErrFork
		}
		out.Records = append(out.Records, receipt)
		out.Next.Position = receipt.Head
	}
	return out, rows.Err()
}

func (r *Relay) Cancel(ctx context.Context, key resource.Resource, op string) error {
	return r.cancel(ctx, key, op, nil)
}

// ResolveNotAccepted accepts positive reconciliation evidence from the fixed
// relay/SDK only. A timeout, expired dedup window or 404 is never such evidence.
func (r *Relay) ResolveNotAccepted(ctx context.Context, key resource.Resource, op string, evidence json.RawMessage) error {
	return r.cancel(ctx, key, op, evidence)
}

func (r *Relay) cancel(ctx context.Context, key resource.Resource, op string, evidence json.RawMessage) error {
	entry, err := r.Pending(ctx, key, op)
	if err != nil {
		return err
	}
	_, a, err := r.access(ctx, key, entry.Candidate.Proof.Action)
	if err != nil {
		return err
	}
	a, err = r.producerOrManager(ctx, key, entry.Candidate, a)
	if err != nil {
		return err
	}
	if entry.State == resource.Sending || entry.State == resource.Ambiguous {
		if len(evidence) == 0 || !json.Valid(evidence) {
			return resource.ErrPending
		}
		if err := r.verifier.VerifyNotAccepted(ctx, entry, evidence); err != nil {
			return err
		}
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		h, e := r.locked(ctx, tx, key, a)
		if e != nil {
			return e
		}
		current, e := r.load(ctx, tx, key, op, true)
		if e != nil {
			return e
		}
		if current.State == resource.Committed {
			return resource.ErrConflict
		}
		if current.State == resource.Cancelled || current.State == resource.Stale {
			return nil
		}
		if current.AttemptID != entry.AttemptID || current.State != entry.State {
			return resource.ErrConflict
		}
		if _, e = tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='cancelled',resolution=$6,updated_at=now() WHERE (tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, key.Tenant, key.Product, key.Kind, key.ID, op, evidence); e != nil {
			return e
		}
		if h.hold != nil && *h.hold == op {
			_, e = tx.Exec(ctx, `UPDATE resource_e2e_resources SET hold_op_id=NULL,updated_at=now() WHERE (tenant,product,kind,resource_id)=($1,$2,$3,$4)`, resourceKey(key)...)
		}
		return e
	})
}

func (r *Relay) Drain(ctx context.Context, key resource.Resource) error {
	_, a, err := r.access(ctx, key, resource.Manage)
	if err != nil {
		return err
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := r.locked(ctx, tx, key, a); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE resource_e2e_resources SET state='draining',updated_at=now() WHERE (tenant,product,kind,resource_id)=($1,$2,$3,$4)`, resourceKey(key)...)
		return e
	})
}

func (r *Relay) Bootstrap(ctx context.Context, value resource.SignedEpoch) error {
	key := value.Resource
	if err := r.scoped(ctx, key); err != nil {
		return err
	}
	verified, err := r.verifier.InitialEpoch(ctx, value)
	if err != nil {
		return err
	}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return err
	}
	a, err := r.verifier.Access(ctx, key, resource.Manage, raw)
	if err != nil {
		return err
	}
	a, err = resource.Reauthorize(ctx, a, r.subject, resource.Manage)
	if err != nil {
		return err
	}
	hash, err := resource.EpochDigest(value.Epoch)
	if err != nil || hash != verified.SHA256() || hash != a.Epoch().SHA256() || value.Generation != "1" || (r.dependencies.RequirePrivateRules && !value.Validation.PrivateRules) {
		return resource.ErrInvalid
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if e := storeBootstrapEvidence(ctx, tx, value.Epoch, a, true); e != nil {
			return e
		}
		h, e := r.insertInitialResource(ctx, tx, value, hash)
		if e != nil {
			return e
		}
		if !h.matches(a) || h.baseline != value.Cutover || h.state == "revoked" {
			return resource.ErrConflict
		}
		return nil
	})
}

func (r *Relay) Rotate(ctx context.Context, value resource.SignedEpoch) error {
	key := value.Resource
	before, a, err := r.access(ctx, key, resource.Manage)
	if err != nil {
		return err
	}
	if before.state != "draining" || before.hold != nil {
		return resource.ErrPending
	}
	verified, err := r.verifier.NextEpoch(ctx, value, a, before.position)
	if err != nil {
		return err
	}
	hash, err := resource.EpochDigest(value.Epoch)
	if err != nil || hash != verified.SHA256() || value.Cutover != before.position || (r.dependencies.RequirePrivateRules && !value.Validation.PrivateRules) {
		return resource.ErrInvalid
	}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return err
	}
	generation, err := value.Generation.Int64()
	if err != nil {
		return err
	}
	return r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		h, e := r.locked(ctx, tx, key, a)
		if e != nil {
			return e
		}
		if h.hold != nil || h.state != "draining" || h.position != before.position {
			return resource.ErrPending
		}
		if err := storeBootstrapEvidence(ctx, tx, value.Epoch, a, false); err != nil {
			return err
		}
		if _, e = tx.Exec(ctx, `INSERT INTO resource_e2e_epochs(tenant,product,kind,resource_id,epoch,generation,policy_sha256,signed_policy)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, key.Tenant, key.Product, key.Kind, key.ID, value.ID, generation, hash, raw); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE resource_e2e_resources SET epoch=$5,generation=$6,policy_sha256=$7,membership_sha256=$8,private_rules=$9,rules_version=$10,state='active',updated_at=now() WHERE(tenant,product,kind,resource_id)=($1,$2,$3,$4)`, key.Tenant, key.Product, key.Kind, key.ID, value.ID, generation, hash, value.MembershipSHA256, value.Validation.PrivateRules, value.Validation.RulesVersion)
		return e
	})
}

// Initial row/epoch insertion is shared by already anchored legacy callers and
// the pre-epoch phase finalizer. The surrounding transaction verifies authority.
func (r *Relay) insertInitialResource(ctx context.Context, tx pgx.Tx, value resource.SignedEpoch, hash string) (resourceHead, error) {
	key := value.Resource
	seq, err := value.Cutover.Seq.Int64()
	if err != nil {
		return resourceHead{}, err
	}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return resourceHead{}, err
	}
	_, e := tx.Exec(ctx, `INSERT INTO resource_e2e_resources(tenant,product,kind,resource_id,epoch,generation,policy_sha256,membership_sha256,private_rules,rules_version,baseline_seq,baseline_head_cipher_sha256,seq,head_cipher_sha256)VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,$9,$10,$11,$10,$11) ON CONFLICT DO NOTHING`, key.Tenant, key.Product, key.Kind, key.ID, value.ID, hash, value.MembershipSHA256, value.Validation.PrivateRules, value.Validation.RulesVersion, seq, value.Cutover.HeadCipherSHA256)
	if e != nil {
		return resourceHead{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO resource_e2e_epochs(tenant,product,kind,resource_id,epoch,generation,policy_sha256,signed_policy)VALUES($1,$2,$3,$4,$5,1,$6,$7)ON CONFLICT DO NOTHING`, key.Tenant, key.Product, key.Kind, key.ID, value.ID, hash, raw)
	if e != nil {
		return resourceHead{}, e
	}
	h, e := r.readHead(ctx, tx, key, true)
	if e != nil {
		return resourceHead{}, e
	}
	return h, nil
}
