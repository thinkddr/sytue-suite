// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// La prueba de propietario de un registro (ADR 0086): HMAC-SHA256 con la clave de
// propietario de la bóveda sobre un resumen canónico del registro. Es la de
// `pq-vault.ts` (computeRecordDigest/signProof) byte a byte; el vector compartido
// está en apps/nucleo/recoverywrap/testdata/boveda_pq_registro.json.
const (
	OwnerProofDomain = "org.sytue.storage.auth.v1"
	vaultProofPrefix = "sytue-pq-vault-proof-v1\x00"
)

// OwnerProof es la prueba tal como va dentro del sobre.
type OwnerProof struct {
	V     int    `json:"v"`
	Alg   string `json:"alg"`
	KeyID string `json:"key_id"`
	MAC   string `json:"mac"`
}

// RecordDigest es el resumen que firma la prueba: SHA-256, en hexadecimal, del JSON
// de la tupla `[dominio, b64(vault), b64(tenant), b64(sub), b64(product),
// b64(object), b64(revision), b64(role), payload_b64, ...b64(previous)]`. Cada
// campo va en base64, así que el JSON solo lleva ASCII sin escapes: la web y Go
// producen los mismos bytes.
func RecordDigest(vaultID string, record storage.Record) (string, error) {
	if record.Validate(record.Scope) != nil {
		return "", storage.ErrRecord
	}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	tuple := []string{
		OwnerProofDomain, b64(vaultID), b64(record.Scope.Tenant), b64(record.Scope.Subject), b64(record.Scope.Product),
		b64(record.ObjectID), b64(record.RevisionID), b64(record.Role), base64.StdEncoding.EncodeToString(record.Payload),
	}
	for _, previous := range record.Previous {
		tuple = append(tuple, b64(previous))
	}
	var raw bytes.Buffer
	encoder := json.NewEncoder(&raw)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(tuple); err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes.TrimSuffix(raw.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:]), nil
}

// RecordMAC es el HMAC de la prueba para ese resumen.
func RecordMAC(ownerKey []byte, digest string) []byte {
	mac := hmac.New(sha256.New, ownerKey)
	mac.Write([]byte(vaultProofPrefix + digest))
	return mac.Sum(nil)
}

func (v *Vault) proof(owner []byte, record storage.Record) ([]byte, error) {
	digest, err := RecordDigest(v.vaultID, record)
	if err != nil {
		return nil, err
	}
	return json.Marshal(OwnerProof{V: 1, Alg: "hmac-sha256", KeyID: v.keyID, MAC: base64.StdEncoding.EncodeToString(RecordMAC(owner, digest))})
}

func (v *Vault) verifyProof(owner []byte, record storage.Record, raw []byte) error {
	var proof OwnerProof
	if json.Unmarshal(raw, &proof) != nil || proof.V != 1 || proof.Alg != "hmac-sha256" || proof.KeyID == "" || proof.MAC == "" {
		return ErrIdentity
	}
	got, err := base64.StdEncoding.DecodeString(proof.MAC)
	if err != nil {
		return ErrIdentity
	}
	digest, err := RecordDigest(v.vaultID, record)
	if err != nil {
		return err
	}
	if !hmac.Equal(got, RecordMAC(owner, digest)) {
		return ErrIdentity
	}
	return nil
}

// Put publica un registro: sella el registro y su prueba en un sobre híbrido (en
// fragmentos si no cabe en uno), lo envía con `transaction` y lo lee de vuelta. La
// transacción puede pertenecer a un intento anterior aceptado: solo se devuelve la
// referencia si lo que hay es exactamente este registro.
func (v *Vault) Put(ctx context.Context, transaction string, record storage.Record) (storage.Reference, error) {
	if !storage.Identifier(transaction) || record.Validate(v.scope) != nil {
		return storage.Reference{}, storage.ErrRecord
	}
	identity, owner, err := v.keys()
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(owner)
	proof, err := v.proof(owner, record)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(proof)
	envelope, err := pqc.SealStorageRecord(v.recipient, v.scope, v.vaultID, record, proof)
	if err != nil {
		return storage.Reference{}, err
	}
	var ref storage.Reference
	if pqc.StorageMessageFits(pqc.NewStorageMessage("record", envelope)) {
		ref, err = v.send(ctx, transaction, "record", envelope)
	} else {
		ref, err = v.putFragments(ctx, transaction, record, proof, identity)
	}
	if err != nil {
		return ref, err
	}
	accepted, err := v.Read(ctx, ref)
	if err != nil {
		return ref, err
	}
	defer clear(accepted.Payload)
	if !record.MatchesExact(accepted) {
		return ref, ErrIdentity
	}
	return ref, nil
}

