// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la capa de compatibilidad de contactos (Google People API y Microsoft
// Graph) con el fake del store: sin PostgreSQL. Cubren la paginación en los dos
// formatos, los parámetros no soportados, los 501 honestos y el AISLAMIENTO (un
// usuario no ve por la vía compat lo que no vería por la nativa).
package contactshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// --- fixtures y helpers ------------------------------------------------------

var compatCreated = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)

func compatContact(name string) domain.Contact {
	return domain.Contact{
		ID: uuid.New(), UID: "uid-" + name, FormattedName: name, GivenName: name,
		Organization: "Sytue", Title: "Ingeniera",
		Emails: []domain.Email{{Email: strings.ToLower(name) + "@ex.com", Type: domain.EmailWork, IsPrimary: true}},
		Phones: []domain.Phone{{Phone: "+34600000000", Type: domain.PhoneCell, IsPrimary: true}},
		ETag:   uuid.New(), CreatedAt: compatCreated, UpdatedAt: compatCreated,
	}
}

// compatRouter monta los dos dialectos con un middleware de auth de mentira que
// inyecta los claims del usuario dado (en producción es Verifier.RequireAuth).
func compatRouter(st contactStore, sub string) http.Handler {
	r := chi.NewRouter()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims := authn.Claims{Subject: sub, Tenant: "acme", Email: sub + "@ex.com"}
			next.ServeHTTP(w, req.WithContext(authn.Into(req.Context(), claims)))
		})
	}
	mountCompat(r, &compatHandlers{store: st, cfg: config.Config{PublicURL: "https://contacts.sytue.eu"}}, auth, auth)
	return r
}

func compatDo(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, http.NoBody))
	return rec
}

func compatBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

func googleReasonOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	e, ok := compatBody(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin sobre de error de Google: %s", rec.Body.String())
	}
	list, _ := e["errors"].([]any)
	if len(list) == 0 {
		t.Fatalf("sobre de Google sin errors[]: %s", rec.Body.String())
	}
	item, _ := list[0].(map[string]any)
	reason, _ := item["reason"].(string)
	return reason
}

func graphCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	e, ok := compatBody(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin sobre de error de Graph: %s", rec.Body.String())
	}
	code, _ := e["code"].(string)
	return code
}

func compatItems(t *testing.T, rec *httptest.ResponseRecorder, key string) []map[string]any {
	t.Helper()
	list, _ := compatBody(t, rec)[key].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		m, _ := it.(map[string]any)
		out = append(out, m)
	}
	return out
}

// --- Google People API -------------------------------------------------------

func TestCompatGoogleConnections(t *testing.T) {
	ada := compatContact("Ada")
	f := &fakeStore{contacts: []domain.Contact{ada, compatContact("Bob")}}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections?personFields="+url.QueryEscape("names,emailAddresses"))
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	got := compatItems(t, rec, "connections")
	if len(got) != 2 {
		t.Fatalf("conexiones = %d", len(got))
	}
	if got[0]["resourceName"] != "people/"+ada.ID.String() {
		t.Fatalf("resourceName = %v", got[0]["resourceName"])
	}
	// personFields se ignora: el recurso viaja completo (nombres + correos + org).
	if _, ok := got[0]["organizations"]; !ok {
		t.Fatalf("el recurso debe ir completo: %+v", got[0])
	}
	if f.gotPageSubject != "user-1" {
		t.Fatalf("el listado debe ir owner-scoped al sub del JWT, fue a %q", f.gotPageSubject)
	}
}

