// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada capa que se abre (ver `ui/recorrido`): primero «Mis bases» y
// después una base abierta como propietario, que es quien lo ve todo.
//
// Con `SYTUE_CAPTURAS_PLAY=<dir>` no comprueba: retrata cada pantalla a tamaño de
// teléfono, que es de donde salen las capturas de la ficha de Google Play
// (apps/fyne/play/README.md).
func TestRecorridoConSesion(t *testing.T) {
	_, _, lista := montar(t, map[string]string{"GET /bases": "[" + baseClientes + "," + basePedidos + "]"})
	montaje.ComprobarRotacion(t, lista, "bases/lista", iconos.Bases, false)
	recorrido.Comprobar(t, lista, 100)

	_, base := abrir(t, nuevoFalso(12))
	montaje.ComprobarRotacion(t, base, "bases/base", iconos.Bases, true)
	recorrido.Comprobar(t, base, 400)
}

// TestLaCabeceraDeLaBaseCabeEnUnTelefono: a 360 de ancho el nombre, «Compartir» y el
// menú «⋯» van en una línea sin pisarse ni salirse de la ventana (así se vio en las
// capturas de teléfono del 2026-10-05, cuando eran tres botones sueltos).
//
// Verlo en rojo: quitar el `Truncar` del nombre en objetoBase, con un nombre largo.
func TestLaCabeceraDeLaBaseCabeEnUnTelefono(t *testing.T) {
	p, w := abrir(t, nuevoFalso(3))
	w.Resize(fyne.NewSize(360, 640))
	p.abierta.Datos.Name = "Clientes de la delegación de Castilla y León, 2026"
	p.Repintar()
	var nombre *ui.Rotulo
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if r, ok := o.(*ui.Rotulo); ok && r.Texto == p.abierta.Datos.Name {
			nombre = r
		}
	})
	if nombre == nil {
		t.Fatal("no se ve el nombre de la base")
	}
	donde := fyne.CurrentApp().Driver().AbsolutePositionForObject
	compartir := boton(w, p.t("share.open"))
	if compartir == nil {
		t.Fatalf("no se ve «%s»", p.t("share.open"))
	}
	if n, c := donde(nombre), donde(compartir); c.Y < n.Y+nombre.Size().Height && c.X < n.X+nombre.Size().Width {
		t.Errorf("«%s» (en %v) pisa el nombre (en %v, %v de ancho)", p.t("share.open"), c, n, nombre.Size().Width)
	}
	mas := menuMasDe(w, p.abierta.Datos.Name)
	if mas == nil {
		t.Fatal("no se ve el menú «⋯» de la base")
	}
	for _, o := range []fyne.CanvasObject{compartir, mas} {
		x := donde(o).X
		if x < 0 || x+o.Size().Width > 360 {
			t.Errorf("%T va de %.0f a %.0f en una ventana de 360", o, x, x+o.Size().Width)
		}
	}
}
