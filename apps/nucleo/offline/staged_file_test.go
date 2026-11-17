// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestStagedFileSurvivesRestartWithBoundedRandomReadsAndNoPlaintext(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("private immutable OS input "), 25000)
	file, err := store.StageFile(ctx, "owner", "upload", struct{ io.Reader }{bytes.NewReader(data)}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := again.OpenStagedFile(ctx, "owner", "upload", file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	for _, offset := range []int64{stagedBlockBytes - 4, 0, int64(len(data)) - 8, stagedBlockBytes + 5} {
		got := make([]byte, 19)
		n, err := reader.ReadAt(got, offset)
		want := data[offset:min(int64(len(data)), offset+19)]
		if !bytes.Equal(got[:n], want) || n != len(want) || n < 19 && !errors.Is(err, io.EOF) || n == 19 && err != nil {
			t.Fatal("random read changed staged input", offset, n, err)
		}
	}
	if _, err := again.OpenStagedFile(ctx, "other", "upload", file); !errors.Is(err, ErrStagedFile) {
		t.Fatal("cross-owner staged source accepted", err)
	}
	block := filepath.Join(dir, clave("owner", "upload", file.blockID(0))+".json")
	wire, err := os.ReadFile(block)
	if err != nil || bytes.Contains(wire, []byte("private immutable")) {
		t.Fatal("plaintext staged source", err)
	}
	wire[len(wire)-1] ^= 1
	if err := os.WriteFile(block, wire, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadAt(make([]byte, 4), 0); !errors.Is(err, ErrCacheCiphertext) {
		t.Fatal("tampered staged source accepted", err)
	}
	if err := again.DeleteStagedFile("owner", "upload", file); err != nil {
		t.Fatal(err)
	}
	if err := again.DeleteStagedFile("owner", "upload", file); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
	if _, err := again.OpenStagedFile(ctx, "owner", "upload", file); !errors.Is(err, ErrStagedFile) {
		t.Fatal("deleted source reopened", err)
	}
}

type stagedPatternReader struct {
	left, offset int64
	maxRead      int
}

func (r *stagedPatternReader) Read(out []byte) (int, error) {
	if len(out) > r.maxRead {
		r.maxRead = len(out)
	}
	if r.left == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(out)), r.left))
	for i := range n {
		out[i] = byte((r.offset + int64(i)) % 251)
	}
	r.offset += int64(n)
	r.left -= int64(n)
	return n, nil
}

