// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/services/docs/internal/wopi"
)

// wopiDiscovery sirve el catálogo que convierte a Sytue Docs en la aplicación
// que abre los ficheros de un host WOPI.
//
// Es la pieza que faltaba para cerrar el círculo: Sytue Drive habla WOPI como
// host desde la migración 0016, pero necesitaba un `discovery.xml` de alguien
// para saber con qué URL abrir cada extensión, y ese alguien era un tercero. En
// producción `WOPI_DISCOVERY_URL` está vacía, así que «abrir en el editor» no
// funcionaba en absoluto. Apuntándola aquí, funciona con el editor de la casa.
func (h *Handler) wopiDiscovery(w http.ResponseWriter, r *http.Request) {
	xmlDoc, err := wopi.Descubrimiento(h.publicURL)
	if err != nil {
		// Sin URL pública configurada NO se sirve un catálogo a medias: un
		// `urlsrc` vacío haría que el host abriera una pantalla en blanco y que
		// el problema pareciera del editor.
		http.Error(w, "wopi discovery not configured", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// Cacheable un rato: el catálogo solo cambia cuando cambia el propio editor,
	// y los hosts lo releen periódicamente por su cuenta.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	_, _ = w.Write(xmlDoc)
}
