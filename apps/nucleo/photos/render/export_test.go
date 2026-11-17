// SPDX-License-Identifier: Apache-2.0
package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

var guardedDecodeCalls atomic.Int32

type queuedExportContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *queuedExportContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func init() {
	for _, spec := range []struct {
		magic  string
		config image.Config
	}{{"PHOTOPIXELS", image.Config{ColorModel: color.GrayModel, Width: miniatura.MaxPixeles + 1, Height: 1}}, {"PHOTOMEMORY", image.Config{ColorModel: color.NRGBA64Model, Width: 6000, Height: 4000}}} {
		config := spec.config
		image.RegisterFormat(spec.magic, spec.magic, func(io.Reader) (image.Image, error) {
			guardedDecodeCalls.Add(1)
			return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil
		}, func(io.Reader) (image.Config, error) { return config, nil })
	}
}

func TestExportUsesExifThenRetainedRecipeAndClearsOnlyOwnedInput(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, patron()); err != nil {
		t.Fatal(err)
	}
	retained := bytes.Clone(encoded.Bytes())
	owned := bytes.Clone(retained)
	recipe := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoRecorte, Params: map[string]float64{"x": .5, "y": 0, "ancho": .5, "alto": 1}}}}
	var output bytes.Buffer
	if err := Exportar(t.Context(), &output, func(context.Context) ([]byte, error) { return owned, nil }, 6, recipe, "png"); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rejilla(img) != "A/B" {
		t.Fatal("export did not apply EXIF before the retained crop", rejilla(img))
	}
	if !bytes.Equal(retained, encoded.Bytes()) || !bytes.Equal(owned, make([]byte, len(owned))) || recipe.Pasos[0].Params["x"] != .5 {
		t.Fatal("export changed retained bytes/recipe or kept its owned input")
	}
}

func TestExportCancellationNeverLoadsQueuedPrivateSource(t *testing.T) {
	release := miniatura.Turno()
	defer release()
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &queuedExportContext{Context: parent, waiting: make(chan struct{})}
	result := make(chan error, 1)
	var loads atomic.Int32
	go func() {
		result <- Exportar(ctx, io.Discard, func(context.Context) ([]byte, error) { loads.Add(1); return nil, nil }, 1, model.RecetaVacia(), "png")
	}()
	select {
	case <-ctx.waiting:
	case <-time.After(time.Second):
		t.Fatal("export never waited for the shared image turn")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || loads.Load() != 0 {
			t.Fatal("canceled export loaded source", err, loads.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("export remained queued after cancellation")
	}
}

func TestExportRejectsPixelAndMemoryLimitsBeforeDecode(t *testing.T) {
	for _, magic := range []string{"PHOTOPIXELS", "PHOTOMEMORY"} {
		before := guardedDecodeCalls.Load()
		err := Exportar(t.Context(), io.Discard, func(context.Context) ([]byte, error) { return []byte(magic), nil }, 1, model.RecetaVacia(), "png")
		if !errors.Is(err, model.ErrInvalidInput) || guardedDecodeCalls.Load() != before {
			t.Fatal("export decoded a source beyond its pixel/memory budget", magic, err)
		}
	}
}
