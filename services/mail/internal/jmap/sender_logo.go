// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/services/mail/internal/logo"
)

// getSenderLogo es GET /api/v1/sender-logo?d=vodafone.es: el logo de la web de ese
// dominio como PNG, para el avatar de la fila. 404 si no tiene uno aprovechable, y
// los clientes vuelven a las iniciales.
//
// El dominio se normaliza AQUÍ (`corp.vodafone.es` → `vodafone.es`) y la caché va
// por el normalizado, así que da igual cómo lo mande el cliente: la web ya lo manda
// limpio y la app nativa, el de la dirección tal cual. Lo que no tiene logo que
// buscar —un buzón personal, una IP, basura— contesta 404 como un dominio sin icono,
// cacheable, y no sale ninguna petición.
func (a *API) getSenderLogo(w http.ResponseWriter, r *http.Request) {
	dominio := logo.Valido(r.URL.Query().Get("d"))
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	var png []byte
	if dominio != "" {
		png = a.logos.Logo(r.Context(), dominio)
	}
	if png == nil {
		// El «no hay» se cachea también en el cliente: sin esto, cada repintado de la
		// lista volvería a preguntar por cada remitente sin icono.
		h.Set("Cache-Control", "private, max-age=86400")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	h.Set("Content-Type", "image/png")
	h.Set("Content-Length", strconv.Itoa(len(png)))
	h.Set("Cache-Control", "private, max-age=604800")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(png)
}
