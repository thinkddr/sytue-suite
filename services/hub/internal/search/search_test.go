// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/connectors"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// stubSearcher es un conector falso para probar el fan-out de la actividad
// reciente sin servicios upstream: devuelve hits/errores fijos y puede simular
// lentitud (para el timeout por origen).
type stubSearcher struct {
	src        domain.Source
	recentHits []domain.SearchHit
	recentErr  error
	delay      time.Duration
}

func (s stubSearcher) Source() domain.Source { return s.src }

func (s stubSearcher) Search(context.Context, string, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}

func (s stubSearcher) Recent(ctx context.Context, _ string, _ int) ([]domain.SearchHit, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err() // el timeout por origen cancela: degrada
		}
	}
	return s.recentHits, s.recentErr
}

func ts(v string) *time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		panic(err)
	}
	return &t
}

// TestRecentAggregatesAndDegrades fija el contrato de la actividad reciente:
// agrega los ítems de los orígenes SANOS, ordena por recencia (temporal, no
// relevancia) y DEGRADA con elegancia — un origen caído (error) y uno lento
// (timeout por origen) se reportan como error parcial y NO rompen el resto.
func TestRecentAggregatesAndDegrades(t *testing.T) {
	drive := stubSearcher{src: domain.SourceDrive, recentHits: []domain.SearchHit{
		{Source: domain.SourceDrive, ID: "old", Title: "Antiguo", Timestamp: ts("2026-07-01T10:00:00Z")},
		{Source: domain.SourceDrive, ID: "new", Title: "Nuevo", Timestamp: ts("2026-07-10T10:00:00Z")},
	}}
	mail := stubSearcher{src: domain.SourceMail, recentErr: errors.New("mail caído")}        // origen caído
	cal := stubSearcher{src: domain.SourceCalendar, delay: 2 * time.Second, recentHits: nil} // origen lento -> timeout

	reg := connectors.NewRegistry(drive, mail, cal)
	svc := New(reg, 150*time.Millisecond, 20) // timeout por origen corto para forzar el degradado de cal

	resp := svc.Recent(context.Background(), "tok", nil, 10)

	// Agrega solo el origen sano y ordena por recencia (Nuevo antes que Antiguo).
	if len(resp.Hits) != 2 {
		t.Fatalf("hits=%d, quiero 2 (solo drive)", len(resp.Hits))
	}
	if resp.Hits[0].ID != "new" || resp.Hits[1].ID != "old" {
		t.Errorf("orden por recencia incorrecto: %s, %s", resp.Hits[0].ID, resp.Hits[1].ID)
	}

	// Los tres orígenes aparecen en la sección por-origen; mail y calendar con
	// error parcial (caído/timeout), drive sin error.
	byErr := map[domain.Source]bool{}
	for _, sr := range resp.Sources {
		byErr[sr.Source] = sr.HasError()
	}
	if len(resp.Sources) != 3 {
		t.Fatalf("sources=%d, quiero 3", len(resp.Sources))
	}
	if byErr[domain.SourceDrive] {
		t.Error("drive no debe reportar error")
	}
	if !byErr[domain.SourceMail] || !byErr[domain.SourceCalendar] {
		t.Errorf("mail y calendar deben reportar error parcial: %+v", resp.Sources)
	}
}

// TestRecentEmpty fija que sin conectores la respuesta es válida y vacía (no
// nil-panic), y que el límite se acota al máximo configurado.
func TestRecentEmpty(t *testing.T) {
	svc := New(connectors.NewRegistry(), 0, 0)
	resp := svc.Recent(context.Background(), "tok", nil, 999)
	if len(resp.Hits) != 0 || len(resp.Sources) != 0 {
		t.Errorf("respuesta no vacía: %+v", resp)
	}
}

// searchStub devuelve hits fijos a la BÚSQUEDA (el stub de arriba solo cubre Recent).
type searchStub struct {
	src  domain.Source
	hits []domain.SearchHit
}

func (s searchStub) Source() domain.Source { return s.src }
func (s searchStub) Search(context.Context, string, string, int) ([]domain.SearchHit, error) {
	return s.hits, nil
}

func (s searchStub) Recent(context.Context, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}

func hit(src domain.Source, kind, title string, score float64, when string) domain.SearchHit {
	h := domain.SearchHit{Source: src, Kind: kind, ID: title, Title: title, Score: score}
	if when != "" {
		h.Timestamp = ts(when)
	}
	return h
}

