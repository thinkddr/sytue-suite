// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/builder"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ListTemplates expone el catálogo curado de plantillas de arranque (presets de
// configuración por stack). Es catálogo público del producto —no datos de
// tenant, no toca Postgres, sin RLS—, lectura para cualquier miembro
// autenticado (como /billing/pricing).
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, r, http.StatusOK, map[string]any{"templates": domain.StarterTemplates})
}

// ListAppCatalog expone el catálogo de apps open-source (ADR 0029): qué se puede
// levantar junto a la app del cliente.
//
// Devuelve DOS listas. `apps` son las que se pueden crear ahora mismo; `pending`
// son las que están escritas y revisadas pero cuya imagen todavía no se ha
// replicado en nuestro registro. Enseñar las segundas es honesto —«esto viene»—
// y esconderlas haría que nadie replicase nunca la imagen, porque nadie las
// echaría de menos. Lo que NO se hace es ofrecerlas: crearlas fallaría al tirar
// de la imagen, después de haber prometido el servicio.
//
// Catálogo del producto, como las plantillas: no toca Postgres ni lleva datos de
// ningún inquilino.
func (h *Handlers) ListAppCatalog(w http.ResponseWriter, r *http.Request) {
	cat, err := h.svc.AppCatalog()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	apps := cat.Apps
	if apps == nil {
		apps = []domain.CatalogApp{}
	}
	pending := cat.PendingMirror
	if pending == nil {
		pending = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"apps":    apps,
		"pending": pending,
		// Las licencias que el catálogo admite: el panel las enseña para que quien
		// despliegue software de otro sepa bajo qué términos lo hace.
		"allowed_licenses": domain.AllowedAppLicenses,
	})
}

// ListInstanceSizes expone el catálogo de tamaños de instancia. Mismo carácter que
// las plantillas: catálogo del producto, sin RLS ni Postgres.
//
// Lo sirve la API en vez de que cada superficie (dashboard, CLI, docs) lleve su
// copia: una copia se queda vieja en cuanto se añade un tamaño o cambia la memoria
// de uno, y entonces la interfaz ofrece opciones que el backend rechaza —o esconde
// las que sí acepta. El único sitio donde se decide sigue siendo domain.
func (h *Handlers) ListInstanceSizes(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"instance_sizes": domain.InstanceSizes,
		"default":        domain.DefaultInstanceSizeName,
		"max_count":      domain.MaxInstanceCount,
	})
}

// ProbeHealthPaths sondea la app del proyecto con las rutas de salud de convención
// y devuelve qué contestó cada una. No cambia nada: la ruta elegida se guarda con
// el PATCH normal del proyecto, así que esto solo INFORMA.
func (h *Handlers) ProbeHealthPaths(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	report, err := h.svc.ProbeHealthPaths(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, report)
}

// SuggestProjectSlug propone un nombre libre para un proyecto nuevo, en el idioma
// negociado. Solo sugiere: no reserva nada, así que dos personas pueden recibir el
// mismo nombre a la vez y la segunda chocará al crear. Reservarlo exigiría un
// apartado con caducidad para un problema que en la práctica no existe (el catálogo
// da miles de combinaciones por idioma).
func (h *Handlers) SuggestProjectSlug(w http.ResponseWriter, r *http.Request) {
	lang := requestLanguage(r)
	slug, available, err := h.svc.SuggestProjectSlug(r.Context(), lang)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	noStore(w)
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"slug": slug, "available": available, "lang": lang,
	})
}

// CheckSlug responde si el nombre que ha escrito el usuario está libre y, si no,
// con qué alternativas. Es lectura pura (GET): no reserva nada.
func (h *Handlers) CheckSlug(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.CheckSlug(r.Context(), r.URL.Query().Get("slug"), requestLanguage(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	noStore(w)
	httpx.JSON(w, r, http.StatusOK, res)
}

// requestLanguage negocia el idioma de la petición: el parámetro `lang` si viene,
// si no la cabecera Accept-Language, y si no el base. La regla vive en domain
// —donde está testeada— y aquí solo se le pasan las dos entradas.
func requestLanguage(r *http.Request) string {
	return domain.NegotiateLanguage(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
}

// noStore impide que una caché por el camino conserve una respuesta que caduca en
// cuanto alguien crea un proyecto: un «libre» viejo lleva justo al 409 que esto
// viene a evitar, y una sugerencia cacheada sería siempre el mismo nombre.
func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

// InspectRepo mira un repositorio público y devuelve cómo desplegarlo, para que
// el alta no tenga que preguntarlo.
//
// # Por qué esto es un endpoint y no una casilla más
//
// El formulario de alta pedía framework, estrategia, comando de instalación,
// comando de build y directorio de salida. Las cinco respuestas están en el
// repositorio que el usuario acaba de pegar, así que preguntárselas es pedirle
// que traduzca su propio proyecto a nuestro vocabulario. Sobre todo `output_dir`,
// que para quien «sabe lo justo de programación» no es una pregunta: es un muro.
//
// # Lo que NO hace
//
// No falla el alta. Una forja que no se sabe leer, un repositorio privado o un
// servidor caído devuelven `detected:false` y el formulario enseña los campos,
// que es como funcionaba antes. Convertir «no pude mirar» en un error de creación
// habría cambiado una molestia por un bloqueo.
//
// La petición sale hacia un host que escribe el usuario, así que va acotada por
// DOS puertas independientes: una allowlist de forjas conocidas (builder.forgeHosts)
// y el guardián de SSRF, que no alcanza redes privadas ni el metadata del
// proveedor aunque el DNS de un host permitido apunte allí.
func (h *Handlers) InspectRepo(w http.ResponseWriter, r *http.Request) {
	repoURL := strings.TrimSpace(r.URL.Query().Get("repo_url"))
	if repoURL == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	det, err := builder.InspectRepo(r.Context(), repoURL, r.URL.Query().Get("root_dir"), "")
	if err != nil {
		// El motivo viaja para poder decirlo en la interfaz —«ese repositorio es
		// privado» y «esa forja no la sabemos leer» se arreglan de forma distinta—,
		// pero la respuesta es 200: no se ha pedido crear nada.
		httpx.JSON(w, r, http.StatusOK, map[string]any{"detected": false, "reason": err.Error()})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"detected": true, "detection": det})
}
