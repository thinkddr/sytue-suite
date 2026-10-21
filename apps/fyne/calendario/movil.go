// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que Calendar enseña en una ventana ESTRECHA (< 768, el `md` de la hoja): la barra de
// AppShell con la hamburguesa que saca el menú como cajón, y el botón flotante de «Crear
// evento» —en el teléfono el menú es un cajón cerrado y la acción principal tiene que
// quedar al alcance del pulgar—. Las dos piezas son del catálogo; aquí solo se conectan.

// nuevaBarraMovil es `ui.BarraMovil` con la hamburguesa. Calendar no le pasa título a
// AppShell ni tiene lista y detalle, así que no hay «volver».
func (p *Pantalla) nuevaBarraMovil() *ui.BarraMovil {
	b := ui.NuevaBarraMovil()
	b.CajonAbierto = p.cajonAbierto
	b.RotuloAbrir, b.RotuloCerrar = p.t("ui.openNav", nil), p.t("ui.closeNav", nil)
	b.AlMenu = func() {
		if p.AlConmutarCajon != nil {
			p.AlConmutarCajon()
		}
	}
	return b
}

// capaFlotante es la capa del MobileFab. No se pinta sin red —`{offline ? null : …}` en
// la web— ni con el cajón abierto, que en la web va por encima (`z-sidebar` contra
// `z-20`).
func (a *App) capaFlotante() *fyne.Container {
	p := a.P
	if p.SinConexion || p.cajonAbierto {
		return container.New(ui.CapaFlotante{})
	}
	return container.New(ui.CapaFlotante{},
		ui.NuevoBotonFlotante(p.t("nav.createEvent", nil), iconos.Mas, func() { a.crearEn(p.Cursor) }))
}
