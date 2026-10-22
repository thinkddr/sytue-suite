// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package qr

import (
	"bytes"
	"image"
	_ "image/png"
	"strings"
	"testing"
)

// TestQRFormatBits contrasta los 15 bits de formato (nivel M) de las 8 máscaras con
// los valores PUBLICADOS en el estándar QR: valida la BCH(15,5) y el XOR de máscara.
func TestQRFormatBits(t *testing.T) {
	want := []int{0x5412, 0x5125, 0x5E7C, 0x5B4B, 0x45F9, 0x40CE, 0x4F97, 0x4AA0}
	for mask, w := range want {
		if got := formatBits(mask); got != w {
			t.Errorf("formatBits(%d) = 0x%X, quiero 0x%X", mask, got, w)
		}
	}
}

// TestQRVersionBits contrasta los 18 bits de versión (v7..v10) con los valores
// publicados: valida la BCH(18,6).
func TestQRVersionBits(t *testing.T) {
	want := map[int]int{7: 0x07C94, 8: 0x085BC, 9: 0x09A99, 10: 0x0A4D3}
	for v, w := range want {
		if got := versionBits(v); got != w {
			t.Errorf("versionBits(%d) = 0x%X, quiero 0x%X", v, got, w)
		}
	}
}

// TestRSGenerator valida el generador Reed–Solomon de grado 2:
// (x+α^0)(x+α^1) = x^2 + (α+1)x + α = [1,3,2] en GF(256) con α=2.
func TestRSGenerator(t *testing.T) {
	if got := rsGenerator(2); !bytes.Equal(got, []byte{1, 3, 2}) {
		t.Fatalf("rsGenerator(2) = %v, quiero [1 3 2]", got)
	}
}

// TestRSDivisible comprueba la invariante de Reed–Solomon: un codeword completo
// (datos ‖ EC) es divisible por el generador, luego su propio resto es cero.
func TestRSDivisible(t *testing.T) {
	data := []byte("sytue-totp-qr-reed-solomon-check")
	const ec = 22
	rem := rsRemainder(data, ec)
	full := append(append([]byte{}, data...), rem...)
	if check := rsRemainder(full, ec); !bytes.Equal(check, make([]byte, ec)) {
		t.Fatalf("resto no nulo del codeword completo: %v", check)
	}
}

// TestQREncodeStructure verifica la estructura básica de un QR de un URI otpauth
// real: dimensiones, localizador con separador y módulo oscuro fijo.
func TestQREncodeStructure(t *testing.T) {
	uri := "otpauth://totp/Sytue:someone.with.a.long.name%40example.com?secret=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&issuer=Sytue&algorithm=SHA1&digits=6&period=30"
	m, err := encode(uri)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if m.size != 17+4*(m.size-17)/4 { // coherencia trivial
		t.Fatalf("tamaño inesperado: %d", m.size)
	}
	// Esquina del localizador superior izquierdo (borde oscuro) y su separador claro.
	if !m.get(0, 0) || !m.get(6, 0) || !m.get(0, 6) {
		t.Fatal("falta el patrón localizador")
	}
	if m.get(7, 0) {
		t.Fatal("el separador del localizador debe ser claro")
	}
	// Módulo oscuro fijo en (8, size-8).
	if !m.get(8, m.size-8) {
		t.Fatal("falta el módulo oscuro fijo")
	}
}

// TestPNG comprueba que sale un PNG decodificable, cuadrado, del tamaño que
// toca y con los dos colores dentro. Un PNG que no abre no se distingue a simple
// vista de uno que sí: la pantalla enseñaría un hueco y nadie sabría por qué.
func TestPNG(t *testing.T) {
	const uri = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=FV-2026-00001&fecha=15-09-2026&importe=110.00"
	b, err := PNG(uri)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img, formato, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("no decodifica: %v", err)
	}
	if formato != "png" {
		t.Fatalf("formato %q, se esperaba png", formato)
	}
	r := img.Bounds()
	if r.Dx() != r.Dy() {
		t.Fatalf("no es cuadrado: %dx%d", r.Dx(), r.Dy())
	}
	// El módulo tiene que medir `escalaPNG` píxeles: si alguien lo baja a 1 el QR
	// sigue siendo «válido» y deja de escanearse en la mitad de las cámaras.
	m, _ := encode(uri)
	if quiero := (m.size + 8) * escalaPNG; r.Dx() != quiero {
		t.Fatalf("lado %d px, se esperaban %d", r.Dx(), quiero)
	}
	// La esquina es zona de silencio (blanca) y el ojo del localizador, oscuro: si
	// salieran iguales, el PNG sería un cuadrado liso y no un código.
	blanco, _, _, _ := img.At(0, 0).RGBA()
	oscuro, _, _, _ := img.At(4*escalaPNG+escalaPNG/2, 4*escalaPNG+escalaPNG/2).RGBA()
	if blanco != 0xffff {
		t.Fatalf("la zona de silencio no es blanca: %04x", blanco)
	}
	if oscuro >= 0x4000 {
		t.Fatalf("el localizador no es oscuro: %04x", oscuro)
	}
}

// TestQRTooLong: un texto que excede la capacidad de v10 (nivel M) da error en vez
// de un QR truncado ilegible.
func TestQRTooLong(t *testing.T) {
	if _, err := encode(strings.Repeat("x", 300)); err == nil {
		t.Fatal("300 bytes no caben en v10-M; se esperaba error")
	}
}
