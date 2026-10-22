// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
)

const maxImagenesProductos = 200

// ImagenesProductos — GET /api/v1/productos/imagenes?registros=id,id.
// Solo identificadores de adjuntos: sus bytes siguen pasando por la descarga
// autenticada y el aislamiento de adjuntos que ya utiliza todo el ERP.
func (h *Handlers) ImagenesProductos(w http.ResponseWriter, r *http.Request) {
	if !h.accesoRegistro(r.Context(), "producto", permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	ids, err := idsDeImagenesProductos(r.URL.Query().Get("registros"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
		return
	}
	imagenes, err := h.store.ImagenesProductos(r.Context(), ids)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"imagenes": imagenes})
}

func idsDeImagenesProductos(valor string) ([]string, error) {
	if strings.TrimSpace(valor) == "" {
		return nil, nil
	}
	partes := strings.Split(valor, ",")
	if len(partes) > maxImagenesProductos {
		return nil, errors.New("at most 200 product identifiers are accepted")
	}
	ids := make([]string, 0, len(partes))
	vistos := make(map[string]bool, len(partes))
	for _, parte := range partes {
		id, err := uuid.Parse(strings.TrimSpace(parte))
		if err != nil {
			return nil, errors.New("registros must contain comma-separated UUIDs")
		}
		clave := id.String()
		if !vistos[clave] {
			ids = append(ids, clave)
			vistos[clave] = true
		}
	}
	return ids, nil
}
