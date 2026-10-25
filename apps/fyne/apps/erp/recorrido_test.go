// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	tamano := fyne.NewSize(1280, 800)
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		tamano = fyne.NewSize(360, 640)
	}
	p, w := pantallaArnes(t, tamano)
	t.Cleanup(w.Close)
	if p.lista.Fase != Exito || len(p.lista.Pagina.Registros) == 0 {
		t.Fatal("las facturas deben estar cargadas antes de girar")
	}
	montaje.ComprobarRotacion(t, w, "erp", iconos.ERP, true)
	recorrido.Comprobar(t, w, 400)
}