func TestStagedStreamSurvivesRestartAbove32MiBWithoutUnboundedReads(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	const size = 33<<20 + 17
	input := &stagedPatternReader{left: size}
	file, err := store.StageStream(t.Context(), "owner", "upload", input, size+stagedBlockBytes)
	if err != nil || file.SizeBytes != size || input.maxRead > stagedBlockBytes {
		t.Fatal("unbounded or incomplete stream", file.SizeBytes, input.maxRead, err)
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, &stagedPatternReader{left: size}); err != nil {
		t.Fatal(err)
	}
	if file.SHA256 != hex.EncodeToString(sum.Sum(nil)) {
		t.Fatal("stream digest changed")
	}
	again, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := again.OpenStagedFile(t.Context(), "owner", "upload", file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	for _, offset := range []int64{0, stagedBlockBytes - 7, size - 9} {
		buffer := make([]byte, 9)
		if _, err := reader.ReadAt(buffer, offset); err != nil {
			t.Fatal(err)
		}
		for i, b := range buffer {
			if b != byte((offset+int64(i))%251) {
				t.Fatal("restart changed random stream bytes")
			}
		}
	}
}

type failingStageBackend struct {
	backend
	writes, failAt int
	failCleanup    bool
}

func (b *failingStageBackend) encrypted() bool {
	x, ok := b.backend.(interface{ encrypted() bool })
	return ok && x.encrypted()
}

func (b *failingStageBackend) put(key string, body []byte) error {
	b.writes++
	if b.writes == b.failAt {
		return io.ErrClosedPipe
	}
	return b.backend.put(key, body)
}

func (b *failingStageBackend) del(key string) error {
	if b.failCleanup {
		return io.ErrClosedPipe
	}
	return b.backend.del(key)
}

func TestStagedStreamFailureNeverPublishesAnEncryptedPrefixAsComplete(t *testing.T) {
	for _, change := range []string{"overflow", "read error", "cancelled", "block write", "completion write"} {
		t.Run(change, func(t *testing.T) {
			store, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var input io.Reader = &stagedPatternReader{left: stagedBlockBytes + 17}
			limit := int64(2 * stagedBlockBytes)
			switch change {
			case "overflow":
				limit = stagedBlockBytes
			case "read error":
				input = io.MultiReader(io.LimitReader(input, stagedBlockBytes), stageErrorReader{})
			case "cancelled":
				input = &cancelStageReader{reader: input, cancel: cancel}
			case "block write":
				store.b = &failingStageBackend{backend: store.b, failAt: 2}
			case "completion write":
				store.b = &failingStageBackend{backend: store.b, failAt: 3}
			}
			file, err := store.StageStream(ctx, "owner", "upload", input, limit)
			if err == nil || file != (StagedFile{}) {
				t.Fatal("failed stream returned a usable completion", file, err)
			}
			rows, err := store.List("owner", "upload")
			if err != nil || len(rows) != 0 {
				t.Fatal("failed capture retained a completion/prefix", len(rows), err)
			}
		})
	}
}

type stageErrorReader struct{}

func (stageErrorReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

type cancelStageReader struct {
	reader io.Reader
	cancel context.CancelFunc
	reads  int
}

func (r *cancelStageReader) Read(out []byte) (int, error) {
	r.reads++
	n, err := r.reader.Read(out)
	if r.reads == 2 {
		r.cancel()
	}
	return n, err
}

func TestStagedStreamRestartCannotAdoptAnInterruptedCapture(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	store.b = &failingStageBackend{backend: store.b, failAt: 2, failCleanup: true}
	if _, err := store.StageStream(t.Context(), "owner", "upload", &stagedPatternReader{left: stagedBlockBytes + 1}, 2*stagedBlockBytes); err == nil {
		t.Fatal("interrupted capture accepted")
	}
	again, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := again.List("owner", "upload")
	if err != nil || len(rows) != 1 {
		t.Fatal("interruption fixture did not retain one uncommitted block", len(rows), err)
	}
	id := strings.TrimSuffix(rows[0].ID, "_0")
	if id == rows[0].ID {
		t.Fatal("not a partial staged block")
	}
	partial := StagedFile{ID: id, SizeBytes: stagedBlockBytes, SHA256: strings.Repeat("1", 64)}
	if _, err := again.OpenStagedFile(t.Context(), "owner", "upload", partial); !errors.Is(err, ErrStagedFile) {
		t.Fatal("partial prefix became complete after restart", err)
	}
	wire, err := os.ReadFile(filepath.Join(dir, clave("owner", "upload", rows[0].ID)+".json"))
	if err != nil || !bytes.HasPrefix(wire, []byte(cacheMagic)) {
		t.Fatal("partial block was not protected by existing store encryption", err)
	}
	file, err := again.StageStream(t.Context(), "owner", "upload", bytes.NewReader([]byte("verified retry")), 1024)
	if err != nil || file.ID == id {
		t.Fatal("retry adopted old incomplete bytes", err)
	}
	if err := again.DeleteStagedFile("owner", "upload", partial); err != nil {
		t.Fatal("scoped partial cleanup", err)
	}
	reader, err := again.OpenStagedFile(t.Context(), "owner", "upload", file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	buffer := make([]byte, file.SizeBytes)
	if _, err := reader.ReadAt(buffer, 0); err != nil || string(buffer) != "verified retry" {
		t.Fatal("restart retry changed source", err)
	}
}

func TestStagedFileRejectsInexactOrCancelledCaptureBeforeCompletion(t *testing.T) {
	store, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{2, 4} {
		if _, err := store.StageFile(t.Context(), "owner", "upload", bytes.NewReader([]byte("abc")), size); err == nil {
			t.Fatal("inexact source accepted", size)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.StageFile(ctx, "owner", "upload", bytes.NewReader(nil), 0); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled source accepted", err)
	}
	rows, err := store.List("owner", "upload")
	if err != nil || len(rows) != 0 {
		t.Fatal("failed capture retained source or completion", len(rows), err)
	}
	file, err := store.StageFile(t.Context(), "owner", "upload", bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatal("empty file rejected", err)
	}
	reader, err := store.OpenStagedFile(t.Context(), "owner", "upload", file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if n, err := reader.ReadAt(make([]byte, 1), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatal("empty source lacks EOF", n, err)
	}
}
