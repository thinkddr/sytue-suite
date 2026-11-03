// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"io"
	"testing"

	"filippo.io/age"
)

func TestHybridIdentityCanonicalRecoveryAndAuthenticatedStreaming(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if identity.Profile() != HybridProfile {
		t.Fatal("new encryption identity did not select the hybrid profile")
	}
	recovered, err := ParseIdentity(identity.String())
	if err != nil || recovered.Profile() != HybridProfile || recovered.Recipient().String() != identity.Recipient().String() {
		t.Fatal("hybrid identity did not recover exactly", err)
	}
	recipient, err := ParseRecipient(HybridProfile, identity.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("synthetic private suite content / 漢字\n"), 6000)
	var sealed bytes.Buffer
	writer, err := age.Encrypt(&sealed, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.Bytes(), plain[:100]) {
		t.Fatal("private content reached the encrypted stream")
	}
	reader, size, err := age.DecryptReaderAt(bytes.NewReader(sealed.Bytes()), int64(sealed.Len()), recovered)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(io.NewSectionReader(reader, 0, size))
	defer clear(actual)
	if err != nil || !bytes.Equal(actual, plain) {
		t.Fatal("hybrid recovered content changed", err)
	}
	corrupted := bytes.Clone(sealed.Bytes())
	corrupted[len(corrupted)/2] ^= 1
	reader, size, err = age.DecryptReaderAt(bytes.NewReader(corrupted), int64(len(corrupted)), recovered)
	if err == nil {
		_, err = io.Copy(io.Discard, io.NewSectionReader(reader, 0, size))
	}
	if err == nil {
		t.Fatal("corrupted hybrid ciphertext was accepted")
	}
	other, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = age.DecryptReaderAt(bytes.NewReader(sealed.Bytes()), int64(sealed.Len()), other); err == nil {
		t.Fatal("another identity opened the hybrid ciphertext")
	}
	if _, err = ParseRecipient(LegacyProfile, identity.Recipient().String()); err == nil {
		t.Fatal("hybrid recipient silently downgraded to a classical profile")
	}
}

func TestLegacyIdentityRemainsReadableWithoutChangingItsProfile(t *testing.T) {
	legacy, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ParseIdentity(legacy.String())
	if err != nil || restored.Profile() != LegacyProfile || restored.Recipient().String() != legacy.Recipient().String() {
		t.Fatal("legacy identity changed during recovery", err)
	}
	if _, err = ParseRecipient(HybridProfile, legacy.Recipient().String()); err == nil {
		t.Fatal("classical recipient mislabeled as post-quantum")
	}
	if _, err = ParseIdentity("not a private identity"); err == nil {
		t.Fatal("invalid private identity accepted")
	}
}
