// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

package altavoz

import (
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gen2brain/malgo"
)

// goertzel es la energía de una frecuencia en un bloque: cuánto del tono que sale por
// el altavoz vuelve por el micro.
func goertzel(x []int16, f float64) float64 {
	k := 2 * math.Cos(2*math.Pi*f/48000)
	var s1, s2 float64
	for _, v := range x {
		s1, s2 = float64(v)+k*s1-s2, s1
	}
	return math.Sqrt(s1*s1+s2*s2-k*s1*s2) / float64(len(x))
}

// eco pone un tono de 1 kHz en el altavoz dos segundos y mide cuánto vuelve por el
// micro en el último segundo.
func eco(t *testing.T, abrir func(c, r func([]int16)) (func(), error)) float64 {
	t.Helper()
	var mu sync.Mutex
	var oido []int16
	fase := 0
	cerrar, err := abrir(func(p []int16) {
		mu.Lock()
		oido = append(oido, p...)
		mu.Unlock()
	}, func(p []int16) {
		for i := range p {
			p[i] = int16(6000 * math.Sin(2*math.Pi*1000*float64(fase)/48000))
			fase++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	cerrar()
	mu.Lock()
	defer mu.Unlock()
	if len(oido) < 48000 {
		t.Fatalf("solo %d muestras del micro en 2 s", len(oido))
	}
	ultimo := oido[len(oido)-48000:]
	var e float64
	for _, v := range ultimo {
		e += float64(v) * float64(v)
	}
	t.Logf("nivel del micro: %.1f RMS", math.Sqrt(e/float64(len(ultimo))))
	vivo := false
	for _, v := range ultimo {
		vivo = vivo || v != 0
	}
	if !vivo {
		t.Fatal("el micro da ceros: sin permiso o sin captura, y entonces no se mide nada")
	}
	return goertzel(ultimo, 1000)
}

// Con SYTUE_ECO_REAL=1, en un Mac de verdad: el tono que sale por los altavoces vuelve
// por el micro con miniaudio y la VoiceProcessingIO lo quita. SUENA un pitido de dos
// segundos dos veces.
func TestLaVozDeMacQuitaElEco(t *testing.T) {
	if os.Getenv("SYTUE_ECO_REAL") == "" {
		t.Skip("sin SYTUE_ECO_REAL")
	}
	sin := eco(t, func(c, r func([]int16)) (func(), error) { return abrirMiniaudio(c, r, Sistema{}, malgo.Duplex) })
	con := eco(t, abrirVoz)
	t.Logf("1 kHz de vuelta: miniaudio %.1f, VoiceProcessingIO %.1f (×%.0f menos)", sin, con, sin/max(con, 1e-9))
	if con*4 > sin {
		t.Errorf("la VoiceProcessingIO no quita el eco: %.1f contra %.1f", con, sin)
	}
}
