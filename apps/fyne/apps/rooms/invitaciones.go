// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La gente de fuera acepta antes de entrar (decidido por el dueño el 2026-10-16:
// «Que acepte primero», también en DM y grupos). Es la sección «Invitaciones» de la
// barra de la web (`responderInvitacion` en ChatApp.tsx): al cargar los canales se
// avisa de las pendientes, y el aviso abre la lista con «Aceptar» y «Rechazar».

// Invitador es lo que la app necesita del servicio para las invitaciones.
type Invitador interface {
	Invitaciones(ctx context.Context) ([]api.RoomsInvitacion, error)
	ResponderInvitacion(ctx context.Context, canal string, aceptar bool) error
}

// TextoInvitacion es la línea de una invitación: quién, de qué organización y a qué.
func TextoInvitacion(t *i18n.Translator, inv api.RoomsInvitacion) string {
	org := api.OrgVisible(inv.Organizacion, inv.NombreOrganizacion)
	if inv.Clase == "direct" {
		return t.T("invitations.direct", i18n.Vars{"name": inv.NombreDeQuienInvita, "org": org})
	}
	return t.T("invitations.channel", i18n.Vars{"name": inv.NombreDeQuienInvita, "org": org, "channel": inv.Nombre})
}

// mirarInvitaciones pide las pendientes y, si hay, lo avisa con la puerta a la lista.
func (sa *Sala) mirarInvitaciones() {
	iv, ok := sa.S.(Invitador)
	if !ok {
		return
	}
	pedir(sa, iv.Invitaciones, func(lista []api.RoomsInvitacion, err error) {
		porEnlace := sa.invitacionesPorEnlace
		sa.invitacionesPorEnlace = false
		if err != nil || len(lista) == 0 {
			return
		}
		if porEnlace {
			sa.abrirInvitaciones(iv, lista)
			return
		}
		t := sa.P.T
		a := ui.NuevoAviso(ui.AvisoInfo, t.T("invitations.title", nil), TextoInvitacion(t, lista[0]))
		a.RotuloAccion = t.T("action.open", nil)
		a.AlPulsarAccion = func() { sa.abrirInvitaciones(iv, lista) }
		sa.P.Avisos.Notificar(a)
	})
}

// AbrirInvitacionesPorEnlace abre la lista desde el aviso del Hub (`?invitations=1`,
// COM-15). Con la app recién abierta el enlace llega antes que los canales: se apunta y
// se abre al cargarlos, que es cuando se miran las invitaciones.
func (sa *Sala) AbrirInvitacionesPorEnlace() {
	sa.invitacionesPorEnlace = true
	if sa.arrancada {
		sa.mirarInvitaciones()
	}
}

// abrirInvitaciones es la lista: cada una con «Aceptar» (entra y abre la conversación)
// y «Rechazar».
func (sa *Sala) abrirInvitaciones(iv Invitador, lista []api.RoomsInvitacion) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filas := container.New(ui.Columna{Hueco: tema.Espacio(3)})
	var d *ui.Dialogo
	for _, inv := range lista {
		var fila *fyne.Container
		responder := func(aceptar bool) {
			pedir(sa, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, iv.ResponderInvitacion(ctx, inv.Canal, aceptar)
			}, func(_ struct{}, err error) {
				if err != nil {
					sa.P.Avisar(ui.AvisoPeligro, t.T("invitations.failed", nil))
					return
				}
				filas.Remove(fila)
				if len(filas.Objects) == 0 {
					d.Cerrar()
				}
				if aceptar {
					sa.cargarCanales()
				}
			})
		}
		aceptar := ui.NuevoBoton(t.T("invitations.accept", nil), func() { responder(true) }).Con(ui.Primario, ui.TallaSM)
		rechazar := ui.NuevoBoton(t.T("invitations.reject", nil), func() { responder(false) }).Con(ui.Secundario, ui.TallaSM)
		fila = container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
			ui.NuevoParrafo(TextoInvitacion(t, inv), tema.TextoSM, ui.Normal, "text", v),
			container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, aceptar, rechazar))
		filas.Add(fila)
	}
	d = ui.NuevoDialogo(t.T("invitations.title", nil), filas).Con(ui.DialogoSM, nil)
	d.RotuloCerrar = t.T("action.close", nil)
	d.Mostrar(lienzo)
}
