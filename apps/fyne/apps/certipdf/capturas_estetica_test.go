// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS un PDF abierto en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`), y la hoja
// «Firmar» encima.
func TestCapturasEsteticas(t *testing.T) {
	p, w := documentoDeRecorrido(t)
	montaje.Capturar(t, w, "certipdf-documento", iconos.PDF)
	// El panel de páginas como cajón en el teléfono (en escritorio va siempre a la vista).
	p.PanelAbierto = true
	p.Recomponer()
	montaje.Capturar(t, w, "certipdf-paginas", iconos.PDF)
	p.PanelAbierto = false
	p.Recomponer()
	montaje.Capturar(t, w, "certipdf-firmar", iconos.PDF, func(montaje.MedidaCaptura) {
		if p.dialogo != nil {
			p.dialogo.cerrar()
		}
		p.abrirDialogo(DialogoFirmar)
	})
}
