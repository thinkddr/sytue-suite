// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package search implementa la búsqueda federada de Sytue Hub: consulta en
// paralelo a los conectores (drive/mail/calendar/contacts/chat) EN NOMBRE del
// usuario, normaliza sus respuestas al modelo canónico (domain.SearchHit) y las
// agrega. Degrada con elegancia: un origen lento o caído no rompe el resto (se
// reporta como error parcial por origen).
package search

import (
	"context"
	"sort"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/hub/internal/connectors"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Service ejecuta búsquedas federadas sobre el registro de conectores.
type Service struct {
	registry    *connectors.Registry
	perSourceTO time.Duration
	limit       int
}

// New construye el servicio de búsqueda. perSourceTimeout acota cada fan-out a
// un conector; defaultLimit es el máximo de resultados por origen.
func New(registry *connectors.Registry, perSourceTimeout time.Duration, defaultLimit int) *Service {
	if perSourceTimeout <= 0 {
		perSourceTimeout = 2500 * time.Millisecond
	}
	if defaultLimit <= 0 {
		defaultLimit = 20
	}
	return &Service{registry: registry, perSourceTO: perSourceTimeout, limit: defaultLimit}
}

// Search ejecuta la búsqueda federada. token es el Bearer JWT del usuario, que
// se propaga a cada servicio para que apliquen su propia autorización.
func (s *Service) Search(ctx context.Context, token string, q domain.SearchQuery) (domain.SearchResponse, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return domain.SearchResponse{}, domain.ErrInvalidInput
	}
	limit := s.clampLimit(q.Limit)
	// Con paginación hay que pedir a cada origen lo suficiente para poder SALTAR: si
	// se piden solo `limit` por origen, la página 2 del agregado no existe. El tope
	// evita que un offset grande dispare la carga sobre los servicios.
	perSource := s.clampLimit(limit + q.Offset)

	resp := s.fanOut(ctx, s.registry.Searchers(q.Sources),
		func(cctx context.Context, sr connectors.Searcher) ([]domain.SearchHit, error) {
			return sr.Search(cctx, token, text, perSource)
		})
	resp.Query = text
	rankHits(resp.Hits) // relevancia (score) y, a igualdad, recencia

	// Los filtros se aplican DESPUÉS de agregar y ordenar: cada conector habla una API
	// distinta y no todas admiten rango de fechas ni tipo, así que hacerlo aquí es lo
	// único que lo garantiza para todos los orígenes por igual.
	resp.Hits = filterHits(resp.Hits, q)
	resp.Total = len(resp.Hits)
	resp.Hits, resp.HasMore = paginate(resp.Hits, q.Offset, limit)
	return resp, nil
}

// filterHits aplica el rango de fechas y los tipos pedidos. Un hit SIN timestamp se
// conserva cuando no hay filtro de fecha, y se descarta cuando lo hay: no se puede
// afirmar que caiga dentro del rango, y colarlo sería mentir sobre el filtro.
func filterHits(hits []domain.SearchHit, q domain.SearchQuery) []domain.SearchHit {
	if q.From == nil && q.To == nil && len(q.Kinds) == 0 {
		return hits
	}
	kinds := make(map[string]struct{}, len(q.Kinds))
	for _, k := range q.Kinds {
		kinds[k] = struct{}{}
	}
	out := hits[:0]
	for _, h := range hits {
		if len(kinds) > 0 {
			if _, ok := kinds[h.Kind]; !ok {
				continue
			}
		}
		if q.From != nil || q.To != nil {
			if h.Timestamp == nil {
				continue
			}
			if q.From != nil && h.Timestamp.Before(*q.From) {
				continue
			}
			if q.To != nil && h.Timestamp.After(*q.To) {
				continue
			}
		}
		out = append(out, h)
	}
	return out
}

// paginate recorta la página pedida del agregado ya ordenado y dice si queda más.
func paginate(hits []domain.SearchHit, offset, limit int) ([]domain.SearchHit, bool) {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(hits) {
		return []domain.SearchHit{}, false
	}
	fin := offset + limit
	if limit <= 0 || fin > len(hits) {
		return hits[offset:], false
	}
	return hits[offset:fin], fin < len(hits)
}

