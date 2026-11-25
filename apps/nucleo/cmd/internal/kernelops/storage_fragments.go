// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package kernelops

import (
	"encoding/json"
	"math"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func init() {
	Operations["storage.record.encode"] = encodeRecordPayload
	Operations["storage.record.decode"] = decodeRecordPayload
	Operations["storage.fragment.seal"] = sealFragment
	Operations["storage.fragment.open"] = openFragment
	Operations["storage.fragment.root.seal"] = sealFragmentRoot
	Operations["storage.fragment.root.open"] = openFragmentRoot
	Operations["storage.fragment.assemble"] = assembleFragments
}

func encodeRecordPayload(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("record"), 96<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var record storage.Record
	if json.Unmarshal(raw, &record) != nil {
		return nil, pqc.ErrEnvelope
	}
	defer clear(record.Payload)
	payload, err := pqc.EncodeStorageRecordPayload(record)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	digest, err := pqc.PayloadDigest(payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{"payload": bytesValue(payload), "digest": digest, "count": (len(payload) + pqc.StorageFragmentBytes - 1) / pqc.StorageFragmentBytes, "fragment_bytes": pqc.StorageFragmentBytes}, nil
}

func decodeRecordPayload(req js.Value) (any, error) {
	payload, err := bytesArg(req.Get("payload"), pqc.MaxEnvelopePayload)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	record, err := pqc.DecodeStorageRecordPayload(scopeArg(req), payload)
	if err != nil {
		return nil, err
	}
	defer clear(record.Payload)
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return bytesValue(raw), nil
}

func integerArg(value js.Value, maximum int) (int, error) {
	if value.Type() != js.TypeNumber {
		return 0, pqc.ErrEnvelope
	}
	number := value.Float()
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > float64(maximum) || math.Trunc(number) != number {
		return 0, pqc.ErrEnvelope
	}
	return int(number), nil
}

func fragmentIndices(req js.Value) (int, int, error) {
	maximum := (pqc.MaxEnvelopePayload + pqc.StorageFragmentBytes - 1) / pqc.StorageFragmentBytes
	index, err := integerArg(req.Get("index"), maximum)
	if err != nil {
		return 0, 0, err
	}
	count, err := integerArg(req.Get("count"), maximum)
	return index, count, err
}

func sealFragment(req js.Value) (any, error) {
	recipient, err := recipientArg(req)
	if err != nil {
		return nil, err
	}
	index, count, err := fragmentIndices(req)
	if err != nil {
		return nil, err
	}
	payload, err := bytesArg(req.Get("payload"), pqc.StorageFragmentBytes)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	envelope, err := pqc.SealStorageFragment(recipient, scopeArg(req), req.Get("room").String(), req.Get("digest").String(), index, count, payload)
	if err != nil {
		return nil, err
	}
	return envelopeValue(envelope), nil
}

func openFragment(req js.Value) (any, error) {
	identity, err := identityArg(req)
	if err != nil {
		return nil, err
	}
	index, count, err := fragmentIndices(req)
	if err != nil {
		return nil, err
	}
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	payload, err := pqc.OpenStorageFragment(identity, scopeArg(req), req.Get("room").String(), req.Get("digest").String(), index, count, envelope)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	return bytesValue(payload), nil
}

// Root descriptors are serialized by this same Go codec on all platforms.
// Browser code passes typed public references and raw owner-proof JSON bytes.
func fragmentRootArg(req js.Value) (pqc.FragmentRoot, error) {
	value := req.Get("root")
	if value.Type() != js.TypeObject || value.Get("v").Type() != js.TypeNumber || value.Get("v").Float() != 1 {
		return pqc.FragmentRoot{}, pqc.ErrEnvelope
	}
	size, err := integerArg(value.Get("size_bytes"), pqc.MaxEnvelopePayload)
	if err != nil {
		return pqc.FragmentRoot{}, err
	}
	proof, err := bytesArg(value.Get("proof"), 4096)
	if err != nil {
		return pqc.FragmentRoot{}, err
	}
	references := value.Get("references")
	maximum := (pqc.MaxEnvelopePayload + pqc.StorageFragmentBytes - 1) / pqc.StorageFragmentBytes
	if !js.Global().Get("Array").Call("isArray", references).Bool() || references.Length() < 1 || references.Length() > maximum {
		clear(proof)
		return pqc.FragmentRoot{}, pqc.ErrEnvelope
	}
	root := pqc.FragmentRoot{Version: 1, SHA256: value.Get("sha256").String(), SizeBytes: size, Proof: proof}
	for i := 0; i < references.Length(); i++ {
		ref := references.Index(i)
		root.References = append(root.References, storage.Reference{RoomID: ref.Get("room_id").String(), EventID: ref.Get("event_id").String()})
	}
	if !root.Valid(req.Get("room").String()) {
		clear(proof)
		return pqc.FragmentRoot{}, pqc.ErrEnvelope
	}
	return root, nil
}

func fragmentRootValue(root pqc.FragmentRoot) map[string]any {
	refs := make([]any, len(root.References))
	for i, ref := range root.References {
		refs[i] = map[string]any{"room_id": ref.RoomID, "event_id": ref.EventID}
	}
	return map[string]any{"v": root.Version, "sha256": root.SHA256, "size_bytes": root.SizeBytes, "references": refs, "proof": bytesValue(root.Proof)}
}

func sealFragmentRoot(req js.Value) (any, error) {
	recipient, err := recipientArg(req)
	if err != nil {
		return nil, err
	}
	root, err := fragmentRootArg(req)
	if err != nil {
		return nil, err
	}
	defer clear(root.Proof)
	envelope, err := pqc.SealStorageFragmentRoot(recipient, scopeArg(req), req.Get("room").String(), root)
	if err != nil {
		return nil, err
	}
	return envelopeValue(envelope), nil
}

func openFragmentRoot(req js.Value) (any, error) {
	identity, err := identityArg(req)
	if err != nil {
		return nil, err
	}
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	root, err := pqc.OpenStorageFragmentRoot(identity, scopeArg(req), req.Get("room").String(), envelope)
	if err != nil {
		return nil, err
	}
	defer clear(root.Proof)
	return fragmentRootValue(root), nil
}

func assembleFragments(req js.Value) (any, error) {
	root, err := fragmentRootArg(req)
	if err != nil {
		return nil, err
	}
	defer clear(root.Proof)
	values := req.Get("parts")
	if !js.Global().Get("Array").Call("isArray", values).Bool() || values.Length() != len(root.References) {
		return nil, pqc.ErrEnvelope
	}
	parts := make([][]byte, values.Length())
	for i := range parts {
		parts[i], err = bytesArg(values.Index(i), pqc.StorageFragmentBytes)
		if err != nil {
			return nil, err
		}
		defer clear(parts[i])
	}
	raw, err := pqc.AssembleStorageFragments(root, req.Get("room").String(), parts)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return bytesValue(raw), nil
}
