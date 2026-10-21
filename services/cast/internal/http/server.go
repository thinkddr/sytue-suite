// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package casthttp es la superficie HTTP de Sytue Cast (CAST-ENCARGO §5): la API
// de vídeos con sesión bajo /api/v1 (alta, subida, biblioteca, edición, personas,
// reproducción, cuota, exportación), lo público sin sesión bajo /public y
// /api/v1/public (portada, canal, ficha, reproducción, comentarios, oEmbed), los
// ficheros firmados en /media, el WebSocket del chat en /ws y el webhook de
// LiveKit en /webhooks.
//
// Quién ve qué lo decide `internal/acceso`, en un solo sitio; aquí solo se
// traducen errores y se pintan respuestas.
package casthttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/diapositivas"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Deps son las dependencias del router HTTP de Cast.
type Deps struct {
	Cfg      config.Config
	Store    *store.Store
	Servicio *service.Servicio
	// Social puede ir a nil: entonces comentarios y chat contestan 503.
	Social *social.Servicio
	// Directo puede ir a nil: entonces se monta la implementación nula (503).
	Directo Directo
	// Diapositivas puede ir a nil: «Presentar con Cast» no está disponible.
	Diapositivas *diapositivas.Servicio
	Verifier     *authn.Verifier
	// Altcha puede ir a nil: entonces no hay denuncia sin sesión.
	Altcha *altcha.Verifier
	// Operador pregunta a Sytue ID por la cola de denuncias del operador. nil =
	// esa cola contesta 403.
	Operador      operador.Autorizar
	ReadyCheckers []httpx.Checker
}

