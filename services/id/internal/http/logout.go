// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleRevoke implementa la revocación de tokens (RFC 7009). Acepta refresh
// tokens; la revocación de un refresh anula su familia completa.
func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "")
		return
	}
	client, ok := s.authenticateClient(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="sytue-id"`)
		tokenError(w, r, http.StatusUnauthorized, "invalid_client", "")
		return
	}
	token := r.Form.Get("token")
	if token == "" {
		// RFC 7009: token desconocido o ausente => 200 igualmente.
		httpx.NoContent(w)
		return
	}
	if rt, err := s.store.GetRefreshToken(r.Context(), s.store.DB(), domain.HashToken(token)); err == nil {
		if rt.ClientID == client.ID {
			_ = s.store.RevokeRefreshChain(r.Context(), nil, rt.ID)
			_ = s.store.Audit(r.Context(), nil, auditEntry(rt.UserID, rt.OrgID, "oauth.revoke", client.ID, r))
		}
	}
	// Respuesta uniforme: no revelar si el token existía.
	w.WriteHeader(http.StatusOK)
}

// handleLogout implementa el end_session endpoint (OIDC RP-Initiated Logout):
// revoca la sesión SSO, borra la cookie, pide a CADA superficie de la suite que
// borre la suya y lleva al destino post-logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := s.requestCookie(r, sessionCookieName); err == nil && c.Value != "" {
		hash := domain.HashToken(c.Value)
		// Revocar los refresh tokens ligados a esta sesión (revocación en cascada):
		// cerrar la sesión SSO debe invalidar también los refresh derivados de ella.
		if sess, err := s.store.GetSessionByToken(r.Context(), hash); err == nil && sess.ID != "" {
			_ = s.store.RevokeRefreshTokensForSession(r.Context(), nil, sess.ID)
		}
		_ = s.store.RevokeSession(r.Context(), nil, hash)
	}
	s.clearSessionCookie(w)

	destino := s.postLogoutTarget(r)
	// El barrido lo dirige SYTUE ID y no cada app, por dos motivos. Cerrar sesión
	// tiene que cerrarla en las diez superficies aunque se inicie AQUÍ (el enlace
	// de /account, que no pasa por ninguna app), y hacerlo desde el IdP es el
	// único punto por el que pasan todos los cierres, vengan de donde vengan.
	barrido := s.suiteLogoutURIs(r.Context())
	if len(barrido) == 0 {
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(logoutSweepPage(barrido, destino)))
}

// postLogoutTarget resuelve a dónde vuelve quien cierra sesión.
func (s *Server) postLogoutTarget(r *http.Request) string {
	q := r.URL.Query()
	target := q.Get("post_logout_redirect_uri")
	if target != "" && q.Get("client_id") != "" {
		// Validar el destino contra los registrados del client (anti open-redirect).
		client, err := s.store.GetClient(r.Context(), q.Get("client_id"))
		if err == nil {
			for _, allowed := range client.PostLogoutRedirectURIs {
				// Estar registrado no basta: se re-comprueba la FORMA (https absoluto,
				// o http en loopback para dev) antes de mandar ahí al navegador. Lo que
				// hay en la columna se escribió en otro momento y por otra puerta —el
				// registro dinámico llegó a guardarla sin validar—, y este es el único
				// punto por el que ese valor sale hacia el navegador: una fila heredada
				// con `javascript:` o con un host cualquiera no se convierte en un
				// redirect solo porque alguien la insertara.
				if allowed == target && validRegistrationRedirect(target) {
					return target
				}
			}
		}
	}
	// Sin destino válido se vuelve al sitio público, NO se responde JSON: quien
	// llega aquí es una persona con un navegador. El enlace «Cerrar sesión» de
	// /account no lleva client_id —no hay ninguno: se está cerrando la sesión del
	// propio IdP—, así que ANTES acababa viendo {"status":"logged_out"} en crudo
	// y creyendo que algo había fallado.
	//
	// El destino es configuración del servicio, nunca un parámetro de la petición:
	// aceptar aquí una URL de la query sería exactamente el open-redirect que la
	// validación de arriba impide.
	return s.cfg.PostLogoutDefaultURI
}

// suiteLogoutURIs devuelve el endpoint de cierre LOCAL de cada superficie de la
// suite. Cada BFF emite una cookie host-only, así que solo su propio host puede
// borrarla: sin este barrido, cerrar sesión en una app dejaba vivas las otras
// nueve hasta que caducara su access token.
//
// La lista se DERIVA de los redirect_uri de los clients de primera parte (los que
// no pertenecen a ninguna organización) filtrados por el dominio de la suite del
// entorno, en vez de estar escrita a mano: dar de alta una superficie nueva ya
// pasa por registrar su client, y una lista aparte se quedaría atrás en silencio.
func (s *Server) suiteLogoutURIs(ctx context.Context) []string {
	clients, err := s.store.ListClients(ctx)
	if err != nil {
		// Best-effort: sin la lista se sigue cerrando la sesión SSO, que es lo que
		// de verdad corta el acceso; las cookies locales caen con su access token.
		s.log.Error("logout: no se pudieron listar los clients para el barrido", "error", err)
		return nil
	}
	suite := suiteDomain(s.cfg.Issuer)
	if suite == "" {
		return nil
	}
	vistos := make(map[string]bool, len(clients))
	out := make([]string, 0, len(clients))
	for _, c := range clients {
		if c.OrgID != "" {
			continue // client de una organización cliente: su sesión no es nuestra
		}
		for _, ru := range c.RedirectURIs {
			uri := sweepURI(ru, suite)
			if uri == "" || vistos[uri] {
				continue
			}
			vistos[uri] = true
			out = append(out, uri)
		}
	}
	sort.Strings(out) // orden estable: la página es determinista y testeable
	return out
}

// sweepURI traduce el redirect_uri de un client al endpoint que cierra la sesión
// de ESA aplicación, o "" si no hay ninguno que pedir.
//
// El TIPO de aplicación se deduce del propio callback, que ya está registrado, en
// vez de una tabla aparte: `/auth/callback` es el BFF de @sytue/auth (las diez
// superficies Astro) y `/user/oauth2/<proveedor>/callback` es la forja, que impone
// esa ruta y cierra por `GET /user/logout`. Lo que no encaje en ninguno se queda
// fuera: pedir un endpoint inventado solo produce un 404 por cada cierre.
func sweepURI(redirectURI, suite string) string {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	// Solo el dominio de ESTE entorno: los clients llevan también los callbacks
	// de dev, y pedirle a sytue.localhost que cierre sesión desde producción sería
	// una petición al equipo de quien navega. Sufijo con punto y no `contains`,
	// que dejaría pasar `sytue.eu.evil.com`.
	if h := u.Hostname(); h != suite && !strings.HasSuffix(h, "."+suite) {
		return ""
	}
	origen := u.Scheme + "://" + u.Host
	switch {
	// POR SUFIJO Y CONSERVANDO EL PREFIJO, no `== "/auth/callback"`.
	//
	// En su propio subdominio cada aplicación vive en la raíz, pero bajo el HOST
	// DE ORGANIZACIÓN —`<slug>.sytue.eu`— viven todas juntas con prefijo:
	// `/mail/auth/callback`, `/one/auth/callback`… Exigir la raíz descartaba esos
	// diez clients enteros, así que cerrar sesión desde la dirección de la
	// organización dejaba abiertas sus diez superficies SIN decir nada: el barrido
	// no fallaba, sencillamente no las incluía.
	//
	// El prefijo se conserva porque ahí no hay nada que sirva `/auth/logout` a
	// pelo: bajo ese host solo `/` tiene regla propia, y todo lo demás entra por
	// el prefijo de su aplicación. Pedirlo sin él es el 404 que se vio.
	case strings.HasSuffix(u.Path, "/auth/callback"):
		return origen + strings.TrimSuffix(u.Path, "/callback") + "/logout?scope=local"
	case strings.HasPrefix(u.Path, "/user/oauth2/") && strings.HasSuffix(u.Path, "/callback"):
		return origen + "/user/logout"
	}
	return ""
}

// logoutSweepPage pinta la página de cierre: un subrecurso por superficie que
// borra su cookie y, en cuanto terminan todos, la navegación al destino.
//
// Subrecursos y no fetch: son peticiones a OTRO origen y un fetch exigiría CORS
// en las diez; una imagen no lo necesita y lleva la cookie igual (misma suite =
// mismo site). No se pinta nada de ellas: lo único que importa es el Set-Cookie
// de borrado que devuelven.
func logoutSweepPage(uris []string, destino string) string {
	var b strings.Builder
	b.WriteString(`<p class="sub">Cerrando sesión en todas tus aplicaciones…</p>`)
	for _, u := range uris {
		b.WriteString(`<img src="` + html.EscapeString(u) + `" alt="" width="1" height="1">`)
	}
	// Sin JS no hay evento de carga que esperar: el enlace es la salida manual.
	b.WriteString(`<noscript><p class="alt"><a class="btn-link" href="` +
		html.EscapeString(destino) + `">Continuar</a></p></noscript>`)
	b.WriteString(`<script>` + logoutSweepScript + `</script>`)
	return logoutPage("Cerrando sesión", destino, b.String())
}

