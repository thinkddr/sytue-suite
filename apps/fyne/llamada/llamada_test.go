// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"bytes"
	"testing"

	"github.com/thesyncim/govpx"
)

// La voz que llega por RED (la web la manda así por defecto) se lee por su primaria, la
// última: con dos redundantes de 3 y 2 bytes delante, lo que queda es la voz de ahora.
//
// VISTO EN ROJO (2026-09-25, en el frontal): leyendo la longitud solo del último byte
// (sin los dos bits altos del tercero), una redundante de 300 bytes descuadra la
// primaria.
func TestLaVozPorREDSeLeePorSuPrimaria(t *testing.T) {
	carga := []byte{
		0x80 | 111, 0, 0, 3, // redundante de 3 bytes
		0x80 | 111, 0, 0, 2, // redundante de 2
		111,           // primaria
		1, 1, 1, 2, 2, // datos de las redundantes
		9, 9, 9, // la primaria
	}
	if got := PrimarioRED(carga); !bytes.Equal(got, []byte{9, 9, 9}) {
		t.Errorf("primaria %v", got)
	}
	larga := append([]byte{0x80 | 111, 0, 0x01, 0x2c, 111}, make([]byte, 300)...)
	larga = append(larga, 7)
	if got := PrimarioRED(larga); !bytes.Equal(got, []byte{7}) {
		t.Errorf("con una redundante de 300 bytes la primaria es %v", got)
	}
	if got := PrimarioRED([]byte{0x80, 0}); got != nil {
		t.Errorf("una cabecera cortada da %v", got)
	}
	if got := PrimarioRED([]byte{111, 5}); !bytes.Equal(got, []byte{5}) {
		t.Errorf("sin redundantes, %v", got)
	}
}

// El altavoz espera al colchón antes de sonar, suma las voces recortando a 16 bits, y
// una cola que se atrasa pierde lo más viejo.
//
// VISTO EN ROJO (2026-09-25, en el frontal): sin la espera del colchón, el primer
// tramo suena con 20 ms dentro y el resto del tramo a cero.
func TestElAltavozMezclaConColchon(t *testing.T) {
	a, b := nuevaCola(), nuevaCola()
	tramo := func(v int16) []int16 {
		s := make([]int16, Tramo)
		for i := range s {
			s[i] = v
		}
		return s
	}
	out := make([]int16, Tramo)
	a.escribir(tramo(30000))
	Mezclar(out, a, b)
	if out[0] != 0 {
		t.Fatalf("con %d muestras en cola ya suena %d; el colchón es %d", Tramo, out[0], Colchon)
	}
	a.escribir(tramo(30000))
	a.escribir(tramo(30000))
	b.escribir(tramo(10000))
	b.escribir(tramo(10000))
	b.escribir(tramo(10000))
	Mezclar(out, a, b)
	if out[0] != 32767 || out[Tramo-1] != 32767 {
		t.Errorf("30000+10000 da %d: sin recortar a 16 bits", out[0])
	}
	for range 20 {
		a.escribir(tramo(1))
	}
	if n := len(a.datos); n != Tope {
		t.Errorf("la cola atrasada guarda %d muestras; el tope es %d", n, Tope)
	}
}

// El colchón crece con los huecos de la red, no con las pausas de quien habla, y
// encoge cuando la red se calma.
func TestElColchonSeAdaptaALaRed(t *testing.T) {
	c := nuevaCola()
	acc := make([]int32, Tramo)
	voz := make([]int16, Tramo)
	sonar := func(tramos int) {
		for range tramos {
			c.leer(acc)
		}
	}
	// Red a golpes: llegan 60 ms, se oyen, y el siguiente paquete llega 40 ms tarde.
	for range 5 {
		c.escribir(voz)
		c.escribir(voz)
		c.escribir(voz)
		sonar(5)
	}
	if c.objetivo <= Colchon || c.objetivo > ColchonMax {
		t.Fatalf("tras cinco huecos de la red el colchón es %d; se esperaba entre %d y %d", c.objetivo, Colchon+Tramo, ColchonMax)
	}
	// Una pausa larga (quien habla se calla un segundo) no es la red.
	d := nuevaCola()
	d.escribir(voz)
	d.escribir(voz)
	d.escribir(voz)
	for range 50 {
		d.leer(acc)
	}
	d.escribir(voz)
	if d.objetivo != Colchon {
		t.Fatalf("una pausa de 1 s hizo crecer el colchón a %d", d.objetivo)
	}
	// Red en calma: sonando sin vaciarse, vuelve al mínimo.
	for range 4 * Calma / Tramo {
		c.escribir(voz)
		c.leer(acc)
	}
	if c.objetivo != Colchon {
		t.Fatalf("tras 20 s en calma el colchón sigue en %d; se esperaba %d", c.objetivo, Colchon)
	}
}

// Un fotograma VP8 de verdad —codificado con govpx— se descodifica a una imagen del
// tamaño y el color de lo que se mandó.
func TestElVideoSeDescodificaAImagen(t *testing.T) {
	const w, h = 64, 48
	enc, err := govpx.NewVP8Encoder(govpx.EncoderOptions{Width: w, Height: h, FPS: 30, TargetBitrateKbps: 500})
	if err != nil {
		t.Fatal(err)
	}
	src := govpx.Image{
		Width: w, Height: h, YStride: w, UStride: w / 2, VStride: w / 2,
		Y: bytes.Repeat([]byte{200}, w*h), U: bytes.Repeat([]byte{128}, w*h/4), V: bytes.Repeat([]byte{128}, w*h/4),
	}
	buf := make([]byte, 1<<20)
	r, err := enc.EncodeInto(buf, src, 0, 1, govpx.EncodeForceKeyFrame)
	if err != nil || len(r.Data) == 0 {
		t.Fatalf("codificar: %v (%d bytes)", err, len(r.Data))
	}
	dec, err := govpx.NewVP8Decoder(govpx.DecoderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(r.Data); err != nil {
		t.Fatal(err)
	}
	f, ok := dec.NextFrame()
	if !ok {
		t.Fatal("sin fotograma")
	}
	img := ImagenI420(f)
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Errorf("imagen de %v", img.Bounds())
	}
	if y := img.Y[img.YOffset(w/2, h/2)]; y < 190 || y > 210 {
		t.Errorf("la luma del centro es %d y se mandó 200", y)
	}
}
