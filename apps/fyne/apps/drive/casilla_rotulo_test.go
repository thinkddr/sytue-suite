// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// rotulosDeCasillas son los nombres accesibles de las casillas visibles, en orden.
func rotulosDeCasillas(o fyne.CanvasObject) []string {
	var r []string
	recorrer(o, func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok && c.SinEtiqueta {
			r = append(r, c.AccessibilityLabel())
		}
	})
	return r
}

// TestLasCasillasNombranElElemento: la casilla de cada fila se anuncia como el
// `aria-label={t('files.select', { name })}` de la web —«Seleccionar informe.pdf»—, la de
// la cabecera como «Seleccionar todo», y en la cuadrícula igual. Con «Seleccionar fila»
// en todas, quien usa lector de pantalla no sabe qué fichero marca.
//
// Las filas de la lista se RECICLAN: al cambiar de carpeta la misma casilla pasa a ser la
// de otro fichero, y el rótulo tiene que seguirla.
func TestLasCasillasNombranElElemento(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 1280
	e.AlSeleccionar = func(string) {}
	e.Elementos = []Elemento{{ID: "c1", Nombre: "Contratos", Carpeta: true}, {ID: "f1", Nombre: "informe.pdf"}}
	sel := func(n string) string { return e.t("files.select", i18n.Vars{"name": n}) }
	obj := e.Objeto(tema.Claro)
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(fyne.NewSize(1020, 400))

	quiero := []string{e.t("files.selectAll"), sel("Contratos"), sel("informe.pdf")}
	if got := rotulosDeCasillas(obj); !slices.Equal(got, quiero) {
		t.Errorf("las casillas se anuncian %q; quiero %q", got, quiero)
	}
	if quiero[1] == e.t("files.select") || quiero[1] == "" {
		t.Fatalf("el catálogo no trae `files.select` con el nombre: %q", quiero[1])
	}

	// Otra carpeta, mismas filas recicladas.
	e.Elementos = []Elemento{{ID: "f2", Nombre: "acta.odt"}, {ID: "f3", Nombre: "presupuesto.ods"}}
	obj = e.Objeto(tema.Claro)
	w.SetContent(obj)
	quiero = []string{e.t("files.selectAll"), sel("acta.odt"), sel("presupuesto.ods")}
	if got := rotulosDeCasillas(obj); !slices.Equal(got, quiero) {
		t.Errorf("recicladas, las casillas se anuncian %q; quiero %q", got, quiero)
	}

	// La cuadrícula.
	e.Rejilla = true
	obj = e.Objeto(tema.Claro)
	w.SetContent(obj)
	if got := rotulosDeCasillas(obj); !slices.Equal(got, quiero[1:]) {
		t.Errorf("en la cuadrícula las casillas se anuncian %q; quiero %q", got, quiero[1:])
	}
}
