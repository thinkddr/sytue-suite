// SPDX-License-Identifier: Apache-2.0
// Package agecipher binds authorized input to the maintained age file format.
// Only public recipients are accepted by Seal. No key exchange, cipher, MAC or
// random-access decryption primitive is implemented by this adapter.
package agecipher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

const (
	maxHeaderBytes = 32 << 10
	maxFooterBytes = 128
)

type header struct {
	Version  int          `json:"v"`
	Binding  ingress.Task `json:"binding"`
	Size     int64        `json:"size"`
	Metadata []byte       `json:"metadata"`
}

type footer struct {
	SHA256 string `json:"sha256"`
}

func publicRecipient(plan ingress.KeyPlan) (age.Recipient, error) {
	if plan.Validate(plan.Scope, plan.Epoch) != nil {
		return nil, ingress.ErrContract
	}
	recipient, err := pqc.ParseRecipient(plan.Profile, plan.Recipient)
	if err != nil || recipient.String() != plan.Recipient {
		return nil, ingress.ErrContract
	}
	return recipient, nil
}

// ValidateKeyPlan lets enrollment and delivery share the same canonical public
// recipient parser. The backend never receives an age private identity.
func ValidateKeyPlan(plan ingress.KeyPlan) error { _, err := publicRecipient(plan); return err }

type contextReader struct {
	context.Context
	io.Reader
}

func (r contextReader) Read(out []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(out)
}

type countedHashWriter struct {
	writer io.Writer
	hash   io.Writer
	count  int64
}

func (w *countedHashWriter) Write(raw []byte) (int, error) {
	n, err := w.writer.Write(raw)
	if n > 0 {
		_, _ = w.hash.Write(raw[:n])
		w.count += int64(n)
	}
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	return n, err
}

// Seal streams the original body into age. The encrypted frame contains its
// domain metadata, exact plaintext digest and all routing bindings. Neither the
// returned descriptor nor the backend's durable plan exposes those contents.
func Seal(ctx context.Context, destination io.Writer, plan ingress.KeyPlan, id, kind string, metadata []byte, source io.Reader, size int64) (ingress.Task, error) {
	if ctx == nil || destination == nil || source == nil || size < 0 || size > storage.MaxFileBytes || len(metadata) > ingress.MaxMetadataBytes || !json.Valid(metadata) {
		return ingress.Task{}, ingress.ErrContract
	}
	recipient, err := publicRecipient(plan)
	if err != nil {
		return ingress.Task{}, err
	}
	task := ingress.Task{Version: 1, Scope: plan.Scope, Epoch: plan.Epoch, ID: id, Kind: kind, KeyID: plan.KeyID, KeyGeneration: plan.Generation}
	if err := task.ValidateBinding(plan.Scope, plan.Epoch); err != nil {
		return ingress.Task{}, err
	}
	if err := ctx.Err(); err != nil {
		return ingress.Task{}, err
	}
	digest := sha256.New()
	ciphertext := &countedHashWriter{writer: destination, hash: digest}
	encrypted, err := age.Encrypt(ciphertext, recipient)
	if err != nil {
		return ingress.Task{}, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = encrypted.Close()
		}
	}()
	if err := writeFrame(ctx, encrypted, task, metadata, source, size); err != nil {
		return ingress.Task{}, err
	}

	if err := encrypted.Close(); err != nil {
		return ingress.Task{}, err
	}
	closed = true
	task.Cipher = storage.StreamSourceRow{Kind: "ingress", ID: id, Size: ciphertext.count, SHA256: hex.EncodeToString(digest.Sum(nil))}
	if err := ctx.Err(); err != nil {
		return ingress.Task{}, err
	}
	return task, task.Validate(plan.Scope, plan.Epoch)
}

type contextReaderAt struct {
	context.Context
	io.ReaderAt
}

func (r contextReaderAt) ReadAt(out []byte, offset int64) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.ReaderAt.ReadAt(out, offset)
}

