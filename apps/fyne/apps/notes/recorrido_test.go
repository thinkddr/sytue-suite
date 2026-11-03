// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	p, w := montarCaptura(t, 1280, 800, "columns")
	p.Repintar()
	montaje.ComprobarRotacion(t, w, "notes/tablero", iconos.Notes, true)
	recorrido.Comprobar(t, w, 400)
	if dir := os.Getenv("SYTUE_CAPTURAS_PLAY"); dir != "" {
		// El recorrido cambia notas y preferencias; esta muestra parte de otra escena
		// sin volver a recorrerla ni avanzar la numeración de las fichas anteriores.
		p, w := montarCaptura(t, 1280, 800, "list")
		recorrido.AMedida(w)
		restaurar := ui.TactilDePrueba(w.Canvas().Size().Width < ui.CorteSM)
		defer restaurar()
		p.orden, p.enfoque, p.ocultarTerminadas = OrdenManual, "", false
		p.Repintar()
		w.Canvas().Focus(nil)
		fuente, destino := p.lienzo.vivas["n3"], p.lienzo.vivas["n1"]
		if fuente == nil || destino == nil {
			t.Fatal("falta una nota de la escena de arrastre")
		}
		fuente.arrastrando, destino.insercion = true, -1
		fuente.Refresh()
		destino.Refresh()
		escribirPNG(t, filepath.Join(dir, "notes-lista-arrastre.png"), w.Canvas().Capture())
	}
}
