// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// La puerta de entrada del protocolo: lo que el host abre cuando alguien pulsa
// «abrir en el editor». Llega como POST de formulario desde OTRO origen (el
// host), con el `access_token` en el cuerpo y el `WOPISrc` en la query — así lo
// define WOPI y así lo manda ya Sytue Drive.
//
// POR QUÉ ES UN PUENTE Y NO LA PÁGINA DEL EDITOR. La superficie de Docs la sirve
// la aplicación Astro, y esa aplicación rechaza con 403 todo POST cuyo `Origin`
// no sea el suyo (checkOrigin, la protección CSRF de la suite). Es correcto que
// lo haga: apagarla para toda la app por una ruta sería cambiar una defensa real
// por una comodidad. Así que el POST de fuera lo recibe ESTE handler, que valida
// el host y entrega al navegador la página del editor con un `location.replace`.
//
// Y EL TOKEN VIAJA EN EL FRAGMENTO, no en la query. El fragmento no se envía al
// servidor: no aparece en el registro de accesos del ingress, ni en el
// `Referer` de la siguiente navegación, ni —por ser un `replace`— en el
// historial como paso atrás. Un 303 con el token en `Location` habría sido dos
// líneas menos y lo habría dejado escrito en la cabecera de respuesta.

// tplPuente entrega la página del editor. El destino se interpola en contexto
// JavaScript y lo escapa html/template: es la razón de usar html/template y no
// una concatenación, porque ahí dentro va un token.
var tplPuente = template.Must(template.New("wopi-bridge").Parse(
	`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="robots" content="noindex">
<title>Sytue Docs</title></head>
<body style="margin:0;background:#0f1113">
<noscript>JavaScript is required to open this file.</noscript>
<script>location.replace({{.}})</script>
</body></html>`,
))

// wopiLanzar recibe el formulario del host y entrega la página del editor.
//
// `modo` es la acción del catálogo (`view` o `edit`). Que el host pida `edit` no
// concede nada: quien decide si se puede escribir es `UserCanWrite`, que llega
// después en el CheckFileInfo.
func (h *Handler) wopiLanzar(modo string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		src := r.URL.Query().Get("WOPISrc")
		// PostFormValue y no FormValue: el token se lee del CUERPO. Aceptarlo
		// también en la query sería bendecir justo la vía que deja credenciales
		// en los registros de todo lo que haya por el camino.
		token := r.PostFormValue("access_token")
		if src == "" || token == "" {
			http.Error(w, "Missing WOPISrc or access token.", http.StatusBadRequest)
			return
		}
		if !hostPermitido(src, h.wopiHosts) {
			http.Error(w, "This file host is not allowed.", http.StatusForbidden)
			return
		}

		frag := url.Values{"src": {src}, "token": {token}, "mode": {modo}}
		// El idioma que manda el host (`<ui=UI_LLCC&>` del catálogo): sin él, el
		// editor abriría en un idioma distinto al de la aplicación desde la que se
		// pulsó. Se pasa tal cual y lo valida el negociador de idioma.
		if lang := r.URL.Query().Get("lang"); lang != "" && len(lang) <= 16 {
			frag.Set("lang", lang)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		// Sin esto, la URL de esta página —que lleva el WOPISrc— se iría como
		// `Referer` en lo siguiente que cargue el editor.
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Esta pantalla se pinta DENTRO del host —así es como previsualiza Drive sin
		// sacarte de la carpeta—, y por eso no puede llevar el `frame-ancestors
		// 'none'` del resto de la suite. Se abre solo a los hosts admitidos: quien
		// puede mandarte un fichero es quien puede enmarcar su editor.
		w.Header().Set("Content-Security-Policy", cspEnmarcado(h.wopiHosts))
		_ = tplPuente.Execute(w, visorWopi(h.publicURL)+"#"+frag.Encode())
	}
}

// visorWopi es la página web que abre el fichero: la de Sytue Docus, en su host y bajo
// su prefijo (`docus.<dominio>/docus/wopi/editor`), que es la que pinta cualquier tipo.
// Desde el 2026-10-12 la web no vive en el host de esta API (ADR 0076), así que la
// ruta relativa de antes acabaría en `docs.<dominio>`, donde ya no hay web.
//
// ponytail: siempre Docus, porque aquí solo llega el WOPISrc y no el tipo del fichero;
// si un día interesa abrir cada tipo en su producto, Drive tendría que mandarlo.
func visorWopi(publicURL string) string {
	u, err := url.Parse(publicURL)
	if err != nil || u.Host == "" {
		return "/docus/wopi/editor"
	}
	if _, dominio, ok := strings.Cut(u.Host, "."); ok {
		u.Host = "docus." + dominio
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/docus/wopi/editor"}).String()
}

// cspEnmarcado abre `frame-ancestors` a los hosts WOPI admitidos, y a nadie más.
//
// Sin hosts la respuesta es 'none' y no un comodín: una lista vacía significa
// «esta instalación no interopera con nadie», no «con todos».
func cspEnmarcado(hosts []string) string {
	var b strings.Builder
	b.WriteString("object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors")
	if len(hosts) == 0 {
		b.WriteString(" 'none'")
		return b.String()
	}
	// 'self' además de la lista: el puente y la pantalla del editor son el mismo
	// origen y uno navega al otro.
	b.WriteString(" 'self'")
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			b.WriteString(" https://" + h)
		}
	}
	return b.String()
}
