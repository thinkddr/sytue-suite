package httpx

import (
	"net"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// WSOriginPatterns es la allowlist para websocket.Accept: la lista cerrada de
// primera parte MÁS el host al que llegó la propia petición.
//
// Desde que las apps cuelgan del host de la organización
// (`corporate.sytue.eu/rooms`), el navegador manda `Origin:
// https://corporate.sytue.eu` y ningún patrón fijo lo casa: nhooyr/websocket
// contestaba 403 en el handshake y Rooms se quedaba sin tiempo real (medido en
// producción el 2026-09-08). Un comodín de zona (`*.sytue.eu`) lo arreglaba,
// pero `*` en filepath.Match casa puntos, así que también abría el socket a
// `x.apps.sytue.eu` y `pr-42.preview.sytue.eu`, donde corre código de clientes
// de Cloud; los tests de cada servicio lo rechazan, y con razón.
//
// El host de la petición es lo exacto: la página que abre el socket vive en el
// mismo host al que se lo pide. Es un cheque de MISMO ORIGEN, no de zona: una
// publicación en `x.apps.sytue.eu` manda ese Origin y el navegador sigue
// poniendo `Host: corporate.sytue.eu`, que no casa. Fuera del navegador Origin
// y Host los elige el cliente, pero esta comprobación nunca protegió de eso: es
// anti-CSRF, y el bearer sigue haciendo falta. Host lleva el puerto si lo hay,
// igual que el Origin que compara nhooyr. Cloudflare y el ingress conservan el
// Host original; si algún día un proxy lo reescribiera, aquí es donde mirar
// X-Forwarded-Host.
func WSOriginPatterns(r *http.Request, base []string, slugs ...string) []string {
	out := make([]string, 0, len(base)+2)
	out = append(out, base...)
	if r.Host == "" {
		return out
	}
	out = append(out, r.Host)

	// EL HOST DE LA PETICIÓN NO BASTA cuando el socket vive en OTRO host que la
	// página, que es justo el reparto de la suite: la página está en
	// `corporate.sytue.eu/rooms` y el socket va a `rooms.sytue.eu/ws`, así que
	// llega `Origin: https://corporate.sytue.eu` con `Host: rooms.sytue.eu` y no
	// casa ni la lista fija ni el host propio. Medido en producción el
	// 2026-09-09: el handshake se cerraba, el cliente reconectaba en bucle y
	// Rooms solo enseñaba los mensajes ajenos al recargar la página.
	//
	// La organización sale del TOKEN YA VERIFICADO, no de la petición: por el
	// contexto (authn.Middleware) o pasada a mano por quien valida el token
	// dentro del handler, como hace el WS de Rooms —el token viaja por query
	// porque el navegador no puede poner cabeceras en el handshake—.
	for _, slug := range slugs {
		if h := tenantHost(r.Host, slug); h != "" {
			out = append(out, h)
		}
	}
	if len(slugs) == 0 {
		if t, err := tenancy.From(r.Context()); err == nil {
			if h := tenantHost(r.Host, t.Slug); h != "" {
				out = append(out, h)
			}
		}
	}
	return out
}

// tenantHost es el host de una organización en LA MISMA zona que la petición:
// `rooms.sytue.eu` + `corporate` da `corporate.sytue.eu`. La zona se toma de la
// petición y no de configuración para que sirva igual en dev, en el piloto y en
// producción, cada uno con su dominio.
//
// El slug pasa por `tenancy.ValidSlug` y esa comprobación no es cosmética: sin
// ella un slug con `*` o `?` se convertiría en un COMODÍN de `filepath.Match` y
// abriría el socket a media zona, que es exactamente lo que esta allowlist
// existe para impedir. El slug viene de un token firmado, pero la validación es
// barata y el fallo sería silencioso.
func tenantHost(reqHost, slug string) string {
	if !tenancy.ValidSlug(slug) {
		return ""
	}
	host, port := reqHost, ""
	if h, p, err := net.SplitHostPort(reqHost); err == nil {
		host, port = h, ":"+p
	}
	// Sin zona (`localhost`) no hay dónde colgar la organización.
	i := strings.Index(host, ".")
	if i < 0 {
		return ""
	}
	return slug + "." + host[i+1:] + port
}
