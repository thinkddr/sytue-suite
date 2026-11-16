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

// originalPreview renders only the explicitly requested unedited editor preview.
// It neither publishes a derivative nor changes the selected recipe or history.
func (r *Repository) originalPreview(ctx context.Context, done func(), id string) (io.ReadCloser, error) {
	asset, err := r.asset(ctx, id)
	if err == nil && asset.Bytes > photorender.MaxOriginal {
		err = model.ErrInvalidInput
	}
	if err != nil {
		done()
		return nil, err
	}
	pngOutput := strings.EqualFold(asset.MediaType, "image/png") || strings.HasSuffix(strings.ToLower(asset.Name), ".png")
	side, _ := model.VariantePrevia.LadoMayor()
	return storage.PipeRead(ctx, done, func(ctx context.Context, out io.Writer) error {
		return photorender.Derivar(ctx, func(ctx context.Context) ([]byte, error) { return r.exportOriginal(ctx, id) }, model.OrientacionDe(asset.Orientation), model.RecetaVacia(), pngOutput, []int{side}, func(_ int, _, _ string, raw []byte) error {
			_, err := out.Write(raw)
			return err
		})
	}), nil
}