// logoutSweepScript navega al destino en cuanto las diez superficies han
// respondido (o a los 2 s, si alguna no lo hace: una app caída no puede dejar a
// nadie atrapado en esta pantalla). El destino viaja en un atributo del <body>
// —ya validado contra la allowlist del client— para que el script sea una
// constante y su hash CSP no dependa de la petición.
const logoutSweepScript = `(function(){` +
	`var b=document.body,d=b.getAttribute('data-next'),i=document.images,n=i.length,c=0,done=false;` +
	`function go(){if(done){return;}done=true;location.replace(d);}` +
	`setTimeout(go,2000);` +
	`if(!n){go();return;}` +
	`function f(){if(++c>=n){go();}}` +
	`for(var k=0;k<n;k++){if(i[k].complete){f();}else{i[k].onload=f;i[k].onerror=f;}}` +
	`})();`

// logoutPage es el shell de la página de cierre: mismo marco que loginPage, con
// el destino en el <body> para el script de barrido.
func logoutPage(title, destino, body string) string {
	return `<!doctype html><html data-theme="light" lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">` +
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<meta name="color-scheme" content="light">` +
		`<meta name="robots" content="noindex"><title>` + html.EscapeString(title) + ` · Sytue ID</title>` +
		`<style>` + loginCSS + `</style><script>` + themeInitJS + `</script></head><body data-next="` + html.EscapeString(destino) + `">` +
		`<main class="card sytue-glass-surface">` +
		`<div class="brand"><span class="logo"><svg viewBox="0 0 40 40" aria-hidden="true">` +
		`<defs><clipPath id="m"><rect width="40" height="40" rx="9"/></clipPath></defs>` +
		`<g clip-path="url(#m)"><rect width="40" height="40" fill="#0A0A0A"/>` +
		`<polygon points="40,14 40,40 14,40" fill="#b98f4f"/></g></svg></span>Sytue<span class="tag">ID</span></div>` +
		`<h1>` + html.EscapeString(title) + `</h1>` + body + `</main></body></html>`
}
