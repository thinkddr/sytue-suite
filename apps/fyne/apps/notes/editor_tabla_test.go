// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasFlechasEntranEnLaTabla: con un párrafo encima de una tabla estrecha, subir desde
// el párrafo de debajo —con el cursor más a la derecha que la tabla— entra en su última
// fila, por la celda más cercana, y no se la salta; de una celda se sube a la de encima
// de su columna, sin pasar por las otras de su fila; y bajar desde el párrafo de arriba
// entra en la primera fila.
//
// VISTO EN ROJO (2026-09-25, en el frontal): con la preferencia por la columna mirando
// TODAS las líneas, el párrafo de arriba (de lado a lado, siempre «en la columna») ganaba
// a la última fila y la flecha saltaba la tabla entera. Es lo que vio Mail.
func TestLasFlechasEntranEnLaTabla(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer(`<p>arriba</p><table><tbody><tr><td>a1</td><td>b1</td></tr>`+
		`<tr><td>a2</td><td>b2</td></tr></tbody></table><p>una línea bastante más ancha que la tabla</p>`), "")
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(500, 300))
	z := ed.zona
	flecha := func(k fyne.KeyName, quiero int, que string) {
		t.Helper()
		z.TypedKey(&fyne.KeyEvent{Name: k})
		if ed.cur.B != quiero {
			t.Fatalf("%s: el cursor queda en el bloque %d (%q) y tenía que ir al %d", que, ed.cur.B,
				ed.doc.Bloques[ed.cur.B].Texto(), quiero)
		}
	}
	flecha(fyne.KeyUp, 4, "subir desde debajo de la tabla")
	flecha(fyne.KeyUp, 2, "subir de b2")
	flecha(fyne.KeyUp, 0, "subir de la primera fila")
	flecha(fyne.KeyDown, 2, "bajar desde el párrafo de arriba")

	ed.moverA(Pos{3, 1}) // en «a2»
	ed.hayX = false
	flecha(fyne.KeyUp, 1, "subir de a2")
}

// TestUnaCeldaCombinadaNoSaleCruzada: los bordes van por celda, así que la que ocupa dos
// columnas no lleva la raya de entre las dos por dentro.
//
// VISTO EN ROJO (2026-09-25, en el frontal): con la rejilla de lado a lado, la raya de
// entre columnas cruzaba «Todo junto».
func TestUnaCeldaCombinadaNoSaleCruzada(t *testing.T) {
	const td = `<td style="border: 1px solid #d4d0c8; padding: 6px 8px"`
	d := Leer(`<table><tbody><tr>` + td + `>Cuarto</td>` + td + `>Cajas</td></tr><tr>` + td +
		` colspan="2">Todo junto</td></tr></tbody></table>`)
	m := Maquetar(d, 300, tema.Claro)
	var junto LineaProsa
	for _, l := range m.Lineas {
		if len(l.Trozos) > 0 && l.Trozos[0].Texto == "Todo junto" {
			junto = l
		}
	}
	if junto.ZonaX1 == 0 {
		t.Fatal("no está la línea de la celda combinada")
	}
	rayas := 0
	for _, a := range m.Adornos {
		if a.Tipo != adornoCaja || a.An != 1 {
			continue
		}
		rayas++
		if a.X > junto.ZonaX0 && a.X < junto.ZonaX1-1 && a.Y < junto.Y+junto.Alto && a.Y+a.Al > junto.Y {
			t.Errorf("una raya vertical cruza la celda combinada en x=%.1f", a.X)
		}
	}
	if rayas < 6 {
		t.Errorf("solo hay %d rayas verticales: faltan bordes", rayas)
	}
}

// TestSeEntraEnUnaCeldaConRowspanDesdeCualquierFila: la celda que ocupa dos filas está en
// las dos, así que subir desde la fila de debajo, en su columna, entra en ella.
//
// VISTO EN ROJO (2026-09-26, en el frontal): agrupando por `Celda.Fila`, la flecha iba a
// «B2», la de la otra columna.
func TestSeEntraEnUnaCeldaConRowspanDesdeCualquierFila(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer(`<table><tbody><tr><td rowspan="2">A</td><td>B1</td></tr>`+
		`<tr><td>B2</td></tr><tr><td>C1</td><td>C2</td></tr></tbody></table>`), "")
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(500, 300))
	ed.moverA(Pos{3, 0}) // en «C1»
	ed.hayX = false
	ed.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if ed.cur.B != 0 {
		t.Errorf("subir de C1 deja el cursor en %q y tenía que entrar en «A»", ed.doc.Bloques[ed.cur.B].Texto())
	}
}
