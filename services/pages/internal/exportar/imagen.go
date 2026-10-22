// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
)

// GenerarImagenPNG genera un archivo PNG determinista en memoria (§9.4)
// con las dimensiones especificadas y relleno con el color primario dado en formato hex.
func GenerarImagenPNG(ancho, alto int, hexColor string) ([]byte, error) {
	if ancho <= 0 {
		ancho = 600
	}
	if alto <= 0 {
		alto = 400
	}

	c := parsearColorHex(hexColor)

	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("imagen: error codificando PNG: %w", err)
	}

	return buf.Bytes(), nil
}

func parsearColorHex(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		// formato #rgb -> #rrggbb
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		// Color por defecto (#2563eb / azul sytue)
		return color.RGBA{R: 37, G: 99, B: 235, A: 255}
	}

	r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return color.RGBA{R: 37, G: 99, B: 235, A: 255}
	}

	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}
