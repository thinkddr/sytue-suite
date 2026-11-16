// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type (
	storagePQWire    = pqc.StorageWire
	storagePQContent = pqc.StorageMessage
)

func storagePQContentFor(kind string, envelope pqc.Envelope) storagePQContent {
	return pqc.NewStorageMessage(kind, envelope)
}

func storagePQContentFits(content storagePQContent) bool {
	return pqc.StorageMessageFits(content)
}

func decodeStoragePQWire(value DecryptedEvent, own string) (storagePQWire, bool, error) {
	wire, found, err := pqc.DecodeStorageMessage(value.Content)
	if err != nil || !found {
		return wire, found, err
	}
	if value.Sender != own || value.Type != event.EventMessage.Type || !value.Reference.Valid() {
		return storagePQWire{}, true, ErrIdentity
	}
	return wire, true, nil
}

func (c *Client) sendStoragePQWire(ctx context.Context, target storage.PublicationTarget, transaction, kind string, envelope pqc.Envelope) (EventRef, error) {
	content := storagePQContentFor(kind, envelope)
	if !storagePQContentFits(content) {
		return EventRef{}, storage.ErrRecord
	}
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return EventRef{}, err
	}
	if ref, handled, err := c.sendAdmittedStoragePQWire(ctx, target, transaction, kind, envelope); handled {
		return ref, err
	}
	return c.sendEventWithTxn(ctx, target.RoomID, event.EventMessage, content, "sytue-storage-"+transaction)
}

// putStoragePQRecord protects every record role, including file-key pages and
// catalog indexes. The outer Matrix encryption remains a maintained transport;
// opening it alone reveals only a context-bound hybrid envelope.
func (c *Client) putStoragePQRecord(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record, proof storageOwnerProof) (EventRef, error) {
	identity, err := c.storagePQIdentity(ctx, "", false)
	if err != nil {
		return EventRef{}, err
	}
	owner, err := json.Marshal(proof)
	if err != nil {
		return EventRef{}, err
	}
	defer clear(owner)
	envelope, err := pqc.SealStorageRecord(identity.Recipient(), record.Scope, target.RoomID, record, owner)
	if err != nil {
		return EventRef{}, err
	}
	var ref EventRef
	if storagePQContentFits(storagePQContentFor("record", envelope)) {
		ref, err = c.sendStoragePQWire(ctx, target, transaction, "record", envelope)
	} else {
		ref, err = c.putStoragePQFragments(ctx, target, transaction, record, owner, identity)
	}
	if err != nil {
		return ref, err
	}
	// The Matrix transaction may already belong to an earlier accepted attempt.
	// Verify that actual receipt before acknowledging this exact caller input.
	accepted, err := c.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role)
	if err != nil {
		return ref, err
	}
	defer clear(accepted.Payload)
	if !record.MatchesExact(accepted) {
		return ref, ErrIdentity
	}
	return ref, nil
}

func (c *Client) putStoragePQFragments(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record, proof []byte, identity *pqc.Identity) (EventRef, error) {
	raw, err := pqc.EncodeStorageRecordPayload(record)
	if err != nil {
		return EventRef{}, err
	}
	defer clear(raw)
	digest, err := pqc.PayloadDigest(raw)
	if err != nil {
		return EventRef{}, err
	}
	count := (len(raw) + pqc.StorageFragmentBytes - 1) / pqc.StorageFragmentBytes
	refs := make([]storage.Reference, 0, count)
	keyID, err := pqc.RecipientID(identity.Recipient())
	if err != nil {
		return EventRef{}, err
	}
	for index := 0; index < count; index++ {
		first := index * pqc.StorageFragmentBytes
		part := raw[first:min(first+pqc.StorageFragmentBytes, len(raw))]
		envelope, err := pqc.SealStorageFragment(identity.Recipient(), record.Scope, target.RoomID, digest, index, count, part)
		if err != nil {
			return EventRef{}, err
		}
		ref, err := c.sendStoragePQWire(ctx, target, transaction+"_pq_fragment_"+strconv.Itoa(index), "fragment", envelope)
		if err != nil {
			return ref, err
		}
		accepted, err := c.readStoragePQFragment(ctx, ref, identity, keyID, record.Scope, target.RoomID, digest, index, count)
		if err != nil {
			return ref, err
		}
		matches := bytes.Equal(accepted, part)
		clear(accepted)
		if !matches {
			return ref, ErrIdentity
		}
		refs = append(refs, ref)
	}
	root := pqc.FragmentRoot{Version: 1, SHA256: digest, SizeBytes: len(raw), References: refs, Proof: bytes.Clone(proof)}
	defer clear(root.Proof)
	envelope, err := pqc.SealStorageFragmentRoot(identity.Recipient(), record.Scope, target.RoomID, root)
	if err != nil {
		return EventRef{}, err
	}
	return c.sendStoragePQWire(ctx, target, transaction, "fragment-root", envelope)
}

