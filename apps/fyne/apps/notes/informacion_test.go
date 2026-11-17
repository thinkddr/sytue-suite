// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestEstadisticasDeLaNotaSonLasDeLaWeb: la misma nota que cuenta NoteInfo.test.tsx da
// los mismos números (4 palabras y 16 caracteres: sin el «•» de las viñetas que pone
// htmltext.Text), con las fechas, el cuaderno y cuántas notas apuntan aquí.
func TestEstadisticasDeLaNotaSonLasDeLaWeb(t *testing.T) {
	creada := time.Date(2026, 10, 20, 8, 15, 0, 0, time.UTC)
	editada := time.Date(2026, 11, 2, 17, 40, 0, 0, time.UTC)
	p, _ := pantallaConServicio(t, map[string]string{
		"GET /notes/linked": `{"notes":[{"id":"x","notebook_id":"c1"},{"id":"y","notebook_id":"c2"}]}`,
	}, api.Note{
		ID: "a", NotebookID: "c1", Kind: "text", Title: "Lista",
		Content:   "<ul><li>pan</li><li>leche</li></ul><p>y <b>café</b></p>",
		CreatedAt: creada, UpdatedAt: editada,
	})
	w := test.NewWindow(nil)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.win = w
	n, _ := p.nota("a")
	tr := p.tr.T

	p.dialogoInformacion(n)
	quiero := map[string]string{
		tr("noteInfo.words", nil):      "4",
		tr("noteInfo.characters", nil): "16",
		tr("noteInfo.reading", nil):    tr("noteInfo.readingValue", i18n.Vars{"count": 1}),
		tr("noteInfo.created", nil):    i18n.FormatDateTime("es", creada.Local()),
		tr("noteInfo.edited", nil):     i18n.FormatDateTime("es", editada.Local()),
		tr("noteInfo.notebook", nil):   "Mudanza",
		tr("noteInfo.backlinks", nil):  tr("notebook.notes", i18n.Vars{"count": 2}),
	}
	var vistos []string
	for range 200 {
		vistos = textosDe(w.Canvas().Overlays().Top())
		if slices.Contains(vistos, quiero[tr("noteInfo.backlinks", nil)]) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !slices.Contains(vistos, tr("note.info", nil)) || !slices.Contains(vistos, "Lista") {
		t.Errorf("el diálogo no dice qué es ni de qué nota: %v", vistos)
	}
	// Cada rótulo va seguido de su valor, en el orden de la web.
	for rotulo, valor := range quiero {
		i := slices.Index(vistos, rotulo)
		if i < 0 || i+1 >= len(vistos) || vistos[i+1] != valor {
			t.Errorf("%q no enseña %q: %v", rotulo, valor, vistos)
		}
	}
}

// TestEstadisticasDeUnaNotaCifradaCerrada: sin la clave no hay texto y se dice «—», no
// un cero que mentiría.
func TestEstadisticasDeUnaNotaCifradaCerrada(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1", ContentCipher: "encv1:xx"})
	p.activo.Cifrado, p.bloqueado = true, true
	n, _ := p.nota("a")
	filas := p.filasInformacion(n, sinDato)
	for _, f := range filas[:3] {
		if f[1] != sinDato {
			t.Errorf("cerrada, %q dice %q", f[0], f[1])
		}
	}
	// Abierta (con el claro en memoria) sí se cuenta.
	p.bloqueado = false
	p.clave = make([]byte, 32)
	n.Contenido = "<p>dos palabras</p>"
	if got := p.filasInformacion(n, sinDato)[0][1]; got != "2" {
		t.Errorf("abierta, las palabras son %q", got)
	}
}

// TestElMenuOfreceInformacion: la entrada está en el «…» de la tarjeta y del árbol y
// abre el diálogo.
func TestElMenuOfreceInformacion(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1", Title: "Lista"})
	w := test.NewWindow(nil)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.win = w
	n, _ := p.nota("a")
	for _, arbol := range []bool{false, true} {
		var entrada *ui.ElementoMenu
		for _, e := range p.menuNota(n, arbol).Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == p.tr.T("note.info", nil) {
				entrada = it
			}
		}
		if entrada == nil {
			t.Fatalf("árbol=%v: el «…» no ofrece «Información»", arbol)
		}
		entrada.AlPulsar()
		if !slices.Contains(textosDe(w.Canvas().Overlays().Top()), p.tr.T("noteInfo.words", nil)) {
			t.Errorf("árbol=%v: «Información» no abre su diálogo", arbol)
		}
		for _, o := range w.Canvas().Overlays().List() {
			w.Canvas().Overlays().Remove(o)
		}
	}
}

func textosDe(o fyne.CanvasObject) []string {
	var out []string
	if o == nil {
		return out
	}
	recorrerInterno(o, func(x fyne.CanvasObject) {
		if s, ok := x.(*canvas.Text); ok && s.Text != "" {
			out = append(out, s.Text)
		}
	})
	return out
}
