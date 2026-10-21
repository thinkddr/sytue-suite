// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Las capturas del diario, en los dos temas y en escritorio y teléfono: las que se
// comparan con las de la web. Sin `SYTUE_CAPTURAS` solo comprueban que pinta.
func TestLasCapturasDelDiario(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, tam := range []fyne.Size{{Width: 1280, Height: 800}, {Width: 390, Height: 844}} {
			nombre := fmt.Sprintf("diario-%dx%d-%s", int(tam.Width), int(tam.Height), map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
			t.Run(nombre, func(t *testing.T) {
				conTema(t, v)
				p, w := montarCaptura(t, tam.Width, tam.Height, "list")
				hoy := time.Now().Format(time.DateOnly)
				c := DeCuaderno(api.Notebook{ID: "d1", Title: hoy, Color: "#86198f", Layout: "list", DailyDate: hoy, NoteCount: 6, OwnerSub: "yo"})
				p.Yo = func() string { return "yo" }
				p.cuadernos = append(p.cuadernos, c)
				p.activo = c
				p.notas = DeNotas(notasDelDiario())
				p.Repintar()
				if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
					t.Fatal("la captura ha salido vacía")
				}
				capturar(t, w, nombre)
			})
		}
	}
}

func notasDelDiario() []api.Note {
	return []api.Note{
		{ID: "a", NotebookID: "d1", Kind: "todo", Title: "Llamar a la gestoría", Checked: true},
		{ID: "b", NotebookID: "d1", Kind: "todo", Title: "Revisar el presupuesto de octubre"},
		{ID: "c", NotebookID: "d1", Kind: "text", Content: "<p>La reunión se mueve al jueves a las 10:00.</p>"},
		{ID: "d", NotebookID: "d1", Kind: "text", Content: "<hr>"},
		{ID: "e", NotebookID: "d1", Kind: "link", Title: "sytue.eu", Data: []byte(`{"url":"https://sytue.eu"}`)},
		{ID: "f", NotebookID: "d1", Kind: "text", Pinned: true, Content: "<p>Ideas para el viaje: Lisboa o Oporto.</p>"},
	}
}

func conTema(t *testing.T, v tema.Variante) {
	t.Helper()
	nombre := "claro"
	if v == tema.Oscuro {
		nombre = "oscuro"
	}
	tema.Elegir(nombre)
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	t.Cleanup(func() {
		tema.Elegir("")
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	})
}
