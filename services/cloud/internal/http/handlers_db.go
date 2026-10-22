// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// ---------------------------------------------------------------------------
// Sytue DB Studio — explorador de tablas, consola SQL e info de la API REST del
// Postgres gestionado del proyecto.
//
// Las respuestas son el objeto/array DESNUDO (sin envoltorio {data:…}) porque
// ese es el contrato que valida el dashboard (loadDbTables/loadDbRows/
// loadDbApiInfo/runSql en apps/cloud-dashboard/src/lib/cloud.ts): si la forma no
// encaja, la UI se cae a datos de demostración.
// ---------------------------------------------------------------------------

// dbStudioService es la porción del servicio que usa DB Studio. Se comprueba con
// un type assertion en vez de ampliar cloudService para no obligar a todos los
// dobles de test de los handlers a implementar cuatro métodos que no ejercitan;
// el *service.Service real sí la satisface (si no, se responde 404 y la UI
// degrada, nunca datos inventados).
type dbStudioService interface {
	DbTables(ctx context.Context, projectID uuid.UUID) ([]service.DbTable, error)
	DbTableRows(ctx context.Context, projectID uuid.UUID, table string, limit int) (service.DbRows, error)
	DbAPIInfo(ctx context.Context, projectID uuid.UUID) (service.DbAPIInfo, error)
	RunDbQuery(ctx context.Context, projectID uuid.UUID, sql string) (service.SQLResult, error)
	DbConnectionString(ctx context.Context, projectID uuid.UUID) (string, error)
}

// El servicio real DEBE satisfacerla: así un cambio de firma es un error de
// compilación y no un 404 silencioso en producción.
var _ dbStudioService = (*service.Service)(nil)

// dbStudio resuelve el servicio y el proyecto de la ruta, o responde el error y
// devuelve ok=false. El proyecto se resuelve SIEMPRE dentro del tenant activo
// (ListProjects va bajo RLS), así que un id/slug ajeno da 404.
func (h *Handlers) dbStudio(w http.ResponseWriter, r *http.Request) (dbStudioService, uuid.UUID, bool) {
	svc, ok := h.svc.(dbStudioService)
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return nil, uuid.Nil, false
	}
	id, err := h.resolveProjectRef(r, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return nil, uuid.Nil, false
	}
	return svc, id, true
}

// resolveProjectRef acepta el UUID del proyecto o su slug: el dashboard navega
// por slug (/projects/mi-app/db) y llama al control-plane con él. La búsqueda
// por slug recorre los proyectos DEL TENANT (RLS), nunca una tabla global.
func (h *Handlers) resolveProjectRef(r *http.Request, ref string) (uuid.UUID, error) {
	if id, err := uuid.Parse(ref); err == nil {
		return id, nil
	}
	if ref == "" {
		return uuid.Nil, domain.ErrNotFound
	}
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		return uuid.Nil, err
	}
	for _, p := range projects {
		if p.Slug == ref {
			return p.ID, nil
		}
	}
	return uuid.Nil, domain.ErrNotFound
}

// ListDbTables introspecciona el esquema del Postgres del proyecto.
func (h *Handlers) ListDbTables(w http.ResponseWriter, r *http.Request) {
	svc, id, ok := h.dbStudio(w, r)
	if !ok {
		return
	}
	tables, err := svc.DbTables(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if tables == nil {
		tables = []service.DbTable{} // array vacío, nunca null: la UI itera sobre él
	}
	httpx.JSON(w, r, http.StatusOK, tables)
}

// GetDbTableRows devuelve las primeras filas de una tabla ("esquema.nombre").
func (h *Handlers) GetDbTableRows(w http.ResponseWriter, r *http.Request) {
	svc, id, ok := h.dbStudio(w, r)
	if !ok {
		return
	}
	table := chi.URLParam(r, "table")
	// chi devuelve el segmento tal cual viaja en la URL cuando lleva escapes
	// (encodeURIComponent del dashboard): se decodifica antes de validarlo.
	if strings.Contains(table, "%") {
		if dec, err := url.PathUnescape(table); err == nil {
			table = dec
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // vacío/basura → 0 → por defecto en el servicio
	rows, err := svc.DbTableRows(r.Context(), id, table, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, rows)
}

// GetDbAPIInfo devuelve el endpoint REST automático y las credenciales
// enmascaradas del Postgres del proyecto.
func (h *Handlers) GetDbAPIInfo(w http.ResponseWriter, r *http.Request) {
	svc, id, ok := h.dbStudio(w, r)
	if !ok {
		return
	}
	info, err := svc.DbAPIInfo(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, info)
}

// GetDbCredentials revela la cadena de conexión completa, con contraseña.
//
// `no-store` explícito: es una credencial y no puede quedarse en la caché del
// navegador ni en un proxy intermedio. Va por su propia ruta y no dentro de
// `/db/api` porque aquel lo puede leer cualquier miembro; esto, solo admin/owner.
func (h *Handlers) GetDbCredentials(w http.ResponseWriter, r *http.Request) {
	svc, id, ok := h.dbStudio(w, r)
	if !ok {
		return
	}
	dsn, err := svc.DbConnectionString(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connection_string": dsn})
}

// RunDbQuery ejecuta SQL del usuario contra la base de datos del proyecto. Un
// SQL inválido NO es un error HTTP: vuelve 200 con el campo `error` para que el
// editor lo pinte (igual que cualquier consola de BD).
func (h *Handlers) RunDbQuery(w http.ResponseWriter, r *http.Request) {
	svc, id, ok := h.dbStudio(w, r)
	if !ok {
		return
	}
	var req struct {
		SQL string `json:"sql"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := svc.RunDbQuery(r.Context(), id, req.SQL)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}
