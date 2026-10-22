// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "git.sytue.eu/sytue/one/apps/fyne/sesion"

func (b *BarraSuite) engancharBusqueda() {
	if b.s == nil {
		return
	}
	b.busquedaSesion = b.s.Estado()
	b.s.AlBuscar(func() {
		b.buscarPendiente = true
		b.atenderBusqueda()
	})
	b.s.AlCambiar(b.cambioBusqueda)
}

func (b *BarraSuite) cambioBusqueda(e sesion.Estado) {
	if e.Sujeto != b.busquedaSesion.Sujeto || e.GeneracionSesion != b.busquedaSesion.GeneracionSesion {
		b.buscador.campo.PonerTexto("")
		b.buscador.alEscribir("")
	}
	b.busquedaSesion = e
	b.atenderBusqueda()
}

// No usa pulsarAtajo: Buscar en el widget significa búsqueda global, aunque
// Calendar, ERP o Notas hayan cedido el atajo a otra función de su pantalla.
func (b *BarraSuite) atenderBusqueda() {
	if !b.buscarPendiente || b.s == nil || !b.s.Estado().Dentro() {
		return
	}
	c := lienzoDe(b.buscador.campo)
	if c == nil {
		return
	}
	b.buscarPendiente = false
	b.buscador.campo.PonerTexto("")
	b.buscador.alEscribir("")
	enfocar(c, b.buscador.campo)
}
