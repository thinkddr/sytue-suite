// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestEnElTelefonoElCanalVaEnLaBarraDeLaSuite: montado como en cmd/rooms, a 390 la barra
// de la app ES la de la suite (arriba del todo y de su alto) con «#equipo», y el
// historial empieza justo debajo: una sola barra y no dos.
func TestEnElTelefonoElCanalVaEnLaBarraDeLaSuite(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	sa, _, w := salaEn(t, servicioConDos(), 1280)
	barra, restaurar := ui.BarraSuiteDePrueba(traductor(t), iconos.Rooms)
	defer restaurar()
	w.SetContent(barra.Montar(sa.P.Objeto()))
	w.Resize(fyne.NewSize(390, 844))
	sa.Arrancar(context.Background())
	fyne.DoAndWait(func() {})
	movil := sa.P.movil
	donde := fyne.CurrentApp().Driver().AbsolutePositionForObject
	if y, alto := donde(movil).Y, movil.Size().Height; y != 0 || alto != ui.AltoBarraSuite() {
		t.Errorf("la barra de la app está en y=%v con %v de alto; quiero arriba del todo y %v", y, alto, ui.AltoBarraSuite())
	}
	if movil.Titulo != "#equipo" {
		t.Errorf("la barra dice %q", movil.Titulo)
	}
	if y := donde(sa.P.Canal.Historial).Y; y != ui.AltoBarraSuite() {
		t.Errorf("el historial empieza en y=%v; quiero justo bajo la barra única (%v)", y, ui.AltoBarraSuite())
	}
}
