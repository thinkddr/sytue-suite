// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"io"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Export renders the authenticated original and retained recipe on this device.
// Only the requested image is loaded; no derivative or original is rewritten.
func (r *Repository) Export(parent context.Context, id, format string) (io.ReadCloser, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "jpg" {
		format = "jpeg"
	}
	if format != "jpeg" && format != "png" {
		return nil, model.ErrInvalidInput
	}
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	asset, err := r.asset(ctx, id)
	if err == nil && asset.Bytes > photorender.MaxOriginal {
		err = model.ErrInvalidInput
	}
	var recipe model.Receta
	if err == nil {
		recipe, err = r.retainedRecipe(ctx, id)
	}
	if err != nil {
		done()
		return nil, err
	}
	return storage.PipeRead(ctx, done, func(ctx context.Context, out io.Writer) error {
		return photorender.Exportar(ctx, out, func(ctx context.Context) ([]byte, error) { return r.exportOriginal(ctx, id) }, model.OrientacionDe(asset.Orientation), recipe, format)
	}), nil
}

func (r *Repository) exportOriginal(ctx context.Context, id string) ([]byte, error) {
	body := storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error { return r.Original(ctx, id, out) })
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, photorender.MaxOriginal+1))
	if err == nil && len(raw) > photorender.MaxOriginal {
		err = model.ErrInvalidInput
	}
	return raw, err
}
