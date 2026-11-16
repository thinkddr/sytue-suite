// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestForwardSourceCommitmentChecksExactEOFAndOriginalScopeWithoutRetainingMIME(t *testing.T) {
	identity := fixtureIdentity()
	port := sourcePort{id: strings.Repeat("a", 64), identity: identity}
	sources := NewSources()
	original := []byte("exact original MIME\r\n")
	source := &forwardSource{reader: bytes.NewReader(original), size: int64(len(original)), hash: sha256.New()}
	id, release, err := sources.Add(t.Context(), port, identity, source, source.size)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = sources.FinalizeSource(t.Context(), port, identity, id); !errors.Is(err, ErrRejected) {
		t.Fatal("unfinished source finalized", err)
	}
	data, err := sources.Read(t.Context(), port, identity, id, 0, len(original))
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal(err)
	}
	clear(data)
	foreign := identity
	foreign.Binding.DeviceID = "FOREIGN"
	if _, err = sources.FinalizeSource(t.Context(), port, foreign, id); !errors.Is(err, ErrRejected) {
		t.Fatal("foreign finalizer admitted", err)
	}
	checksum, err := sources.FinalizeSource(t.Context(), port, identity, id)
	expected := sha256.Sum256(original)
	if err != nil || checksum != hex.EncodeToString(expected[:]) {
		t.Fatal("original EOF/hash not fixed", checksum, err)
	}
	extra := &forwardSource{reader: bytes.NewReader(append(bytes.Clone(original), 'x')), size: int64(len(original)), hash: sha256.New()}
	extraID, drop, _ := sources.Add(t.Context(), port, identity, extra, extra.size)
	defer drop()
	value, err := sources.Read(t.Context(), port, identity, extraID, 0, len(original))
	clear(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.FinalizeSource(t.Context(), port, identity, extraID); !errors.Is(err, ErrRejected) {
		t.Fatal("extra plaintext beyond declared size accepted", err)
	}
	clear(original)
}

func TestForwardSourceNeverBuffersOrRewindsPlaintext(t *testing.T) {
	source := &forwardSource{reader: strings.NewReader("exact")}
	out := make([]byte, 3)
	if n, err := source.ReadAt(out, 0); n != 3 || err != nil || string(out) != "exa" {
		t.Fatal(n, err)
	}
	if _, err := source.ReadAt(out, 0); !errors.Is(err, ErrRejected) {
		t.Fatal("rewind retained plaintext", err)
	}
	if n, err := source.ReadAt(out, 3); n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("short original accepted", n, err)
	}
}
