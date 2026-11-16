// SPDX-License-Identifier: Apache-2.0
package filesdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestBlockCodecKeepsExactContextAndReleasesOnlyAuthenticatedPlaintext(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	plain := bytes.Repeat([]byte("client-only-file-data"), 300)
	block, ciphertext, err := PrepareBlock(scope, "object", "revision", 7, bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(ciphertext)
	if bytes.Equal(ciphertext, plain) || bytes.Contains(ciphertext, []byte(block.File.Key.Key)) {
		t.Fatal("opaque provider bytes exposed plaintext or key")
	}
	sum := sha256.Sum256(ciphertext)
	if hex.EncodeToString(sum[:]) != block.Blob.SHA256 {
		t.Fatal("receipt did not commit actual SDK wire")
	}
	restored := bytes.Clone(ciphertext)
	if err := DecryptBlockInPlace(scope, "object", "revision", 7, block, restored); err != nil || !bytes.Equal(restored, plain) {
		t.Fatal("maintained SDK file roundtrip", err)
	}
	clear(restored)
	for _, change := range []string{"tenant", "subject", "product", "object", "revision", "part", "size", "ciphertext", "wire-hash", "version", "key"} {
		t.Run(change, func(t *testing.T) {
			actual := bytes.Clone(ciphertext)
			descriptor := block
			expected := scope
			object, revision, part := "object", "revision", 7
			switch change {
			case "tenant":
				expected.Tenant = "foreign"
			case "subject":
				expected.Subject = "bob"
			case "product":
				expected.Product = "drive"
			case "object":
				object = "foreign"
			case "revision":
				revision = "foreign"
			case "part":
				part = 8
			case "size":
				actual = actual[:len(actual)-1]
			case "ciphertext":
				actual[len(actual)/2] ^= 1
			case "wire-hash":
				descriptor.File.Hashes.SHA256 = base64.RawStdEncoding.EncodeToString(make([]byte, 32))
			case "version":
				descriptor.File.Version = "v1"
			case "key":
				descriptor.File.Key.Key = "invalid"
			}
			if err := DecryptBlockInPlace(expected, object, revision, part, descriptor, actual); err == nil {
				t.Fatal("changed block authenticated", change)
			}
			if bytes.IndexFunc(actual, func(r rune) bool { return r != 0 }) != -1 {
				t.Fatal("failed block retained data", change)
			}
		})
	}
}

type blockReadWitness struct{ calls int }

func (w *blockReadWitness) Read([]byte) (int, error) { w.calls++; return 0, io.EOF }
func TestBlockCodecRejectsInvalidScopeBeforeReadingAndNeverTruncates(t *testing.T) {
	witness := &blockReadWitness{}
	if _, data, err := PrepareBlock(storage.Scope{}, "object", "revision", 0, witness, 1); err == nil || len(data) != 0 || witness.calls != 0 {
		t.Fatal("invalid file scope reached source")
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	for _, size := range []int64{-1, storage.MaxCipherBlockBytes + 1} {
		if _, data, err := PrepareBlock(scope, "object", "revision", 0, witness, size); err == nil || len(data) != 0 {
			t.Fatal("unbounded block read")
		}
	}
	if witness.calls != 0 {
		t.Fatal("invalid size reached source")
	}
	if _, data, err := PrepareBlock(scope, "object", "revision", 0, strings.NewReader("too-long"), 3); err == nil || len(data) != 0 {
		t.Fatal("source was silently truncated")
	}
	if _, data, err := PrepareBlock(scope, "object", "revision", 0, strings.NewReader("short"), 10); err == nil || len(data) != 0 {
		t.Fatal("missing source bytes accepted")
	}
	block, data, err := PrepareBlock(scope, "object", "revision", 0, strings.NewReader(""), 0)
	if err != nil || block.Blob.SizeBytes != 0 || DecryptBlockInPlace(scope, "object", "revision", 0, block, data) != nil {
		t.Fatal("empty file block changed", err)
	}
}
