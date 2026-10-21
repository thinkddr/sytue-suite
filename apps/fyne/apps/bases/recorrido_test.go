// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
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
	recorrido.Comprobar(t, lista, 100)

	_, base := abrir(t, nuevoFalso(12))
	recorrido.Comprobar(t, base, 400)
}

// TestLaCabeceraDeLaBaseCabeEnUnTelefono: a 360 de ancho el nombre y los tres botones del
// propietario no caben en una fila; tienen que partir, no pisarse ni salirse de la
// ventana (así se vio en las capturas de teléfono del 2026-10-05).
//
// Verlo en rojo: quitar el `flujo` de la cabecera en objetoBase.
func TestLaCabeceraDeLaBaseCabeEnUnTelefono(t *testing.T) {
	p, w := abrir(t, nuevoFalso(3))
	w.Resize(fyne.NewSize(360, 640))
	p.Repintar()
	// El nombre de la base, entero y sin nada encima: el primer botón va después o debajo.
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
	if n, c := donde(nombre), donde(compartir); c.Y < n.Y+nombre.Size().Height && c.X < n.X+nombre.MinSize().Width {
		t.Errorf("«%s» (en %v) pisa el nombre (en %v, %v de ancho)", p.t("share.open"), c, n, nombre.MinSize().Width)
	}
	for _, clave := range []string{"share.open", "base.rename", "base.delete"} {
		b := boton(w, p.t(clave))
		if b == nil {
			t.Fatalf("no se ve «%s»", p.t(clave))
		}
		x := fyne.CurrentApp().Driver().AbsolutePositionForObject(b).X
		if x < 0 || x+b.Size().Width > 360 {
			t.Errorf("«%s» va de %.0f a %.0f en una ventana de 360", p.t(clave), x, x+b.Size().Width)
		}
	}
}
