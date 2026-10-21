// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/contacts/internal/compat"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// Capa de compatibilidad de contactos: Sytue Contacts respondiendo un subconjunto
// PRAGMÁTICO de la Google People API v1 (/compat/google) y de Microsoft Graph
// contacts (/compat/graph), para que una integración escrita contra Google o
// Microsoft —con base URL configurable— funcione sin cambios. La traducción de
// modelos vive en internal/compat (pura y testeable); aquí solo hay handlers finos.
//
// AISLAMIENTO (crítico): estos handlers usan EXACTAMENTE el mismo store
// owner-scoped que la API nativa, con el sub del JWT de la petición. Nunca hacen
// consultas por su cuenta, así que el tenant (RLS) y el dueño se aplican igual, y
// un contacto ajeno o inexistente responden lo mismo: 404. La autenticación es el
// Bearer de Sytue ID (mismo middleware): no se emula ningún flujo OAuth.
//
// Lo que NO está implementado responde 501 con mensaje claro (nunca un 200 vacío):
// escritura (createContact/updateContact/deletePerson), otherContacts, searchContacts,
// contactGroups, people.getBatchGet, sincronización por syncToken, fotos binarias,
// y en Graph las carpetas de contactos, delta, escritura y /users/{id}/….

// Tamaños de página de la emulación: por defecto y techo de cada dialecto (el
// techo deja margen bajo el límite del store, que sirve como mucho 500 filas).
const (
	googleDefaultPage = 100
	googleMaxPage     = 250
	graphDefaultPage  = 100
	graphMaxPage      = 250
	graphMaxSkip      = 100000
)

// compatHandlers sirve ambos dialectos sobre el mismo store que la API nativa.
type compatHandlers struct {
	store contactStore
	cfg   config.Config
}

// Scopes de la API People de Google que abren /compat/google a una app de terceros registrada con
// permisos de Google (cuenta de Google emulada, ver authn.RequireGoogleAPIs).
var (
	googleScopesLectura   = []string{googleScope + "contacts.readonly"}
	googleScopesEscritura = []string{googleScope + "contacts"}
)

const googleScope = "https://www.googleapis.com/auth/"

// mountCompat monta las dos capas. /compat/google lleva su propia verja
// (googleAuth), que admite además los tokens de apps de terceros con scopes de
// Google; /compat/graph sigue solo con la de la suite.
func mountCompat(r chi.Router, h *compatHandlers, googleAuth, auth func(http.Handler) http.Handler) {
	r.Route("/compat/google", func(g chi.Router) {
		g.Use(googleAuth)
		g.Get("/v1/people/me/connections", h.googleConnections)
		g.Get("/v1/people/{personId}", h.googlePerson)
		g.Post("/v1/people:createContact", h.googleCreateContact)
		// `people/{id}:updateContact` y `:deleteContact`: la acción va pegada al id en
		// el mismo tramo de la ruta, así que se separa en el manejador.
		g.Patch("/v1/people/{personId}", h.googlePersonAction)
		g.Delete("/v1/people/{personId}", h.googlePersonAction)
		g.NotFound(h.googleNotImplemented)
		g.MethodNotAllowed(h.googleNotImplemented)
	})

	r.Route("/compat/graph", func(g chi.Router) {
		g.Use(auth)
		g.Get("/v1.0/me/contacts", h.graphContacts)
		g.Get("/v1.0/me/contacts/{id}", h.graphContact)
		g.NotFound(h.graphNotImplemented)
		g.MethodNotAllowed(h.graphNotImplemented)
	})
}

// ---------------------------------------------------------------------------
// Respuestas de error en cada dialecto
// ---------------------------------------------------------------------------

// failGoogle responde el sobre de error de Google con el estado que la API nativa
// habría devuelto para ese mismo error de dominio.
func failGoogle(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGoogleError(e.Status, e.Message))
}

