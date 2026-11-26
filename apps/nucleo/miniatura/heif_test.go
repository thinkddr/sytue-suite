// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"

	"github.com/gen2brain/h265/heic"
)

// caja arma una caja de ISOBMFF.
func caja(tipo string, cuerpo ...[]byte) []byte {
	b := bytes.Join(cuerpo, nil)
	out := make([]byte, 8, 8+len(b))
	binary.BigEndian.PutUint32(out, uint32(8+len(b)))
	copy(out[4:], tipo)
	return append(out, b...)
}

// cajaCompleta es una FullBox: versión y banderas delante.
func cajaCompleta(tipo string, version byte, banderas uint32, cuerpo ...[]byte) []byte {
	vb := []byte{version, byte(banderas >> 16), byte(banderas >> 8), byte(banderas)}
	return caja(tipo, append([][]byte{vb}, cuerpo...)...)
}

func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

// heifCon es la cabecera de un HEIC cuyo elemento principal (el 1) lleva `irot` con ese
// giro; otro elemento (el 2) lleva otro, que no debe contar.
func heifCon(giro byte, anchos bool) []byte {
	ipco := caja("ipco",
		cajaCompleta("ispe", 0, 0, u32(4032), u32(3024)),
		caja("irot", []byte{giro}),
		caja("irot", []byte{3}))
	var banderas uint32
	asociacion := func(i int) []byte { return []byte{byte(0x80 | i)} }
	if anchos {
		banderas = 1
		asociacion = func(i int) []byte { return u16(uint16(0x8000 | i)) }
	}
	ipma := cajaCompleta("ipma", 0, banderas, u32(2),
		u16(2), []byte{1}, asociacion(3),
		u16(1), []byte{2}, asociacion(1), asociacion(2))
	meta := cajaCompleta("meta", 0, 0,
		cajaCompleta("hdlr", 0, 0, u32(0), []byte("pict"), make([]byte, 13)),
		cajaCompleta("pitm", 0, 0, u16(1)),
		caja("iprp", ipco, ipma))
	return append(caja("ftyp", []byte("heic"), u32(0), []byte("mif1heic")), meta...)
}

// El giro sale del `irot` del elemento PRINCIPAL, con índices de 7 y de 15 bits, y lo
// que no es una cabecera HEIF no da giro.
func TestGiroHEIF(t *testing.T) {
	for _, anchos := range []bool{false, true} {
		for giro, quiere := range map[byte]int{0: 0, 1: 90, 2: 180, 3: 270} {
			g, ok := giroHEIF(heifCon(giro, anchos))
			if !ok || g != quiere {
				t.Errorf("irot %d (índices anchos %v): %d, %v; quiero %d", giro, anchos, g, ok, quiere)
			}
		}
	}
	cfg := girarConfigHEIF(image.Config{Width: 4032, Height: 3024}, heifCon(1, false))
	if cfg.Width != 3024 || cfg.Height != 4032 {
		t.Errorf("girado un cuarto, la cabecera dice %d×%d", cfg.Width, cfg.Height)
	}
	cfg = girarConfigHEIF(image.Config{Width: 4032, Height: 3024}, heifCon(2, false))
	if cfg.Width != 4032 {
		t.Errorf("girado media vuelta cambia la forma: %d×%d", cfg.Width, cfg.Height)
	}
	// Basura, cortes y tamaños que se salen: «no se sabe», nunca un pánico.
	buena := heifCon(1, true)
	for _, b := range [][]byte{nil, []byte("hola"), buena[:40], buena[:len(buena)-3], append([]byte{0xff, 0xff, 0xff, 0xff}, buena...)} {
		if g, ok := giroHEIF(b); ok && g != 0 {
			t.Errorf("de %d bytes rotos sale un giro %d", len(b), g)
		}
	}
}

// Un HEIC se mide y se decodifica como cualquier otra imagen: la galería ya no deja
// fuera las fotos del iPhone que llegan sin convertir (Windows, Linux).
//
// VISTO EN ROJO (2026-11-25): sin el decodificador, Cabecera da «formato desconocido» y
// Decodificar ErrMedioNoSoportado.
func TestUnHEICSeMideYSeDecodifica(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 64))
	for y := range 64 {
		for x := range 96 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 2), G: uint8(y * 3), B: 128, A: 255})
		}
	}
	var b bytes.Buffer
	if err := heic.Encode(&b, img, heic.EncodeOptions{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	cfg, formato, err := Cabecera(b.Bytes())
	if err != nil || formato != "heic" || cfg.Width != 96 || cfg.Height != 64 {
		t.Fatalf("cabecera %+v, %q, %v", cfg, formato, err)
	}
	dec, err := Decodificar(b.Bytes())
	if err != nil || dec.Bounds().Dx() != 96 || dec.Bounds().Dy() != 64 {
		t.Fatalf("decodificar: %v, %v", dec, err)
	}
	// Con pérdida, pero el color de una esquina se parece al original.
	r, g, _, _ := dec.At(40, 30).RGBA()
	if d := int(r>>8) - 80; d < -24 || d > 24 {
		t.Errorf("rojo %d en (40,30), y era 80", r>>8)
	}
	if d := int(g>>8) - 90; d < -24 || d > 24 {
		t.Errorf("verde %d en (40,30), y era 90", g>>8)
	}
	// Y Render saca su miniatura en JPEG.
	if _, err := Render(b.Bytes(), 32, "image/jpeg"); err != nil {
		t.Errorf("render: %v", err)
	}
}

// La lista de índices de un elemento de `ipma`: de 7 y de 15 bits, y un recuento que dice
// más índices de los que caben en lo que queda (un fichero cortado o que miente) se
// rechaza antes de reservar nada por él.
func TestIndicesIpma(t *testing.T) {
	if got, resto, ok := indicesIpma([]byte{2, 0x81, 0x03, 9}, 0); !ok || len(got) != 2 || got[0] != 1 || got[1] != 3 || len(resto) != 1 {
		t.Errorf("7 bits: %v, resto %v, %v", got, resto, ok)
	}
	if got, _, ok := indicesIpma(append([]byte{1}, u16(0x8000|300)...), 1); !ok || len(got) != 1 || got[0] != 300 {
		t.Errorf("15 bits: %v, %v", got, ok)
	}
	for nombre, b := range map[string][]byte{
		"vacía":                nil,
		"recuento que miente":  {255, 0x81},
		"15 bits sin terminar": {2, 0x80, 0x01, 0x80},
		"7 bits sin terminar":  {3, 0x81, 0x82},
	} {
		anchos := byte(0)
		if nombre == "15 bits sin terminar" {
			anchos = 1
		}
		if got, _, ok := indicesIpma(b, anchos); ok {
			t.Errorf("%s: %v, aceptada", nombre, got)
		}
	}
}
