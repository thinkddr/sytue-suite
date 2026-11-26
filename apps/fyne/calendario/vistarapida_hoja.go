// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// abrirVistaRapidaHoja es la vista rápida en el TELÉFONO, como en `EventPopover.tsx`
// (`useEnHoja`): un evento de la vista de mes mide 40 en una pantalla de 390 y el panel
// no cabe junto a él, así que se presenta como la hoja de los diálogos —el título del
// evento como título, el detalle en el cuerpo y las acciones en el pie, a todo el
// ancho—. Mismo contenido y mismas acciones que el panel de escritorio.
func (a *App) abrirVistaRapidaHoja(c fyne.Canvas, o Ocurrencia) {
	p := a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var d *ui.Dialogo
	cerrar := func(despues func()) func() {
		return func() {
			d.Cerrar()
			if despues != nil {
				despues()
			}
		}
	}
	solo := o.SoloLectura
	var pie fyne.CanvasObject
	var eliminar *ui.Boton
	switch {
	case solo:
		pie = container.New(ui.PieAcciones{},
			ui.NuevoBoton(p.t("popover.copyToMine", nil), cerrar(func() { a.duplicar(o) })).Con(ui.Secundario, ui.TallaMD))
	default:
		// Duplicar | Editar en el pie, con el principal abajo a la derecha. «Eliminar» NO
		// va en él: con tres acciones la primera sale sola a todo el ancho, y la hoja
		// abría con un botón rojo macizo encima de todo (auditoría del 2026-11-24). Va al
		// final del cuerpo, de fantasma de peligro y a su ancho: se encuentra, pero no
		// es lo primero que se ve.
		pie = container.New(ui.PieAcciones{},
			ui.NuevoBoton(p.t("popover.duplicate", nil), cerrar(func() { a.duplicar(o) })).Con(ui.Secundario, ui.TallaMD),
			ui.NuevoBoton(p.t("common.edit", nil), cerrar(func() { a.editar(o) })).Con(ui.Primario, ui.TallaMD))
		eliminar = ui.NuevoBoton(p.t("common.delete", nil), cerrar(func() { a.eliminar(o) })).Con(ui.Fantasma, ui.TallaMD).
			ConIcono(iconos.Papelera, 16)
		eliminar.Destructivo = true
	}
	d = ui.NuevoDialogo(resumenDe(o, p.t("event.untitled", nil)), nil).Con(ui.DialogoSM, pie)
	ancho := ui.AnchoCuerpoDialogo(d, c.Size())
	parrafo := func(s, token string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, token, v).A(ancho)
	}
	var detalle []fyne.CanvasObject
	if cuando := p.cuandoDe(o); cuando != "" {
		// El punto del color del calendario delante del «cuándo»: en la hoja no hay
		// cabecera propia donde ponerlo.
		punto := puntoDe(o.Color, ladoPuntoRapida)
		detalle = append(detalle, container.NewBorder(nil, nil, container.NewCenter(punto), nil,
			ui.NuevoParrafo(cuando, tema.TextoSM, ui.Normal, "text-muted", v).A(ancho-ladoPuntoRapida-espacio2)))
	}
	if Disponible(o) {
		detalle = append(detalle, parrafo(p.t("popover.showsAsFree", nil), "text-muted"))
	}
	if o.Event.Location != "" {
		detalle = append(detalle, parrafo(o.Event.Location, "text-muted"))
	}
	if o.Event.Description != "" {
		detalle = append(detalle, parrafo(o.Event.Description, "text"))
	}
	hijos := []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, detalle...)}
	if inv := p.seccionInvitados(o.Event, ancho, tema.TextoSM); inv != nil {
		hijos = append(hijos, inv)
	}
	if o.Event.ConferenceURL != "" && a.AbrirEnlace != nil {
		url := o.Event.ConferenceURL
		unirse := ui.NuevoBoton(p.t("popover.join", nil), func() { a.AbrirEnlace(url) }).Con(ui.Primario, ui.TallaMD)
		hijos = append(hijos, container.NewStack(unirse))
	}
	if a.Notas != nil && o.Event.UID != "" {
		hueco := container.New(ui.Columna{})
		hijos = append(hijos, hueco)
		uid := o.Event.UID
		a.lanzar(func(vida context.Context) {
			l, err := a.notasDe(vida, uid)
			a.enUI(func() {
				if err != nil {
					return
				}
				hueco.Objects = []fyne.CanvasObject{p.seccionNotas(o, l, func(ruta string) {
					d.Cerrar()
					if a.AbrirNotas != nil {
						a.AbrirNotas(ruta)
					}
				})}
				hueco.Refresh()
				d.Recolocar()
			})
		})
	}
	if respuesta := a.miRespuesta(o); respuesta != "" {
		botones := p.botonesRespuesta(respuesta, ui.TallaMD, func(estado string) {
			cerrar(func() { a.responder(o, estado) })()
		})
		// `border-border-subtle border-t pt-3`, como en la web.
		filete := canvas.NewRectangle(tema.Color("border-subtle", v))
		hijos = append(hijos, container.New(&conFileteArriba{Pad: tema.Espacio(3)}, filete,
			container.New(ui.Columna{Hueco: espacio2},
				ui.NuevoParrafo(p.t("popover.respond", nil), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho),
				container.NewGridWithColumns(3, botones...))))
	}
	if solo {
		hijos = append(hijos, ui.NuevoParrafo(p.t("popover.readOnly", nil), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho))
	}
	// «Descargar .ics» y «Eliminar», al final del cuerpo y a su ancho: se encuentran sin
	// ser lo primero que se ve.
	var finales []fyne.CanvasObject
	if a.Cal != nil && o.Event.ID != "" {
		finales = append(finales, ui.NuevoBoton(p.t("popover.downloadIcs", nil), cerrar(func() { a.descargarEventoICS(o) })).
			Con(ui.Fantasma, ui.TallaMD).ConIcono(iconos.Descargar, 16))
	}
	if eliminar != nil {
		finales = append(finales, eliminar)
	}
	if len(finales) > 0 {
		hijos = append(hijos, container.NewHBox(finales...))
	}
	d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...))
	a.mostrarDialogo(d, c)
}

// notasDe pide a Notepads las notas de un evento, con el mismo plazo que el panel.
func (a *App) notasDe(vida context.Context, uid string) ([]api.Note, error) {
	ctx, parar := context.WithTimeout(vida, plazo)
	defer parar()
	return a.Notas(ctx, uid)
}
