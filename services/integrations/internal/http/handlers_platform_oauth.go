// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Credenciales con las que SYTUE se presenta ante cada proveedor de terceros.
//
// PARA QUÉ. Con una app registrada por Sytue ante Google o Microsoft, migrar el
// correo de una empresa es UN CLIC: su admin aprueba una pantalla de permisos y
// listo. Sin ella, cada organización tiene que darse de alta como desarrollador
// ante el proveedor, declarar el redirect_uri, esperar su revisión y pegar aquí
// client_id y secret. Sigue funcionando, pero deja de ser algo que se hace en una
// tarde, y es la diferencia entre que un cliente migre o no migre.
//
// Registrar la app ante cada proveedor es trabajo de OPERACIÓN —darse de alta,
// declarar ámbitos, pasar revisión—; lo que estos endpoints resuelven es el otro
// medio: que, una vez hecho, meter la credencial no exija editar un JSON en el
// chart y reiniciar los pods, que es como estaba y es la razón de que en
// producción estuviera sencillamente vacío.
//
// VIVEN EN LA SUPERFICIE DE OPERADOR, fuera del JWT de la suite: son de la
// plataforma, no de ninguna organización, y ningún miembro de un tenant debe
// poder ni leerlas ni saber cuáles hay.

type platformOAuthReq struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// KeepSecret conserva el secret guardado y cambia solo el client_id. Existe
	// porque varios proveedores enseñan el secret UNA vez: sin esto, corregir una
	// errata en el client_id obligaría a rotar el secret ante el proveedor.
	KeepSecret bool `json:"keep_secret"`
	// UpdatedBy es quién lo hace, que llega del BFF de plataforma tras comprobar
	// que es admin de plataforma. Es para auditar, no para autorizar: lo que
	// autoriza es el token de operador.
	UpdatedBy string `json:"updated_by"`
}

// ListPlatformOAuthClients — GET /operator/oauth-clients.
//
// Nunca devuelve secretos, solo si los hay. Un secreto que se puede leer de vuelta
// acaba en un log, en una captura de pantalla o en el historial de quien
// administra; y para comprobar que una credencial es la correcta basta con ver el
// client_id, que además es público por diseño (viaja en la URL de autorización).
func (h *Handlers) ListPlatformOAuthClients(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListPlatformOAuthClients(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Lista vacía y no null: un `null` obliga a cada cliente a comprobarlo antes
	// de iterar, y basta que uno se olvide para que la pantalla reviente en vez de
	// decir «todavía no hay ninguna».
	if list == nil {
		list = []store.PlatformOAuthClient{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"clients": list})
}

// PutPlatformOAuthClient — PUT /operator/oauth-clients/{template}.
func (h *Handlers) PutPlatformOAuthClient(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "template"))
	if key == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The template key is missing."))
		return
	}
	var req platformOAuthReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"client_id is required: it is what Sytue identifies itself with to the provider.",
		))
		return
	}
	// El secret NO se recorta con TrimSpace hasta comprobar que no es solo espacio:
	// hay proveedores cuyos secrets llevan caracteres raros, y recortar por dentro
	// sería corromperlo. Solo se descarta si está en blanco entero.
	secret := req.ClientSecret
	if strings.TrimSpace(secret) == "" {
		secret = ""
	}
	if err := h.store.PutPlatformOAuthClient(r.Context(), key, clientID, secret,
		strings.TrimSpace(req.UpdatedBy), req.KeepSecret); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Sin esto habría que esperar a que caduque la caché para que la credencial
	// recién pegada sirva de algo, y quien la acaba de guardar va a probarla
	// inmediatamente: parecería que no funciona.
	h.oauthClients.invalidate()
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"template_key": key, "client_id": clientID, "status": "saved",
	})
}

// DeletePlatformOAuthClient — DELETE /operator/oauth-clients/{template}.
//
// Las conexiones ya establecidas siguen vivas con el refresh token que guardaron:
// esto no las revoca, retira el camino de un clic para las NUEVAS. Revocar el
// acceso concedido se hace desde el proveedor, que es quien lo emitió.
func (h *Handlers) DeletePlatformOAuthClient(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "template"))
	if key == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The template key is missing."))
		return
	}
	if err := h.store.DeletePlatformOAuthClient(r.Context(), key); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	h.oauthClients.invalidate()
	w.WriteHeader(http.StatusNoContent)
}

// OperatorListTemplates — GET /operator/templates.
func (h *Handlers) OperatorListTemplates(w http.ResponseWriter, r *http.Request) {
	tpls, err := h.store.ListAllTemplates(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"templates": tpls})
}

type setTemplateEnabledReq struct {
	Enabled   bool   `json:"enabled"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// OperatorSetTemplateEnabled — PUT /operator/templates/{template}/enabled.
func (h *Handlers) OperatorSetTemplateEnabled(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "template"))
	if key == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The template key is missing."))
		return
	}
	var req setTemplateEnabledReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.SetTemplateEnabled(r.Context(), key, req.Enabled); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"template_key": key,
		"enabled":      req.Enabled,
		"status":       "saved",
	})
}
