// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que Calendar enseña en una ventana ESTRECHA (< 768, el `md` de la hoja): la barra de
// AppShell con la hamburguesa que saca el menú como cajón. Y el «+» flotante de «Crear
// evento», que desde el 2026-11-25 va en todos los anchos: en el teléfono el menú es un
// cajón cerrado y la acción principal tiene que quedar al alcance del pulgar, y en
// escritorio es el mismo control. Las dos piezas son del catálogo; aquí solo se conectan.

// nuevaBarraMovil es `ui.BarraMovil` con la hamburguesa. Calendar no le pasa título a
// AppShell ni tiene lista y detalle, así que no hay «volver». En el mes del teléfono la
// barra es su cabecera: `☰ · noviembre · [6]`, y el ☰ avisa de lo pendiente del cajón.
func (p *Pantalla) nuevaBarraMovil() *ui.BarraMovil {
	b := ui.NuevaBarraMovil()
	b.CajonAbierto = p.cajonAbierto
	b.RotuloAbrir, b.RotuloCerrar = p.t("ui.openNav", nil), p.t("ui.closeNav", nil)
	if p.compacta() {
		b.TituloCentrado = true
		b.Titulo = TituloCompacto(p.locale(), p.Cursor, p.ahora(), p.zona())
		b.Acciones = p.accionesBarraCompacta()
		if n := p.pendientesMenu(); n > 0 {
			b.Aviso = p.t("nav.menuPending", i18n.Vars{"count": n})
		}
	}
	b.AlMenu = func() {
		if p.AlConmutarCajon != nil {
			p.AlConmutarCajon()
		}
	}
	return b
}

// capaFlotante es la capa del «+» flotante (`BotonFlotante` en CalendarApp.tsx). No se
// pinta sin red —`{offline ? null : …}` en la web— ni con el cajón abierto, que en la web
// va por encima (`z-sidebar` contra `z-20`). En el mes del teléfono, sí: ahí era un «+»
// de la barra, y crea igual en el día elegido, que es el cursor.
func (a *App) capaFlotante() *fyne.Container {
	p := a.P
	if p.SinConexion || p.cajonAbierto {
		return container.New(ui.CapaFlotante{})
	}
	return container.New(ui.CapaFlotante{},
		ui.NuevoBotonFlotante(p.t("nav.createEvent", nil), func() { a.crearEn(p.Cursor) }))
}