// NewRouter construye el router completo de Sytue Cast.
func NewRouter(d Deps) http.Handler {
	if d.Directo == nil {
		d.Directo = sinDirecto{}
	}
	r := httpx.NewRouter(httpx.RouterOptions{
		// Sin timeout global: el WebSocket del chat vive horas y una subida de 8 GiB,
		// minutos. Las rutas REST llevan el suyo por grupo, abajo.
		// El tope de cuerpo es el del original (§1.6: 8 GiB) más un margen; las
		// rutas que esperan JSON siguen acotadas por DecodeJSON.
		MaxBodyBytes: domain.MaxOriginalBytes + (1 << 20),
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{svc: d.Servicio, social: d.Social, directo: d.Directo, diapositivas: d.Diapositivas, cfg: d.Cfg, verifier: d.Verifier, altcha: d.Altcha, operador: d.Operador}

	// Con sesión: cubo en memoria por persona (por réplica, como Notes).
	apiLimiter := httpx.NewTokenBucket(50, 100)
	// Sin sesión, el cubo va por IP y COMPARTIDO entre réplicas (pg.RateLimiter): la
	// portada y los enlaces públicos son lo único que puede visitar cualquiera.
	publicLimiter := pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 5, Burst: 40})
	// Los ficheros de /media van con su token firmado, así que el tope es más ancho:
	// un reproductor HLS pide un segmento cada pocos segundos por calidad.
	mediaLimiter := pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 20, Burst: 200})

	rest := func(next http.Handler) http.Handler { return http.TimeoutHandler(next, 30*time.Second, "") }

	// Lo público, DOS VECES con el mismo tope (como Photos): `/public` es por donde
	// pregunta la web (su BFF, dentro del clúster); `/api/v1/public` es lo mismo por
	// el ingress, que solo publica /api/v1, para las apps nativas.
	publico := func(pub chi.Router) {
		pub.Use(rest)
		pub.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: publicLimiter}))
		pub.Use(noIndexar)
		pub.Get("/explore", h.Explore)
		pub.Get("/c/{org}", h.Canal)
		pub.Get("/videos/{id}", h.VerPublico)
		pub.Get("/videos/{id}/play", h.ReproducirPublico)
		pub.Get("/videos/{id}/slides", h.DiapositivasPublico)
		pub.Get("/oembed", h.OEmbed)
		montarSocialPublico(pub, h)
		montarModeracionPublica(pub, h, d.Store.DB())
		montarDirecto(chi.NewRouter(), pub, chi.NewRouter(), d.Directo)
	}
	r.Route("/public", publico)

	r.Route("/api/v1", func(api chi.Router) {
		api.Route("/public", publico)

		api.Group(func(auth chi.Router) {
			auth.Use(d.Verifier.RequireAuth)
			auth.Use(d.Verifier.RequireTenant)
			auth.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

			// La subida del original va SIN el timeout de 30 s: dura lo que dure el fichero.
			auth.Put("/videos/{id}/original", h.SubirOriginal)

			auth.Group(func(rapido chi.Router) {
				rapido.Use(rest)
				rapido.Get("/videos", h.Biblioteca)
				// Literal ANTES de /videos/{id}: si no, «shared» sería un identificador.
				rapido.Get("/videos/shared", h.Compartidos)
				rapido.Post("/videos", h.Crear)
				// «Subir a Cast» desde Drive: GET comprueba y enseña, POST crea y copia.
				rapido.Get("/videos:from-drive", h.PrevisualizarDeDrive)
				rapido.Post("/videos:from-drive", h.SubirDeDrive)
				// Cuánto ocupará cada calidad antes de subir, y las de un vídeo ya creado.
				rapido.Get("/videos:estimate", h.Estimar)
				rapido.Get("/videos/{id}/qualities", h.Calidades)
				rapido.Put("/videos/{id}/qualities", h.ElegirCalidades)
				rapido.Get("/videos/{id}", h.Ver)
				rapido.Patch("/videos/{id}", h.Actualizar)
				rapido.Delete("/videos/{id}", h.Borrar)
				rapido.Get("/videos/{id}/play", h.Reproducir)
				// «Presentar con Cast»: quien presenta avisa de la diapositiva; quien mira la lee.
				rapido.Put("/lives/{id}/slide", h.AvanzarDiapositiva)
				rapido.Get("/videos/{id}/slides", h.Diapositivas)
				rapido.Put("/videos/{id}/poster", h.SubirPoster)
				rapido.Post("/videos/{id}/poster:frame", h.PosterDeFotograma)

				// Con quién se comparte (visibilidad `people`).
				rapido.Get("/videos/{id}/people", h.Personas)
				rapido.Post("/videos/{id}/people", h.Compartir)
				rapido.Delete("/videos/{id}/people/{user_id}", h.Descompartir)

				rapido.Get("/quota", h.Cuota)
				rapido.Get("/export", h.Export)

				montarSocial(rapido, h)
				montarModeracion(rapido, h)
				montarDirecto(rapido, chi.NewRouter(), chi.NewRouter(), d.Directo)
			})
		})
	})

	// Ficheros firmados: sin sesión (el token lo es), por IP, sin timeout (un
	// original de gigas se descarga a su ritmo).
	r.Group(func(m chi.Router) {
		m.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: mediaLimiter}))
		m.Get("/media/{id}/{token}/*", h.Media)
		// Los reproductores preguntan el tamaño con HEAD antes de pedir rangos.
		m.Head("/media/{id}/{token}/*", h.Media)
	})

	// El WebSocket del chat y el webhook de LiveKit, en la raíz. El handshake del
	// chat va por el cubo público por IP: abrir conexiones en bucle es barato para
	// quien lo hace y caro para el hub.
	r.Group(func(ws chi.Router) {
		ws.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: publicLimiter}))
		montarSocialRaiz(ws, h)
	})
	montarDirecto(chi.NewRouter(), chi.NewRouter(), r, d.Directo)

	// De quién se cree el X-Forwarded-For (TRUSTED_PROXIES). Envuelve el router ya
	// construido —chi no admite Use tras registrar rutas—, como en Analytics: así le
	// llega a todo, también a los topes por IP y al registro de peticiones.
	return conProxies{Router: r, de: d.Cfg.TrustedProxies}
}

// conProxies deja en cada petición los proxies de confianza. Sigue siendo un
// chi.Router para quien recorre las rutas (el test del Ingress).
type conProxies struct {
	chi.Router
	de clientip.TrustedProxies
}

func (c conProxies) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c.Router.ServeHTTP(w, req.WithContext(clientip.WithTrustedProxies(req.Context(), c.de)))
}

// noIndexar: lo público se sirve a la web, que decide qué indexa; la API en sí no.
func noIndexar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r)
	})
}
