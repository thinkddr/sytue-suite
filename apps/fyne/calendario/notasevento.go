// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las notas de Sytue Notepads que cuelgan del evento: EventNotes.tsx, en la vista rápida
// entre el detalle y las acciones.
//
//	<div class="border-border mt-3 grid gap-1.5 border-t pt-2.5">
//	  <span class="sytue-meta">Notas</span>
//	  <ul class="grid gap-0.5">  <a class="flex items-center gap-1.5 text-xs text-primary-text
//	                              hover:underline"><Icon lock|text 13/> título truncado</a>
//	  <a class="inline-flex items-center gap-1.5 text-xs font-medium text-primary-text">
//	    <Icon plus 13/> Escribir una nota para este evento</a>
//
// El vínculo lo guarda Notepads: aquí solo se pregunta, con el UID del evento. Si
// Notepads no contesta, la sección no sale —ofrecer «escribir una nota» sin adónde ir es
// peor que no ofrecerlo—. Los enlaces abren la app de Notepads por su esquema y, si no
// está instalada, su web.

// ladoIconoNota es el `size={13}` de los iconos de la sección.
const ladoIconoNota float32 = 13

// RutaNota es el enlace a una nota en Notepads: `/notes/?notebook=…&note=…`.
func RutaNota(n api.Note) string {
	return "/notes/?notebook=" + url.QueryEscape(n.NotebookID) + "&note=" + url.QueryEscape(n.ID)
}

// RutaNotaNueva es «Escribir una nota para este evento»: Notepads crea la nota ya ligada
// al evento (`?link=calendar_event:<uid>`) y con su título (`label`).
func RutaNotaNueva(uid, titulo string) string {
	return "/notes/?link=calendar_event:" + url.QueryEscape(uid) + "&label=" + url.QueryEscape(titulo)
}

// seccionNotas es la sección entera para las notas de `o`.
func (p *Pantalla) seccionNotas(o Ocurrencia, notas []api.Note, abrir func(ruta string)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ir := func(ruta string) func() {
		return func() {
			if abrir != nil {
				abrir(ruta)
			}
		}
	}
	hijos := []fyne.CanvasObject{alto(ui.Texto(p.t("notes.linked", nil), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)), tema.TextoXS.Linea)}
	if len(notas) > 0 {
		filas := make([]fyne.CanvasObject, len(notas))
		for i, n := range notas {
			titulo := n.Title
			if titulo == "" {
				titulo = p.t("notes.untitled", nil)
			}
			icono := iconos.Texto
			if n.ContentCipher != "" {
				icono = iconos.Candado
			}
			filas[i] = enlaceConIcono(icono, titulo, ui.Normal, 1, ir(RutaNota(n)), v)
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: espacio0p5}, filas...))
	}
	crear := enlaceConIcono(iconos.Mas, p.t("notes.create", nil), ui.Medio, -1, ir(RutaNotaNueva(o.Event.UID, o.Event.Summary)), v)
	hijos = append(hijos, encajar(crear))
	filete := canvas.NewRectangle(tema.Color("border", v))
	return container.New(&conFileteArriba{Arr: tema.Espacio(3), Pad: tema.Espacio(2.5)},
		filete, container.New(ui.Columna{Hueco: espacio1p5}, hijos...))
}

// enlaceConIcono es el `<a>` de la sección: el icono de 13 en la tinta del enlace y el
// texto. Con `elastico` 1 el texto se queda con el ancho y se recorta si no cabe (el
// `flex` de una nota); con -1 todo va a su medida (el `inline-flex` de «Escribir»).
func enlaceConIcono(icono iconos.Icono, texto string, peso ui.Peso, elastico int, alPulsar func(), v tema.Variante) fyne.CanvasObject {
	e := ui.NuevoEnlace(texto, tema.TextoXS, peso, "primary-text", alPulsar)
	glifo := iconos.Trazo(icono, ladoIconoNota, "primary-text", v).Objeto()
	return container.New(filaElastica{Hueco: espacio1p5, Elastico: elastico}, glifo, e)
}
