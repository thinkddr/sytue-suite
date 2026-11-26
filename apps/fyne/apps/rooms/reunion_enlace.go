// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// CreadorReuniones es un servicio SIN llamadas propias que sí crea el enlace de una
// reunión de un proveedor: Sytue Office con Microsoft Teams o Google Meet, que no dejan
// meter sus llamadas en otra app y se abren en la suya (D-21 del plan de la edición
// local). En los canales donde hay proveedor, «Llamar» de la cabecera pasa a ser
// «Reunión de <proveedor>», y pulsarlo crea la reunión y publica su enlace en el canal.
//
// Es opcional: el servicio de Rooms de la suite no lo cumple, y la cabecera sigue como
// siempre.
type CreadorReuniones interface {
	// ProveedorReunion es el nombre del proveedor con el que se crearía la reunión del
	// canal, o "" si no hay.
	ProveedorReunion(canal string) string
	// CrearReunion la crea (con el título, donde el proveedor lo admite), publica su
	// enlace en el canal y devuelve ese mensaje.
	CrearReunion(ctx context.Context, canal, titulo string) (api.RoomsMessage, error)
}

// prepararReunion pone «Llamar» de la cabecera según el proveedor de reuniones del canal
// c. Si el servicio no crea reuniones, no toca nada.
func (sa *Sala) prepararReunion(c Canal) {
	cr, ok := sa.S.(CreadorReuniones)
	if !ok {
		return
	}
	cab := sa.P.Canal.Cabecera
	cab.RotuloLlamar = ""
	prov := cr.ProveedorReunion(c.ID)
	cab.SinLlamar = prov == ""
	if prov != "" {
		cab.RotuloLlamar = sa.P.T.T("link.joinMeeting", i18n.Vars{"provider": prov})
	}
}

// crearReunion crea la reunión del canal abierto y, publicado su enlace, recarga el
// final del canal; si falla, lo dice.
func (sa *Sala) crearReunion(cr CreadorReuniones) {
	canal, titulo := sa.abierto, sa.P.Canal.Cabecera.Canal.Nombre
	if canal == "" || sa.reunionEnCurso {
		return
	}
	sa.reunionEnCurso = true
	pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) { return cr.CrearReunion(ctx, canal, titulo) },
		func(_ api.RoomsMessage, err error) {
			sa.reunionEnCurso = false
			if err != nil {
				sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("meetings.createFailed", nil))
				return
			}
			if canal == sa.abierto {
				sa.recargarMensajes()
			}
		})
}
