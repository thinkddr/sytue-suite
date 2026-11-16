// SPDX-License-Identifier: Apache-2.0
package render

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// MaxOriginal is the retained compressed-source limit already used by Photos.
const (
	MaxOriginal        = 96 << 20
	CalidadExportacion = 92
)

// Imagen applies the existing EXIF and ordered recipe to a guarded decode.
// Callers hold miniatura's shared turn throughout load, processing and encoding.
func Imagen(ctx context.Context, raw []byte, orientation model.Orientacion, recipe model.Receta) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > MaxOriginal {
		return nil, model.ErrInvalidInput
	}
	if err := recipe.Valida(); err != nil {
		return nil, err
	}
	decoded, err := miniatura.Decodificar(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: imagen no soportada o demasiado grande para procesarla", model.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output := AplicarReceta(Orientar(decoded, orientation), recipe)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return output, nil
}

// Exportar waits before loading one owned private source buffer, then clears it.
// It never writes an original, recipe or retained derivative.
func Exportar(ctx context.Context, out io.Writer, load func(context.Context) ([]byte, error), orientation model.Orientacion, recipe model.Receta, format string) error {
	if err := recipe.Valida(); err != nil {
		return err
	}
	release, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return err
	}
	defer release()
	raw, err := load(ctx)
	defer clear(raw)
	if err != nil {
		return err
	}
	img, err := Imagen(ctx, raw, orientation, recipe)
	if err != nil {
		return err
	}
	if format == "png" {
		err = png.Encode(out, img)
	} else {
		err = jpeg.Encode(out, AplanarSobreBlanco(img), &jpeg.Options{Quality: CalidadExportacion})
	}
	if err != nil {
		return fmt.Errorf("photos: encode export: %w", err)
	}
	return ctx.Err()
}

// AplanarSobreBlanco preserves the Photos JPEG background and transparency rule.
func AplanarSobreBlanco(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Over)
	return dst
}