func TestCompatGoogleConnectionsPagination(t *testing.T) {
	f := &fakeStore{contacts: []domain.Contact{compatContact("Ada"), compatContact("Bob")}}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections?pageSize=1")
	token, _ := compatBody(t, rec)["nextPageToken"].(string)
	if token == "" {
		t.Fatalf("falta nextPageToken: %s", rec.Body.String())
	}
	if got := compatItems(t, rec, "connections"); len(got) != 1 {
		t.Fatalf("primera página: %+v", got)
	}
	// Pide una fila de más para saber si hay siguiente, sin contar el total.
	if f.gotPageLimit != 2 || f.gotPageOffset != 0 {
		t.Fatalf("limit/offset de la primera página = %d/%d", f.gotPageLimit, f.gotPageOffset)
	}

	rec = compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections?pageSize=1&pageToken="+url.QueryEscape(token))
	if body := compatBody(t, rec); body["nextPageToken"] != nil {
		t.Fatalf("la última página no debe traer token: %s", rec.Body.String())
	}
	if f.gotPageOffset != 1 {
		t.Fatalf("offset de la segunda página = %d", f.gotPageOffset)
	}

	rec = compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections?pageToken=basura")
	if rec.Code != http.StatusBadRequest || googleReasonOf(t, rec) != "badRequest" {
		t.Fatalf("token ajeno: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompatGoogleConnectionsUnsupported(t *testing.T) {
	h := compatRouter(&fakeStore{}, "user-1")
	tests := []struct{ name, query string }{
		{name: "orden no emulado", query: "?sortOrder=LAST_MODIFIED_DESCENDING"},
		{name: "sincronización incremental", query: "?requestSyncToken=true"},
		{name: "fuente que no guardamos", query: "?sources=READ_SOURCE_TYPE_PROFILE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections"+tc.query)
			if rec.Code != http.StatusBadRequest || googleReasonOf(t, rec) != "badRequest" {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
	rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/me/connections?sources=READ_SOURCE_TYPE_CONTACT")
	if rec.Code != http.StatusOK {
		t.Fatalf("la fuente que sí guardamos debe aceptarse: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompatGooglePerson(t *testing.T) {
	ada := compatContact("Ada")
	f := &fakeStore{contact: ada}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/"+ada.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := compatBody(t, rec); body["resourceName"] != "people/"+ada.ID.String() {
		t.Fatalf("persona: %+v", body)
	}

	// Un id ilegible es 404 (indistinguible de «no existe»), no 400.
	if rec := compatDo(h, http.MethodGet, "/compat/google/v1/people/no-existe"); rec.Code != http.StatusNotFound {
		t.Fatalf("id ilegible: %d (%s)", rec.Code, rec.Body.String())
	}

	f.contactErr = domain.ErrNotFound
	rec = compatDo(h, http.MethodGet, "/compat/google/v1/people/"+uuid.NewString())
	if rec.Code != http.StatusNotFound || googleReasonOf(t, rec) != "notFound" {
		t.Fatalf("no encontrado: %d (%s)", rec.Code, rec.Body.String())
	}

	// people/me es el perfil del usuario, no un contacto: 501 explícito.
	rec = compatDo(h, http.MethodGet, "/compat/google/v1/people/me")
	if rec.Code != http.StatusNotImplemented || googleReasonOf(t, rec) != "notImplemented" {
		t.Fatalf("people/me: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompatGoogleNotImplemented(t *testing.T) {
	h := compatRouter(&fakeStore{}, "user-1")
	rec := compatDo(h, http.MethodGet, "/compat/google/v1/otherContacts")
	if rec.Code != http.StatusNotImplemented || googleReasonOf(t, rec) != "notImplemented" {
		t.Fatalf("otherContacts: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = compatDo(h, http.MethodPost, "/compat/google/v1/people:batchCreateContacts")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("escritura por lotes no emulada: %d (%s)", rec.Code, rec.Body.String())
	}
}

// createContact entra en la libreta por defecto con una vCard de verdad;
// updateContact cambia SOLO los campos pedidos y respeta el etag; deleteContact borra.
func TestCompatGoogleWriteContacts(t *testing.T) {
	libreta := domain.AddressBook{ID: uuid.New()}
	ada := compatContact("Ada")
	f := &fakeStore{book: libreta, contact: ada}
	h := compatRouter(f, "user-1")
	pedir := func(method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	rec := pedir(http.MethodPost, "/compat/google/v1/people:createContact",
		`{"names":[{"givenName":"Grace","familyName":"Hopper"}],"emailAddresses":[{"value":"grace@navy.mil","type":"work"}],`+
			`"phoneNumbers":[{"value":"+1 555 0100","type":"mobile"}],"organizations":[{"name":"US Navy","title":"Rear Admiral"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("createContact: %d (%s)", rec.Code, rec.Body.String())
	}
	nueva := f.gotUpsert
	if nueva.AddressBookID != libreta.ID || nueva.Contact.FormattedName != "Grace Hopper" || nueva.Contact.Organization != "US Navy" ||
		len(nueva.Contact.Phones) != 1 || nueva.Contact.Phones[0].Type != domain.PhoneCell || nueva.Contact.UID == "" ||
		!strings.Contains(nueva.CardData, "BEGIN:VCARD") {
		t.Fatalf("contacto creado: %+v", nueva)
	}

	rec = pedir(http.MethodPatch, "/compat/google/v1/people/"+ada.ID.String()+":updateContact?updatePersonFields=organizations",
		`{"etag":"`+ada.ETag.String()+`","organizations":[{"name":"Analytical Engines"}],"names":[{"givenName":"Ignorado"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("updateContact: %d (%s)", rec.Code, rec.Body.String())
	}
	if c := f.gotUpsert.Contact; c.Organization != "Analytical Engines" || c.GivenName != "Ada" || c.UID != ada.UID || len(c.Emails) != 1 {
		t.Fatalf("update toca solo organizations: %+v", c)
	}

	for nombre, c := range map[string]struct{ method, target, body string }{
		"sin updatePersonFields":    {http.MethodPatch, "/compat/google/v1/people/" + ada.ID.String() + ":updateContact", `{}`},
		"campo que no se escribe":   {http.MethodPatch, "/compat/google/v1/people/" + ada.ID.String() + ":updateContact?updatePersonFields=addresses", `{}`},
		"cuerpo con un campo ajeno": {http.MethodPost, "/compat/google/v1/people:createContact", `{"names":[{"givenName":"X"}],"birthdays":[]}`},
		"persona sin nombre":        {http.MethodPost, "/compat/google/v1/people:createContact", `{"emailAddresses":[{"value":"a@b.c"}]}`},
	} {
		if rec := pedir(c.method, c.target, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d (%s)", nombre, rec.Code, rec.Body.String())
		}
	}

	if rec := pedir(http.MethodDelete, "/compat/google/v1/people/"+ada.ID.String()+":deleteContact", ""); rec.Code != http.StatusOK {
		t.Fatalf("deleteContact: %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- Microsoft Graph ---------------------------------------------------------

func TestCompatGraphContacts(t *testing.T) {
	ada, bob := compatContact("Ada"), compatContact("Bob")
	f := &fakeStore{contacts: []domain.Contact{ada, bob}}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts?$select=displayName")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	got := compatItems(t, rec, "value")
	if len(got) != 2 || got[0]["displayName"] != "Ada" || got[0]["id"] != ada.ID.String() {
		t.Fatalf("contactos: %+v", got)
	}
	if got[0]["mobilePhone"] != "+34600000000" || got[0]["companyName"] != "Sytue" {
		t.Fatalf("campos de Graph: %+v", got[0])
	}

	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts?$top=1")
	next, _ := compatBody(t, rec)["@odata.nextLink"].(string)
	if !strings.HasPrefix(next, "https://contacts.sytue.eu/compat/graph/v1.0/me/contacts?") ||
		!strings.Contains(next, "%24skip=1") {
		t.Fatalf("@odata.nextLink = %q", next)
	}
	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts?$top=1&$skip=1")
	if got := compatItems(t, rec, "value"); len(got) != 1 || got[0]["displayName"] != "Bob" {
		t.Fatalf("$skip=1: %+v", got)
	}
	if body := compatBody(t, rec); body["@odata.nextLink"] != nil {
		t.Fatalf("última página con nextLink: %s", rec.Body.String())
	}
}

func TestCompatGraphUnsupportedAndGet(t *testing.T) {
	ada := compatContact("Ada")
	f := &fakeStore{contact: ada}
	h := compatRouter(f, "user-1")

	for _, q := range []string{"?$filter=" + url.QueryEscape("startswith(displayName,'A')"), "?$orderby=displayName", "?$count=true", "?$search=ada"} {
		rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts"+q)
		if rec.Code != http.StatusBadRequest || graphCodeOf(t, rec) != "invalidRequest" {
			t.Fatalf("%s: estado = %d (%s)", q, rec.Code, rec.Body.String())
		}
	}

	rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts/"+ada.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := compatBody(t, rec); body["id"] != ada.ID.String() {
		t.Fatalf("contacto: %+v", body)
	}

	f.contactErr = domain.ErrNotFound
	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contacts/"+uuid.NewString())
	if rec.Code != http.StatusNotFound || graphCodeOf(t, rec) != "itemNotFound" {
		t.Fatalf("no encontrado: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/contactFolders")
	if rec.Code != http.StatusNotImplemented || graphCodeOf(t, rec) != "notImplemented" {
		t.Fatalf("contactFolders: %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- Aislamiento -------------------------------------------------------------

// isolatedStore reproduce la verja REAL del store: todo va owner-scoped por el
// subject, y lo de otro usuario simplemente no existe (404 / lista vacía).
type isolatedStore struct {
	*fakeStore
	owner string
}

func (s *isolatedStore) GetContact(ctx context.Context, subject string, id uuid.UUID) (domain.Contact, error) {
	if subject != s.owner {
		return domain.Contact{}, domain.ErrNotFound
	}
	return s.fakeStore.GetContact(ctx, subject, id)
}

func (s *isolatedStore) ListOwnerContactsPage(ctx context.Context, subject string, limit, offset int) ([]domain.Contact, error) {
	if subject != s.owner {
		return nil, nil
	}
	return s.fakeStore.ListOwnerContactsPage(ctx, subject, limit, offset)
}

// TestCompatAislamiento: el usuario B no ve por la vía compat los contactos de A —
// ni listando ni pidiendo el id directo, en ninguno de los dos dialectos.
func TestCompatAislamiento(t *testing.T) {
	ada := compatContact("Ada")
	base := &fakeStore{contacts: []domain.Contact{ada}, contact: ada}
	st := &isolatedStore{fakeStore: base, owner: "user-a"}

	// Control: el dueño sí ve lo suyo.
	owner := compatRouter(st, "user-a")
	if got := compatItems(t, compatDo(owner, http.MethodGet, "/compat/google/v1/people/me/connections"), "connections"); len(got) != 1 {
		t.Fatalf("el dueño debe ver su contacto: %+v", got)
	}
	if rec := compatDo(owner, http.MethodGet, "/compat/graph/v1.0/me/contacts/"+ada.ID.String()); rec.Code != http.StatusOK {
		t.Fatalf("el dueño debe ver su contacto por id: %d", rec.Code)
	}

	intruder := compatRouter(st, "user-b")
	listados := []struct{ name, target, key string }{
		{name: "google connections", target: "/compat/google/v1/people/me/connections", key: "connections"},
		{name: "graph contacts", target: "/compat/graph/v1.0/me/contacts", key: "value"},
	}
	for _, tc := range listados {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(intruder, http.MethodGet, tc.target)
			if rec.Code != http.StatusOK {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
			if got := compatItems(t, rec, tc.key); len(got) != 0 {
				t.Fatalf("el intruso ve contactos ajenos: %+v", got)
			}
		})
	}

	directos := []struct{ name, target string }{
		{name: "google persona ajena", target: "/compat/google/v1/people/" + ada.ID.String()},
		{name: "graph contacto ajeno", target: "/compat/graph/v1.0/me/contacts/" + ada.ID.String()},
	}
	for _, tc := range directos {
		t.Run(tc.name, func(t *testing.T) {
			if rec := compatDo(intruder, http.MethodGet, tc.target); rec.Code != http.StatusNotFound {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}
