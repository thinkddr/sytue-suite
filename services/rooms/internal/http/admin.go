// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración de Rooms desde la consola de la organización.
//
// Hasta aquí Rooms no tenía NADA de administración: ni una ruta `/admin`, ni una
// sección en la consola. El efecto práctico es el que se nota cuando pasa algo:
// un canal privado se archiva por error y nadie de fuera puede deshacerlo;
// alguien deja la empresa y no hay forma de saber en qué canales estaba sin
// preguntar equipo por equipo; y no existe una respuesta a «cuántos canales hay»
// que no sea abrir la base de datos.
//
// La autorización va A LA PUERTA con `RequireRole("owner","admin")` —el mismo
// patrón que Drive (`server.go`: `adm.Use(d.Verifier.RequireRole(...))`)— y el
// aislamiento por organización lo sigue aplicando RLS con el tenant del token.
// Dentro NO se comprueba la pertenencia al canal, que es justo lo que distingue
// esta superficie de la del producto.
//
// El paquete se llama `chathttp` y no `http`: el directorio es `internal/http`,
// pero el nombre corto chocaría con el `net/http` de la biblioteca estándar en
// cada fichero que importe los dos.
package chathttp

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// adminStore es lo que la administración necesita del almacén. Interfaz propia y
// no un añadido a `chatService`: esa la implementan los dobles de todos los
// tests de handlers, y ampliarla obligaría a tocarlos para métodos que no usan.
type adminStore interface {
	ListAllChannels(ctx context.Context, includeArchived bool) ([]store.AdminChannel, error)
	SetChannelArchivedAsAdmin(ctx context.Context, id uuid.UUID, archived bool) (domain.Channel, error)
	Stats(ctx context.Context) (store.AdminStats, error)
}

// AdminHandlers sirve /api/v1/admin de Rooms.
type AdminHandlers struct {
	st adminStore
}

// NewAdminHandlers construye los handlers de administración.
func NewAdminHandlers(st adminStore) *AdminHandlers { return &AdminHandlers{st: st} }

// ListChannels devuelve todos los canales de la organización.
//
// `?archived=1` incluye los archivados. Por defecto NO salen: son la excepción y
// llenarían la lista de ruido justo cuando se busca algo que está vivo.
func (h *AdminHandlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	incluirArchivados := r.URL.Query().Get("archived") == "1"
	items, err := h.st.ListAllChannels(r.Context(), incluirArchivados)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if items == nil {
		items = []store.AdminChannel{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// archiveRequest es el cuerpo de PATCH /admin/channels/{channelID}.
type archiveRequest struct {
	Archived *bool `json:"archived"`
}

// SetChannelArchived archiva o restaura un canal.
//
// Solo `archived`: el nombre, el tema y el propósito los lleva el equipo dueño
// del canal. Un administrador de la organización necesita poder DESHACER un
// archivado —y retirar de la vista un canal que ya no toca—, no reescribir de
// qué habla la gente.
func (h *AdminHandlers) SetChannelArchived(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req archiveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Archived == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Falta el campo archived."))
		return
	}
	ch, err := h.st.SetChannelArchivedAsAdmin(r.Context(), id, *req.Archived)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

// Stats devuelve el resumen de Rooms de la organización.
func (h *AdminHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.st.Stats(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, st)
}
