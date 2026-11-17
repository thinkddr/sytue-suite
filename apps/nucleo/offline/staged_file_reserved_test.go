//go:build !js

// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func reservedDescriptor(data []byte) StagedFile {
	sum := sha256.Sum256(data)
	return StagedFile{ID: "CapturedOriginalSource", SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
}

type inspectReservedReader struct {
	reader  io.Reader
	inspect func()
}

func (r inspectReservedReader) Read(p []byte) (int, error) { r.inspect(); return r.reader.Read(p) }

func TestReservedStagedFileRetainsOriginalIntentBeforeReadsAndRecoversLostACK(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	s, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("private reserved source "), 15000)
	file := reservedDescriptor(data)
	reads := 0
	source := inspectReservedReader{bytes.NewReader(data), func() {
		reads++
		var reservation stagedReservation
		found, e := s.Read("owner", "atomic", stagedReservationID(file), &reservation)
		if e != nil || !found || reservation.File != file || reservation.Owner != "owner" || reservation.Namespace != "atomic" {
			t.Fatal("source read preceded exact durable reservation", e)
		}
	}}
	got, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, source)
	if err != nil || got != file || reads == 0 {
		t.Fatal("reserved capture failed", err)
	}
	s, err = OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.StageFileReserved(t.Context(), "owner", "atomic", file, nil)
	if err != nil || got != file {
		t.Fatal("lost capture ACK required original source again", err)
	}
	if _, err := s.StageFileReserved(t.Context(), "other", "atomic", file, nil); err == nil {
		t.Fatal("reserved source crossed owner")
	}
	if _, err := s.StageFileReserved(t.Context(), "owner", "other", file, nil); err == nil {
		t.Fatal("reserved source crossed namespace")
	}
	var block []byte
	found, err := s.Read("owner", "atomic", file.blockID(0), &block)
	if err != nil || !found {
		t.Fatal(err)
	}
	block[0] ^= 1
	if err := s.Write("owner", "atomic", file.blockID(0), block); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, bytes.NewReader(data)); err == nil {
		t.Fatal("corrupt completed source was replaced/repaired")
	}
}

func TestReservedStagedFileNeverRebindsIncompleteOrCompletedSource(t *testing.T) {
	for _, fault := range []string{"interrupted", "changed-bytes", "missing-source"} {
		t.Run(fault, func(t *testing.T) {
			s, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
			if err != nil {
				t.Fatal(err)
			}
			data := bytes.Repeat([]byte("exact retained source"), 20000)
			file := reservedDescriptor(data)
			var input io.Reader = bytes.NewReader(data)
			switch fault {
			case "interrupted":
				input = io.MultiReader(io.LimitReader(input, stagedBlockBytes), stageErrorReader{})
			case "changed-bytes":
				changed := bytes.Clone(data)
				changed[0] ^= 1
				input = bytes.NewReader(changed)
			case "missing-source":
				input = nil
			}
			if got, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, input); err == nil || got != (StagedFile{}) {
				t.Fatal("incomplete/changed input completed", err)
			}
			var complete StagedFile
			if found, err := s.Read("owner", "atomic", file.ID, &complete); err != nil || found {
				t.Fatal("failed source retained usable completion", err)
			}
			changed := file
			changed.SHA256 = hex.EncodeToString(bytes.Repeat([]byte{1}, 32))
			reads := 0
			if _, err := s.StageFileReserved(t.Context(), "owner", "atomic", changed, inspectReservedReader{bytes.NewReader(data), func() { reads++ }}); err == nil || reads != 0 {
				t.Fatal("old ID accepted changed intent/source read", err)
			}
			got, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, bytes.NewReader(data))
			if err != nil || got != file {
				t.Fatal("original exact source could not resume", err)
			}
			if err := s.DeleteStagedFile("owner", "atomic", file); err != nil {
				t.Fatal(err)
			}
			var reservation stagedReservation
			if found, err := s.Read("owner", "atomic", stagedReservationID(file), &reservation); err != nil || found {
				t.Fatal("cleanup retained staging reservation", err)
			}
		})
	}
}

func TestReservedStagedFileDeniesForeignCompletionCancellationAndUnencryptedStore(t *testing.T) {
	s, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	var unencrypted Store
	if _, err := unencrypted.StageFileReserved(t.Context(), "owner", "atomic", reservedDescriptor(nil), bytes.NewReader(nil)); !errors.Is(err, ErrEncryptedStore) {
		t.Fatal("unencrypted staging admitted", err)
	}
	data := []byte("already captured")
	file, err := s.StageFile(t.Context(), "owner", "atomic", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, nil); err == nil {
		t.Fatal("unreserved completion became reserved authority")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	file = reservedDescriptor(data)
	if _, err := s.StageFileReserved(ctx, "owner", "atomic", file, bytes.NewReader(data)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled source capture continued", err)
	}
}

func TestReservedStagedFileDeniesInternalRecordIDCollisionsBeforeSourceIO(t *testing.T) {
	s, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"source_0", "source_reservation", "input_root_operation", "member_operation_1"} {
		file := reservedDescriptor(nil)
		file.ID = id
		reads := 0
		if _, err := s.StageFileReserved(t.Context(), "owner", "atomic", file, inspectReservedReader{bytes.NewReader(nil), func() { reads++ }}); err == nil || reads != 0 {
			t.Fatal("reserved source ID collided with internal journal rows", id, err)
		}
	}
}
