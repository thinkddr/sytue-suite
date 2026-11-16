// SPDX-License-Identifier: Apache-2.0
package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// Derivar decodes one original, applies EXIF and the recipe once, then builds the
// existing cascade. The callback borrows one encoded image until it returns.
// Its durable owner must retain that image before advertising its fingerprint.
func Derivar(ctx context.Context, load func(context.Context) ([]byte, error), orientation model.Orientacion, recipe model.Receta, pngOutput bool, sides []int, retain func(int, string, string, []byte) error) error {
	if retain == nil || load == nil {
		return model.ErrInvalidInput
	}
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
	cascade := miniatura.Cascada(img, sides)
	ordered := make([]int, 0, len(cascade))
	for side := range cascade {
		ordered = append(ordered, side)
	}
	slices.Sort(ordered)
	for _, side := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := retainDerived(cascade[side], side, pngOutput, retain); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func retainDerived(img image.Image, side int, pngOutput bool, retain func(int, string, string, []byte) error) error {
	var buffer bytes.Buffer
	format, contentType := "jpg", "image/jpeg"
	if pngOutput {
		format, contentType = "png", "image/png"
		if err := png.Encode(&buffer, img); err != nil {
			return fmt.Errorf("photos: codificar la variante PNG de %d px: %w", side, err)
		}
	} else if err := jpeg.Encode(&buffer, AplanarSobreBlanco(img), &jpeg.Options{Quality: miniatura.CalidadJPEG}); err != nil {
		return fmt.Errorf("photos: codificar la variante de %d px: %w", side, err)
	}
	defer clear(buffer.Bytes())
	return retain(side, format, contentType, buffer.Bytes())
}
