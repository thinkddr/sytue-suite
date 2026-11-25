// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que la auditoría estética del 2026-11-24 vio en Drive en el teléfono.

// exploradorEnElTelefono monta el explorador con cuatro carpetas y tres ficheros, en
// Marco, con la barra de la suite y el dedo, a 390 × 844.
func exploradorEnElTelefono(t *testing.T) (*Explorador, fyne.Window) {
	t.Helper()
	t.Cleanup(ui.TactilDePrueba(true))
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 390
	for i, n := range []string{"Actas", "Contratos firmados", "Diseño", "Fotografías del viaje"} {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("c%d", i), Nombre: n, Carpeta: true, Tamano: -1})
	}
	for i := range 3 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("f%d", i), Nombre: fmt.Sprintf("acta-%d.md", i), Tamano: 10})
	}
	arm := ui.NuevoArmazon(e.Objeto(tema.Claro))
	barra, parar := ui.BarraSuiteDePrueba(e.tr, iconos.Drive)
	t.Cleanup(parar)
	w := test.NewTempWindow(t, barra.Montar(arm))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	return e, w
}

// TestLaFranjaDeCarpetasNoPartaUnaTarjeta: en Marco la franja de carpetas mide hasta el
// 45 % del cuerpo, y a 390 × 844 eso dejaba la tercera tarjeta partida bajo la cabecera
// «NOMBRE», con su nombre tapado. Ahora la franja acaba entre dos filas: cada tarjeta se
// ve entera o no se ve, y las demás se alcanzan desplazando la franja.
//
// VISTO EN ROJO con el 45 % a secas: «Diseño» enseña 71 de sus 102 px.
func TestLaFranjaDeCarpetasNoPartaUnaTarjeta(t *testing.T) {
	e, _ := exploradorEnElTelefono(t)
	franja := e.franjaScroll
	if franja == nil || len(e.tarjetas) != 4 {
		t.Fatalf("sin franja de carpetas: %v, %d tarjetas", franja, len(e.tarjetas))
	}
	d := fyne.CurrentApp().Driver()
	arriba := d.AbsolutePositionForObject(franja).Y
	abajo := arriba + franja.Size().Height
	enteras := 0
	for _, tj := range e.tarjetas {
		y := d.AbsolutePositionForObject(tj.objeto).Y
		fin := y + tj.objeto.Size().Height
		switch {
		case y >= arriba-0.5 && fin <= abajo+0.5:
			enteras++
		case y < abajo-0.5:
			t.Errorf("«%s» se ve partida: de %v a %v en una franja que acaba en %v", tj.el.Nombre, y, fin, abajo)
		}
	}
	if enteras == 0 || enteras == len(e.tarjetas) {
		t.Errorf("se ven enteras %d de %d tarjetas: el fixture no recorta la franja", enteras, len(e.tarjetas))
	}
	if franja.Size().Height > TopeFranja*e.listaFranja.Size().Height+0.5 {
		t.Errorf("la franja mide %v y pasa del 45 %% del cuerpo", franja.Size().Height)
	}
}

// TestLaBarraDeSeleccionCabeEnElTelefono: con dos marcados, «Mover», «Descargar» y
// «Papelera» con su rótulo no caben entre el número y el aspa a 390, y la tira dejaba
// «Papeler» contra el borde. En el teléfono, si no caben, van solo con su glifo (con el
// rótulo como nombre accesible); con ratón, o si caben, con su rótulo.
//
// VISTO EN ROJO sin el paso a solo glifo: las tres siguen con su rótulo.
func TestLaBarraDeSeleccionCabeEnElTelefono(t *testing.T) {
	e, _ := exploradorEnElTelefono(t)
	e.Seleccion["f0"], e.Seleccion["c1"] = true, true
	b, ok := e.barraSeleccion(tema.Claro, 2).(*ui.BarraDeLote)
	if !ok {
		t.Fatal("la barra de selección no es una ui.BarraDeLote")
	}
	ventana := test.NewTempWindow(t, b)
	ventana.SetPadded(false)
	ventana.Resize(fyne.NewSize(390, ui.MedidaBarraDeLoteEn(b, 390).Height))
	for _, a := range b.Acciones {
		if !a.SoloIcono || a.Rotulo != a.Texto || a.Texto == "" {
			t.Errorf("«%s» en el teléfono: solo glifo %v, nombre %q", a.Texto, a.SoloIcono, a.Rotulo)
		}
		if fin := fyne.CurrentApp().Driver().AbsolutePositionForObject(a).X + a.Size().Width; fin > 390 {
			t.Errorf("«%s» acaba en x = %v, fuera de la barra", a.Texto, fin)
		}
	}

	// Con ratón la barra es la de siempre, con sus rótulos.
	restaurar := ui.TactilDePrueba(false)
	defer restaurar()
	b, _ = e.barraSeleccion(tema.Claro, 2).(*ui.BarraDeLote)
	for _, a := range b.Acciones {
		if a.SoloIcono {
			t.Errorf("con ratón «%s» pierde su rótulo", a.Texto)
		}
	}
}
