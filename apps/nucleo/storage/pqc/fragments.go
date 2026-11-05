// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const StorageFragmentBytes = 16 << 10

type FragmentRoot struct {
	Version    int                 `json:"v"`
	SHA256     string              `json:"sha256"`
	SizeBytes  int                 `json:"size_bytes"`
	References []storage.Reference `json:"references"`
	Proof      json.RawMessage     `json:"proof"`
}
type fragmentContext struct {
	recordContext
	SHA256 string `json:"sha256"`
	Index  int    `json:"index"`
	Count  int    `json:"count"`
}

func validPayloadDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	raw, err := hex.DecodeString(digest)
	return err == nil && hex.EncodeToString(raw) == digest
}

func PayloadDigest(payload []byte) (string, error) {
	if len(payload) == 0 || len(payload) > MaxEnvelopePayload {
		return "", ErrEnvelope
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func StorageFragmentContext(scope storage.Scope, room, digest string, index, count int) ([]byte, error) {
	if _, err := StorageContext(scope, room, "record"); err != nil {
		return nil, err
	}
	if !validPayloadDigest(digest) || count < 1 || count > (MaxEnvelopePayload+StorageFragmentBytes-1)/StorageFragmentBytes || index < 0 || index >= count {
		return nil, ErrEnvelope
	}
	return json.Marshal(fragmentContext{recordContext: recordContext{Domain: "org.sytue.storage.pq.fragment.v1", Version: 1, Scope: scope, Room: room}, SHA256: digest, Index: index, Count: count})
}

func SealStorageFragment(recipient Recipient, scope storage.Scope, room, digest string, index, count int, part []byte) (Envelope, error) {
	context, err := StorageFragmentContext(scope, room, digest, index, count)
	if err != nil || len(part) == 0 || len(part) > StorageFragmentBytes {
		return Envelope{}, ErrEnvelope
	}
	return Seal(recipient, context, part)
}

func OpenStorageFragment(identity *Identity, scope storage.Scope, room, digest string, index, count int, envelope Envelope) ([]byte, error) {
	context, err := StorageFragmentContext(scope, room, digest, index, count)
	if err != nil {
		return nil, err
	}
	raw, err := Open(identity, context, envelope)
	if err != nil || len(raw) == 0 || len(raw) > StorageFragmentBytes {
		clear(raw)
		return nil, ErrEnvelope
	}
	return raw, nil
}

func (root FragmentRoot) Valid(room string) bool {
	if root.Version != 1 || !validPayloadDigest(root.SHA256) || root.SizeBytes < 1 || root.SizeBytes > MaxEnvelopePayload ||
		len(root.References) != (root.SizeBytes+StorageFragmentBytes-1)/StorageFragmentBytes ||
		len(root.Proof) == 0 || len(root.Proof) > 4096 || !json.Valid(root.Proof) {
		return false
	}
	seen := map[storage.Reference]bool{}
	for _, ref := range root.References {
		if !ref.Valid() || ref.RoomID != room || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}

func SealStorageFragmentRoot(recipient Recipient, scope storage.Scope, room string, root FragmentRoot) (Envelope, error) {
	context, err := StorageContext(scope, room, "fragment-root")
	if err != nil || !root.Valid(room) {
		return Envelope{}, ErrEnvelope
	}
	raw, err := json.Marshal(root)
	defer clear(raw)
	if err != nil {
		return Envelope{}, err
	}
	return Seal(recipient, context, raw)
}

func OpenStorageFragmentRoot(identity *Identity, scope storage.Scope, room string, envelope Envelope) (FragmentRoot, error) {
	context, err := StorageContext(scope, room, "fragment-root")
	if err != nil {
		return FragmentRoot{}, err
	}
	raw, err := Open(identity, context, envelope)
	defer clear(raw)
	if err != nil {
		return FragmentRoot{}, err
	}
	var root FragmentRoot
	if json.Unmarshal(raw, &root) != nil || !root.Valid(room) {
		return FragmentRoot{}, ErrEnvelope
	}
	return root, nil
}

// AssembleStorageFragments is the same strict byte reduction in native and
// Wasm callers. A partial, reordered, repeated or substituted set yields no data.
func AssembleStorageFragments(root FragmentRoot, room string, parts [][]byte) ([]byte, error) {
	if !root.Valid(room) || len(parts) != len(root.References) {
		return nil, ErrEnvelope
	}
	raw := make([]byte, 0, root.SizeBytes)
	for index, part := range parts {
		wanted := min(StorageFragmentBytes, root.SizeBytes-index*StorageFragmentBytes)
		if len(part) != wanted {
			clear(raw)
			return nil, ErrEnvelope
		}
		raw = append(raw, part...)
	}
	digest, err := PayloadDigest(raw)
	if err != nil || digest != root.SHA256 {
		clear(raw)
		return nil, ErrEnvelope
	}
	return raw, nil
}
