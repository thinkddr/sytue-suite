// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageKeyCodecPreservesV1FieldsAndImmutableBinding(t *testing.T) {
	binding := StorageKeyBinding{Scope: storage.Scope{Tenant: "équipe", Subject: "person<>&💻", Product: "drive"}, IdentityVersion: storage.IsolatedIdentity, Homeserver: "storage.test", UserID: "@owner:storage.test", RecoveryKeyID: "recovery"}
	secret, err := NewStorageKeySecret(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"v", "scope", "identity_version", "homeserver", "user_id", "recovery_key_id", "profile", "key_id", "recipient", "identity"} {
		if len(fields[name]) == 0 {
			t.Fatal("v1 secret field disappeared", name)
		}
	}
	if len(fields) != 10 {
		t.Fatal("v1 secret wire layout changed", len(fields))
	}
	decoded, identity, err := DecodeStorageKeySecret(raw, binding)
	if err != nil || identity == nil || decoded != secret || identity.Recipient().String() != secret.Recipient {
		t.Fatal("shared native/Wasm secret recovery changed the v1 identity", err)
	}
	if !ValidStorageKeyPin(secret.Pin(), binding.RecoveryKeyID) || ValidStorageKeyPin(secret.Pin(), "another-key") {
		t.Fatal("pin lost its recovery binding")
	}
	for _, fault := range []string{"scope", "version", "server", "user", "recovery"} {
		t.Run(fault, func(t *testing.T) {
			changed := binding
			switch fault {
			case "scope":
				changed.Scope.Subject = "bob"
			case "version":
				changed.IdentityVersion = storage.LegacyIdentity
			case "server":
				changed.Homeserver = "elsewhere.test"
			case "user":
				changed.UserID = "@elsewhere:storage.test"
			case "recovery":
				changed.RecoveryKeyID = "another-recovery"
			}
			if _, identity, err := DecodeStorageKeySecret(raw, changed); err == nil || identity != nil {
				t.Fatal("foreign binding opened a private identity", fault)
			}
		})
	}
	pinName := StorageKeyPinID(binding.Scope)
	keyName := StorageKeySecretID(binding.Scope, strings.Repeat("1", 64))
	if !storage.Identifier(pinName) || !storage.Identifier(keyName) || !strings.HasPrefix(pinName, "pqc_pin_") || !strings.HasPrefix(keyName, "pqc_key_") {
		t.Fatal("SSSS names changed protocol or exceed vault limits")
	}
	different := binding.Scope
	different.Product = "mail"
	if StorageKeyPinID(different) == pinName || StorageKeySecretID(different, strings.Repeat("1", 64)) == keyName {
		t.Fatal("different applications shared a key namespace")
	}
}

func TestStorageKeyCodecRejectsClassicalPrivateIdentityInHybridSlot(t *testing.T) {
	binding := StorageKeyBinding{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}, IdentityVersion: storage.IsolatedIdentity, Homeserver: "storage.test", UserID: "@owner:storage.test", RecoveryKeyID: "recovery"}
	secret, err := NewStorageKeySecret(binding)
	if err != nil {
		t.Fatal(err)
	}
	secret.Profile = LegacyProfile
	raw, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if _, identity, err := DecodeStorageKeySecret(raw, binding); err == nil || identity != nil {
		t.Fatal("a changed profile silently downgraded the persisted key")
	}
	if _, err := NewStorageKeySecret(StorageKeyBinding{}); err == nil {
		t.Fatal("a key was created without an immutable binding")
	}
}
