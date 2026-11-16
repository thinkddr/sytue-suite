// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func TestStoragePQIdentityPinsOwnerServerScopeRecoveryAndHybridProfile(t *testing.T) {
	client := &Client{cfg: Config{Tenant: "acme", Subject: "alice", Bootstrap: Bootstrap{StorageProduct: "notes", StorageIdentityVersion: storage.IsolatedIdentity, MatrixUserID: "@notes:storage.test", HomeserverURL: "http://proxy-a.test"}}}
	secret, err := client.newStoragePQSecret("recovery-key")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if secret.Profile != pqc.HybridProfile || secret.Homeserver != "storage.test" {
		t.Fatal("new key did not pin the maintained hybrid identity")
	}
	if _, identity, err := client.decodeStoragePQSecret(raw, "recovery-key"); err != nil || identity == nil {
		t.Fatal("valid hybrid identity could not be recovered", err)
	}
	// Matrix identifies the account/server, not one device's HTTP proxy URL.
	client.cfg.Bootstrap.HomeserverURL = "http://proxy-b.test"
	if _, identity, err := client.decodeStoragePQSecret(raw, "recovery-key"); err != nil || identity == nil {
		t.Fatal("transport proxy changed the recovery identity", err)
	}
	for _, fault := range []string{"tenant", "subject", "application", "matrix-user", "identity-version", "recovery-key", "profile", "recipient", "private-key", "key-id"} {
		t.Run(fault, func(t *testing.T) {
			value := secret
			switch fault {
			case "tenant":
				value.Scope.Tenant = "other"
			case "subject":
				value.Scope.Subject = "bob"
			case "application":
				value.Scope.Product = "mail"
			case "matrix-user":
				value.UserID = "@notes:other.test"
			case "identity-version":
				value.IdentityVersion = storage.LegacyIdentity
			case "recovery-key":
				value.RecoveryKeyID = "another-recovery-key"
			case "profile":
				value.Profile = pqc.LegacyProfile
			case "recipient":
				value.Recipient = "invalid"
			case "private-key":
				value.Identity = "invalid"
			case "key-id":
				value.KeyID = strings.Repeat("0", 64)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(encoded)
			if _, identity, err := client.decodeStoragePQSecret(encoded, "recovery-key"); err == nil || identity != nil {
				t.Fatal("foreign or downgraded private identity was accepted", fault)
			}
		})
	}
	scope := secret.Scope
	if !storage.Identifier(storagePQSecretID(scope, secret.KeyID)) || !storage.Identifier(storagePQPinID(scope)) {
		t.Fatal("SSSS secret names exceed the maintained vault contract")
	}
	other := scope
	other.Subject = "bob"
	if storagePQSecretID(other, secret.KeyID) == storagePQSecretID(scope, secret.KeyID) || storagePQPinID(other) == storagePQPinID(scope) {
		t.Fatal("different owners shared a secret namespace")
	}
}

func TestStoragePQClientUsesBoundParsedKeyCacheAndRetiresItOnClose(t *testing.T) {
	client := &Client{cfg: Config{Tenant: "acme", Subject: "alice", Bootstrap: Bootstrap{StorageProduct: "mail", StorageIdentityVersion: storage.IsolatedIdentity, MatrixUserID: "@mail:storage.test"}}}
	secret, err := client.newStoragePQSecret("recovery")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	_, first, err := client.decodeStoragePQSecret(raw, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := client.decodeStoragePQSecret(raw, "recovery")
	if err != nil || first != again {
		t.Fatal("native client rebuilt same validated key", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	// Inspect the decoder independently; a closed client itself cannot publish.
	_, after, err := client.storageKeyDecoder.Decode(raw, client.storagePQBinding("recovery"))
	if err != nil || after == first {
		t.Fatal("closed client retained its parsed key cache", err)
	}
	client.storageKeyDecoder.Reset()
}