func (c *Client) readStoragePQFragment(ctx context.Context, ref EventRef, identity *pqc.Identity, keyID string, scope storage.Scope, room, digest string, index, count int) ([]byte, error) {
	if !ref.Valid() || ref.RoomID != room {
		return nil, ErrIdentity
	}
	value, err := c.Read(ctx, ref)
	if err != nil {
		return nil, err
	}
	wire, found, err := decodeStoragePQWire(value, c.session.UserID.String())
	if err != nil {
		return nil, err
	}
	if !found || wire.Kind != "fragment" || wire.Envelope.KeyID != keyID {
		return nil, ErrIdentity
	}
	return pqc.OpenStorageFragment(identity, scope, room, digest, index, count, wire.Envelope)
}

func (c *Client) openStoragePQFragments(ctx context.Context, value DecryptedEvent, wire storagePQWire, identity *pqc.Identity, scope storage.Scope) (storage.Record, []byte, error) {
	root, err := pqc.OpenStorageFragmentRoot(identity, scope, value.Reference.RoomID, wire.Envelope)
	if err != nil {
		return storage.Record{}, nil, err
	}
	defer clear(root.Proof)
	parts := make([][]byte, len(root.References))
	defer func() {
		for _, part := range parts {
			clear(part)
		}
	}()
	for index, ref := range root.References {
		part, err := c.readStoragePQFragment(ctx, ref, identity, wire.Envelope.KeyID, scope, value.Reference.RoomID, root.SHA256, index, len(root.References))
		if err != nil {
			return storage.Record{}, nil, err
		}
		parts[index] = part
	}
	raw, err := pqc.AssembleStorageFragments(root, value.Reference.RoomID, parts)
	defer clear(raw)
	if err != nil {
		return storage.Record{}, nil, err
	}
	record, err := pqc.DecodeStorageRecordPayload(scope, raw)
	if err != nil {
		return storage.Record{}, nil, err
	}
	return record, bytes.Clone(root.Proof), nil
}

// openStoragePQValue adapts the new inner packet to the existing owner-proof
// validator. Legacy data is read as legacy; malformed or unsupported PQ data
// never falls through to a classical interpretation or a replacement key.
func (c *Client) openStoragePQValue(ctx context.Context, value DecryptedEvent, scope storage.Scope, own string) (DecryptedEvent, bool, error) {
	wire, found, err := decodeStoragePQWire(value, own)
	if err != nil || !found {
		return value, false, err
	}
	if scope != c.storageScope() || wire.Kind == "fragment" {
		return DecryptedEvent{}, false, ErrIdentity
	}
	identity, err := c.storagePQIdentity(ctx, wire.Envelope.KeyID, false)
	if err != nil {
		return DecryptedEvent{}, false, err
	}
	var record storage.Record
	var proof []byte
	if wire.Kind == "record" {
		record, proof, err = pqc.OpenStorageRecord(identity, scope, value.Reference.RoomID, wire.Envelope)
	} else {
		record, proof, err = c.openStoragePQFragments(ctx, value, wire, identity, scope)
	}
	defer clear(record.Payload)
	defer clear(proof)
	if err != nil {
		return DecryptedEvent{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return DecryptedEvent{}, false, err
	}
	if c.closed.Load() {
		return DecryptedEvent{}, false, ErrClosed
	}
	raw, err := json.Marshal(map[string]any{"org.sytue.storage": record, "org.sytue.storage_auth": json.RawMessage(proof)})
	if err != nil {
		return DecryptedEvent{}, false, err
	}
	value.Content = raw
	return value, true, nil
}

func storagePQHistoryFragment(value DecryptedEvent, own string) (bool, error) {
	wire, found, err := decodeStoragePQWire(value, own)
	if err != nil {
		return false, err
	}
	return found && wire.Kind == "fragment", nil
}
