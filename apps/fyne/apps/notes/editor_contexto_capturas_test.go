// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLasCapturasDeLasBarrasDeContexto pinta las dos barras que salen según dónde está el
// cursor: la de la tabla (el cursor en una celda) y la burbuja de la selección, en el
// teléfono (390, táctil) y en el escritorio (1280), en claro y en oscuro. Sin
// `SYTUE_CAPTURAS` solo comprueba que pintan.
func TestLasCapturasDeLasBarrasDeContexto(t *testing.T) {
	const tabla = `<p>Recuento de la mudanza:</p><table><tbody><tr><th>Cuarto</th><th>Cajas</th><th>Notas</th></tr>` +
		`<tr><td>Cocina</td><td>12</td><td>frágil</td></tr><tr><td>Salón</td><td>7</td><td></td></tr></tbody></table><p>Fin.</p>`
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, tam := range []fyne.Size{{Width: 390, Height: 520}, {Width: 1280, Height: 520}} {
			for _, caso := range []string{"tabla", "burbuja"} {
				nombre := fmt.Sprintf("%s-%dx%d-%s", caso, int(tam.Width), int(tam.Height), map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
				t.Run(nombre, func(t *testing.T) {
					conTema(t, v)
					movil := tam.Width < ui.CorteSM
					t.Cleanup(ui.TactilDePrueba(movil))
					t.Cleanup(ui.MovilDePrueba(movil))
					ed := NuevoEditorNota(trInterno(t), Leer(tabla), "")
					w := test.NewWindow(ed)
					t.Cleanup(w.Close)
					w.SetPadded(false)
					w.Resize(tam)
					w.Canvas().Focus(ed.Zona())
					if caso == "tabla" {
						ed.cur = Pos{2, 2} // «Cajas», en la cabecera
						ed.ancla = ed.cur
					} else {
						ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 9} // «Recuento»
					}
					ed.repintar()
					ed.Refresh()
					if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
						t.Fatal("la captura ha salido vacía")
					}
					capturar(t, w, nombre)
				})
			}
		}
	}
}
