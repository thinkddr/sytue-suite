// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package video

import (
	"math"
	"testing"
)

// tono es `s` segundos de un seno de `hz` a 48 kHz.
func tono(hz, s float64) []int16 {
	n := int(s * 48000)
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(12000 * math.Sin(2*math.Pi*hz*float64(i)/48000))
	}
	return out
}

// cruces cuenta los cruces por cero de subida: en un seno, uno por periodo.
func cruces(x []int16) int {
	n := 0
	for i := 1; i < len(x); i++ {
		if x[i-1] < 0 && x[i] >= 0 {
			n++
		}
	}
	return n
}

// estirar pasa toda la entrada por un estirador a `v`, de 20 ms en 20 ms como la tarjeta.
func estirar(entrada []int16, v float64) []int16 {
	e := estirador{vel: v}
	fuente := append([]int16(nil), entrada...)
	var out []int16
	tramo := make([]int16, 960)
	for {
		n := e.leer(tramo, &fuente)
		out = append(out, tramo[:n]...)
		if n < len(tramo) {
			return out
		}
	}
}

// TestVelocidadEscalaLaDuracion: a 2× dura la mitad y a 0,5× el doble (salvo los
// últimos milisegundos, que se quedan esperando más entrada).
func TestVelocidadEscalaLaDuracion(t *testing.T) {
	entrada := tono(440, 2)
	for _, v := range []float64{0.5, 0.75, 1.25, 1.5, 2} {
		got := float64(len(estirar(entrada, v)))
		quiero := float64(len(entrada)) / v
		if math.Abs(got-quiero) > 0.02*48000/v {
			t.Errorf("a %v× salen %v muestras y deberían ser unas %v", v, got, quiero)
		}
	}
}

// TestVelocidadConservaElTono: a cualquier velocidad, un seno de 440 Hz sigue cruzando
// por cero 440 veces por segundo de SALIDA (leer más deprisa daría 880 a 2×), y sin
// huecos: ningún periodo se aleja mucho del de 440 Hz.
func TestVelocidadConservaElTono(t *testing.T) {
	entrada := tono(440, 2)
	for _, v := range []float64{0.5, 1.5, 2} {
		out := estirar(entrada, v)
		hz := float64(cruces(out)) / (float64(len(out)) / 48000)
		if math.Abs(hz-440) > 440*0.03 {
			t.Errorf("a %v× el tono sale a %.1f Hz", v, hz)
		}
		ultimo := -1
		for i := 1; i < len(out); i++ {
			if out[i-1] < 0 && out[i] >= 0 {
				if ultimo >= 0 {
					if p := i - ultimo; p < 90 || p > 130 {
						t.Fatalf("a %v× hay un periodo de %d muestras en %d (el de 440 Hz es 109)", v, p, i)
					}
				}
				ultimo = i
			}
		}
	}
}

// TestVolverAUnoNoPierdeSonido: soltar devuelve lo que quedaba sin estirar, y su
// principio sigue en fase con lo último que sonó.
func TestVolverAUnoNoPierdeSonido(t *testing.T) {
	entrada := tono(440, 1)
	e := estirador{vel: 1.5}
	fuente := append([]int16(nil), entrada...)
	sonado := make([]int16, 960*10)
	e.leer(sonado, &fuente)
	resto := append(e.soltar(), fuente...)
	todo := make([]int16, 0, len(sonado)+len(resto))
	todo = append(append(todo, sonado...), resto...)
	hz := float64(cruces(todo)) / (float64(len(todo)) / 48000)
	if math.Abs(hz-440) > 440*0.03 {
		t.Errorf("al soltar el tono sale a %.1f Hz", hz)
	}
	// 10 tramos de salida a 1,5× han gastado 15 de entrada: queda el resto, entero.
	if quiero := len(entrada) - 960*15; math.Abs(float64(len(resto)-quiero)) > 2*holguraWSOLA {
		t.Errorf("tras soltar quedan %d muestras y deberían ser unas %d", len(resto), quiero)
	}
}
