// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package kernelops

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func init() {
	Operations["storage.message.encode"] = encodeStorageMessage
	Operations["storage.message.fits"] = storageMessageFits
	Operations["storage.message.decode"] = decodeStorageMessage
}

func encodeStorageMessage(req js.Value) (any, error) {
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	raw, err := pqc.EncodeStorageMessage(req.Get("kind").String(), envelope)
	if err != nil {
		return nil, err
	}
	return bytesValue(raw), nil
}

func decodeStorageMessage(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("message"), storage.MaxRecordBytes)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	wire, found, err := pqc.DecodeStorageMessage(raw)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"found": false}, nil
	}
	return map[string]any{"found": true, "kind": wire.Kind, "envelope": envelopeValue(wire.Envelope)}, nil
}

func storageMessageFits(req js.Value) (any, error) {
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	return pqc.StorageMessageFits(pqc.NewStorageMessage(req.Get("kind").String(), envelope)), nil
}
