// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Activar/desactivar avisos de mensajes directos» (`toggleDmNotify`). En la web es un
// ajuste de `localStorage` que decide si un directo sale como aviso del NAVEGADOR; en
// nativo los avisos los reparte el Hub a todas las apps y dispositivos, así que el
// ajuste es SUYO: una regla de silencio (`/notifications/prefs`) sobre el aviso que
// Rooms le manda por un directo.
//
// Rooms publica `rooms.message` SOLO a los miembros de un directo que no han sido
// mencionados (`destinatariosCTE` de services/rooms/internal/store/push.go) y
// `rooms.mention` a los mencionados; callar el primero es callar los directos y dejar
// las menciones, que es lo que hace el ajuste de la web.

// PrefAvisoDirectos es la regla del Hub que calla los avisos de los directos.
var PrefAvisoDirectos = api.HubNotificationPref{Source: "rooms", Kind: "rooms.message"}

// directosCalladosEn dice si esas reglas callan los directos.
func directosCalladosEn(ps []api.HubNotificationPref) bool {
	return slices.ContainsFunc(ps, func(p api.HubNotificationPref) bool {
		return p.Source == PrefAvisoDirectos.Source && p.Kind == PrefAvisoDirectos.Kind && p.Muted
	})
}

// cargarAvisosDirectos lee la regla al arrancar; sin Hub, los avisos siguen activos.
func (sa *Sala) cargarAvisosDirectos() {
	if sa.PrefsAviso == nil {
		return
	}
	pedir(sa, sa.PrefsAviso, func(ps []api.HubNotificationPref, err error) {
		if err == nil {
			sa.directosCallados = directosCalladosEn(ps)
		}
	})
}

// AlternarAvisosDirectos cambia el ajuste: se apunta ya y vuelve si el Hub dice que no.
func (sa *Sala) AlternarAvisosDirectos() {
	if sa.PonerPrefAviso == nil {
		return
	}
	callar := !sa.directosCallados
	sa.directosCallados = callar
	p := PrefAvisoDirectos
	p.Muted = callar
	pedir(sa, func(ctx context.Context) (api.HubNotificationPref, error) { return sa.PonerPrefAviso(ctx, p) },
		func(_ api.HubNotificationPref, err error) {
			if err != nil {
				sa.directosCallados = !callar
			}
		})
}