func (v *Vault) send(ctx context.Context, transaction, kind string, envelope pqc.Envelope) (storage.Reference, error) {
	raw, err := pqc.EncodeStorageMessage(kind, envelope)
	if err != nil {
		return storage.Reference{}, err
	}
	// La política de una publicación autorizada (WithPublicationPolicy), justo
	// antes de cada envío: un fragmento más tampoco sale si ya no vale.
	if err := v.checkPublication(ctx); err != nil {
		return storage.Reference{}, err
	}
	saved, err := v.server.AppendPQVaultRecord(ctx, v.scope, v.vaultID, transaction, raw)
	if err != nil {
		return storage.Reference{}, err
	}
	ref := storage.Reference{RoomID: saved.VaultID, EventID: saved.EventID}
	if saved.VaultID != v.vaultID || !ref.Valid() {
		return storage.Reference{}, ErrIdentity
	}
	return ref, nil
}

func (v *Vault) putFragments(ctx context.Context, transaction string, record storage.Record, proof []byte, identity *pqc.Identity) (storage.Reference, error) {
	raw, err := pqc.EncodeStorageRecordPayload(record)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(raw)
	digest, err := pqc.PayloadDigest(raw)
	if err != nil {
		return storage.Reference{}, err
	}
	count := (len(raw) + pqc.StorageFragmentBytes - 1) / pqc.StorageFragmentBytes
	refs := make([]storage.Reference, 0, count)
	for index := range count {
		first := index * pqc.StorageFragmentBytes
		part := raw[first:min(first+pqc.StorageFragmentBytes, len(raw))]
		envelope, err := pqc.SealStorageFragment(v.recipient, v.scope, v.vaultID, digest, index, count, part)
		if err != nil {
			return storage.Reference{}, err
		}
		ref, err := v.send(ctx, transaction+"_pq_fragment_"+strconv.Itoa(index), "fragment", envelope)
		if err != nil {
			return ref, err
		}
		accepted, err := v.fragment(ctx, ref, identity, digest, index, count)
		if err != nil {
			return ref, err
		}
		same := bytes.Equal(accepted, part)
		clear(accepted)
		if !same {
			return ref, ErrIdentity
		}
		refs = append(refs, ref)
	}
	root := pqc.FragmentRoot{Version: 1, SHA256: digest, SizeBytes: len(raw), References: refs, Proof: bytes.Clone(proof)}
	defer clear(root.Proof)
	envelope, err := pqc.SealStorageFragmentRoot(v.recipient, v.scope, v.vaultID, root)
	if err != nil {
		return storage.Reference{}, err
	}
	return v.send(ctx, transaction, "fragment-root", envelope)
}

func (v *Vault) fetch(ctx context.Context, ref storage.Reference) (pqc.StorageWire, error) {
	if !ref.Valid() || ref.RoomID != v.vaultID {
		return pqc.StorageWire{}, ErrIdentity
	}
	saved, err := v.server.PQVaultRecord(ctx, v.scope, v.vaultID, ref.EventID)
	if err != nil {
		return pqc.StorageWire{}, err
	}
	if saved.VaultID != ref.RoomID || saved.EventID != ref.EventID {
		return pqc.StorageWire{}, ErrIdentity
	}
	wire, found, err := pqc.DecodeStorageMessage(saved.Payload)
	if err != nil || !found {
		return pqc.StorageWire{}, storage.ErrRecord
	}
	return wire, nil
}

