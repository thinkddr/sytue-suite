// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS la lista de facturas en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`).
func TestCapturasEsteticas(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	montaje.Capturar(t, w, "erp-lista", iconos.ERP, func(m montaje.MedidaCaptura) {
		p.Ventana = m.Tam
		p.Repintar()
	})
	// La ficha de una factura, con su barra de etapas.
	p.abrirFicha(&p.lista.Pagina.Registros[1], nil)
	montaje.Capturar(t, w, "erp-ficha", iconos.ERP, func(m montaje.MedidaCaptura) {
		p.Ventana = m.Tam
		p.Repintar()
	})
}
