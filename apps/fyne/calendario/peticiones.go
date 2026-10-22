// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La franja de solo lectura de los calendarios compartidos de lector, con su «Pedir
// acceso de edición», y la del dueño que llega desde el aviso: el espejo de las
// `FranjaPeticion` que `CalendarApp.tsx` pone bajo la cabecera. La pieza es la común
// (`peticion`); aquí solo está de qué calendarios.

// engancharPeticiones rehace el estado de las franjas tras cargar los calendarios.
func (a *App) engancharPeticiones() {
	p := a.P
	if p.peticiones == nil {
		p.peticiones = map[string]*peticion.Estado{}
	}
	vivos := map[string]bool{}
	for _, c := range p.Compartidos {
		if c.Permission != api.PermisoLeer || a.Cal == nil {
			continue
		}
		vivos[c.ID] = true
		e := p.peticiones[c.ID]
		if e == nil {
			e = &peticion.Estado{}
			p.peticiones[c.ID] = e
		}
		pedida := c.EditRequested
		peticion.Enganchar(e, a.Cal.Peticiones(c.ID), peticion.Opciones{
			SinPermiso: true,
			YaPedida:   func(context.Context) (bool, error) { return pedida, nil },
			Recomponer: a.pintar,
			Fallo:      func(texto string) { a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, texto, "")) },
			T:          p.t,
			Concedido:  "calendar.access.granted", Recurso: c.ID,
			AlConceder: a.cargarCalendarios,
		})
	}
	for id, e := range p.peticiones {
		if !vivos[id] {
			e.Soltar()
			delete(p.peticiones, id)
		}
	}
}

// verPeticionesDe enseña al dueño, en la franja, quién le pide editar `id`: es a lo
// que lleva el aviso del Hub.
func (a *App) verPeticionesDe(id string) {
	if a.Cal == nil {
		return
	}
	p := a.P
	p.delAviso = id
	peticion.Enganchar(&p.peticionesDueno, a.Cal.Peticiones(id), peticion.Opciones{
		PuedeConceder: true, Recomponer: a.pintar, T: p.t,
	})
}

// conFranjas pone encima de `cuerpo` las franjas que toquen, o lo devuelve tal cual.
func (p *Pantalla) conFranjas(cuerpo fyne.CanvasObject) fyne.CanvasObject {
	var franjas []fyne.CanvasObject
	if !p.SinConexion {
		for _, c := range p.Compartidos {
			e := p.peticiones[c.ID]
			if e == nil || (p.Visibles != nil && !p.Visibles[c.ID]) {
				continue
			}
			franjas = append(franjas, peticion.Franja(e, true, p.t("readOnly.sharedWithYou", i18n.Vars{"name": c.Title()}), p.t))
		}
	}
	if p.delAviso != "" {
		for _, c := range p.Calendarios {
			if c.ID == p.delAviso {
				if f := peticion.Franja(&p.peticionesDueno, false, "", p.t); f != nil {
					franjas = append(franjas, f)
				}
			}
		}
	}
	if len(franjas) == 0 {
		return cuerpo
	}
	col := container.New(ui.Columna{}, franjas...)
	return container.New(&arriba{Alto: col.MinSize().Height}, col, cuerpo)
}
