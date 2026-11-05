// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	EnvelopeVersion       = 1
	MaxEnvelopeContext    = 4096
	MaxEnvelopePayload    = 64 << 10
	MaxEnvelopeCiphertext = MaxEnvelopePayload + MaxEnvelopeContext + 8192
)

var (
	ErrEnvelope   = errors.New("pqc: invalid context-bound hybrid envelope")
	envelopeMagic = []byte("sytue-pq-envelope-v1\x00")
)

// Envelope exposes only the public hybrid profile and recipient commitment.
// Application records, owner attestations, indexes and file keys remain inside
// the maintained age hybrid encryption.
type Envelope struct {
	Version    int    `json:"v"`
	Profile    string `json:"profile"`
	KeyID      string `json:"key_id"`
	Ciphertext []byte `json:"ciphertext"`
}

func RecipientID(recipient Recipient) (string, error) {
	if recipient == nil {
		return "", ErrIdentity
	}
	key, err := ParseRecipient(HybridProfile, recipient.String())
	if err != nil {
		return "", ErrIdentity
	}
	sum := sha256.Sum256([]byte(HybridProfile + "\x00" + key.String()))
	return hex.EncodeToString(sum[:]), nil
}

// Seal binds the exact context inside age. Length framing is application
// serialization; age owns the combiner, randomness and authenticated stream.
func Seal(recipient Recipient, context, payload []byte) (Envelope, error) {
	if len(context) == 0 || len(context) > MaxEnvelopeContext || len(payload) > MaxEnvelopePayload {
		return Envelope{}, ErrEnvelope
	}
	keyID, err := RecipientID(recipient)
	if err != nil {
		return Envelope{}, err
	}
	canonical, err := ParseRecipient(HybridProfile, recipient.String())
	if err != nil {
		return Envelope{}, ErrIdentity
	}
	var ciphertext bytes.Buffer
	writer, err := age.Encrypt(&ciphertext, canonical)
	if err != nil {
		return Envelope{}, err
	}
	var size [4]byte
	// G115: len(context) está acotado a MaxEnvelopeContext (4 KiB) por la guarda de entrada.
	binary.BigEndian.PutUint32(size[:], uint32(len(context))) //nolint:gosec // acotado por la guarda de entrada
	for _, part := range [][]byte{envelopeMagic, size[:], context, payload} {
		if _, err := writer.Write(part); err != nil {
			return Envelope{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return Envelope{}, err
	}
	if ciphertext.Len() > MaxEnvelopeCiphertext {
		return Envelope{}, ErrEnvelope
	}
	return Envelope{Version: EnvelopeVersion, Profile: HybridProfile, KeyID: keyID, Ciphertext: ciphertext.Bytes()}, nil
}

// Open verifies the entire age stream before releasing any application byte.
// A classical identity is never a fallback for a declared hybrid envelope.
func Open(identity *Identity, context []byte, envelope Envelope) ([]byte, error) {
	if identity == nil || identity.Profile() != HybridProfile || len(context) == 0 || len(context) > MaxEnvelopeContext || envelope.Version != EnvelopeVersion || envelope.Profile != HybridProfile || len(envelope.Ciphertext) == 0 || len(envelope.Ciphertext) > MaxEnvelopeCiphertext {
		return nil, ErrEnvelope
	}
	keyID, err := RecipientID(identity.Recipient())
	if err != nil || keyID != envelope.KeyID {
		return nil, ErrEnvelope
	}
	reader, err := age.Decrypt(bytes.NewReader(envelope.Ciphertext), identity)
	if err != nil {
		return nil, ErrEnvelope
	}
	limit := len(envelopeMagic) + 4 + MaxEnvelopeContext + MaxEnvelopePayload
	raw, err := io.ReadAll(io.LimitReader(reader, int64(limit+1)))
	defer clear(raw)
	if err != nil {
		return nil, ErrEnvelope
	}
	return openStream(raw, context, limit)
}

// openStream separates the framed stream that age already authenticated: the
// magic, the declared context length, the bound context and the payload. The
// frame layout is shared with Seal; the two must not drift apart.
func openStream(raw, context []byte, limit int) ([]byte, error) {
	if len(raw) > limit || len(raw) < len(envelopeMagic)+4 || !bytes.Equal(raw[:len(envelopeMagic)], envelopeMagic) {
		return nil, ErrEnvelope
	}
	start := len(envelopeMagic) + 4
	n := int(binary.BigEndian.Uint32(raw[len(envelopeMagic):start]))
	if n < 1 || n > MaxEnvelopeContext || n > len(raw)-start || !bytes.Equal(raw[start:start+n], context) || len(raw)-start-n > MaxEnvelopePayload {
		return nil, ErrEnvelope
	}
	return bytes.Clone(raw[start+n:]), nil
}

type recordContext struct {
	Domain  string        `json:"domain"`
	Version int           `json:"v"`
	Scope   storage.Scope `json:"scope"`
	Room    string        `json:"room"`
}
type recordHeader struct {
	Version    int             `json:"v"`
	ObjectID   string          `json:"object_id"`
	RevisionID string          `json:"revision_id"`
	Role       string          `json:"role"`
	Previous   []string        `json:"previous,omitempty"`
	Proof      json.RawMessage `json:"proof,omitempty"`
}

// StorageContext is shared by Go and Wasm. Callers pass typed scope/room values,
// never duplicate JSON context ordering or cryptography in JavaScript.
func StorageContext(scope storage.Scope, room, domain string) ([]byte, error) {
	if !scope.Valid() || len(room) < 2 || len(room) > 255 || room[0] != '!' || (domain != "record" && domain != "fragment-root") {
		return nil, ErrEnvelope
	}
	return json.Marshal(recordContext{Domain: "org.sytue.storage.pq." + domain + ".v1", Version: 1, Scope: scope, Room: room})
}

// SealStorageRecord preserves exact payload JSON bytes after a metadata header.
// Removing an unnecessary inner base64 reduces event size; transports must still
// page oversized ciphertext and must never lower the historical record limit.
func encodeStorageRecord(record storage.Record, proof []byte) ([]byte, error) {
	if record.Validate(record.Scope) != nil || len(proof) > 4096 || len(proof) > 0 && !json.Valid(proof) {
		return nil, ErrEnvelope
	}
	header, err := json.Marshal(recordHeader{Version: record.Version, ObjectID: record.ObjectID, RevisionID: record.RevisionID, Role: record.Role, Previous: record.Previous, Proof: proof})
	if err != nil || len(header)+4+len(record.Payload) > MaxEnvelopePayload {
		return nil, ErrEnvelope
	}
	raw := make([]byte, 4+len(header)+len(record.Payload))
	// G115: la guarda de arriba acota el total a MaxEnvelopePayload (64 KiB).
	binary.BigEndian.PutUint32(raw[:4], uint32(len(header))) //nolint:gosec // acotado por la guarda de arriba
	copy(raw[4:], header)
	copy(raw[4+len(header):], record.Payload)
	return raw, nil
}

func decodeStorageRecord(scope storage.Scope, raw []byte) (storage.Record, []byte, error) {
	if !scope.Valid() || len(raw) < 4 || len(raw) > MaxEnvelopePayload {
		return storage.Record{}, nil, ErrEnvelope
	}
	n := int(binary.BigEndian.Uint32(raw[:4]))
	if n < 1 || n > MaxEnvelopePayload || n > len(raw)-4 {
		return storage.Record{}, nil, ErrEnvelope
	}
	var header recordHeader
	if json.Unmarshal(raw[4:4+n], &header) != nil || len(header.Proof) > 4096 || len(header.Proof) > 0 && !json.Valid(header.Proof) {
		return storage.Record{}, nil, ErrEnvelope
	}
	record := storage.Record{Version: header.Version, Scope: scope, ObjectID: header.ObjectID, RevisionID: header.RevisionID, Role: header.Role, Previous: header.Previous, Payload: bytes.Clone(raw[4+n:])}
	if record.Validate(scope) != nil {
		clear(record.Payload)
		return storage.Record{}, nil, ErrEnvelope
	}
	return record, bytes.Clone(header.Proof), nil
}

func SealStorageRecord(recipient Recipient, scope storage.Scope, room string, record storage.Record, proof []byte) (Envelope, error) {
	context, err := StorageContext(scope, room, "record")
	if err != nil || record.Scope != scope || len(proof) == 0 {
		return Envelope{}, ErrEnvelope
	}
	raw, err := encodeStorageRecord(record, proof)
	defer clear(raw)
	if err != nil {
		return Envelope{}, err
	}
	return Seal(recipient, context, raw)
}

// OpenStorageRecord authenticates all bytes before returning an ordinary record.
// The caller must also verify its symmetric owner attestation: a public age
// recipient is encryption capability, never authority to author application data.
func OpenStorageRecord(identity *Identity, scope storage.Scope, room string, envelope Envelope) (storage.Record, []byte, error) {
	context, err := StorageContext(scope, room, "record")
	if err != nil {
		return storage.Record{}, nil, err
	}
	raw, err := Open(identity, context, envelope)
	defer clear(raw)
	if err != nil {
		return storage.Record{}, nil, ErrEnvelope
	}
	record, proof, err := decodeStorageRecord(scope, raw)
	if err != nil || len(proof) == 0 {
		clear(record.Payload)
		clear(proof)
		return storage.Record{}, nil, ErrEnvelope
	}
	return record, proof, nil
}

// EncodeStorageRecordPayload is deterministic and excludes the randomized owner
// attestation. Independent hybrid fragments can therefore resume an older
// accepted prefix without mixing incompatible whole-stream encryption attempts.
func EncodeStorageRecordPayload(record storage.Record) ([]byte, error) {
	return encodeStorageRecord(record, nil)
}

func DecodeStorageRecordPayload(scope storage.Scope, raw []byte) (storage.Record, error) {
	record, proof, err := decodeStorageRecord(scope, raw)
	defer clear(proof)
	if err != nil || len(proof) != 0 {
		clear(record.Payload)
		return storage.Record{}, ErrEnvelope
	}
	return record, nil
}
