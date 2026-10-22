// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lo que está fallando en la plataforma, para que se pueda MIRAR.
//
// Estas rutas van detrás del gate de plataforma como el resto: un registro de
// errores enseña rutas internas, correos y a veces el nombre de una
// organización cliente, así que es información de la plataforma y no de una
// organización.
package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// ventanaLogs acota cuánto hacia atrás se puede pedir. Un día es lo que se mira
// cuando algo va mal ahora; para lo de la semana pasada está el SIEM, que es
// donde vive el histórico de verdad.
const ventanaLogsMax = 24 * time.Hour

// handlePlatformLogs — GET /api/v1/platform/logs — los problemas recientes,
// AGRUPADOS.
//
// Agrupa a propósito en vez de paginar líneas: mil veces el mismo error no son
// mil cosas que mirar, son una que lleva pasando mil veces. Un volcado crudo es
// lo que hace que un panel de logs se abra una vez y no se vuelva a abrir.
func (s *Server) handlePlatformLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	desde := time.Now().Add(-ventanaLogsMax)
	if v := q.Get("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 24 {
			desde = time.Now().Add(-time.Duration(n) * time.Hour)
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	grupos, err := s.store.ListLogGroups(r.Context(), desde,
		strings.TrimSpace(q.Get("level")), strings.TrimSpace(q.Get("service")),
		q.Get("q"), limit)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(grupos))
	for _, g := range grupos {
		items = append(items, map[string]any{
			"level":   g.Level,
			"service": g.Service,
			"message": g.Message,
			"count":   g.Count,
			"first":   g.First.UTC().Format(time.RFC3339),
			"last":    g.Last.UTC().Format(time.RFC3339),
			"sample":  g.Sample,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handlePlatformLogStats — GET /api/v1/platform/logs/stats — los números de
// cabecera y la tendencia por hora.
//
// Va aparte de la lista porque se refrescan a ritmos distintos: la lista cambia
// al filtrar y las cifras no, y unirlas obligaría a recalcular el histograma
// entero cada vez que alguien escribe una letra en el buscador.
func (s *Server) handlePlatformLogStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.LogStatsSince(r.Context(), time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	horas := make([]map[string]any, 0, len(st.PorHora))
	for _, b := range st.PorHora {
		horas = append(horas, map[string]any{
			"hour":   b.Hour.UTC().Format(time.RFC3339),
			"errors": b.Errors,
			"warns":  b.Warns,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"errors_24h":        st.Errors24h,
		"warns_24h":         st.Warns24h,
		"errors_1h":         st.Errors1h,
		"services_affected": st.Services,
		"by_hour":           horas,
	})
}