func (v *Vault) fragment(ctx context.Context, ref storage.Reference, identity *pqc.Identity, digest string, index, count int) ([]byte, error) {
	wire, err := v.fetch(ctx, ref)
	if err != nil {
		return nil, err
	}
	if wire.Kind != "fragment" || wire.Envelope.KeyID != v.keyID {
		return nil, storage.ErrRecord
	}
	return pqc.OpenStorageFragment(identity, v.scope, v.vaultID, digest, index, count, wire.Envelope)
}

// Read abre y autentica un registro: el sobre (con la identidad de la bóveda) y la
// prueba de propietario. Un fragmento suelto no es un registro.
func (v *Vault) Read(ctx context.Context, ref storage.Reference) (storage.Record, error) {
	wire, err := v.fetch(ctx, ref)
	if err != nil {
		return storage.Record{}, err
	}
	return v.open(ctx, ref, wire)
}

func (v *Vault) open(ctx context.Context, ref storage.Reference, wire pqc.StorageWire) (storage.Record, error) {
	identity, owner, err := v.keys()
	if err != nil {
		return storage.Record{}, err
	}
	defer clear(owner)
	if wire.Envelope.KeyID != v.keyID {
		return storage.Record{}, ErrIdentity
	}
	var (
		record storage.Record
		proof  []byte
	)
	switch wire.Kind {
	case "record":
		record, proof, err = pqc.OpenStorageRecord(identity, v.scope, ref.RoomID, wire.Envelope)
	case "fragment-root":
		record, proof, err = v.openFragments(ctx, identity, ref, wire)
	default:
		return storage.Record{}, storage.ErrRecord
	}
	defer clear(proof)
	if err != nil {
		return storage.Record{}, err
	}
	if err := v.verifyProof(owner, record, proof); err != nil {
		clear(record.Payload)
		return storage.Record{}, err
	}
	return record, nil
}

func (v *Vault) openFragments(ctx context.Context, identity *pqc.Identity, ref storage.Reference, wire pqc.StorageWire) (storage.Record, []byte, error) {
	root, err := pqc.OpenStorageFragmentRoot(identity, v.scope, ref.RoomID, wire.Envelope)
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
	for index, partRef := range root.References {
		part, err := v.fragment(ctx, partRef, identity, root.SHA256, index, len(root.References))
		if err != nil {
			return storage.Record{}, nil, err
		}
		parts[index] = part
	}
	raw, err := pqc.AssembleStorageFragments(root, ref.RoomID, parts)
	defer clear(raw)
	if err != nil {
		return storage.Record{}, nil, err
	}
	record, err := pqc.DecodeStorageRecordPayload(v.scope, raw)
	if err != nil {
		return storage.Record{}, nil, err
	}
	return record, bytes.Clone(root.Proof), nil
}

// Page es una página del historial de la bóveda, del más reciente al más antiguo.
// Los fragmentos sueltos se saltan (los abre su raíz); Scanned cuenta todo lo leído.
type Page struct {
	Entries []storage.RecordEntry
	Scanned int
	Next    string
}

// History lee una página del historial antes de `cursor` y autentica cada registro.
// Un registro que no abre o cuya prueba no verifica corta la lectura: nunca se
// devuelve una página a medias como si fuera completa.
func (v *Vault) History(ctx context.Context, cursor string, limit int) (Page, error) {
	if _, _, err := v.keys(); err != nil {
		return Page{}, err
	}
	page, err := v.server.PQVaultRecords(ctx, v.scope, v.vaultID, cursor, limit)
	if err != nil {
		return Page{}, err
	}
	out := Page{Scanned: len(page.Entries), Next: page.NextCursor}
	for _, item := range page.Entries {
		ref := storage.Reference{RoomID: item.VaultID, EventID: item.EventID}
		if item.VaultID != v.vaultID || !ref.Valid() {
			return Page{}, ErrIdentity
		}
		wire, found, err := pqc.DecodeStorageMessage(item.Payload)
		if err != nil || !found {
			return Page{}, storage.ErrRecord
		}
		if wire.Kind == "fragment" {
			continue
		}
		record, err := v.open(ctx, ref, wire)
		if err != nil {
			return Page{}, err
		}
		out.Entries = append(out.Entries, storage.RecordEntry{Reference: ref, Record: record})
	}
	return out, ctx.Err()
}
