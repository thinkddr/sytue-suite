// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"errors"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// errSinEquipos: ni equipo ni conversaciones de fuera; no hay nada que enseñar.
var errSinEquipos = errors.New("fuentes: rooms sin equipos")

// Rooms cuenta los canales y lo pendiente con la regla de la lista (api.PendientesRooms:
// un canal silenciado o con «nada» no suma; con «menciones», solo las menciones) y enseña los
// canales públicos de la casa. Nunca descifra ni enseña con quién es un directo.
// Peticiones: 2-4.
func Rooms(_ Comunes, r *api.Rooms) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		equipos, err := r.Teams(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		fuera, pendFuera, errFuera := r.ExternalChannels(ctx)
		canales := make([]api.RoomsChannel, 0, len(fuera))
		var pend []api.RoomsUnread
		pendientesCompletos := errFuera == nil
		switch {
		case len(equipos) > 0:
			if canales, err = r.Channels(ctx, equipos[0].ID); err != nil {
				return widgets.Detail{}, err
			}
			pendEquipo, errPend := r.Unread(ctx, equipos[0].ID)
			pend = pendEquipo
			pendientesCompletos = pendientesCompletos && errPend == nil
		case errFuera != nil || len(fuera) == 0:
			return widgets.Detail{}, errors.Join(errSinEquipos, errFuera)
		}
		if errFuera != nil {
			// Sin las de fuera, el número de conversaciones mentiría.
			return widgets.Detail{}, errFuera
		}
		return detalleRooms(append(canales, fuera...), append(pend, pendFuera...), pendientesCompletos), nil
	}
}

func detalleRooms(canales []api.RoomsChannel, pend []api.RoomsUnread, pendientesCompletos bool) widgets.Detail {
	porCanal := make(map[string]api.RoomsUnread, len(pend))
	for _, p := range pend {
		porCanal[p.ChannelID] = p
	}
	visto := make(map[string]bool, len(canales))
	var sinLeer int64
	var items []widgets.Item
	for _, c := range canales {
		if c.Archived || c.ID == "" || visto[c.ID] {
			continue
		}
		visto[c.ID] = true
		p := porCanal[c.ID]
		n := api.PendientesRooms(c.Muted, c.NotifyLevel, p.Unread, p.Mentions)
		sinLeer += int64(n)
		if c.Kind == "public" && !c.External && c.Name != "" && !llenos(items) {
			insignia := ""
			if pendientesCompletos && n > 0 {
				insignia = strconv.Itoa(n)
			}
			items = append(items, widgets.Item{
				Title: "#" + c.Name, Subtitle: c.Topic, Badge: insignia, Unread: pendientesCompletos && n > 0,
			})
		}
	}
	contadores := []widgets.Counter{{Kind: "channels", Value: int64(len(visto))}}
	if pendientesCompletos {
		contadores = append([]widgets.Counter{{Kind: "unread", Value: min(sinLeer, 1_000_000_000_000)}}, contadores...)
	}
	return widgets.Detail{Counters: contadores, Items: widgets.SanitizeItems(items)}
}
