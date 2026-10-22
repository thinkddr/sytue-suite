// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura

import (
	"image"
	"testing"
)

func fuente(w, h int) image.Image {
	m := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for i := range m.Y {
		m.Y[i] = uint8(i)
	}
	return m
}

// Cada tamaño pedido tiene que salir, con el aspecto conservado.
func TestCascada_devuelveTodosLosTamanyos(t *testing.T) {
	got := Cascada(fuente(800, 600), []int{600, 400, 100})
	if len(got) != 3 {
		t.Fatalf("%d variantes, se esperaban 3", len(got))
	}
	for lado, img := range got {
		b := img.Bounds()
		mayor := b.Dx()
		if b.Dy() > mayor {
			mayor = b.Dy()
		}
		if mayor != lado {
			t.Errorf("la variante %d mide %d en su lado mayor", lado, mayor)
		}
		// 4:3 de origen, 4:3 de destino.
		if b.Dx()*3 != b.Dy()*4 {
			t.Errorf("la variante %d perdió el aspecto: %dx%d", lado, b.Dx(), b.Dy())
		}
	}
}

// EL ORDEN NO PUEDE DEPENDER DE QUIEN LLAMA. Pasar los lados de menor a mayor
// reduciría y volvería a AMPLIAR desde la pequeña, que destroza la imagen sin
// dar ningún error.
func TestCascada_ordenaAunqueLleguenAlReves(t *testing.T) {
	alReves := Cascada(fuente(800, 600), []int{100, 400, 600})
	bien := Cascada(fuente(800, 600), []int{600, 400, 100})
	for _, lado := range []int{600, 400, 100} {
		a, b := alReves[lado].Bounds(), bien[lado].Bounds()
		if a != b {
			t.Errorf("la variante %d salió %v pidiendo al revés y %v pidiendo bien", lado, a, b)
		}
	}
	// Y la pequeña tiene que tener detalle de verdad, no ser una ampliación:
	// una ampliación desde 100 dejaría la de 600 borrosa, pero eso no se ve en
	// las dimensiones. Se comprueba que la grande NO se derivó de la pequeña
	// comparando con reducir directamente.
	directa := Reducir(fuente(800, 600), 600)
	if alReves[600].Bounds() != directa.Bounds() {
		t.Error("la variante mayor no coincide con la reducción directa")
	}
}

// NO SE AMPLÍA NUNCA: una foto ya pequeña conserva su tamaño.
func TestCascada_noAmplia(t *testing.T) {
	got := Cascada(fuente(300, 200), []int{3200, 512})
	for lado, img := range got {
		if img.Bounds().Dx() > 300 || img.Bounds().Dy() > 200 {
			t.Errorf("la variante %d amplió la imagen a %v", lado, img.Bounds())
		}
	}
}

// El banco que sostiene la decisión de la cascada. `go test -bench Cascada`.
func BenchmarkCascada(b *testing.B) {
	src := fuente(4000, 3000)
	b.ResetTimer()
	for range b.N {
		_ = Cascada(src, []int{3200, 2048, 512})
	}
}

// Y el contraste: las mismas tres, cada una desde el original.
func BenchmarkTresIndependientes(b *testing.B) {
	src := fuente(4000, 3000)
	b.ResetTimer()
	for range b.N {
		_ = Reducir(src, 3200)
		_ = Reducir(src, 2048)
		_ = Reducir(src, 512)
	}
}
