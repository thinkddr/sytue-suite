// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"io"
)

type pipeReadBody struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (b *pipeReadBody) Close() error { b.cancel(); return b.PipeReader.Close() }

// PipeRead adapts the existing authenticated device writer to a cancellable body.
// It streams without buffering the entity, propagates writer errors and calls
// done only after the writer has joined. The caller must close the returned body.
func PipeRead(parent context.Context, done func(), write func(context.Context, io.Writer) error) io.ReadCloser {
	ctx, cancel := context.WithCancel(parent)
	input, output := io.Pipe()
	stop := context.AfterFunc(ctx, func() { _ = input.CloseWithError(ctx.Err()); _ = output.CloseWithError(ctx.Err()) })
	go func() {
		if done != nil {
			defer done()
		}
		defer cancel()
		defer stop()
		err := write(ctx, output)
		_ = output.CloseWithError(err)
	}()
	return &pipeReadBody{PipeReader: input, cancel: cancel}
}
