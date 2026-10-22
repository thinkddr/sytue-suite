// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// barraFinalSobra atiende lo que ninguna ruta reconoce.
//
// Las páginas del IdP viven sin barra final —`/account`, `/login`— y la barra la
// añade cualquiera: un marcador, una URL escrita a mano, un enlace copiado de
// otro sitio. Eso terminaba en el 404 JSON de la API —`{"error":{"code":
// "not_found"…}}` en crudo, comprobado con navegador el 2026-09-08— y en la
// pantalla más vista de la suite. GET y HEAD con barra final se redirigen a la
// ruta sin ella; todo lo demás sigue siendo el 404 de siempre.
//
// Solo la ruta: ni esquema ni host, así que no puede mandar fuera del IdP.
func barraFinalSobra(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if len(p) > 1 && strings.HasSuffix(p, "/") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		u := *r.URL
		u.Path = strings.TrimRight(p, "/")
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return
	}
	httpx.Fail(w, r, httpx.ErrNotFound)
}
