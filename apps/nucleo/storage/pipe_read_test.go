// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestPipeReadStreamsOneChunkAndCloseJoinsItsCancelledWriter(t *testing.T) {
	done := make(chan struct{})
	body := PipeRead(t.Context(), func() { close(done) }, func(ctx context.Context, out io.Writer) error {
		chunk := bytes.Repeat([]byte{7}, 4096)
		for range 100000 {
			if _, err := out.Write(chunk); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	first := make([]byte, 4096)
	if _, err := io.ReadFull(body, first); err != nil || first[0] != 7 {
		t.Fatal("first streaming chunk", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("closed reader leaked its writer/work lease")
	}
}

func TestPipeReadPropagatesProviderFailureAndParentCancellation(t *testing.T) {
	failure := errors.New("authenticated provider is unavailable")
	body := PipeRead(t.Context(), nil, func(_ context.Context, out io.Writer) error {
		_, _ = io.WriteString(out, "verified chunk")
		return failure
	})
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if string(raw) != "verified chunk" || !errors.Is(err, failure) {
		t.Fatal("provider failure became successful EOF", len(raw), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	cancelled := PipeRead(ctx, func() { close(done) }, func(_ context.Context, out io.Writer) error { _, err := out.Write([]byte("unread chunk")); return err })
	defer func() { _ = cancelled.Close() }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not join blocked writer")
	}
	if _, err := cancelled.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("reader ignored cancellation", err)
	}
}
