// SPDX-License-Identifier: Apache-2.0
// Copyright (c) Sytue. Licensed under Apache-2.0.

// Package visual compara capturas sin ocultar diferencias por antialiasing.
// La política predeterminada exige igualdad en cada canal de cada píxel.
package visual

import (
	"fmt"
	"image"
	"image/color"
)

// Tolerance solo relaja el gate cuando el llamador lo pide expresamente.
type Tolerance struct {
	Channel uint8  `json:"channel"`
	Pixels  uint64 `json:"pixels"`
}

// Bounds delimita todos los píxeles distintos, con máximos exclusivos.
type Bounds struct {
	MinX int `json:"minX"`
	MinY int `json:"minY"`
	MaxX int `json:"maxX"`
	MaxY int `json:"maxY"`
}

// Result conserva también las diferencias que una tolerancia explícita acepta.
// Exact solo es verdadero cuando no ha cambiado ningún canal.
type Result struct {
	Width               int       `json:"width"`
	Height              int       `json:"height"`
	TotalPixels         uint64    `json:"totalPixels"`
	ChangedPixels       uint64    `json:"changedPixels"`
	PixelsOverTolerance uint64    `json:"pixelsOverTolerance"`
	MaxChannelDelta     uint8     `json:"maxChannelDelta"`
	Bounds              *Bounds   `json:"bounds"`
	Tolerance           Tolerance `json:"tolerance"`
	Exact               bool      `json:"exact"`
	Accepted            bool      `json:"accepted"`
}

// Compare no redimensiona, recorta ni alinea las imágenes. Las posiciones se
// comparan respecto del origen de cada captura; un tamaño distinto es un error.
// El diff deja transparentes los píxeles iguales y marca los distintos en magenta.
// El contrato utiliza canales de ocho bits: los modelos de dieciséis bits se
// rechazan antes de convertirlos. Una Image personalizada debe respetar ese
// contrato; la CLI comprueba también la profundidad original del PNG.
func Compare(reference, native image.Image, tolerance Tolerance) (Result, *image.NRGBA, error) {
	if reference == nil || native == nil {
		return Result{}, nil, fmt.Errorf("las dos capturas son obligatorias")
	}
	for _, capture := range []image.Image{reference, native} {
		if err := require8Bit(capture); err != nil {
			return Result{}, nil, err
		}
	}
	a, b := reference.Bounds(), native.Bounds()
	if a.Size() != b.Size() {
		return Result{}, nil, fmt.Errorf("dimensiones distintas: web %dx%d, nativo %dx%d", a.Dx(), a.Dy(), b.Dx(), b.Dy())
	}
	if a.Empty() {
		return Result{}, nil, fmt.Errorf("la captura está vacía")
	}
	r := Result{Width: a.Dx(), Height: a.Dy(), TotalPixels: uint64(a.Dx()) * uint64(a.Dy()), Tolerance: tolerance}
	diff := image.NewNRGBA(image.Rect(0, 0, r.Width, r.Height))
	for y := range r.Height {
		for x := range r.Width {
			ca := color.NRGBAModel.Convert(reference.At(a.Min.X+x, a.Min.Y+y)).(color.NRGBA)
			cb := color.NRGBAModel.Convert(native.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			delta := max(channelDelta(ca.R, cb.R), channelDelta(ca.G, cb.G), channelDelta(ca.B, cb.B), channelDelta(ca.A, cb.A))
			r.MaxChannelDelta = max(r.MaxChannelDelta, delta)
			if delta == 0 {
				continue
			}
			r.ChangedPixels++
			if delta > tolerance.Channel {
				r.PixelsOverTolerance++
			}
			diff.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			if r.Bounds == nil {
				r.Bounds = &Bounds{MinX: x, MinY: y, MaxX: x + 1, MaxY: y + 1}
			} else {
				r.Bounds.MinX = min(r.Bounds.MinX, x)
				r.Bounds.MinY = min(r.Bounds.MinY, y)
				r.Bounds.MaxX = max(r.Bounds.MaxX, x+1)
				r.Bounds.MaxY = max(r.Bounds.MaxY, y+1)
			}
		}
	}
	r.Exact = r.ChangedPixels == 0
	r.Accepted = r.PixelsOverTolerance <= tolerance.Pixels
	return r, diff, nil
}

func require8Bit(img image.Image) error {
	// El caso concreto cubre las imágenes estándar; el modelo cubre también
	// Uniform y los wrappers que conservan la profundidad de su origen.
	switch img.(type) {
	case *image.RGBA64, *image.NRGBA64, *image.Gray16, *image.Alpha16:
		return fmt.Errorf("captura de 16 bits: el gate compara exclusivamente canales de 8 bits")
	}
	switch img.ColorModel().Convert(color.NRGBA{A: 255}).(type) {
	case color.RGBA64, color.NRGBA64, color.Gray16, color.Alpha16:
		return fmt.Errorf("modelo de color de 16 bits: el gate compara exclusivamente canales de 8 bits")
	}
	return nil
}

func channelDelta(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
