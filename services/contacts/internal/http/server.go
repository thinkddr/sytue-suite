// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package http implementa la superficie HTTP de Sytue Contacts: API REST propia
// (libretas, contactos, grupos, búsqueda) y la exportación/importación vCard. El
// servidor CardDAV se monta en internal/carddav bajo /dav.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS). Aislamiento por tenant vía
// RLS (el tenant se inyecta en el contexto desde los claims).
package contactshttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	carddavsrv "git.sytue.eu/sytue/one/services/contacts/internal/carddav"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// Deps son las dependencias del router HTTP de Contacts.
type Deps struct {
	Cfg           config.Config
	Store         *store.Store
	Verifier      *authn.Verifier
	CardDAV       http.Handler
	ReadyCheckers []httpx.Checker
}

// Registrar los verbos WebDAV en chi ANTES de montar el router CardDAV
// (si no, PROPFIND/REPORT caen en 405). Idempotente.
func init() { httpx.EnableWebDAVMethods() }

// NewRouter construye el router completo de Sytue Contacts.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 30 * time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{store: d.Store, cfg: d.Cfg}
	tenantAuth := func(next http.Handler) http.Handler {
		return d.Verifier.RequireAuth(d.Verifier.RequireTenant(next))
	}
	// ponytail: cubo en memoria, o sea POR RÉPLICA (el tope real se multiplica por
	// el número de instancias). Si hiciera falta que fuera del servicio entero,
	// pg.RateLimiter ya lo comparte por Postgres como hace Sytue ID.
	apiLimiter := httpx.NewTokenBucket(50, 100)

	// API REST autenticada bajo /api/v1.
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		// Tope por IDENTIDAD del token, DESPUÉS de RequireAuth (antes no hay sujeto
		// que leer). No por IP: detrás del ingress todas las peticiones llegan con
		// la misma, así que un cubo por IP sería un cubo para todos los clientes.
		// Alto a propósito: corta el barrido del directorio entero, no el uso normal.
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// «Exportar tus datos»: la entrega de la agenda (platform/pkg/portabilidad).
		api.Get("/export", h.Export)

		// Libretas de direcciones (colecciones CardDAV).
		api.Get("/address-books", h.ListAddressBooks)
		api.Post("/address-books", h.CreateAddressBook)
		api.Get("/address-books/{id}", h.GetAddressBook)
		api.Patch("/address-books/{id}", h.UpdateAddressBook)
		api.Delete("/address-books/{id}", h.DeleteAddressBook)

		// Contactos dentro de una libreta.
		api.Get("/address-books/{id}/contacts", h.ListContacts)
		api.Get("/address-books/{id}/contacts/page", h.ListContactsPage)
		api.Post("/address-books/{id}/contacts", h.CreateContact)
		// Importación CSV (Google Contacts / Outlook) a una libreta.
		api.Post("/address-books/{id}/import-csv", h.ImportCSV)

		// Acciones EN LOTE (multiseleccion), por UID, owner-scoped. Antes de
		// /contacts/{id} para que "batch"/"photo" no se tomen por un {id}.
		api.Post("/contacts/batch/trash", h.BatchTrash)
		api.Post("/contacts/batch/favorite", h.BatchFavorite)

		// Listado paginado de TODOS los contactos del usuario (?limit=&offset=).
		// Existe para poder LLEVARSE los contactos: la búsqueda exige `q` y el
		// listado por libreta corta en 500, así que sin esto una exportación se
		// quedaba corta en silencio, que es la peor forma de quedarse corta.
		api.Get("/contacts/all", h.ListAllContacts)

		// Foto de contacto (PHOTO embebida): servir/definir/quitar por ?uid=.
		api.Get("/contacts/photo", h.GetPhoto)
		api.Put("/contacts/photo", h.SetPhoto)
		api.Delete("/contacts/photo", h.RemovePhoto)

		// Contactos por ID (independiente de la libreta en la ruta).
		api.Get("/contacts/{id}", h.GetContact)
		api.Put("/contacts/{id}", h.UpdateContact)
		api.Delete("/contacts/{id}", h.DeleteContact)
		api.Get("/contacts/{id}/vcard", h.ExportContactVCard)
		// Papelera: restaurar un contacto borrado.
		api.Post("/contacts/{id}/restore", h.RestoreContact)

		// Búsqueda de contactos del usuario (owner-scoped en el store).
		api.Get("/contacts", h.SearchContacts)

		// Favoritos (destacados) del usuario.
		api.Get("/favorites", h.ListFavorites)
		api.Post("/favorites", h.ToggleFavorite)

		// Papelera del usuario (soft-delete existente): listar + eliminar definitivo.
		api.Get("/trash", h.ListTrash)
		api.Delete("/trash/{id}", h.PurgeContact)

		// Exportación CSV de todos los contactos del usuario.
		api.Get("/export-csv", h.ExportCSV)

		// Grupos.
		api.Get("/address-books/{id}/groups", h.ListGroups)
		api.Post("/address-books/{id}/groups", h.CreateGroup)
		api.Get("/groups/{id}", h.GetGroup)
		api.Put("/groups/{id}", h.UpdateGroup)
		api.Delete("/groups/{id}", h.DeleteGroup)
		// Pertenencia EN LOTE: añadir/quitar varios contactos (por UID) de un grupo.
		api.Post("/groups/{id}/members", h.BatchGroupMembers)
	})

	// Capa de compatibilidad ENTRANTE: Google People API y Microsoft Graph sobre el
	// MISMO store owner-scoped y el MISMO middleware de auth que /api/v1 (ver
	// handlers_compat.go). Prefijos /compat/google y /compat/graph.
	mountCompat(r, &compatHandlers{store: d.Store, cfg: d.Cfg}, func(next http.Handler) http.Handler {
		return d.Verifier.RequireGoogleAPIs(googleScopesLectura, googleScopesEscritura)(d.Verifier.RequireTenant(next))
	}, tenantAuth)

	// Endpoint INTERNO service-to-service (ver internal.go): Sytue ID refleja
	// aquí el avatar propio en las fichas de contacto que coincidan por correo.
	// Fuera de /api/v1: no hereda tenantAuth (el tenant viaja en el cuerpo).
	// Sin secreto configurado, fail-closed: la ruta no se monta.
	if d.Cfg.InternalSecret != "" {
		r.With(func(next http.Handler) http.Handler {
			return internalAuth(d.Cfg.InternalSecret, next)
		}).Post("/internal/avatar-sync", h.AvatarSync)
	}

	// CardDAV (RFC 6352) bajo /dav, autenticado con el mismo JWT.
	if d.CardDAV != nil {
		r.Mount("/dav", tenantAuth(carddavsrv.WithTenant(d.CardDAV)))
		// Descubrimiento de servicio (RFC 6764): redirige /.well-known/carddav.
		r.Get("/.well-known/carddav", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/dav/", http.StatusMovedPermanently)
		})
	}

	return r
}
