// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func keyDecoderFixture(t testing.TB) (StorageKeyBinding, StorageKeySecret, []byte) {
	t.Helper()
	binding := StorageKeyBinding{Scope: storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}, IdentityVersion: storage.IsolatedIdentity, Homeserver: "matrix.test", UserID: "@owner:matrix.test", RecoveryKeyID: "owner-recovery"}
	secret, err := NewStorageKeySecret(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	return binding, secret, raw
}

func TestStorageKeyDecoderReusesOnlyExactOwnedBytesAndBinding(t *testing.T) {
	binding, secret, raw := keyDecoderFixture(t)
	var decoder StorageKeyDecoder
	defer decoder.Reset()
	supplied := bytes.Clone(raw)
	got, identity, err := decoder.Decode(supplied, binding)
	if err != nil || got != secret {
		t.Fatal("valid key rejected", err)
	}
	clear(supplied)
	again, same, err := decoder.Decode(raw, binding)
	if err != nil || again != secret || same != identity {
		t.Fatal("exact secure-store snapshot was needlessly reconstructed", err)
	}
	for _, fault := range []string{"tenant", "subject", "product", "version", "homeserver", "user", "recovery"} {
		t.Run(fault, func(t *testing.T) {
			if _, _, err := decoder.Decode(raw, binding); err != nil {
				t.Fatal(err)
			}
			other := binding
			switch fault {
			case "tenant":
				other.Scope.Tenant = "other"
			case "subject":
				other.Scope.Subject = "other"
			case "product":
				other.Scope.Product = "drive"
			case "version":
				other.IdentityVersion = storage.LegacyIdentity
			case "homeserver":
				other.Homeserver = "other.test"
			case "user":
				other.UserID = "@other:matrix.test"
			case "recovery":
				other.RecoveryKeyID = "other"
			}
			if _, key, err := decoder.Decode(raw, other); err == nil || key != nil {
				t.Fatal("warm decoder accepted crossed binding", fault)
			}
		})
	}
	second, err := NewStorageKeySecret(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw2, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = decoder.Decode(raw, binding)
	changed, key2, err := decoder.Decode(raw2, binding)
	if err != nil || changed.KeyID != second.KeyID || key2.Recipient().String() != second.Recipient || key2 == identity {
		t.Fatal("valid changed secure-store bytes reused old key", err)
	}
}

func TestStorageKeyDecoderNeverMasksCorruptionOrRetainsOwnedBytesAfterReset(t *testing.T) {
	binding, secret, raw := keyDecoderFixture(t)
	for _, fault := range []string{"identity", "recipient", "key-id", "profile", "version", "binding", "malformed", "empty", "oversize"} {
		t.Run(fault, func(t *testing.T) {
			var decoder StorageKeyDecoder
			defer decoder.Reset()
			_, _, err := decoder.Decode(raw, binding)
			if err != nil {
				t.Fatal(err)
			}
			bad := secret
			switch fault {
			case "identity":
				bad.Identity = "invalid"
			case "recipient":
				bad.Recipient = "invalid"
			case "key-id":
				bad.KeyID = "invalid"
			case "profile":
				bad.Profile = LegacyProfile
			case "version":
				bad.Version = 2
			case "binding":
				bad.Scope.Subject = "other"
			}
			encoded, _ := json.Marshal(bad)
			switch fault {
			case "malformed":
				encoded = []byte("{")
			case "empty":
				encoded = nil
			case "oversize":
				encoded = bytes.Repeat([]byte(" "), 16385)
			}
			retained := decoder.raw
			if _, identity, err := decoder.Decode(encoded, binding); err == nil || identity != nil {
				t.Fatal("changed secure-store corruption was hidden by key cache", fault)
			}
			if !bytes.Equal(retained, make([]byte, len(retained))) || decoder.identity != nil || decoder.raw != nil {
				t.Fatal("failed decode retained owned key cache")
			}
		})
	}
	var decoder StorageKeyDecoder
	if _, _, err := decoder.Decode(raw, binding); err != nil {
		t.Fatal(err)
	}
	retained := decoder.raw
	decoder.Reset()
	if !bytes.Equal(retained, make([]byte, len(retained))) || decoder.identity != nil || decoder.raw != nil || decoder.secret != (StorageKeySecret{}) {
		t.Fatal("retirement retained parsed key or owned serialization")
	}
}

func TestStorageKeyDecoderConcurrentReplacementStillOpensOnlyItsOwnEnvelope(t *testing.T) {
	binding, first, raw1 := keyDecoderFixture(t)
	second, err := NewStorageKeySecret(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := json.Marshal(second)
	inputs := [][]byte{raw1, raw2}
	secrets := []StorageKeySecret{first, second}
	envelopes := make([]Envelope, 2)
	payload := []byte(`{"integer":900719925474099312345,"private":"unchanged"}`)
	context := []byte("artificial-interop-context")
	for i, secret := range secrets {
		recipient, err := ParseRecipient(secret.Profile, secret.Recipient)
		if err != nil {
			t.Fatal(err)
		}
		envelopes[i], err = Seal(recipient, context, payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	var decoder StorageKeyDecoder
	defer decoder.Reset()
	var jobs sync.WaitGroup
	for worker := range 8 {
		jobs.Go(func() {
			for turn := range 24 {
				i := (worker + turn) % 2
				secret, identity, err := decoder.Decode(inputs[i], binding)
				if err != nil || secret.KeyID != secrets[i].KeyID {
					t.Error("concurrent key cache crossed immutable bytes")
					return
				}
				opened, err := Open(identity, context, envelopes[i])
				if err != nil || !bytes.Equal(opened, payload) {
					t.Error("concurrent parsed key changed hybrid decryption")
					return
				}
				clear(opened)
				if turn%7 == 0 {
					decoder.Reset()
				}
			}
		})
	}
	jobs.Wait()
}

func BenchmarkStorageKeyDecoder(b *testing.B) {
	binding, _, raw := keyDecoderFixture(b)
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprint("cached=", cached), func(b *testing.B) {
			b.ReportAllocs()
			var decoder StorageKeyDecoder
			defer decoder.Reset()
			if cached {
				if _, _, err := decoder.Decode(raw, binding); err != nil {
					b.Fatal(err)
				}
			}
			for b.Loop() {
				var err error
				if cached {
					_, _, err = decoder.Decode(raw, binding)
				} else {
					_, _, err = DecodeStorageKeySecret(raw, binding)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