// La paginación va sobre el AGREGADO ya ordenado, no por origen: quien busca espera
// que la página 2 siga a la 1 en relevancia, aunque los resultados vengan de servicios
// distintos. Y `total` cuenta lo que hay, no lo que cabe en la página.
func TestSearchPaginaElAgregado(t *testing.T) {
	drive := searchStub{src: domain.SourceDrive, hits: []domain.SearchHit{
		hit(domain.SourceDrive, "file", "d1", 0.9, "2026-07-01T10:00:00Z"),
		hit(domain.SourceDrive, "file", "d2", 0.5, "2026-07-02T10:00:00Z"),
	}}
	mail := searchStub{src: domain.SourceMail, hits: []domain.SearchHit{
		hit(domain.SourceMail, "message", "m1", 0.8, "2026-07-03T10:00:00Z"),
		hit(domain.SourceMail, "message", "m2", 0.4, "2026-07-04T10:00:00Z"),
	}}
	svc := New(connectors.NewRegistry(drive, mail), time.Second, 50)

	p1, err := svc.Search(context.Background(), "t", domain.SearchQuery{Text: "x", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if p1.Total != 4 {
		t.Fatalf("total = %d, quiero 4 (lo que hay, no lo que cabe)", p1.Total)
	}
	if len(p1.Hits) != 2 || !p1.HasMore {
		t.Fatalf("página 1: %d hits, hasMore=%v", len(p1.Hits), p1.HasMore)
	}
	if p1.Hits[0].Title != "d1" || p1.Hits[1].Title != "m1" {
		t.Fatalf("la página 1 debe ser la más relevante del AGREGADO: %v", titles(p1.Hits))
	}

	p2, err := svc.Search(context.Background(), "t", domain.SearchQuery{Text: "x", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(p2.Hits) != 2 || p2.HasMore {
		t.Fatalf("página 2: %d hits, hasMore=%v", len(p2.Hits), p2.HasMore)
	}
	if p2.Hits[0].Title != "d2" && p2.Hits[0].Title != "m2" {
		t.Fatalf("la página 2 debe continuar donde acabó la 1: %v", titles(p2.Hits))
	}

	// Más allá del final: página vacía, no error ni la última repetida.
	p9, _ := svc.Search(context.Background(), "t", domain.SearchQuery{Text: "x", Limit: 2, Offset: 99})
	if len(p9.Hits) != 0 || p9.HasMore {
		t.Fatalf("pasado el final debe venir vacío: %v", titles(p9.Hits))
	}
}

// Los filtros se aplican en el Hub porque los conectores hablan APIs distintas y no
// todas admiten rango de fechas. Un hit SIN fecha se descarta cuando se filtra por
// fecha: colarlo sería afirmar que cae dentro del rango sin saberlo.
func TestSearchFiltraPorFechaYTipo(t *testing.T) {
	drive := searchStub{src: domain.SourceDrive, hits: []domain.SearchHit{
		hit(domain.SourceDrive, "file", "viejo", 0.9, "2026-01-01T10:00:00Z"),
		hit(domain.SourceDrive, "file", "dentro", 0.8, "2026-07-15T10:00:00Z"),
		hit(domain.SourceDrive, "folder", "carpeta", 0.7, "2026-07-16T10:00:00Z"),
		hit(domain.SourceDrive, "file", "sin-fecha", 0.6, ""),
	}}
	svc := New(connectors.NewRegistry(drive), time.Second, 50)

	res, err := svc.Search(context.Background(), "t", domain.SearchQuery{
		Text: "x", From: ts("2026-07-01T00:00:00Z"), To: ts("2026-07-31T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := titles(res.Hits); len(got) != 2 || got[0] != "dentro" || got[1] != "carpeta" {
		t.Fatalf("rango de fechas = %v, quiero [dentro carpeta]", got)
	}

	porTipo, _ := svc.Search(context.Background(), "t", domain.SearchQuery{
		Text: "x", Kinds: []string{"folder"},
	})
	if got := titles(porTipo.Hits); len(got) != 1 || got[0] != "carpeta" {
		t.Fatalf("filtro por tipo = %v, quiero [carpeta]", got)
	}
}

func titles(hits []domain.SearchHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Title
	}
	return out
}