// Open verifies the entire bounded ciphertext and decrypted frame before it
// returns metadata/body to a domain executor. The body is random-access through
// age's maintained ReaderAt; no plaintext spool or whole MIME allocation exists.
func Open(ctx context.Context, source io.ReaderAt, task ingress.Task, identity age.Identity) ([]byte, *io.SectionReader, error) {
	_, recipient, identityErr := pqc.PublicIdentity(identity)
	if ctx == nil || source == nil || identityErr != nil || task.Validate(task.Scope, task.Epoch) != nil || ingress.PublicFingerprint(recipient) != task.KeyID {
		return nil, nil, ingress.ErrContract
	}
	safe := contextReaderAt{Context: ctx, ReaderAt: source}
	if err := verifyCiphertext(safe, task.Cipher); err != nil {
		return nil, nil, err
	}
	reader, plainSize, err := age.DecryptReaderAt(safe, task.Cipher.Size, identity)
	if err != nil {
		return nil, nil, ingress.ErrContract
	}
	h, offset, err := readHeader(reader, task)
	if err != nil {
		return nil, nil, err
	}
	if plainSize < offset+h.Size+1 || plainSize > offset+h.Size+maxFooterBytes {
		clear(h.Metadata)
		return nil, nil, ingress.ErrContract
	}
	if err := verifyBody(ctx, reader, offset, h.Size); err != nil {
		clear(h.Metadata)
		return nil, nil, err
	}
	return h.Metadata, io.NewSectionReader(contextReaderAt{Context: ctx, ReaderAt: reader}, offset, h.Size), nil
}

func verifyCiphertext(source io.ReaderAt, row storage.StreamSourceRow) error {
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	n, err := io.CopyBuffer(hash, io.NewSectionReader(source, 0, row.Size), buffer)
	if err != nil {
		return err
	}
	if n != row.Size || hex.EncodeToString(hash.Sum(nil)) != row.SHA256 {
		return ingress.ErrContract
	}
	var extra [1]byte
	nExtra, err := source.ReadAt(extra[:], row.Size)
	if nExtra != 0 || !errors.Is(err, io.EOF) {
		return ingress.ErrContract
	}
	return nil
}

func readHeader(reader io.ReaderAt, expected ingress.Task) (header, int64, error) {
	buffer := make([]byte, maxHeaderBytes)
	defer clear(buffer)
	n, err := reader.ReadAt(buffer, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return header{}, 0, ingress.ErrContract
	}
	end := bytes.IndexByte(buffer[:n], '\n')
	if end < 0 {
		return header{}, 0, ingress.ErrContract
	}
	var h header
	if json.Unmarshal(buffer[:end], &h) != nil || h.Version != 1 || h.Binding.Cipher != (storage.StreamSourceRow{}) || h.Size < 0 || h.Size > storage.MaxFileBytes || len(h.Metadata) > ingress.MaxMetadataBytes || !json.Valid(h.Metadata) {
		return header{}, 0, ingress.ErrContract
	}
	expected.Cipher = storage.StreamSourceRow{}
	if h.Binding != expected {
		clear(h.Metadata)
		return header{}, 0, ingress.ErrContract
	}
	return h, int64(end + 1), nil
}

func verifyBody(ctx context.Context, reader io.ReaderAt, offset, size int64) error {
	digest := sha256.New()
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	n, err := io.CopyBuffer(digest, io.NewSectionReader(contextReaderAt{Context: ctx, ReaderAt: reader}, offset, size), buffer)
	if err != nil {
		return err
	}
	if n != size {
		return ingress.ErrContract
	}
	trailer := make([]byte, maxFooterBytes)
	defer clear(trailer)
	nTail, err := reader.ReadAt(trailer, offset+size)
	if !errors.Is(err, io.EOF) || nTail == 0 || trailer[nTail-1] != '\n' {
		return ingress.ErrContract
	}
	var f footer
	if json.Unmarshal(trailer[:nTail], &f) != nil || f.SHA256 != hex.EncodeToString(digest.Sum(nil)) {
		return ingress.ErrContract
	}
	return nil
}

func writeFrame(ctx context.Context, encrypted io.Writer, task ingress.Task, metadata []byte, source io.Reader, size int64) error {
	raw, err := json.Marshal(header{Version: 1, Binding: task, Size: size, Metadata: metadata})
	if err != nil || len(raw)+1 > maxHeaderBytes {
		clear(raw)
		return ingress.ErrContract
	}
	raw = append(raw, '\n')
	_, err = encrypted.Write(raw)
	clear(raw)
	if err != nil {
		return err
	}
	plainHash := sha256.New()
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	input := contextReader{Context: ctx, Reader: source}
	n, err := io.CopyBuffer(io.MultiWriter(encrypted, plainHash), io.LimitReader(input, size), buffer)
	if err != nil {
		return err
	}
	if n != size {
		return io.ErrUnexpectedEOF
	}
	var extra [1]byte
	nExtra, err := input.Read(extra[:])
	clear(extra[:])
	if nExtra != 0 || !errors.Is(err, io.EOF) {
		return ingress.ErrContract
	}
	if err := json.NewEncoder(encrypted).Encode(footer{SHA256: hex.EncodeToString(plainHash.Sum(nil))}); err != nil {
		return err
	}
	return nil
}
