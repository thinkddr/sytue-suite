// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// CommittedSource captures the actual CURRENT Read session resolved by this
// Relay. It exposes only immutable committed history and public SDK evidence;
// it cannot acquire a hold, stage a candidate, publish, or revive old producer A.
func (r *Relay) CommittedSource(ctx context.Context, key resource.Resource) (resource.TrustedCommittedSource, resource.VerifiedAccess, error) {
	_, access, err := r.access(ctx, key, resource.Read)
	if err != nil {
		return nil, resource.VerifiedAccess{}, err
	}
	if _, ok := access.CurrentSessionBinding(); !ok {
		return nil, resource.VerifiedAccess{}, resource.ErrRevoked
	}
	return committedSource{relay: r, key: key, access: access}, access, nil
}

type committedSource struct {
	relay  *Relay
	key    resource.Resource
	access resource.VerifiedAccess
}

var _ resource.TrustedCommittedSource = committedSource{}

func (s committedSource) current(ctx context.Context, key resource.Resource) (resource.VerifiedAccess, error) {
	if s.relay == nil || key != s.key {
		return resource.VerifiedAccess{}, resource.ErrRevoked
	}
	_, access, err := s.relay.access(ctx, key, resource.Read)
	if err != nil {
		return resource.VerifiedAccess{}, err
	}
	before, original := s.access.CurrentSessionBinding()
	actual, current := access.CurrentSessionBinding()
	if !original || !current || before != actual || access.Epoch().SHA256() != s.access.Epoch().SHA256() || access.Capability().SHA256() != s.access.Capability().SHA256() {
		return resource.VerifiedAccess{}, resource.ErrRevoked
	}
	return access, nil
}

func (s committedSource) CommittedPublication(ctx context.Context, key resource.Resource, op string) (resource.CommittedPublication, error) {
	var out resource.CommittedPublication
	access, err := s.current(ctx, key)
	if err != nil {
		return out, err
	}
	err = s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.readCommittedPublicationTx(ctx, tx, key, op, access)
		return err
	})
	if err != nil {
		return resource.CommittedPublication{}, err
	}
	if _, err := s.current(ctx, key); err != nil {
		return resource.CommittedPublication{}, err
	}
	return out, nil
}

func (s committedSource) readCommittedPublicationTx(ctx context.Context, tx pgx.Tx, key resource.Resource, op string, access resource.VerifiedAccess) (resource.CommittedPublication, error) {
	var out resource.CommittedPublication
	head, err := s.relay.locked(ctx, tx, key, access)
	if err != nil {
		return out, err
	}
	entry, err := s.relay.load(ctx, tx, key, op, false)
	if err != nil {
		return out, err
	}
	if entry.State != resource.Committed || entry.Receipt == nil {
		return out, resource.ErrPending
	}
	args := append(resourceKey(key), op)
	var generation, base, count int64
	err = tx.QueryRow(ctx, `SELECT c.epoch,c.generation,e.policy_sha256,c.publication_sha256,c.candidate_sha256,c.reservation_sha256,c.reservation,c.base_seq,c.base_head_cipher_sha256,c.op_count FROM resource_e2e_candidates c JOIN resource_e2e_epochs e USING(tenant,product,kind,resource_id,epoch) WHERE(c.tenant,c.product,c.kind,c.resource_id,c.op_id)=($1,$2,$3,$4,$5)`, args...).Scan(&out.Epoch, &generation, &out.PolicySHA256, &out.PublicationSHA256, &out.CandidateSHA256, &out.DescriptorSHA256, &out.Descriptor, &base, &out.Base.HeadCipherSHA256, &count)
	if err != nil {
		return out, err
	}
	out.Generation, err = resource.Sequence(generation)
	if err != nil || generation < 1 {
		return out, resource.ErrInvalid
	}
	out.Base.Seq, err = resource.Sequence(base)
	if err != nil {
		return out, err
	}
	out.End, err = out.Base.Seq.Next(count)
	if err != nil {
		return out, err
	}
	out.State, out.Resource, out.OpID, out.AttemptID, out.Candidate = entry.State, key, op, entry.AttemptID, entry.Candidate
	receipt, err := scanCommittedReceipt(ctx, tx, key, op, args, entry, head)
	if err != nil {
		return out, err
	}
	out.Receipt = receipt
	return out, nil
}

func scanCommittedReceipt(ctx context.Context, tx pgx.Tx, key resource.Resource, op string, args []any, entry resource.JournalEntry, head resourceHead) (resource.Receipt, error) {
	receipt := resource.Receipt{Resource: key, OpID: op}
	var receiptBase, receiptEnd int64
	err := tx.QueryRow(ctx, `SELECT epoch,publication_sha256,base_seq,seq,base_head_cipher_sha256,head_cipher_sha256,room_id,event_id FROM resource_e2e_receipts WHERE(tenant,product,kind,resource_id,op_id)=($1,$2,$3,$4,$5)`, args...).Scan(&receipt.Epoch, &receipt.PublicationSHA256, &receiptBase, &receiptEnd, &receipt.Base.HeadCipherSHA256, &receipt.Head.HeadCipherSHA256, &receipt.Reference.RoomID, &receipt.Reference.EventID)
	if err != nil {
		return receipt, err
	}
	receipt.Base.Seq, err = resource.Sequence(receiptBase)
	if err != nil {
		return receipt, err
	}
	receipt.Head.Seq, err = resource.Sequence(receiptEnd)
	if err != nil || !receipt.Valid() || receipt != *entry.Receipt {
		return receipt, resource.ErrInvalid
	}
	currentEnd, err := head.position.Seq.Int64()
	if err != nil || receiptEnd > currentEnd || (receiptEnd == currentEnd && receipt.Head != head.position) {
		return receipt, resource.ErrFork
	}
	return receipt, nil
}

func (s committedSource) CheckCommittedPublication(ctx context.Context, key resource.Resource, op, descriptorSHA string, receipt resource.Receipt) error {
	stored, err := s.CommittedPublication(ctx, key, op)
	if err != nil {
		return err
	}
	if stored.DescriptorSHA256 != descriptorSHA || stored.Receipt != receipt {
		return resource.ErrFork
	}
	return nil
}

func (s committedSource) PublicEvidence(ctx context.Context, key resource.Resource, hash string) (json.RawMessage, error) {
	access, err := s.current(ctx, key)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.relay.locked(ctx, tx, key, access); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT value FROM resource_e2e_public_evidence WHERE tenant=$1 AND product=$2 AND sha256=$3`, key.Tenant, key.Product, hash).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.current(ctx, key); err != nil {
		return nil, err
	}
	return raw, nil
}