// failGraph responde el sobre de error de Microsoft Graph.
func failGraph(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGraphError(e.Status, e.Message))
}

// compatError normaliza el error a *httpx.Error (los de dominio, con el MISMO
// mapeo de códigos que la API nativa; los de traducción, a 400 con el término
// exacto) y registra los 5xx sin filtrar la causa al cliente.
func compatError(r *http.Request, err error) *httpx.Error {
	if errors.Is(err, compat.ErrUnsupportedParam) || errors.Is(err, compat.ErrInvalidPageToken) {
		return httpx.ErrBadRequest.WithMessage(err.Error())
	}
	// Los errores que ya son de API (400/501 de la propia emulación) se respetan;
	// los de dominio pasan por el MISMO mapeo que la API nativa.
	var e *httpx.Error
	if !errors.As(err, &e) {
		e = httpx.AsError(toAPIError(err))
	}
	if e.Status >= http.StatusInternalServerError {
		log.From(r.Context()).Error("compat: error de servidor", "code", e.Code, "error", e.Error())
	}
	return e
}

// errNotImplemented describe un endpoint del dialecto que no se emula.
func errNotImplemented(dialect string) error {
	return httpx.NewError(http.StatusNotImplemented, "not_implemented",
		"Sytue Contacts emula un subconjunto de "+dialect+"; este endpoint no está implementado.")
}

func (h *compatHandlers) googleNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGoogle(w, r, errNotImplemented("la Google People API v1"))
}

func (h *compatHandlers) graphNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGraph(w, r, errNotImplemented("Microsoft Graph (contacts)"))
}

// ---------------------------------------------------------------------------
// Lectura compartida por los dos dialectos
// ---------------------------------------------------------------------------

// contactsPage lee una página de los contactos del usuario pidiendo UNA fila de
// más: así se sabe si hay página siguiente sin contar el total (que a escala sería
// una consulta cara y, peor, un número que envejece entre páginas).
func (h *compatHandlers) contactsPage(ctx context.Context, sub string, offset, size int) (page []domain.Contact, next int, err error) {
	rows, err := h.store.ListOwnerContactsPage(ctx, sub, size+1, offset)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) > size {
		return rows[:size], offset + size, nil
	}
	return rows, 0, nil
}

// compatContact resuelve un contacto por id a través del store owner-scoped: un id
// ilegible, ajeno o inexistente responden lo mismo (404).
func (h *compatHandlers) compatContact(r *http.Request, id string) (domain.Contact, error) {
	cid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return domain.Contact{}, domain.ErrNotFound
	}
	return h.store.GetContact(r.Context(), subFromCtx(r), cid)
}

// ---------------------------------------------------------------------------
// Google People API v1 — /compat/google
// ---------------------------------------------------------------------------

// googleConnections responde people.connections.list: TODOS los contactos del
// usuario (todas sus libretas), paginados por `pageSize`/`pageToken`. `sources`
// solo admite READ_SOURCE_TYPE_CONTACT —lo único que Sytue guarda—: pedir el perfil
// del directorio devolvería una lista incompleta sin decirlo, así que es 400.
// `personFields` se ignora a propósito (siempre se devuelve el contacto completo).
func (h *compatHandlers) googleConnections(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "pageSize", "pageToken", "sources"); err != nil {
		failGoogle(w, r, err)
		return
	}
	for _, s := range q["sources"] {
		if s != "" && s != "READ_SOURCE_TYPE_CONTACT" {
			failGoogle(w, r, httpx.ErrBadRequest.WithMessage(
				"sources only accepts READ_SOURCE_TYPE_CONTACT: Sytue does not expose directory profiles through this endpoint.",
			))
			return
		}
	}
	offset, err := compat.DecodePageToken(q.Get("pageToken"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}

	size := compat.PageSize(q.Get("pageSize"), googleDefaultPage, googleMaxPage)
	contacts, next, err := h.contactsPage(r.Context(), subFromCtx(r), offset, size)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	people := make([]compat.Person, 0, len(contacts))
	for _, c := range contacts {
		people = append(people, compat.PersonFromContact(c))
	}
	token := ""
	if next > 0 {
		token = compat.EncodePageToken(next)
	}
	httpx.JSON(w, r, http.StatusOK, compat.NewConnections(people, token))
}

