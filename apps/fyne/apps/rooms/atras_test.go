// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
)

func TestElAtrasVuelveDelHiloYDelCanalSinPerderBorrador(t *testing.T) {
	sa, _, w := salaEn(t, servicioConDos(), 390)
	sa.Atajos(w.Canvas())
	sa.Arrancar(context.Background())
	defer sa.Parar()
	canal := sa.abierto
	borrador := ambito{canal: canal}
	sa.guardarBorrador(borrador, "Por terminar")
	sa.raizHilo = "h1"
	sa.P.PonerDerecho(container.NewStack())
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	atras()
	if sa.P.Canal.Derecho != nil || sa.raizHilo != "" || sa.abierto != canal {
		t.Fatal("atrás debe cerrar el hilo conservando el canal")
	}
	atras()
	if sa.abierto != "" || !sa.P.armazon.CajonAbierto || !sa.P.Canal.sinCanal {
		t.Fatal("atrás no vuelve del canal a la lista")
	}
	if sa.borradores[borrador] != "Por terminar" {
		t.Fatal("volver a los canales ha perdido el borrador")
	}
	sa.Elegir(Canal{ID: canal, Nombre: "equipo"})
	if sa.P.Canal.Compositor.Texto() != "Por terminar" {
		t.Fatal("al volver al canal no se recupera el borrador")
	}
	w.Resize(fyne.NewSize(844, 390))
	atras()
	if sa.abierto != "" || sa.borradores[borrador] != "Por terminar" {
		t.Fatal("al girar el teléfono Atrás dejó de volver a los canales conservando el borrador")
	}
}
