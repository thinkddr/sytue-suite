// SPDX-License-Identifier: Apache-2.0
package eml

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLeerConLimiteChecksExactEOFAndNeverReturnsSuccessfulTruncation(t *testing.T) {
	raw := []byte("From: owner@example.test\r\nContent-Type: text/plain\r\n\r\nexact body")
	value, err := LeerConLimite(t.Context(), bytes.NewReader(raw), int64(len(raw)))
	if err != nil || value.Texto != "exact body" {
		t.Fatal("exact authenticated limit failed", err)
	}
	value, err = LeerConLimite(t.Context(), bytes.NewReader(raw), int64(len(raw)-1))
	if !errors.Is(err, ErrLimiteMensaje) || value.Texto != "" || len(value.Adjuntos) != 0 {
		t.Fatal("overrun became a successful partial message", err)
	}
	for _, limit := range []int64{0, -1, 1<<63 - 1} {
		if _, err := LeerConLimite(t.Context(), bytes.NewReader(raw), limit); !errors.Is(err, ErrLimiteInvalido) {
			t.Fatal("invalid MIME capability accepted", limit, err)
		}
	}
}

func TestLeerConLimiteCountsMultipartEpilogueAndCancellation(t *testing.T) {
	raw := []byte("Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nhello\r\n--x--\r\n" + strings.Repeat("epilogue", 4000))
	value, err := LeerConLimite(t.Context(), bytes.NewReader(raw), int64(len(raw)-1))
	if !errors.Is(err, ErrLimiteMensaje) || value.Texto != "" {
		t.Fatal("bytes hidden after final boundary bypassed limit", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = LeerConLimite(ctx, bytes.NewReader(raw), int64(len(raw))); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled MIME exposed data", err)
	}
}

type repeatedMIMEReader struct {
	remaining int64
	largest   int
}

func (r *repeatedMIMEReader) Read(p []byte) (int, error) {
	if len(p) > r.largest {
		r.largest = len(p)
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), r.remaining))
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= int64(n)
	return n, nil
}

func TestLeerConAdjuntosStreamsLargerAuthenticatedAttachmentWithoutMaterializingIt(t *testing.T) {
	const size = int64(64 << 20)
	header := "From: owner@example.test\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=large.bin\r\n\r\n"
	body := &repeatedMIMEReader{remaining: size}
	var received int64
	calls := 0
	value, err := LeerConAdjuntos(t.Context(), io.MultiReader(strings.NewReader(header), body), int64(len(header))+size, func(ctx context.Context, metadata Adjunto, source io.Reader) error {
		calls++
		if ctx.Err() != nil || metadata.Nombre != "large.bin" || metadata.Tipo != "application/octet-stream" || metadata.Datos != nil {
			t.Fatal("invalid attachment stream metadata")
		}
		n, err := io.CopyBuffer(io.Discard, source, make([]byte, 64<<10))
		received += n
		return err
	})
	if err != nil || calls != 1 || received != size || body.remaining != 0 || body.largest > 64<<10 || len(value.Adjuntos) != 1 || value.Adjuntos[0].Datos != nil {
		t.Fatal("large attachment was truncated or materialized", err, calls, received, body.largest)
	}
}

func TestLeerConAdjuntosCancellationInsideConsumerRejectsRemainingBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	header := "Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=cancel.bin\r\n\r\n"
	value, err := LeerConAdjuntos(ctx, io.MultiReader(strings.NewReader(header), &repeatedMIMEReader{remaining: 1 << 20}), 2<<20, func(_ context.Context, _ Adjunto, source io.Reader) error {
		cancel()
		var buf [1]byte
		n, err := source.Read(buf[:])
		if n != 0 || !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled attachment yielded bytes", n, err)
		}
		return err
	})
	if !errors.Is(err, context.Canceled) || len(value.Adjuntos) != 0 {
		t.Fatal("cancelled streamed MIME returned content", err)
	}
}

func TestLeerConLimiteBase64WhitespaceUsesBoundedIterativeReader(t *testing.T) {
	raw := "Content-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\n" + strings.Repeat(" ", 1<<20) + "eA=="
	value, err := LeerConLimite(t.Context(), strings.NewReader(raw), int64(len(raw)))
	if err != nil || value.Texto != "x" {
		t.Fatal("whitespace-only base64 prefix changed decoded data", err)
	}
}

type failingMIMEReader struct{ error error }

func (r failingMIMEReader) Read([]byte) (int, error) { return 0, r.error }
func TestLeerConLimiteNeverHidesAnInterruptedAuthenticatedStream(t *testing.T) {
	interruption := errors.New("artificial stream interrupted")
	for _, kind := range []string{"text/plain", "application/octet-stream"} {
		t.Run(kind, func(t *testing.T) {
			header := "Content-Type: " + kind + "\r\n\r\npartial"
			value, err := LeerConLimite(t.Context(), io.MultiReader(strings.NewReader(header), failingMIMEReader{interruption}), 4096)
			if !errors.Is(err, interruption) {
				t.Fatal("interrupted MIME became successful truncation", err, value.Texto)
			}
		})
	}
}
