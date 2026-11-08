// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Cast lee la cuota, lo mío, lo publicado en la organización y lo que está en directo,
// sin búsqueda, aunque la app esté en otra sección. Si una de las cuatro falla (salvo un
// 403/404), se conserva lo publicado: un directo no desaparece por un fallo puntual.
// Peticiones: 4.
func Cast(_ Comunes, cl *api.Cast) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		mios, err := cl.Videos(ctx, api.CastMine, "", "", "", 24)
		if err != nil {
			return widgets.Detail{}, err
		}
		org, err := cl.Videos(ctx, api.CastOrgScope, api.CastStatusReady, "", "", 12)
		if err := secundaria(err); err != nil {
			return widgets.Detail{}, err
		}
		vivos, err := cl.Videos(ctx, api.CastOrgScope, api.CastStatusLive, "", "", 8)
		if err := secundaria(err); err != nil {
			return widgets.Detail{}, err
		}
		q, err := cl.Quota(ctx)
		if err := secundaria(err); err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		// Un backend viejo no distingue el plan de un almacén externo (nil): no hay
		// porcentaje que publicar.
		if err == nil && q.ExternalStorage != nil && !*q.ExternalStorage {
			if contador, valido := widgets.StoragePercent(q.UsedBytes, q.LimitBytes); valido {
				d.Counters = append(d.Counters, contador)
			}
		}
		if mios.NextCursor == "" {
			d.Counters = append(d.Counters, widgets.Counter{Kind: "episodes", Value: int64(len(mios.Videos))})
		}
		d.Items = itemsDeCast(vivos.Videos, mios.Videos, sinRepetir(org.Videos, mios.Videos, vivos.Videos))
		return d, nil
	}
}

func sinRepetir(vs []api.CastVideo, otras ...[]api.CastVideo) []api.CastVideo {
	visto := map[string]bool{}
	for _, l := range otras {
		for _, x := range l {
			visto[x.ID] = true
		}
	}
	var out []api.CastVideo
	for _, x := range vs {
		if !visto[x.ID] {
			out = append(out, x)
		}
	}
	return out
}

func itemsDeCast(vivos, mios, org []api.CastVideo) []widgets.Item {
	items := make([]widgets.Item, 0, widgets.MaxItems)
	anadir := func(lista []api.CastVideo, directo bool) {
		for _, v := range lista {
			if llenos(items) {
				return
			}
			titulo := strings.TrimSpace(v.Title)
			if titulo == "" {
				continue
			}
			insignia, color := duracion(v.DurationMS), ""
			enDirecto := directo || v.Status == api.CastStatusLive
			if enDirecto {
				insignia, color = "LIVE", "#dc2626"
			}
			items = append(items, widgets.Item{
				Title: titulo, Subtitle: v.Owner.Name, Badge: insignia,
				StartAt: unix(v.CreatedAt), Color: color, Unread: enDirecto,
			})
		}
	}
	anadir(vivos, true)
	anadir(mios, false)
	anadir(org, false)
	return widgets.SanitizeItems(items)
}

func duracion(ms int64) string {
	if ms <= 0 {
		return ""
	}
	seg := ms / 1000
	minutos, resto := seg/60, seg%60
	if minutos >= 60 {
		return strconv.FormatInt(minutos/60, 10) + ":" + dosCifras(minutos%60) + ":" + dosCifras(resto)
	}
	return strconv.FormatInt(minutos, 10) + ":" + dosCifras(resto)
}

func dosCifras(v int64) string {
	if v < 10 {
		return "0" + strconv.FormatInt(v, 10)
	}
	return strconv.FormatInt(v, 10)
}
