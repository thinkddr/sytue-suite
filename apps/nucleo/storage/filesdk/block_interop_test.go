// SPDX-License-Identifier: Apache-2.0
package filesdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type blockInteropCase struct {
	Product string
	Size    int
}

func blockFixturePlain(n int) []byte {
	raw := make([]byte, n)
	for i := range raw {
		raw[i] = byte(i*71 + 29)
	}
	return raw
}

// This compatibility fixture passes only artificial keys across native and
// actual Go/Wasm processes. Owner-attestation and publication authority are
// tested by the existing SDK lifecycle tests, not invented by this fixture.
func TestBlockCodecNativeWasmHybridInteroperability(t *testing.T) {
	root := os.Getenv("SYTUE_BLOCK_INTEROP_ROOT")
	role := os.Getenv("SYTUE_BLOCK_INTEROP_ROLE")
	if root == "" || role == "" {
		t.Skip("explicit remote native/Wasm vector exchange required")
	}
	products := storage.Products()
	cases := make([]blockInteropCase, 0, 2+len(products))
	cases = append(cases, blockInteropCase{"mail", 0}, blockInteropCase{"mail", int(storage.MaxCipherBlockBytes)})
	for _, product := range products {
		cases = append(cases, blockInteropCase{product, 256<<10 + 37})
	}
	write := func(name string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	const room = "!file-codec:interop.test"
	proof := []byte("{\"fixture_owner_attestation\":true}")
	var identity *pqc.Identity
	switch role {
	case "producer":
		var err error
		identity, err = pqc.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		write("fixture-key", []byte(identity.String()))
	case "consumer":
		raw := read("fixture-key")
		var err error
		identity, err = pqc.ParseIdentity(string(raw))
		clear(raw)
		if err != nil || identity.Profile() != pqc.HybridProfile {
			t.Fatal("hybrid fixture identity rejected", err)
		}
	default:
		t.Fatal("invalid fixture role")
	}
	for i, c := range cases {
		prefix := strconv.Itoa(i)
		scope := storage.Scope{Tenant: "interop", Subject: "owner", Product: c.Product}
		if role == "producer" {
			plain := blockFixturePlain(c.Size)
			block, ciphertext, err := PrepareBlock(scope, "object", "revision", i, bytes.NewReader(plain), int64(c.Size))
			clear(plain)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(block)
			if err != nil {
				t.Fatal(err)
			}
			record := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "manifest", Payload: payload}
			envelope, err := pqc.SealStorageRecord(identity.Recipient(), scope, room, record, proof)
			clear(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			write(prefix+".envelope", raw)
			write(prefix+".cipher", ciphertext)
			clear(ciphertext)
		} else {
			raw := read(prefix + ".envelope")
			var envelope pqc.Envelope
			if json.Unmarshal(raw, &envelope) != nil {
				t.Fatal("hybrid fixture envelope malformed")
			}
			record, actualProof, err := pqc.OpenStorageRecord(identity, scope, room, envelope)
			if err != nil || !bytes.Equal(proof, actualProof) || record.ObjectID != "object" || record.RevisionID != "revision" || record.Role != "manifest" {
				t.Fatal("native hybrid manifest did not open under Go/Wasm", err)
			}
			clear(actualProof)
			var block Block
			if json.Unmarshal(record.Payload, &block) != nil {
				t.Fatal("native block descriptor malformed")
			}
			clear(record.Payload)
			ciphertext := read(prefix + ".cipher")
			if err := DecryptBlockInPlace(scope, "object", "revision", i, block, ciphertext); err != nil {
				t.Fatal("native SDK block did not open under Go/Wasm", err)
			}
			expected := blockFixturePlain(c.Size)
			actualHash := sha256.Sum256(ciphertext)
			expectedHash := sha256.Sum256(expected)
			clear(expected)
			clear(ciphertext)
			if hex.EncodeToString(actualHash[:]) != hex.EncodeToString(expectedHash[:]) {
				t.Fatal("native/Wasm file content changed")
			}
			foreign := scope
			foreign.Subject = "different"
			if opened, _, err := pqc.OpenStorageRecord(identity, foreign, room, envelope); err == nil {
				clear(opened.Payload)
				t.Fatal("hybrid file manifest escaped owner scope")
			}
		}
	}
	t.Log("maintained SDK file cipher and hybrid private manifest agree across processes for all products, empty file and 8 MiB boundary")
}
