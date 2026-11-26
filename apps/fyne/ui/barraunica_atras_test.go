// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestAtrasYEscapePleganLaSuiteDelTelefono es el `useMobileBack` de la cara de la suite en
// TopBar.tsx: con la suite desplegada en el teléfono, el «atrás» de Android (sin nada
// enfocado, que es lo normal con el dedo) y Escape la pliegan y no hacen nada más. Antes
// `Descartar` no la miraba: Escape no hacía nada y el «atrás» mandaba la app al fondo con
// la suite abierta. VISTO EN ROJO sin el bloque de `barraSuiteDe` de `Descartar`.
func TestAtrasYEscapePleganLaSuiteDelTelefono(t *testing.T) {
	defer TactilDePrueba(true)()
	b, parar := BarraSuiteDePrueba(traductorEs(t), iconos.Mail)
	t.Cleanup(parar)
	w := test.NewWindow(b.Montar(container.NewWithoutLayout()))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 700))
	c := w.Canvas()

	test.Tap(b.cuenta.boton)
	if !b.suiteDesplegada() {
		t.Fatal("la entrada no despliega la suite")
	}
	teclas := c.OnTypedKey()
	if teclas == nil {
		t.Fatal("el lienzo del teléfono no atiende las teclas sueltas («atrás» sin foco)")
	}
	teclas(&fyne.KeyEvent{Name: mobile.KeyBack})
	if b.suiteAbierta {
		t.Error("«atrás» con la suite desplegada no la pliega")
	}

	// Escape, subido por un control enfocado o sin foco: lo mismo.
	test.Tap(b.cuenta.boton)
	if !Descartar(c) || b.suiteAbierta {
		t.Error("Escape con la suite desplegada no la pliega")
	}
	// Plegada no hay nada que plegar: Escape sigue su camino.
	if Descartar(c) {
		t.Error("con la suite plegada Escape dice que ha cerrado algo")
	}

	// En ancho la suite está siempre a la vista: Escape no se la queda.
	test.Tap(b.cuenta.boton)
	w.Resize(fyne.NewSize(1280, 800))
	if Descartar(c) {
		t.Error("en ancho Escape dice que ha plegado una suite que no se puede plegar")
	}
}
