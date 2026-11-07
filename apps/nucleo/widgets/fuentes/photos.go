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

// paginaFotos es la primera página de la línea de tiempo.
const paginaFotos = 100

// Photos lee la primera página de la línea de tiempo SIN filtros ni búsqueda y los
// álbumes, aunque la app esté en un álbum o filtrando. Sin los álbumes (salvo un
// 403/404) se conserva lo publicado. Peticiones: 2.
func Photos(c Comunes, ph *api.Photos) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		fotos, siguiente, err := ph.Timeline(ctx, "", paginaFotos, false, "")
		if err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		if siguiente == "" {
			d.Counters = []widgets.Counter{{Kind: "photos", Value: int64(len(fotos))}}
		}
		lista, err := ph.Albums(ctx)
		if err := secundaria(err); err != nil {
			return widgets.Detail{}, err
		}
		albumes := itemsDeAlbumes(lista)
		d.Items = widgets.SanitizeItems(append(albumes, itemsDeFotos(c, fotos)...))
		return d, nil
	}
}

func itemsDeFotos(c Comunes, fotos []api.Photo) []widgets.Item {
	items := make([]widgets.Item, 0, min(len(fotos), widgets.MaxItems))
	for _, f := range fotos {
		titulo := primero(f.Description, f.Name)
		if titulo == "" {
			continue
		}
		insignia := ""
		if unix(f.TakenAt) > 0 {
			insignia = f.TakenAt.In(c.zona()).Format("02/01")
		}
		if f.Favorite {
			insignia = "★ " + insignia
		}
		items = append(items, widgets.Item{
			Title: titulo, Subtitle: f.Camera, Badge: strings.TrimSpace(insignia),
			StartAt: unix(f.TakenAt), Unread: f.Favorite,
		})
		if llenos(items) {
			break
		}
	}
	return items
}

func itemsDeAlbumes(lista []api.Album) []widgets.Item {
	items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
	for _, a := range lista {
		if titulo := strings.TrimSpace(a.Title); titulo != "" {
			items = append(items, widgets.Item{Title: titulo, Badge: strconv.Itoa(a.Count)})
		}
		if llenos(items) {
			break
		}
	}
	return items
}
