// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrUnsupportedMedia = miniatura.ErrMedioNoSoportado

// Thumbnail derives only from the captured authenticated private revision. No
// legacy thumbnail/content endpoint, server derivative or plaintext file is used.
func (r *Repository) Thumbnail(parent context.Context, id string) (io.ReadCloser, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	version, err := r.version(ctx, "file", id)
	if err != nil {
		done()
		return nil, err
	}
	item, data, raw, err := r.readEntry(ctx, version)
	clear(raw)
	if err != nil {
		done()
		return nil, err
	}
	if version.Commit.State != "live" || data.Binary == nil {
		done()
		return nil, ErrNotFound
	}
	if err := r.requireLive(ctx, item); err != nil {
		done()
		return nil, err
	}
	outType, ok := miniatura.TipoSalida(item.ContentType)
	if !ok || item.SizeBytes > storage.MaxDriveThumbnailBytes {
		done()
		return nil, ErrUnsupportedMedia
	}
	out, err := miniatura.RenderConContexto(ctx, func(ctx context.Context) ([]byte, error) {
		var input bytes.Buffer
		err := r.readBody(ctx, data.observation, *data.Binary, item, "binary", &input)
		return input.Bytes(), err
	}, storage.DriveThumbnailSide, outType)
	if err != nil {
		done()
		return nil, err
	}
	return r.stream(ctx, done, func(_ context.Context, w io.Writer) error {
		defer clear(out)
		_, err := w.Write(out)
		return err
	}), nil
}
