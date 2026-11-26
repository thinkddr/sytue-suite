// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"bytes"
	"errors"
	"image"
	"image/png"
)

// # La miniatura del Explorador de Windows (CA-06)
//
// En Windows la miniatura del sistema es la que enseña el Explorador:
// `SHCreateItemFromParsingName` da el `IShellItemImageFactory` del fichero y su
// `GetImage` (con SIIGBF_THUMBNAILONLY, para no quedarse con el icono genérico como
// póster) un mapa de bits de 32 bits por píxel. Lo COM está en
// `miniatura_windows.go`, en Go puro; aquí, lo que se prueba en cualquier sistema: pasar
// esos píxeles (BGRA de GDI) a una imagen de Go y a PNG.

// maxLadoExplorador es el lado más grande que se acepta del mapa de bits.
const maxLadoExplorador = 8192

// errPixeles: el mapa de bits no tiene el tamaño que dice.
var errPixeles = errors.New("videoteca: la miniatura del sistema no tiene el tamaño que dice")

// deBGRA pasa los píxeles BGRA de un mapa de bits de GDI (de arriba abajo, sin
// relleno) a RGBA de Go, que es premultiplicado. El alfa de GDI puede venir de tres
// maneras, y se reconoce cuál:
//   - todo a cero: el mapa no tiene alfa (lo normal en un fotograma) y es opaco;
//   - premultiplicado (ningún color pasa de su alfa), como lo pide AlphaBlend: tal cual;
//   - si no, sin premultiplicar: se premultiplica.
func deBGRA(px []byte, ancho, alto int) (*image.RGBA, error) {
	if ancho <= 0 || alto <= 0 || ancho > maxLadoExplorador || alto > maxLadoExplorador || len(px) < ancho*alto*4 {
		return nil, errPixeles
	}
	n := ancho * alto
	sinAlfa, premultiplicado := true, true
	for i := range n {
		b, g, r, a := px[4*i], px[4*i+1], px[4*i+2], px[4*i+3]
		if a != 0 {
			sinAlfa = false
		}
		if r > a || g > a || b > a {
			premultiplicado = false
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for i := range n {
		b, g, r, a := px[4*i], px[4*i+1], px[4*i+2], px[4*i+3]
		switch {
		case sinAlfa:
			a = 0xff
		case !premultiplicado:
			r, g, b = premultiplicar(r, a), premultiplicar(g, a), premultiplicar(b, a)
		}
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = r, g, b, a
	}
	return img, nil
}

func premultiplicar(c, a byte) byte {
	return byte((uint16(c)*uint16(a) + 127) / 255) //nolint:gosec // G115: c·a/255 cabe en un byte
}

// aPNG codifica la miniatura para guardarla como póster.
func aPNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
