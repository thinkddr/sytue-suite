// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaCuentaDeLaBarraDiceDeQuienEsYLoEnseñaEnSuPista es el `TopBarButton` de
// AccountMenu.tsx: el botón se llama «Cuenta de Ana García» y, como la campana, lo
// enseña en una pista DEBAJO al llegar con el tabulador; pulsarlo la cierra.
//
// Dos defectos que guarda, VISTOS EN ROJO: la barra pisaba el nombre con «Cuenta» a
// secas en cada colocación (el lector oía uno u otro según quién pintara el último), y
// el avatar era el único control de la barra sin pista, aquí y en la web.
func TestLaCuentaDeLaBarraDiceDeQuienEsYLoEnseñaEnSuPista(t *testing.T) {
	ultimoCierrePista = time.Time{}
	b, parar := BarraSuiteDePrueba(traductorEs(t), iconos.Mail)
	t.Cleanup(parar)
	w := test.NewWindow(b.Montar(container.NewWithoutLayout()))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	cuenta := b.cuenta.boton

	// Sin ficha todavía, «Cuenta»: nunca «Cuenta de » con el hueco.
	if cuenta.Nombre != "Cuenta" {
		t.Errorf("sin ficha el botón se llama %q y quiero «Cuenta»", cuenta.Nombre)
	}

	// Llega la ficha y la barra se vuelve a colocar: el nombre se queda con la persona.
	b.cuenta.mu.Lock()
	b.cuenta.ficha = api.Profile{Name: "Ana García"}
	b.cuenta.mu.Unlock()
	b.cuenta.pintar()
	fyne.DoAndWait(func() {})
	w.Resize(fyne.NewSize(1200, 800))
	w.Resize(fyne.NewSize(1280, 800))
	if cuenta.Nombre != "Cuenta de Ana García" {
		t.Errorf("tras colocarse la barra el botón se llama %q y quiero «Cuenta de Ana García»", cuenta.Nombre)
	}

	// Con el teclado, su pista al instante, debajo y con ese nombre.
	w.Canvas().Focus(cuenta)
	p := cuenta.pista
	if p == nil || !p.Abierta() {
		t.Fatal("el avatar de la cuenta no enseña pista con el foco; la campana sí")
	}
	if p.Rotulo != "Cuenta de Ana García" || !p.Abajo {
		t.Errorf("la pista dice %q (abajo=%v): quiero «Cuenta de Ana García», debajo", p.Rotulo, p.Abajo)
	}
	if p.Portal == nil {
		t.Error("la pista no sale por el portal de la pantalla: el contenido la taparía")
	}

	// Pulsarlo abre el menú y la cierra: no se queda encima de él.
	test.Tap(cuenta)
	if p.Abierta() {
		t.Error("tras pulsar la cuenta su pista sigue abierta encima del menú")
	}
	b.cuenta.menu.Cerrar()
}
