// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package llamada

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

// muestraEn es el índice de la muestra en el instante `d`.
func muestraEn(d time.Duration) int { return int(d.Seconds() * Muestreo) }

// sonoroEntre dice si hay alguna muestra distinta de cero en [desde, hasta).
func sonoroEntre(desde, hasta time.Duration) bool {
	for i := muestraEn(desde); i < muestraEn(hasta); i++ {
		if MuestraTimbre(i) != 0 {
			return true
		}
	}
	return false
}

// frecuenciaEntre estima la frecuencia contando los cruces por cero de subida.
func frecuenciaEntre(desde, hasta time.Duration) float64 {
	cruces, antes := 0, MuestraTimbre(muestraEn(desde))
	for i := muestraEn(desde) + 1; i < muestraEn(hasta); i++ {
		m := MuestraTimbre(i)
		if antes < 0 && m >= 0 {
			cruces++
		}
		antes = m
	}
	return float64(cruces) / (hasta - desde).Seconds()
}

// El bitono de `lib/timbre.ts`: re5 (587 Hz) de 0 a 180 ms, silencio hasta 220 ms, la5
// (880 Hz) hasta 440 ms y silencio hasta la tanda siguiente, a los 3 s. Con la rampa de
// 20 ms no chasquea: la primera muestra es 0 y la de pico no pasa del 0,14.
func TestTimbreBitonoConSuCadencia(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	if MuestraTimbre(0) != 0 {
		t.Errorf("el timbre empieza en %d: sin rampa chasquea", MuestraTimbre(0))
	}
	if f := frecuenciaEntre(ms(30), ms(150)); math.Abs(f-587.33) > 15 {
		t.Errorf("la primera nota suena a %.0f Hz y es un re5 (587)", f)
	}
	if sonoroEntre(ms(181), ms(219)) {
		t.Error("entre las dos notas no hay silencio")
	}
	if f := frecuenciaEntre(ms(250), ms(400)); math.Abs(f-880) > 15 {
		t.Errorf("la segunda nota suena a %.0f Hz y es un la5 (880)", f)
	}
	if sonoroEntre(ms(441), CadenciaTimbre) {
		t.Error("tras el bitono no calla hasta la siguiente tanda")
	}
	tope := int16(math.Ceil(volumenTimbre * math.MaxInt16))
	for i := range muestraEn(ms(500)) {
		if m := MuestraTimbre(i); m > tope || m < -tope {
			t.Fatalf("la muestra %d (%d) pasa del volumen de la web", i, m)
		}
	}
	// Cada tanda es la primera, desplazada 3 s.
	for tanda := 1; tanda < 10; tanda++ {
		base := tanda * muestraEn(CadenciaTimbre)
		for i := range muestraEn(ms(450)) {
			if MuestraTimbre(base+i) != MuestraTimbre(i) {
				t.Fatalf("la tanda %d no repite el bitono (muestra %d)", tanda, i)
			}
		}
	}
}

// A los 30 s se calla: diez tandas y ni una más.
func TestTimbreSeCallaEnElTope(t *testing.T) {
	tandas := 0
	for d := time.Duration(0); d < TopeTimbre+2*CadenciaTimbre; d += CadenciaTimbre {
		if sonoroEntre(d, d+500*time.Millisecond) {
			tandas++
		}
	}
	if tandas != 10 {
		t.Errorf("suenan %d tandas y son 10 (cada 3 s durante 30 s)", tandas)
	}
	if sonoroEntre(TopeTimbre, TopeTimbre+time.Second) {
		t.Error("pasado el tope sigue sonando")
	}
	var tb Timbre
	pcm := make([]int16, Tramo)
	for range muestraEn(TopeTimbre)/Tramo - 1 {
		if !tb.Rellenar(pcm) {
			t.Fatal("el timbre se acaba antes del tope")
		}
	}
	if tb.Rellenar(pcm) {
		t.Error("el último tramo antes del tope no dice que se acabó")
	}
}

// tarjetaTimbre es una tarjeta de mentira que apunta cómo se abre y se cierra. Con
// `espera`, Abrir no vuelve hasta que se cierra: un audio del sistema que tarda.
type tarjetaTimbre struct {
	mu         sync.Mutex
	capturado  bool
	reproducir func([]int16)
	abierta    chan struct{}
	cerrada    chan struct{}
	espera     chan struct{}
	err        error
}

func nuevaTarjetaTimbre() *tarjetaTimbre {
	return &tarjetaTimbre{abierta: make(chan struct{}), cerrada: make(chan struct{})}
}

func (d *tarjetaTimbre) Abrir(capturado, reproducir func([]int16)) (func(), error) {
	if d.espera != nil {
		<-d.espera
	}
	if d.err != nil {
		return nil, d.err
	}
	d.mu.Lock()
	d.capturado, d.reproducir = capturado != nil, reproducir
	d.mu.Unlock()
	close(d.abierta)
	var una sync.Once
	return func() { una.Do(func() { close(d.cerrada) }) }, nil
}

// esperar falla si `c` no se cierra en un rato.
func esperar(t *testing.T, c chan struct{}, que string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatal(que)
	}
}

// SonarTimbre abre la tarjeta SOLO para reproducir, la cierra al pararlo y, si nadie lo
// para, la cierra sola al llegar al tope. Una tarjeta que no abre no rompe nada.
func TestTimbreSuenaPorLaTarjetaYSeCierraSolo(t *testing.T) {
	d := nuevaTarjetaTimbre()
	parar := SonarTimbre(d)
	esperar(t, d.abierta, "el timbre no abre la tarjeta")
	if d.capturado || d.reproducir == nil {
		t.Fatal("el timbre abre el micrófono o no reproduce")
	}
	pcm := make([]int16, Tramo)
	d.reproducir(pcm)
	parar()
	parar() // dos veces, sin daño
	select {
	case <-d.cerrada:
	default:
		t.Fatal("parar no cierra la tarjeta")
	}

	d = nuevaTarjetaTimbre()
	SonarTimbre(d)
	esperar(t, d.abierta, "el timbre no abre la tarjeta")
	for range muestraEn(TopeTimbre)/Tramo + 1 {
		d.reproducir(pcm)
	}
	esperar(t, d.cerrada, "pasado el tope la tarjeta sigue abierta")

	SonarTimbre(&tarjetaTimbre{err: errors.New("sin tarjeta")})()
}

// Abrir el audio puede tardar, y SonarTimbre se llama desde el hilo de la interfaz:
// vuelve sin esperar a la tarjeta, y pararlo antes de que abra la cierra nada más abrir.
//
// VISTO EN ROJO (2026-11-09): con Abrir en la misma rutina, la prueba se queda colgada.
func TestTimbreNoEsperaALaTarjeta(t *testing.T) {
	d := nuevaTarjetaTimbre()
	d.espera = make(chan struct{})
	vuelve := make(chan func())
	go func() { vuelve <- SonarTimbre(d) }()
	var parar func()
	select {
	case parar = <-vuelve:
	case <-time.After(5 * time.Second):
		close(d.espera)
		t.Fatal("SonarTimbre espera a que abra la tarjeta")
	}
	parar()
	close(d.espera)
	esperar(t, d.cerrada, "parado antes de abrir, la tarjeta se queda abierta")
}
