// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStorageMessageRejectsDowngradeAndPreservesEnvelope(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := Seal(identity.Recipient(), []byte("wire-test"), []byte("private bytes"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeStorageMessage("record", envelope)
	if err != nil {
		t.Fatal(err)
	}
	wire, found, err := DecodeStorageMessage(raw)
	if err != nil || !found || wire.Kind != "record" || !bytes.Equal(wire.Envelope.Ciphertext, envelope.Ciphertext) {
		t.Fatalf("wire round trip: found=%v err=%v", found, err)
	}
	opened, err := Open(identity, []byte("wire-test"), wire.Envelope)
	if err != nil || string(opened) != "private bytes" {
		t.Fatalf("envelope changed: %v", err)
	}
	var content map[string]any
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	content["org.sytue.storage"] = map[string]any{"payload": "downgrade"}
	mixed, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := DecodeStorageMessage(mixed); !found || err == nil {
		t.Fatal("mixed classical/PQ message accepted")
	}
	invalid, _ := json.Marshal(map[string]any{StorageNamespace: nil})
	if _, found, err := DecodeStorageMessage(invalid); !found || err == nil {
		t.Fatal("invalid PQ packet fell back to legacy")
	}
	legacy, _ := json.Marshal(map[string]any{"org.sytue.storage": map[string]any{}})
	if _, found, err := DecodeStorageMessage(legacy); found || err != nil {
		t.Fatal("legacy packet was reclassified")
	}
	for _, kind := range []string{"fragment", "fragment-root"} {
		if _, err := EncodeStorageMessage(kind, envelope); err != nil {
			t.Fatalf("valid kind %s rejected: %v", kind, err)
		}
	}
	if _, err := EncodeStorageMessage("unknown", envelope); err == nil {
		t.Fatal("unknown packet kind accepted")
	}
	envelope.KeyID = "not-a-recipient-commitment"
	if _, err := EncodeStorageMessage("record", envelope); err == nil {
		t.Fatal("invalid recipient commitment accepted")
	}
}
