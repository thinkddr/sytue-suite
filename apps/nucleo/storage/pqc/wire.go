// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"encoding/hex"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const StorageNamespace = "org.sytue.storage_pq"

type StorageWire struct {
	Version  int      `json:"v"`
	Kind     string   `json:"kind"`
	Envelope Envelope `json:"envelope"`
}

type StorageMessage struct {
	MsgType string      `json:"msgtype"`
	Body    string      `json:"body"`
	Wire    StorageWire `json:"org.sytue.storage_pq"`
}

func NewStorageMessage(kind string, envelope Envelope) StorageMessage {
	return StorageMessage{MsgType: "m.text", Body: "", Wire: StorageWire{Version: 1, Kind: kind, Envelope: envelope}}
}

func StorageMessageFits(content StorageMessage) bool {
	raw, err := json.Marshal(content)
	return err == nil && len(raw) <= storage.MaxRecordBytes
}

// DecodeStorageMessage is shared by native and web; neither adapter implements
// a second base64/envelope codec or interprets a malformed PQ packet as legacy.
// Event sender, room and Matrix encryption provenance remain transport checks.
func DecodeStorageMessage(raw []byte) (StorageWire, bool, error) {
	var content map[string]json.RawMessage
	if json.Unmarshal(raw, &content) != nil {
		return StorageWire{}, false, ErrEnvelope
	}
	packet, found := content[StorageNamespace]
	if !found {
		return StorageWire{}, false, nil
	}
	if len(raw) > storage.MaxRecordBytes {
		return StorageWire{}, true, ErrEnvelope
	}
	if _, legacy := content["org.sytue.storage"]; legacy {
		return StorageWire{}, true, ErrEnvelope
	}
	if _, legacy := content["org.sytue.storage_auth"]; legacy {
		return StorageWire{}, true, ErrEnvelope
	}
	var wire StorageWire
	if json.Unmarshal(packet, &wire) != nil || wire.Version != 1 || wire.Envelope.Version != EnvelopeVersion || wire.Envelope.Profile != HybridProfile || len(wire.Envelope.Ciphertext) == 0 || len(wire.Envelope.Ciphertext) > MaxEnvelopeCiphertext {
		return StorageWire{}, true, ErrEnvelope
	}
	key, err := hex.DecodeString(wire.Envelope.KeyID)
	if err != nil || len(key) != 32 || hex.EncodeToString(key) != wire.Envelope.KeyID {
		return StorageWire{}, true, ErrEnvelope
	}
	switch wire.Kind {
	case "record", "fragment", "fragment-root":
	default:
		return StorageWire{}, true, ErrEnvelope
	}
	return wire, true, nil
}

func EncodeStorageMessage(kind string, envelope Envelope) ([]byte, error) {
	raw, err := json.Marshal(NewStorageMessage(kind, envelope))
	if err != nil {
		return nil, err
	}
	if _, found, err := DecodeStorageMessage(raw); err != nil || !found {
		return nil, ErrEnvelope
	}
	return raw, nil
}
