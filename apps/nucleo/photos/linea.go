// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # La línea de tiempo proyectada
//
// Cada página de la línea de tiempo leía, comprobaba y descodificaba la galería ENTERA
// para ordenarla y quedarse con cien (con 50.000 fotos, medio segundo largo por página en
// la prueba de carga del almacén local, §5.6 del plan de Sytue Office). Las vistas de la
// galería solo cambian cuando cambia lo que lee el catálogo, y eso lo dice la revisión
// del motor (records.Repository.CatalogRevision: sube con cada cabeza nueva y con el
// sello del Crypto), así que se proyectan una vez por revisión, ya en orden, y cada
// página es recorrerlas. El resultado es el mismo: las mismas fotos (fuera de la
// papelera y sin las purgadas), el mismo orden total (fecha y id) y el mismo corte; la
// página devuelve copias, y el estado de un original externo se mira, como siempre, al
// devolverla. Lo que busca texto sigue por el camino de siempre, y también todo en la
// edición conectada: solo se proyecta cuando el motor recuerda lo comprobado
// (records.VerifiedContentGeneration, el almacén en claro de Sytue Office).

// lineaProyectada son las vistas de la galería fuera de la papelera, en el orden de la
// línea de tiempo, para la revisión del catálogo de la que salieron.
type lineaProyectada struct {
	mu       sync.Mutex
	valida   bool
	revision uint64
	vistas   []api.Photo
}

// lineaDeTiempo son las vistas proyectadas de la revisión actual del catálogo; las
// proyecta si no las hay. No se cambian: quien las use, copia (copiaDeVista).
func (r *Repository) lineaDeTiempo(ctx context.Context) ([]api.Photo, error) {
	revision, err := r.engine.CatalogRevision()
	if err != nil {
		return nil, err
	}
	r.linea.mu.Lock()
	if r.linea.valida && r.linea.revision == revision {
		vistas := r.linea.vistas
		r.linea.mu.Unlock()
		return vistas, nil
	}
	r.linea.mu.Unlock()
	assets, err := r.assets(ctx)
	if err != nil {
		return nil, err
	}
	vistas := make([]api.Photo, 0, len(assets))
	for _, asset := range assets {
		if asset.DeletedAt == nil {
			vistas = append(vistas, photoView(asset))
		}
	}
	slices.SortFunc(vistas, compararEnLinea)
	// Se guarda con la revisión leída ANTES de proyectar: si el catálogo cambió entretanto,
	// la próxima página ve otra revisión y vuelve a proyectar.
	r.linea.mu.Lock()
	r.linea.valida, r.linea.revision, r.linea.vistas = true, revision, vistas
	r.linea.mu.Unlock()
	return vistas, nil
}

// paginaProyectada es la página de limit fotos (con o sin solo las favoritas) que va
// tras el cursor after (si conCursor), sacada de la línea proyectada en vez de leer y
// ordenar la galería entera.
func (r *Repository) paginaProyectada(ctx context.Context, conCursor bool, after timelineCursor, limit int, favorites bool) ([]api.Photo, string, error) {
	vistas, err := r.lineaDeTiempo(ctx)
	if err != nil {
		return nil, "", err
	}
	values := make([]api.Photo, 0, limit+1)
	for _, view := range vistas {
		if favorites && !view.Favorite {
			continue
		}
		if conCursor && (view.TakenAt.After(after.TakenAt) || view.TakenAt.Equal(after.TakenAt) && view.ID >= after.ID) {
			continue
		}
		values = append(values, copiaDeVista(view))
		if len(values) > limit {
			break
		}
	}
	return r.paginaDeLinea(values, limit)
}

// olvidar suelta lo proyectado (al cerrar).
func (l *lineaProyectada) olvidar() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.valida, l.vistas = false, nil
}

// copiaDeVista es v sin compartir nada con la proyección.
func copiaDeVista(v api.Photo) api.Photo {
	v.Tags = slices.Clone(v.Tags)
	if v.Location != nil {
		lugar := *v.Location
		v.Location = &lugar
	}
	if v.DeletedAt != nil {
		t := *v.DeletedAt
		v.DeletedAt = &t
	}
	if v.PurgeAt != nil {
		t := *v.PurgeAt
		v.PurgeAt = &t
	}
	return v
}
