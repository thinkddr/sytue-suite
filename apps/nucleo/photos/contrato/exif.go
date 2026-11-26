// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contrato

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"time"
)

// Los tamaños y desplazamientos de abajo son los de una foto de prueba de unos cientos de
// bytes compuesta aquí: las conversiones a uint16 y uint32 no desbordan (gosec G115).

// Las fotos de las pruebas se COMPONEN (como en `photos/exif`): un JPEG de colores y,
// si hace falta, su bloque EXIF con la fecha de captura y la posición del GPS.

// JPEG es una foto de ancho×alto con un degradado de tono; cada tono da otros bytes.
func JPEG(ancho, alto int, tono uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := range alto {
		for x := range ancho {
			img.Set(x, y, color.RGBA{tono, uint8(x * 7), uint8(y * 5), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// Lugar son unas coordenadas en grados.
type Lugar struct{ Lat, Lon float64 }

// ConExif es la foto jpg con un bloque EXIF: la fecha de captura (si no es cero), su
// orientación (si no es 0) y la posición (si no es nil).
func ConExif(jpg []byte, tomada time.Time, orientacion uint16, lugar *Lugar) []byte {
	tiff := bloqueTIFF(tomada, orientacion, lugar)
	cuerpo := append([]byte("Exif\x00\x00"), tiff...)
	out := make([]byte, 0, len(jpg)+len(cuerpo)+4)
	out = append(out, jpg[:2]...) // SOI
	out = append(out, 0xFF, 0xE1)
	out = binary.BigEndian.AppendUint16(out, uint16(len(cuerpo)+2)) //nolint:gosec // G115: ver arriba
	out = append(out, cuerpo...)
	return append(out, jpg[2:]...)
}

type entradaTIFF struct {
	tag, tipo uint16
	n         uint32
	valor     uint32
	datos     []byte // lo que no cabe en los cuatro bytes del registro
}

func ascii(tag uint16, s string) entradaTIFF {
	d := append([]byte(s), 0)
	e := entradaTIFF{tag: tag, tipo: 2, n: uint32(len(d))} //nolint:gosec // G115: ver arriba
	if len(d) <= 4 {
		var v [4]byte
		copy(v[:], d)
		e.valor = binary.LittleEndian.Uint32(v[:])
	} else {
		e.datos = d
	}
	return e
}

func grados(tag uint16, g float64) entradaTIFF {
	g = math.Abs(g)
	d := math.Floor(g)
	m := math.Floor((g - d) * 60)
	s := ((g-d)*60 - m) * 60
	var b []byte
	for _, p := range [][2]uint32{{uint32(d), 1}, {uint32(m), 1}, {uint32(math.Round(s * 10000)), 10000}} {
		b = binary.LittleEndian.AppendUint32(b, p[0])
		b = binary.LittleEndian.AppendUint32(b, p[1])
	}
	return entradaTIFF{tag: tag, tipo: 5, n: 3, datos: b}
}

// bloqueTIFF monta IFD0 (orientación y punteros), el sub-IFD de Exif (la fecha) y el de
// GPS, con el área de datos al final.
func bloqueTIFF(tomada time.Time, orientacion uint16, lugar *Lugar) []byte {
	var ifd0, exif, gps []entradaTIFF
	if orientacion != 0 {
		ifd0 = append(ifd0, entradaTIFF{tag: 0x0112, tipo: 3, n: 1, valor: uint32(orientacion)})
	}
	if !tomada.IsZero() {
		exif = append(exif, ascii(0x9003, tomada.Format("2006:01:02 15:04:05")))
	}
	if lugar != nil {
		ns, eo := "N", "E"
		if lugar.Lat < 0 {
			ns = "S"
		}
		if lugar.Lon < 0 {
			eo = "W"
		}
		gps = []entradaTIFF{ascii(0x0001, ns), grados(0x0002, lugar.Lat), ascii(0x0003, eo), grados(0x0004, lugar.Lon)}
	}
	tamIFD := func(n int) int { return 2 + n*12 + 4 }
	n0 := len(ifd0) + 1
	if len(gps) > 0 {
		n0++
	}
	off0 := 8
	offExif := off0 + tamIFD(n0)
	offGPS := offExif + tamIFD(len(exif))
	offDatos := offGPS
	if len(gps) > 0 {
		offDatos += tamIFD(len(gps))
	}
	ifd0 = append(ifd0, entradaTIFF{tag: 0x8769, tipo: 4, n: 1, valor: uint32(offExif)}) //nolint:gosec // G115: ver arriba
	if len(gps) > 0 {
		ifd0 = append(ifd0, entradaTIFF{tag: 0x8825, tipo: 4, n: 1, valor: uint32(offGPS)}) //nolint:gosec // G115: ver arriba
	}
	var datos []byte
	escribir := func(es []entradaTIFF) []byte {
		b := binary.LittleEndian.AppendUint16(nil, uint16(len(es))) //nolint:gosec // G115: ver arriba
		for _, e := range es {
			b = binary.LittleEndian.AppendUint16(b, e.tag)
			b = binary.LittleEndian.AppendUint16(b, e.tipo)
			b = binary.LittleEndian.AppendUint32(b, e.n)
			if e.datos != nil {
				b = binary.LittleEndian.AppendUint32(b, uint32(offDatos+len(datos))) //nolint:gosec // G115: ver arriba
				datos = append(datos, e.datos...)
			} else {
				b = binary.LittleEndian.AppendUint32(b, e.valor)
			}
		}
		return binary.LittleEndian.AppendUint32(b, 0)
	}
	out := []byte{'I', 'I', 42, 0, 8, 0, 0, 0}
	out = append(out, escribir(ifd0)...)
	out = append(out, escribir(exif)...)
	if len(gps) > 0 {
		out = append(out, escribir(gps)...)
	}
	return append(out, datos...)
}
