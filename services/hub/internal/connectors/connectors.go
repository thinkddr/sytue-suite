// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package connectors define las interfaces tipadas de los conectores a los
// servicios de la suite (drive/mail/calendar/contacts/chat) y un cliente HTTP
// base reutilizable. Cada conector traduce el modelo propio de su servicio al
// modelo canónico de Hub (domain.SearchHit, deep-links de intents).
//
// Diseño: un conector NO conoce el almacenamiento de Hub; solo habla la API REST
// del servicio upstream, propagando el Bearer JWT de Sytue ID del usuario (las
// llamadas se hacen EN NOMBRE del usuario, no con credenciales de servicio, para
// que la autorización por tenant/recurso la siga aplicando cada servicio).
package connectors

import (
	"context"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Searcher es un conector capaz de buscar en un servicio de la suite y devolver
// resultados ya normalizados al modelo canónico de Hub. También expone la
// «actividad reciente» del usuario (búsqueda sin término, orden temporal).
type Searcher interface {
	// Source identifica el servicio que respalda este conector.
	Source() domain.Source
	// Search consulta el servicio con el término dado, en nombre del usuario
	// (token = Bearer JWT de Sytue ID), y normaliza los resultados. Debe
	// respetar el límite y cancelarse con el contexto.
	Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error)
	// Recent devuelve los ítems más recientes del usuario en este servicio SIN
	// término de búsqueda (actividad reciente cross-app, orden temporal upstream),
	// en nombre del usuario. Un conector cuyo servicio aún no expone un listado de
	// recientes consumible por Hub devuelve (nil, nil): el origen simplemente no
	// aporta actividad (degradación elegante, como un origen caído en la búsqueda).
	Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error)
}

// Registry agrupa los conectores disponibles indexados por origen.
type Registry struct {
	searchers map[domain.Source]Searcher
}

// NewRegistry construye un Registry a partir de los conectores dados.
func NewRegistry(searchers ...Searcher) *Registry {
	m := make(map[domain.Source]Searcher, len(searchers))
	for _, s := range searchers {
		if s != nil {
			m[s.Source()] = s
		}
	}
	return &Registry{searchers: m}
}

// Searcher devuelve el conector de un origen, si está registrado.
func (r *Registry) Searcher(src domain.Source) (Searcher, bool) {
	s, ok := r.searchers[src]
	return s, ok
}

// Searchers devuelve los conectores de los orígenes pedidos (vacío = todos los
// registrados), preservando un orden estable por el catálogo de domain.AllSources.
func (r *Registry) Searchers(sources []domain.Source) []Searcher {
	if len(sources) == 0 {
		sources = domain.AllSources()
	}
	out := make([]Searcher, 0, len(sources))
	for _, src := range sources {
		if s, ok := r.searchers[src]; ok {
			out = append(out, s)
		}
	}
	return out
}
