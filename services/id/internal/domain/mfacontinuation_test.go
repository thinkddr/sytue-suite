// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

// El token que sostiene el segundo factor entre «pídeme el código» y «aquí está»
// no puede caducar por cosas que pasan solas.
//
// Ataba la validez a que la IP y el User-Agent fueran IDÉNTICOS, y las dos
// cambian sin que el usuario haga nada: el móvil salta de wifi a datos, una red
// dual resuelve una petición por IPv4 y la siguiente por IPv6, y el navegador se
// actualiza mientras el formulario está abierto. Cuando eso pasaba, un código
// correcto se rechazaba como incorrecto y no había forma de salir del bucle:
// justo el lockout que un segundo factor no debe provocar.
func TestMFAContinuationSobreviveALoQueCambiaSolo(t *testing.T) {
	const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/140.0.7259.2 Safari/537.36"
	// El mismo navegador tras actualizarse solo: otra versión, mismo todo.
	const chromeActualizado = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/141.0.7390.55 Safari/537.36"

	ahora := time.Now()
	cont := MFAContinuation{
		UserAgent: chrome,
		IP:        "203.0.113.7",
		ExpiresAt: ahora.Add(5 * time.Minute),
	}

	if !cont.Valid(ahora, chrome) {
		t.Fatal("el caso normal no vale: nada más que probar")
	}
	// Lo que ANTES rompía y ahora tiene que funcionar.
	if !cont.Valid(ahora, chromeActualizado) {
		t.Error("el navegador se actualizó a mitad del segundo factor y perdió la sesión")
	}

	// Y lo que tiene que seguir rechazándose: otro navegador y otro sistema.
	const firefox = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:130.0) Gecko/20100101 Firefox/130.0"
	if cont.Valid(ahora, firefox) {
		t.Error("un token llevado a otro navegador se aceptó")
	}
	const chromeEnWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/140.0.7259.2 Safari/537.36"
	if cont.Valid(ahora, chromeEnWindows) {
		t.Error("un token llevado a otro sistema operativo se aceptó")
	}

	// La caducidad sigue mandando: es lo que de verdad acota la ventana.
	if cont.Valid(ahora.Add(6*time.Minute), chrome) {
		t.Error("un token caducado se aceptó")
	}
}