// googlePerson responde people.get de un contacto propio (`people/{id}`, con el
// MISMO id que la API nativa). El alias `me` es la persona autenticada, que no es
// un contacto de la libreta: se responde 501 en vez de fabricar un perfil.
func (h *compatHandlers) googlePerson(w http.ResponseWriter, r *http.Request) {
	if err := compat.CheckParams(r.URL.Query()); err != nil {
		failGoogle(w, r, err)
		return
	}
	id := compat.SplitResourceName(chi.URLParam(r, "personId"))
	if id == compat.MeAlias {
		failGoogle(w, r, httpx.NewError(http.StatusNotImplemented, "not_implemented",
			"El perfil del propio usuario (people/me) no está implementado; usa people/me/connections o el id de un contacto."))
		return
	}
	c, err := h.compatContact(r, id)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.PersonFromContact(c))
}

// googleCreateContact responde people.createContact en la libreta por defecto del
// usuario, por el MISMO camino que la API nativa (vCard canónica, proyección, ctag).
func (h *compatHandlers) googleCreateContact(w http.ResponseWriter, r *http.Request) {
	if err := compat.CheckParams(r.URL.Query(), "personFields"); err != nil {
		failGoogle(w, r, err)
		return
	}
	var p compat.Person
	if err := decodePeopleJSON(w, r, &p); err != nil {
		failGoogle(w, r, err)
		return
	}
	c, err := p.ApplyTo(domain.Contact{}, compat.PersonWritableFields)
	if err != nil {
		failGoogle(w, r, personError(err))
		return
	}
	sub := subFromCtx(r)
	libreta, err := h.store.EnsureDefaultAddressBook(r.Context(), sub)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	c.UID = domain.NewUID()
	card, err := domain.EncodeContact(c, "")
	if err != nil {
		failGoogle(w, r, personError(err))
		return
	}
	saved, err := h.store.CreateContact(r.Context(), sub, store.UpsertContact{
		AddressBookID: libreta.ID, Contact: c, CardData: card, ResourceName: domain.ResourceName(c.UID),
	})
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.PersonFromContact(saved))
}

// googlePersonAction despacha `people/{id}:updateContact` (PATCH) y
// `people/{id}:deleteContact` (DELETE).
func (h *compatHandlers) googlePersonAction(w http.ResponseWriter, r *http.Request) {
	id, accion, _ := strings.Cut(compat.SplitResourceName(chi.URLParam(r, "personId")), ":")
	switch {
	case r.Method == http.MethodPatch && accion == "updateContact":
		h.googleUpdateContact(w, r, id)
	case r.Method == http.MethodDelete && accion == "deleteContact":
		c, err := h.compatContact(r, id)
		if err == nil {
			err = h.store.DeleteContact(r.Context(), subFromCtx(r), c.ID)
		}
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{})
	default:
		h.googleNotImplemented(w, r)
	}
}

