// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pageshttp es la superficie HTTP de la API de Sytue Web
// (PAGES-ENCARGO §6): la API con sesión bajo /api/v1 (solo dentro del clúster, A1),
// lo público de las webs de los clientes bajo /_pages, los webhooks de las
// pasarelas y el ask de Caddy. El servidor de las webs en sí es otro modo del
// binario (`pages sitios`, internal/sitios).
//
// Aquí se fijan los GRUPOS —verjas, plazos, topes de cuerpo y cubos— y cada área
// registra sus rutas en ellos desde su propio fichero de montaje (handlers.go,
// dominios.go, tienda.go, ia.go…), que su dueño sustituye entero.
package pageshttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Plazos y topes de cuerpo por grupo (§6.1, §20.3).
const (
	// plazoAPI es el de casi todo: una petición, una transacción.
	plazoAPI = 30 * time.Second
	// plazoLargo es el de las subidas de recursos, las publicaciones y los zip.
	// Nunca 0: sin plazo, una subida parada retiene su conexión para siempre.
	plazoLargo = 5 * time.Minute

	// maxCuerpo es el de una publicación (64 MiB de ficheros) con el sobre multipart.
	maxCuerpo = 65 << 20
	// maxCuerpoAPI deja pasar un borrador entero (el IR tope es de 2 MiB).
	maxCuerpoAPI = 4 << 20
	// maxCuerpoPublico es el de un checkout o un formulario de una web.
	maxCuerpoPublico = 256 << 10
	// maxCuerpoWebhook es el de una notificación de pasarela (Mollie se acota
	// además a 8 KiB en su manejador).
	maxCuerpoWebhook = 1 << 20
)

// grupos son los routers donde cada área registra sus rutas. Cada uno lleva ya su
// verja, su plazo y su tope de cuerpo: el manejador solo añade lo suyo (el rol, el
// cubo por sitio, `X-Sytue-Publicador`…).
type grupos struct {
	// api es /api/v1 con la sesión de una ORGANIZACIÓN (una cuenta personal no
	// entra) y el plazo de 30 s. Solo lo llaman el BFF y el Hub, por dentro (A1).
	api chi.Router
	// apiLenta es api con el plazo de 5 min: subidas, publicaciones y zip.
	apiLenta chi.Router
	// apiPersonal es /api/v1 con sesión pero SIN la verja de organización: solo
	// GET /export («Exportar tus datos» entrega vacío a una cuenta personal).
	apiPersonal chi.Router
	// publico es /_pages, lo que llaman las webs de los clientes (y el código
	// exportado, §9.5): sin sesión, con HSTS sin subdominios y sin COEP.
	publico chi.Router
	// webhooks es /webhooks: su verificación es su única autenticación. Lleva un
	// cubo compartido por proveedor.
	webhooks chi.Router
	// borde es la raíz con un cubo global en memoria: GET /_borde/permitido.
	borde chi.Router
}

// NewRouter construye el router completo de la API de Sytue Web.
func NewRouter(d Deps) http.Handler {
	r, g := construir(d)
	montarRutas(g, &Handlers{Deps: d})
	return r
}

// montarRutas registra las rutas de todas las áreas. Aparte de NewRouter para que el
// test de solapes monte el árbol ENTERO: dos áreas que montan el mismo prefijo se
// tapan sin que ninguna de las dos lo note (ver TestNingunaRutaQuedaTapada).
func montarRutas(g grupos, h *Handlers) {
	montarNucleo(g, h)
	montarSitios(g, h)
	montarDominios(g, h)
	montarAnalitica(g, h)
	montarBorde(g, h)
	montarPublicoTienda(g, h)
	montarDenuncias(g, h)
	montarTienda(g, h)
	montarWebhooks(g, h)
	montarFormularios(g, h)
	montarIA(g, h)
}

