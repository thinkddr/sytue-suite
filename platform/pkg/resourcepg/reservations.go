// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Evidence is content-addressed canonical Go SDK bytes. The publication path
// asks once which hashes exist and only copies previously unseen evidence.
type capturedPublicEvidence interface {
	Valid() bool
	EvidenceHashes() []string
	Evidence(string) (resource.EvidenceRecord, error)
}

func storeResourceEvidence(ctx context.Context, tx pgx.Tx, key resource.Resource, c capturedPublicEvidence) error {
	if !c.Valid() {
		return resource.ErrInvalid
	}
	hashes := c.EvidenceHashes()
	rows, e := tx.Query(ctx, `SELECT sha256 FROM resource_e2e_public_evidence WHERE tenant=$1 AND product=$2 AND sha256=ANY($3)`, key.Tenant, key.Product, hashes)
	if e != nil {
		return e
	}
	present := map[string]bool{}
	for rows.Next() {
		var hash string
		if e = rows.Scan(&hash); e != nil {
			rows.Close()
			return e
		}
		present[hash] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	batch := &pgx.Batch{}
	for _, hash := range hashes {
		if present[hash] {
			continue
		}
		record, e := c.Evidence(hash)
		if e != nil {
			return e
		}
		batch.Queue(`INSERT INTO resource_e2e_public_evidence(tenant,product,sha256,value)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, key.Tenant, key.Product, record.SHA256, []byte(record.Value))
	}
	if batch.Len() == 0 {
		return nil
	}
	return tx.SendBatch(ctx, batch).Close()
}

type resourceHoldSource struct {
	relay  *Relay
	access resource.VerifiedAccess
}

var _ resource.TrustedHoldSource = resourceHoldSource{}

func (s resourceHoldSource) HeldReservation(ctx context.Context, key resource.Resource, op string) (resource.HeldReservation, error) {
	var out resource.HeldReservation
	if key != s.access.Epoch().Value().Resource {
		return out, resource.ErrRevoked
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		h, e := s.relay.locked(ctx, tx, key, s.access)
		if e != nil {
			return e
		}
		entry, e := s.relay.load(ctx, tx, key, op, false)
		if e != nil {
			return e
		}
		if h.hold == nil || *h.hold != op || (entry.State != resource.Sending && entry.State != resource.Ambiguous) {
			return resource.ErrPending
		}
		var generation, base, count int64
		args := append(resourceKey(key), op)
		e = tx.QueryRow(ctx, `SELECT epoch,generation,publication_sha256,candidate_sha256,reservation_sha256,reservation,base_seq,base_head_cipher_sha256,op_count FROM resource_e2e_candidates WHERE(tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, args...).Scan(&out.Epoch, &generation, &out.PublicationSHA256, &out.CandidateSHA256, &out.DescriptorSHA256, &out.Descriptor, &base, &out.Base.HeadCipherSHA256, &count)
		if e != nil {
			return e
		}
		out.State = entry.State
		out.Resource = key
		out.OpID = op
		out.AttemptID = entry.AttemptID
		out.Generation, _ = resource.Sequence(generation)
		out.Base.Seq, _ = resource.Sequence(base)
		out.End, _ = resource.Sequence(base + count)
		out.PolicySHA256 = h.policy
		out.Candidate = entry.Candidate
		return nil
	})
	return out, err
}

func (s resourceHoldSource) PublicEvidence(ctx context.Context, key resource.Resource, hash string) (json.RawMessage, error) {
	var raw []byte
	if key != s.access.Epoch().Value().Resource {
		return nil, resource.ErrRevoked
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := s.relay.locked(ctx, tx, key, s.access); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT value FROM resource_e2e_public_evidence WHERE tenant=$1 AND product=$2 AND sha256=$3`, key.Tenant, key.Product, hash).Scan(&raw)
	})
	return raw, err
}

// ReconcileAccepted is ACK-only for an existing immutable send hold. Current
// management authority may read retained public evidence; it cannot revive the
// producer device, acquire a new send, or regenerate that producer's ciphertext.
func (r *Relay) ReconcileAccepted(ctx context.Context, key resource.Resource, op string, ref storage.Reference) (resource.Receipt, error) {
	_, a, err := r.access(ctx, key, resource.Manage)
	if err != nil {
		return resource.Receipt{}, err
	}
	var current resource.JournalEntry
	err = r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := r.locked(ctx, tx, key, a); e != nil {
			return e
		}
		var e error
		current, e = r.load(ctx, tx, key, op, false)
		return e
	})
	if err != nil {
		return resource.Receipt{}, err
	}
	if current.Receipt != nil {
		if current.Receipt.Reference != ref {
			return resource.Receipt{}, resource.ErrFork
		}
		return *current.Receipt, nil
	}
	reserved, err := resource.RestoreReservation(ctx, key, op, resourceHoldSource{relay: r, access: a})
	if err != nil {
		return resource.Receipt{}, err
	}
	accepted, err := r.verifier.VerifyReservedAcceptance(ctx, reserved, ref)
	if err != nil {
		return resource.Receipt{}, err
	}
	if accepted.Resource() != key || accepted.Reference() != ref || accepted.PublicationSHA256() != reserved.Value().PublicationSHA256 || accepted.ReservationSHA256() != reserved.SHA256() || accepted.CipherSHA256() != current.Candidate.Proof.OuterCipherSHA256 {
		return resource.Receipt{}, resource.ErrInvalid
	}
	receipt := resource.Receipt{Resource: key, Epoch: current.Candidate.Proof.Epoch, OpID: op, PublicationSHA256: accepted.PublicationSHA256(), Reference: ref, Base: current.Candidate.Proof.ExpectedBase, Head: resource.Position{Seq: reserved.Value().End, HeadCipherSHA256: accepted.CipherSHA256()}}
	return r.commitReceipt(ctx, a, current, receipt, reserved.SHA256())
}

// Store the public pre-cipher context by its signed commitment. A recovered
// device must not reconstruct an old creator/recipient from a later epoch.
func storeBootstrapEvidence(ctx context.Context, tx pgx.Tx, e resource.Epoch, a resource.VerifiedAccess, initial bool) error {
	kinds := []resource.BootstrapKind{resource.BootstrapCheckpoint}
	if initial {
		kinds = append(kinds, resource.BootstrapBaseline)
	}
	for _, kind := range kinds {
		position := e.Cutover
		expected := e.Checkpoint.ContextSHA256
		if kind == resource.BootstrapBaseline {
			position = resource.Position{Seq: e.Baseline.Position.Seq}
			expected = e.Baseline.ContextSHA256
		}
		value, err := resource.NewBootstrapContext(kind, e.Resource, e.RoomID, e.ID, e.Recipient, position, a.Capability().Enrollment().Scope, e.Issuer)
		if err != nil {
			return err
		}
		hash, err := resource.BootstrapContextSHA256(value)
		if err != nil || hash != expected {
			return resource.ErrInvalid
		}
		raw, err := resource.BootstrapContextBytes(value)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO resource_e2e_public_evidence(tenant,product,sha256,value)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, e.Resource.Tenant, e.Resource.Product, hash, raw); err != nil {
			return err
		}
	}
	return nil
}
