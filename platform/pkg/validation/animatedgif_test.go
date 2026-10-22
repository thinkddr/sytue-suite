// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func gifForValidation(t *testing.T, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for range frames {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}))
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Una cabecera GIF o una duración declarada no demuestra que se pueda animar,
// ni limita la memoria que reservará la composición de fotogramas.
func TestAnimatedGIFRejectsStaticTruncatedAndExpensive(t *testing.T) {
	animated := gifForValidation(t, 2)
	if err := AnimatedGIF(animated, 1024, 1024, 32); err != nil {
		t.Fatalf("dos fotogramas seguros: %v", err)
	}
	huge := bytes.Clone(animated)
	binary.LittleEndian.PutUint16(huge[6:8], 1024)
	binary.LittleEndian.PutUint16(huge[8:10], 1024)
	cases := map[string][]byte{
		"static":                 gifForValidation(t, 1),
		"header only":            []byte("GIF89a"),
		"truncated":              animated[:len(animated)-1],
		"composed canvas budget": huge,
		"trailing payload":       append(bytes.Clone(animated), 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if err := AnimatedGIF(data, 1024, 1024, 32); !errors.Is(err, ErrInvalidAnimatedGIF) {
				t.Fatalf("aceptó GIF inválido: %v", err)
			}
		})
	}
	if err := AnimatedGIF(animated, len(animated)-1, 1024, 32); err == nil {
		t.Error("ignoró el límite de bytes")
	}
	if err := AnimatedGIF(animated, 1024, 3, 32); err == nil {
		t.Error("ignoró el límite de dimensiones")
	}
}