// construir monta el router base y sus grupos, sin rutas de ningún área.
func construir(d Deps) (*chi.Mux, grupos) {
	r := httpx.NewRouter(httpx.RouterOptions{MaxBodyBytes: maxCuerpo})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)
	var g grupos

	// ponytail: cubo en memoria, por réplica, como SyERP: con sesión solo frena
	// bucles de clientes propios. pg.RateLimiter si hiciera falta del servicio entero.
	apiLimiter := httpx.NewTokenBucket(50, 100)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))
		g.apiPersonal = api.With(conPlazo(plazoAPI), httpx.MaxBodyBytes(maxCuerpoAPI))
		api.Group(func(org chi.Router) {
			// Pages es de organizaciones (solo Pro y Enterprise, §17): la verja NO
			// puede ser el rol, porque la persona es `owner` de su cuenta personal.
			org.Use(d.Verifier.RequireOrgKind("org"))
			g.api = org.With(conPlazo(plazoAPI), httpx.MaxBodyBytes(maxCuerpoAPI))
			g.apiLenta = org.With(conPlazo(plazoLargo))
		})
	})

	r.Route("/_pages", func(pub chi.Router) {
		pub.Use(cabecerasDeWeb, conPlazo(plazoAPI), httpx.MaxBodyBytes(maxCuerpoPublico))
		g.publico = pub
	})

	webhookLimiter := pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 50, Burst: 100})
	r.Route("/webhooks", func(wh chi.Router) {
		wh.Use(conPlazo(plazoAPI), httpx.MaxBodyBytes(maxCuerpoWebhook))
		wh.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: webhookLimiter, KeyFunc: claveDeWebhook}))
		g.webhooks = wh
	})

	// El cubo del ask va dentro de su manejador, solo para lo que pregunta a la base (borde.go).
	g.borde = r.With(conPlazo(plazoAPI))
	return r, g
}

// cabecerasDeWeb ajusta las cabeceras de fábrica a lo que sirve una web de un
// CLIENTE: HSTS sin includeSubDomains ni preload —el Host es su dominio, y
// fijárselo a sus subdominios o precargarlo no nos corresponde— y sin COEP, que
// no pinta nada en una respuesta JSON que llaman otros orígenes (§9.5).
func cabecerasDeWeb(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if h.Get("Strict-Transport-Security") != "" {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		h.Del("Cross-Origin-Embedder-Policy")
		next.ServeHTTP(w, r)
	})
}

// conPlazo fija el plazo de la petición entera: el del contexto y también los de
// lectura y escritura de la conexión, que el servidor deja en 15/30 s y que una
// subida o un zip de minutos necesitan ampliar.
func conPlazo(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limite := time.Now().Add(d)
			rc := http.NewResponseController(w)
			_ = rc.SetReadDeadline(limite)
			_ = rc.SetWriteDeadline(limite)
			ctx, cancel := context.WithDeadline(r.Context(), limite)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// claveDeWebhook reparte el cubo de /webhooks por proveedor (`webhook:<proveedor>`) y,
// cuando la IP identifica a quien llama, también por IP: quien inunda con notificaciones
// falsas gasta su cubo y no el de la pasarela (WEB-11). Detrás del túnel del piloto no hay
// IP real y el cubo sigue siendo por proveedor; lo que se pierda ahí lo recoge el barrido.
// Lo que no es un proveedor conocido comparte uno: si no, cada ruta inventada crearía su
// propia fila en platform_rate_limits.
func claveDeWebhook(r *http.Request) string {
	proveedor, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/webhooks/"), "/")
	if !pasarelas[proveedor] {
		return "webhook:otro"
	}
	if ip := ipDeVisitante(r); clientip.TrustedProxiesFrom(r.Context()).Identifica(ip) {
		return "webhook:" + proveedor + ":" + ip
	}
	return "webhook:" + proveedor
}

var pasarelas = map[string]bool{"mollie": true, "adyen": true, "redsys": true, "stripe": true}