// googleUpdateContact cambia SOLO los campos de updatePersonFields (obligatorio, como
// en Google). El `etag` del cuerpo, si viene, es la precondición: si el contacto
// cambió entretanto, 412 en vez de pisar el cambio de otro.
func (h *compatHandlers) googleUpdateContact(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "updatePersonFields", "personFields"); err != nil {
		failGoogle(w, r, err)
		return
	}
	var campos []string
	for _, f := range strings.Split(q.Get("updatePersonFields"), ",") {
		if f = strings.TrimSpace(f); f != "" {
			campos = append(campos, f)
		}
	}
	if len(campos) == 0 {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("updatePersonFields is required."))
		return
	}
	var p compat.Person
	if err := decodePeopleJSON(w, r, &p); err != nil {
		failGoogle(w, r, err)
		return
	}
	actual, err := h.compatContact(r, id)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	c, err := p.ApplyTo(actual, campos)
	if err != nil {
		failGoogle(w, r, personError(err))
		return
	}
	var siCoincide *uuid.UUID
	if p.ETag != "" {
		etag, err := uuid.Parse(p.ETag)
		if err != nil {
			failGoogle(w, r, httpx.NewError(http.StatusPreconditionFailed, "precondition_failed", "The etag does not match the current contact."))
			return
		}
		siCoincide = &etag
	}
	base, _ := h.store.GetCardObjectByResource(r.Context(), actual.AddressBookID, domain.ResourceName(actual.UID))
	card, err := domain.EncodeContact(c, base.Data)
	if err != nil {
		failGoogle(w, r, personError(err))
		return
	}
	saved, err := h.store.UpdateContact(r.Context(), subFromCtx(r), actual.ID, store.UpsertContact{
		AddressBookID: actual.AddressBookID, Contact: c, CardData: card, ResourceName: base.ResourceName,
	}, siCoincide)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.PersonFromContact(saved))
}

// decodePeopleJSON decodifica una persona rechazando lo que la emulación no escribe,
// por su nombre: un contacto sin sus direcciones es peor que un 400.
func decodePeopleJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	err := httpx.DecodeJSON(w, r, dst)
	if err == nil {
		return nil
	}
	if _, field, ok := strings.Cut(err.Error(), "unknown field "); ok {
		return httpx.ErrBadRequest.WithMessage("The body carries a field the emulation does not write: " +
			field + ". Remove it or use the native /api/v1 API.")
	}
	return err
}

// personError pasa los errores de traducción a 400 con su explicación.
func personError(err error) error {
	switch {
	case errors.Is(err, compat.ErrUnsupportedPersonField):
		return httpx.ErrBadRequest.WithMessage(err.Error() + " (writable: names, emailAddresses, phoneNumbers, organizations)")
	case errors.Is(err, domain.ErrInvalidContact):
		return httpx.ErrBadRequest.WithMessage("The person needs a name, or at least an organization.")
	}
	return err
}

// ---------------------------------------------------------------------------
// Microsoft Graph (contacts) — /compat/graph
// ---------------------------------------------------------------------------

// graphContacts responde GET /v1.0/me/contacts con paginación OData ($top/$skip) y
// @odata.nextLink absoluto. $filter/$orderby/$search/$count no están implementados:
// 400 nombrándolos, en vez de devolver la lista entera como si se hubieran aplicado.
func (h *compatHandlers) graphContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "$top", "$skip"); err != nil {
		failGraph(w, r, err)
		return
	}
	top := compat.PageSize(q.Get("$top"), graphDefaultPage, graphMaxPage)
	skip := compat.PageSize(q.Get("$skip"), 0, graphMaxSkip)

	contacts, next, err := h.contactsPage(r.Context(), subFromCtx(r), skip, top)
	if err != nil {
		failGraph(w, r, err)
		return
	}
	out := make([]compat.GraphContact, 0, len(contacts))
	for _, c := range contacts {
		out = append(out, compat.GraphFromContact(c))
	}
	link := compat.NextLink(h.cfg.PublicURL, r.URL.Path, top, next)
	httpx.JSON(w, r, http.StatusOK, compat.NewGraphCollection(out, link))
}

// graphContact responde GET /v1.0/me/contacts/{id}.
func (h *compatHandlers) graphContact(w http.ResponseWriter, r *http.Request) {
	if err := compat.CheckParams(r.URL.Query()); err != nil {
		failGraph(w, r, err)
		return
	}
	c, err := h.compatContact(r, chi.URLParam(r, "id"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GraphFromContact(c))
}