// Recent agrega la ACTIVIDAD RECIENTE de cada origen (sin término de búsqueda),
// en paralelo y con la misma degradación elegante que la búsqueda federada (un
// origen sin recientes o caído no rompe el resto). sources vacío = todos los
// registrados. El orden es puramente temporal: la actividad reciente no tiene
// «relevancia», solo fecha. No falla a nivel de servicio (los errores por origen
// se reportan en cada SourceResult), de ahí que no devuelva error.
func (s *Service) Recent(ctx context.Context, token string, sources []domain.Source, limit int) domain.SearchResponse {
	limit = s.clampLimit(limit)

	resp := s.fanOut(ctx, s.registry.Searchers(sources),
		func(cctx context.Context, sr connectors.Searcher) ([]domain.SearchHit, error) {
			return sr.Recent(cctx, token, limit)
		})
	rankByRecency(resp.Hits)
	return resp
}

// clampLimit acota el límite por origen al máximo configurado (0/negativo = máx).
func (s *Service) clampLimit(n int) int {
	if n <= 0 || n > s.limit {
		return s.limit
	}
	return n
}

// fanOut ejecuta call sobre cada searcher EN PARALELO, con timeout por origen y
// degradación elegante: un origen que falla se reporta como error parcial
// (SourceResult.Error) y NO aborta el resto. Devuelve la respuesta agregada con
// la sección por-origen en orden estable; el ranking de hits lo aplica cada
// llamador (Search por relevancia, Recent por recencia). Compartido por ambos.
func (s *Service) fanOut(
	ctx context.Context,
	searchers []connectors.Searcher,
	call func(context.Context, connectors.Searcher) ([]domain.SearchHit, error),
) domain.SearchResponse {
	start := time.Now()
	resultsCh := make(chan domain.SourceResult, len(searchers))

	for _, sr := range searchers {
		go func(sr connectors.Searcher) {
			cctx, cancel := context.WithTimeout(ctx, s.perSourceTO)
			defer cancel()

			began := time.Now()
			hits, err := call(cctx, sr)
			out := domain.SourceResult{
				Source: sr.Source(),
				Hits:   hits,
				Took:   time.Since(began).Milliseconds(),
			}
			if err != nil {
				// Error parcial: se reporta pero NO aborta la federación.
				out.Error = err.Error()
				out.Hits = nil
				log.From(ctx).Warn("fan-out federado: origen falló",
					"source", sr.Source(), "error", err)
			}
			resultsCh <- out
		}(sr)
	}

	var resp domain.SearchResponse
	for range searchers {
		o := <-resultsCh
		resp.Sources = append(resp.Sources, o)
		resp.Hits = append(resp.Hits, o.Hits...)
	}

	// Orden estable de la sección por-origen (para respuestas deterministas).
	sort.SliceStable(resp.Sources, func(i, j int) bool {
		return resp.Sources[i].Source < resp.Sources[j].Source
	})
	resp.TookMS = time.Since(start).Milliseconds()
	return resp
}

// rankByRecency ordena los resultados SOLO por recencia (timestamp descendente;
// los sin fecha al final). Es el orden de la actividad reciente: puramente
// temporal, sin noción de relevancia.
func rankByRecency(hits []domain.SearchHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i].Timestamp, hits[j].Timestamp
		switch {
		case a == nil:
			return false
		case b == nil:
			return true
		default:
			return a.After(*b)
		}
	})
}

// rankHits ordena los resultados agregados: primero por score descendente y, a
// igualdad, por recencia (timestamp descendente; los sin fecha al final).
func rankHits(hits []domain.SearchHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		switch {
		case a.Timestamp == nil && b.Timestamp == nil:
			return false
		case a.Timestamp == nil:
			return false
		case b.Timestamp == nil:
			return true
		default:
			return a.Timestamp.After(*b.Timestamp)
		}
	})
}
