// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// montarConector registra, en el árbol SIN sesión, la única ruta del consumidor: leer.
// La credencial es el token de la conexión, en `Authorization: Bearer` (nunca en la URL,
// que acaba en registros de acceso). No se publica en el ingress, que solo enruta
// `/api/v1`: la llaman los consumidores por dentro. No hay ruta de escritura para un token
// de conector: no puede escribir porque no existe el camino, no porque un `if` lo impida.
func (h *Handlers) montarConector(r chi.Router) {
	r.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: h.limiteConector, KeyFunc: claveDeConector})).
		Get("/_bases/v1/connector/read", h.leerConector)
}

// tokenDeConector saca el Bearer de la petición ("" si no lo hay).
func tokenDeConector(r *http.Request) string {
	const p = "Bearer "
	a := r.Header.Get("Authorization")
	if len(a) <= len(p) || !strings.EqualFold(a[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(a[len(p):])
}

// claveDeConector limita por token (su principio no es secreto útil: el token mide 47).
//
// NO lleva la dirección de quien conecta: `RemoteAddr` trae el PUERTO de origen, así que
// cada conexión TCP nueva estrenaba cubo y el límite no limitaba nada (cada lectura carga
// hasta 10.000 filas y escribe una entrada de auditoría). Lo que no tiene forma de token
// comparte un solo cubo: no puede leer nada y así no llena el mapa de claves.
func claveDeConector(r *http.Request) string {
	t := tokenDeConector(r)
	if !strings.HasPrefix(t, store.PrefijoTokenConector) || len(t) != len(store.PrefijoTokenConector)+43 {
		return "conn:mal"
	}
	return "conn:" + t[:12]
}

func (h *Handlers) leerConector(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// Quién lee y para qué organización lo dice el propio consumidor (cabeceras), y ha de
	// casar con la conexión: ver store.LeerPorConector. No es autenticación del servicio
	// —la ruta no sale del clúster y la credencial sigue siendo el token—: es lo que impide
	// que un token concedido a un producto se use desde otro, o en otra organización.
	lectura, err := h.Store.LeerPorConector(r.Context(), tokenDeConector(r),
		r.Header.Get("X-Sytue-Consumer"), r.Header.Get("X-Sytue-Tenant"), r.Header.Get("User-Agent"))
	if errors.Is(err, domain.ErrNoExiste) {
		// Sin token, mal formado, revocado o caducado: el mismo 401, sin decir cuál.
		w.Header().Set("WWW-Authenticate", `Bearer realm="sytue-bases-connector"`)
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("connection_invalid"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, lectura)
}

// --- gestión (con sesión, propietario) ---

type entradaConexion struct {
	Name     string `json:"name"`
	ViewID   string `json:"view_id"`
	Days     int    `json:"days"`
	Consumer string `json:"consumer"`
}

func (h *Handlers) crearConexion(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	tid, err2 := id(r, "tid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaConexion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.ViewID != "" {
		if in.ViewID, err = idDe(in.ViewID); err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
	}
	c, token, err := h.Store.CrearConexion(r.Context(), a, tid, in.ViewID, in.Name, in.Consumer, in.Days)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	// El token se entrega UNA vez, aquí, y no se guarda en claro.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, struct {
		store.Conexion
		Token string `json:"token"`
	}{c, token})
}

func (h *Handlers) listarConexiones(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cs, err := h.Store.ListarConexiones(r.Context(), a, bid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, cs)
}

func (h *Handlers) revocarConexion(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	cid, err2 := id(r, "cid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.RevocarConexion(r.Context(), a, cid))
}

func (h *Handlers) rotarConexion(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	cid, err2 := id(r, "cid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	version, token, err := h.Store.RotarConexion(r.Context(), a, cid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"version": version, "token": token})
}
