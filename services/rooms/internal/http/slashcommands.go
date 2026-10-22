// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// SLASH COMMANDS de terceros: gestión (admin) + ejecución (cualquiera del canal).

// errCommandFailed es el 502 de «el comando no contestó». 502 y no 500: el fallo está
// en el servicio de un tercero, y decir 500 mandaría a mirar los registros de Rooms.
var errCommandFailed = httpx.NewError(http.StatusBadGateway, "command_failed",
	"The command did not respond.")

type createCommandRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	UsageHint   string `json:"usage_hint,omitempty"`
	RequestURL  string `json:"request_url"`
}

// CreateCommand registra un comando (admin del workspace). Devuelve el token de
// verificación y el secreto de firma: el segundo hay que pegarlo en el servicio que
// atiende el comando para que compruebe que la llamada viene de Rooms.
func (ih *intHandlers) CreateCommand(w http.ResponseWriter, r *http.Request) {
	var req createCommandRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	cmd, err := ih.svc.CreateSlashCommand(r.Context(), req.Name, req.Description, req.UsageHint, req.RequestURL, actor)
	if err != nil {
		if errors.Is(err, store.ErrSlashNameTaken) {
			// 409 y no 400: el nombre es válido, lo que pasa es que ya está cogido —y
			// eso quien lo pide puede arreglarlo cambiándolo.
			httpx.Fail(w, r, httpx.ErrConflict.WithCode("command_name_taken"))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"command": cmd})
}

// ListCommands lista los comandos de la organización. Lo puede pedir cualquiera: el
// autocompletado del compositor los necesita. Los secretos solo viajan para un admin.
func (ih *intHandlers) ListCommands(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	cmds, err := ih.svc.ListSlashCommands(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if cmds == nil {
		cmds = []domain.SlashCommand{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"commands": cmds})
}

// DeleteCommand borra un comando (admin del workspace).
func (ih *intHandlers) DeleteCommand(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DeleteSlashCommand(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type executeCommandRequest struct {
	ChannelID string `json:"channel_id"`
	// Input es la línea entera tal como se escribió: «/desplegar prod». El servicio la
	// parte, porque separar el nombre de los argumentos es su regla, no la del cliente.
	Input string `json:"input"`
}

// ExecuteCommand ejecuta un comando de terceros y devuelve su respuesta.
//
// POST (mutación: puede publicar en el canal). La verja de canal la aplica el servicio:
// sin ella, un comando serviría para publicar donde no se está.
func (ih *intHandlers) ExecuteCommand(w http.ResponseWriter, r *http.Request) {
	var req executeCommandRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	channelID, err := uuid.Parse(req.ChannelID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_channel_id"))
		return
	}
	actor, name := caller(r)
	res, err := ih.svc.ExecuteSlashCommand(r.Context(), channelID, actor, name, req.Input)
	switch {
	case err == nil:
		httpx.JSON(w, r, http.StatusOK, res)
	case errors.Is(err, domain.ErrNotFound):
		// El comando no existe. Se dice con claridad: un 404 mudo dejaría a la persona
		// pensando que el canal es el problema.
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("command_not_found"))
	case errors.Is(err, service.ErrSlashFailed):
		// El comando existe pero no contestó. 502 y no 500: el fallo es del otro lado.
		httpx.Fail(w, r, errCommandFailed)
	default:
		httpx.Fail(w, r, toAPIError(err))
	}
}
