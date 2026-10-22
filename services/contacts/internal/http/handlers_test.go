// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// --- Fake del store ----------------------------------------------------------

// fakeStore implementa contactStore con salidas/errores programables por método.
type fakeStore struct {
	book    domain.AddressBook
	bookErr error
	books   []domain.AddressBook

	gotNewBook store.NewAddressBook

	contact    domain.Contact
	contactErr error
	contacts   []domain.Contact
	cards      []domain.CardObject

	gotPageSubject string
	gotPageLimit   int
	gotPageOffset  int

	gotUpsert store.UpsertContact

	card    domain.CardObject
	cardErr error

	group    domain.Group
	groupErr error
	groups   []domain.Group

	gotUpsertGroup store.UpsertGroup

	deleteErr error

	// Favoritos / papelera / import-export.
	favUIDs    []string
	favErr     error
	gotFavUID  string
	gotFavOn   bool
	trashItems []store.TrashItem
	trashErr   error
	emailSet   map[string]struct{}
	created    []store.UpsertContact // acumula cada CreateContact (import)

	// Lote / foto.
	batchN        int
	batchErr      error
	gotBatchUIDs  []string
	gotBatchOn    bool
	gotBatchAdd   bool
	gotPhotoUID   string
	gotPhotoData  string
	photoDataURI  string
	photoEtag     uuid.UUID
	photoErr      error
	removePhotoOK error
}

func (f *fakeStore) EnsureDefaultAddressBook(context.Context, string) (domain.AddressBook, error) {
	return f.book, f.bookErr
}

func (f *fakeStore) ListAddressBooks(context.Context, string) ([]domain.AddressBook, error) {
	return f.books, f.bookErr
}

func (f *fakeStore) CreateAddressBook(_ context.Context, na store.NewAddressBook) (domain.AddressBook, error) {
	f.gotNewBook = na
	return f.book, f.bookErr
}

func (f *fakeStore) GetAddressBook(context.Context, string, uuid.UUID) (domain.AddressBook, error) {
	return f.book, f.bookErr
}

func (f *fakeStore) UpdateAddressBook(context.Context, string, uuid.UUID, store.AddressBookPatch) (domain.AddressBook, error) {
	return f.book, f.bookErr
}

func (f *fakeStore) DeleteAddressBook(context.Context, string, uuid.UUID) error { return f.deleteErr }

func (f *fakeStore) ListContacts(context.Context, string, uuid.UUID, int) ([]domain.Contact, error) {
	return f.contacts, f.contactErr
}

func (f *fakeStore) CreateContact(_ context.Context, _ string, in store.UpsertContact) (domain.Contact, error) {
	f.gotUpsert = in
	f.created = append(f.created, in)
	return f.contact, f.contactErr
}

func (f *fakeStore) GetContact(context.Context, string, uuid.UUID) (domain.Contact, error) {
	return f.contact, f.contactErr
}

func (f *fakeStore) UpdateContact(_ context.Context, _ string, _ uuid.UUID, in store.UpsertContact, _ *uuid.UUID) (domain.Contact, error) {
	f.gotUpsert = in
	return f.contact, f.contactErr
}
func (f *fakeStore) DeleteContact(context.Context, string, uuid.UUID) error { return f.deleteErr }
func (f *fakeStore) SearchContacts(context.Context, string, string, *uuid.UUID, int) ([]domain.Contact, error) {
	return f.contacts, f.contactErr
}

func (f *fakeStore) GetCardObjectByResource(context.Context, uuid.UUID, string) (domain.CardObject, error) {
	return f.card, f.cardErr
}

func (f *fakeStore) ListGroups(context.Context, string, uuid.UUID) ([]domain.Group, error) {
	return f.groups, f.groupErr
}

func (f *fakeStore) CreateGroup(_ context.Context, _ string, in store.UpsertGroup) (domain.Group, error) {
	f.gotUpsertGroup = in
	return f.group, f.groupErr
}

func (f *fakeStore) GetGroup(context.Context, string, uuid.UUID) (domain.Group, error) {
	return f.group, f.groupErr
}

func (f *fakeStore) UpdateGroup(_ context.Context, _ string, _ uuid.UUID, in store.UpsertGroup, _ *uuid.UUID) (domain.Group, error) {
	f.gotUpsertGroup = in
	return f.group, f.groupErr
}
func (f *fakeStore) DeleteGroup(context.Context, string, uuid.UUID) error { return f.deleteErr }

func (f *fakeStore) SetFavorite(_ context.Context, _, uid string, on bool) error {
	f.gotFavUID = uid
	f.gotFavOn = on
	return f.favErr
}

func (f *fakeStore) ListFavoriteUIDs(context.Context, string) ([]string, error) {
	return f.favUIDs, f.favErr
}

func (f *fakeStore) ListTrash(context.Context, string, int) ([]store.TrashItem, error) {
	return f.trashItems, f.trashErr
}
func (f *fakeStore) RestoreContact(context.Context, string, uuid.UUID) error { return f.deleteErr }
func (f *fakeStore) PurgeContact(context.Context, string, uuid.UUID) error   { return f.deleteErr }

func (f *fakeStore) ContactEmailSet(context.Context, uuid.UUID) (map[string]struct{}, error) {
	return f.emailSet, f.contactErr
}

func (f *fakeStore) ListOwnerContacts(context.Context, string) ([]domain.Contact, error) {
	return f.contacts, f.contactErr
}

func (f *fakeStore) ListCardObjects(_ context.Context, bookID uuid.UUID) ([]domain.CardObject, error) {
	var out []domain.CardObject
	for _, o := range f.cards {
		if o.AddressBookID == bookID {
			out = append(out, o)
		}
	}
	return out, nil
}

// ListOwnerContactsPage reproduce el corte por limit/offset del store sobre la
// lista programada (la capa de compatibilidad pagina con él).
func (f *fakeStore) ListOwnerContactsPage(_ context.Context, subject string, limit, offset int) ([]domain.Contact, error) {
	f.gotPageSubject, f.gotPageLimit, f.gotPageOffset = subject, limit, offset
	if f.contactErr != nil {
		return nil, f.contactErr
	}
	if offset >= len(f.contacts) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.contacts) {
		end = len(f.contacts)
	}
	return f.contacts[offset:end], nil
}

func (f *fakeStore) TrashContacts(_ context.Context, _ string, uids []string) (int, error) {
	f.gotBatchUIDs = uids
	return f.batchN, f.batchErr
}

func (f *fakeStore) SetFavorites(_ context.Context, _ string, uids []string, on bool) (int, error) {
	f.gotBatchUIDs = uids
	f.gotBatchOn = on
	return f.batchN, f.batchErr
}

func (f *fakeStore) ModifyGroupMembers(_ context.Context, _ string, _ uuid.UUID, uids []string, add bool) (domain.Group, error) {
	f.gotBatchUIDs = uids
	f.gotBatchAdd = add
	return f.group, f.groupErr
}

func (f *fakeStore) SetContactPhoto(_ context.Context, _, uid, dataURI string) error {
	f.gotPhotoUID = uid
	f.gotPhotoData = dataURI
	return f.photoErr
}

func (f *fakeStore) RemoveContactPhoto(_ context.Context, _, uid string) error {
	f.gotPhotoUID = uid
	return f.removePhotoOK
}

func (f *fakeStore) GetContactPhoto(_ context.Context, _, uid string) (string, uuid.UUID, error) {
	f.gotPhotoUID = uid
	return f.photoDataURI, f.photoEtag, f.photoErr
}

func (f *fakeStore) SyncAvatarByEmail(_ context.Context, emails []string, dataURI string) (int, error) {
	f.gotPhotoData = dataURI
	return len(emails), f.photoErr
}

// --- Helpers -----------------------------------------------------------------

func newHandlers(f *fakeStore) *Handlers {
	return &Handlers{store: f, cfg: config.Config{}}
}

func req(method, target, body string, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	ctx := authn.Into(r.Context(), authn.Claims{Subject: "user-1", Email: "u@ex.com", Tenant: "acme"})
	if params != nil {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
	}
	return r.WithContext(ctx)
}

func id() map[string]string { return map[string]string{"id": uuid.NewString()} }

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

// --- Libretas ----------------------------------------------------------------

func TestListAddressBooksOK(t *testing.T) {
	f := &fakeStore{books: []domain.AddressBook{{Name: "personal"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListAddressBooks(rec, req(http.MethodGet, "/", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if _, ok := decode(t, rec)["address_books"]; !ok {
		t.Error("falta clave address_books")
	}
}

func TestListAddressBooksEnsureError(t *testing.T) {
	f := &fakeStore{bookErr: domain.ErrConflict}
	rec := httptest.NewRecorder()
	newHandlers(f).ListAddressBooks(rec, req(http.MethodGet, "/", "", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreateAddressBookDefaultsDisplayName(t *testing.T) {
	f := &fakeStore{book: domain.AddressBook{Name: "n"}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateAddressBook(rec, req(http.MethodPost, "/", `{"name":"work"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	// display_name vacío se rellena con name.
	if f.gotNewBook.DisplayName != "work" || f.gotNewBook.OwnerSub != "user-1" {
		t.Errorf("NewAddressBook mal mapeado: %+v", f.gotNewBook)
	}
}

func TestCreateAddressBookBadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).CreateAddressBook(rec, req(http.MethodPost, "/", `{bad`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetAddressBookBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).GetAddressBook(rec, req(http.MethodGet, "/", "", map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetAddressBookNotFound(t *testing.T) {
	f := &fakeStore{bookErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetAddressBook(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestUpdateAddressBookOK(t *testing.T) {
	f := &fakeStore{book: domain.AddressBook{Name: "n"}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateAddressBook(rec, req(http.MethodPatch, "/", `{"display_name":"X"}`, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestDeleteAddressBookDefaultImmutable(t *testing.T) {
	f := &fakeStore{deleteErr: domain.ErrDefaultImmutable}
	rec := httptest.NewRecorder()
	newHandlers(f).DeleteAddressBook(rec, req(http.MethodDelete, "/", "", id()))
	// ErrDefaultImmutable => 400 en Contacts.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestDeleteAddressBookOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).DeleteAddressBook(rec, req(http.MethodDelete, "/", "", id()))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Contactos ---------------------------------------------------------------

func TestListContactsOK(t *testing.T) {
	f := &fakeStore{contacts: []domain.Contact{{FormattedName: "Ana"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListContacts(rec, req(http.MethodGet, "/?limit=5", "", id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["contacts"]; !ok {
		t.Error("falta clave contacts")
	}
}

func TestCreateContactOK(t *testing.T) {
	f := &fakeStore{contact: domain.Contact{FormattedName: "Ana", ETag: uuid.New()}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateContact(rec, req(http.MethodPost, "/", `{"formatted_name":"Ana","emails":[{"email":"a@x.com"}]}`, id()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	// Se serializa la vCard y se genera el UID + resource name.
	if f.gotUpsert.CardData == "" || f.gotUpsert.ResourceName == "" {
		t.Errorf("UpsertContact incompleto: %+v", f.gotUpsert)
	}
	if et := rec.Header().Get("ETag"); et == "" {
		t.Error("falta cabecera ETag")
	}
}

// TestCreateContactConTarjetaEntera: con `Content-Type: text/vcard` el alta guarda la
// tarjeta de fuera ENTERA —sus X-*, con UID nuevo— y no el JSON de la API.
func TestCreateContactConTarjetaEntera(t *testing.T) {
	f := &fakeStore{contact: domain.Contact{FormattedName: "Ana", ETag: uuid.New()}}
	tarjeta := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:de-otro-sitio\r\nFN:Ana Ruiz\r\n" +
		"EMAIL;TYPE=INTERNET;TYPE=WORK:ana@acme.es\r\nX-SOCIALPROFILE:https://ejemplo.es/ana\r\nEND:VCARD\r\n"
	r := req(http.MethodPost, "/", tarjeta, id())
	r.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	rec := httptest.NewRecorder()
	newHandlers(f).CreateContact(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	got := f.gotUpsert
	if got.Contact.FormattedName != "Ana Ruiz" || len(got.Contact.Emails) != 1 || got.Contact.Emails[0].Type != domain.EmailWork {
		t.Errorf("proyección = %+v", got.Contact)
	}
	if !strings.Contains(got.CardData, "X-SOCIALPROFILE") {
		t.Errorf("la tarjeta guardada pierde los X-*:\n%s", got.CardData)
	}
	if got.Contact.UID == "de-otro-sitio" || strings.Contains(got.CardData, "de-otro-sitio") {
		t.Error("se reutiliza el UID de fuera: importar dos veces chocaría")
	}
	if got.ResourceName != domain.ResourceName(got.Contact.UID) {
		t.Errorf("recurso = %q para el UID %q", got.ResourceName, got.Contact.UID)
	}

	// Una tarjeta que no es una ficha es un 400, y una desmesurada, un 413.
	for cuerpo, quiero := range map[string]int{
		"BEGIN:VCARD\r\nVERSION:4.0\r\nEMAIL:x@y.es\r\nEND:VCARD\r\n": http.StatusBadRequest,
		strings.Repeat("x", maxTarjeta+1):                             http.StatusRequestEntityTooLarge,
	} {
		r := req(http.MethodPost, "/", cuerpo, id())
		r.Header.Set("Content-Type", "text/vcard")
		rec := httptest.NewRecorder()
		newHandlers(&fakeStore{}).CreateContact(rec, r)
		if rec.Code != quiero {
			t.Errorf("cuerpo de %d bytes: code = %d, want %d", len(cuerpo), rec.Code, quiero)
		}
	}
}

func TestCreateContactValidationError(t *testing.T) {
	// Sin nombre formateado ni given/family => ErrInvalidContact (400).
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).CreateContact(rec, req(http.MethodPost, "/", `{"note":"x"}`, id()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateContactComposesNameFromParts(t *testing.T) {
	f := &fakeStore{contact: domain.Contact{FormattedName: "Ana Ruiz"}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateContact(rec, req(http.MethodPost, "/", `{"given_name":"Ana","family_name":"Ruiz"}`, id()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotUpsert.Contact.FormattedName != "Ana Ruiz" {
		t.Errorf("FormattedName compuesto = %q", f.gotUpsert.Contact.FormattedName)
	}
}

func TestGetContactOK(t *testing.T) {
	f := &fakeStore{contact: domain.Contact{FormattedName: "Ana", ETag: uuid.New()}}
	rec := httptest.NewRecorder()
	newHandlers(f).GetContact(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("falta ETag")
	}
}

func TestUpdateContactOK(t *testing.T) {
	f := &fakeStore{
		contact: domain.Contact{UID: "uid-1", FormattedName: "Ana", AddressBookID: uuid.New(), ETag: uuid.New()},
		card:    domain.CardObject{ResourceName: "uid-1.vcf"},
	}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateContact(rec, req(http.MethodPut, "/", `{"formatted_name":"Ana Nueva"}`, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	// El UID se conserva del contacto actual.
	if f.gotUpsert.Contact.UID != "uid-1" {
		t.Errorf("UID = %q, want uid-1 (estable)", f.gotUpsert.Contact.UID)
	}
}

func TestUpdateContactNotFound(t *testing.T) {
	f := &fakeStore{contactErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateContact(rec, req(http.MethodPut, "/", `{"formatted_name":"X"}`, id()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteContactOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).DeleteContact(rec, req(http.MethodDelete, "/", "", id()))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestExportContactVCard(t *testing.T) {
	f := &fakeStore{
		contact: domain.Contact{UID: "uid-1"},
		card:    domain.CardObject{Data: "BEGIN:VCARD\nEND:VCARD", ETag: uuid.New()},
	}
	rec := httptest.NewRecorder()
	newHandlers(f).ExportContactVCard(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/vcard") {
		t.Errorf("content-type = %q, want text/vcard", ct)
	}
	if !strings.Contains(rec.Body.String(), "BEGIN:VCARD") {
		t.Error("body no contiene la vCard")
	}
}

func TestExportContactVCardCardNotFound(t *testing.T) {
	f := &fakeStore{contact: domain.Contact{UID: "u"}, cardErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).ExportContactVCard(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestSearchContactsMissingQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).SearchContacts(rec, req(http.MethodGet, "/?q=", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSearchContactsBadBookID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).SearchContacts(rec, req(http.MethodGet, "/?q=ana&address_book_id=nope", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSearchContactsOK(t *testing.T) {
	f := &fakeStore{contacts: []domain.Contact{}}
	rec := httptest.NewRecorder()
	target := "/?q=ana&address_book_id=" + uuid.NewString() + "&limit=3"
	newHandlers(f).SearchContacts(rec, req(http.MethodGet, target, "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

// --- Grupos ------------------------------------------------------------------

func TestListGroupsOK(t *testing.T) {
	f := &fakeStore{groups: []domain.Group{{Name: "amigos"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListGroups(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["groups"]; !ok {
		t.Error("falta clave groups")
	}
}

func TestCreateGroupOK(t *testing.T) {
	f := &fakeStore{group: domain.Group{Name: "amigos", ETag: uuid.New()}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateGroup(rec, req(http.MethodPost, "/", `{"name":"amigos","members":["a","a","b"," "]}`, id()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	// dedupe: "a" duplicado y vacío se eliminan.
	if len(f.gotUpsertGroup.Group.Members) != 2 {
		t.Errorf("Members = %v, want [a b]", f.gotUpsertGroup.Group.Members)
	}
}

func TestCreateGroupEmptyNameError(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).CreateGroup(rec, req(http.MethodPost, "/", `{"name":"  "}`, id()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateGroupKeepsNameWhenEmpty(t *testing.T) {
	f := &fakeStore{group: domain.Group{UID: "g-1", Name: "original", ETag: uuid.New()}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateGroup(rec, req(http.MethodPut, "/", `{"members":["x"]}`, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotUpsertGroup.Group.Name != "original" {
		t.Errorf("Name = %q, want original (conservado)", f.gotUpsertGroup.Group.Name)
	}
}

func TestGetGroupNotFound(t *testing.T) {
	f := &fakeStore{groupErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetGroup(rec, req(http.MethodGet, "/", "", id()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteGroupOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).DeleteGroup(rec, req(http.MethodDelete, "/", "", id()))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Favoritos / papelera / import-export ------------------------------------

func TestListFavoritesOK(t *testing.T) {
	f := &fakeStore{favUIDs: []string{"uid-1", "uid-2"}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListFavorites(rec, req(http.MethodGet, "/", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["uids"]; !ok {
		t.Error("falta clave uids")
	}
}

func TestToggleFavoriteOK(t *testing.T) {
	f := &fakeStore{}
	rec := httptest.NewRecorder()
	newHandlers(f).ToggleFavorite(rec, req(http.MethodPost, "/", `{"uid":"uid-1","favorite":true}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotFavUID != "uid-1" || f.gotFavOn != true {
		t.Errorf("SetFavorite mal mapeado: uid=%q on=%v", f.gotFavUID, f.gotFavOn)
	}
}

func TestToggleFavoriteMissingUID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).ToggleFavorite(rec, req(http.MethodPost, "/", `{"favorite":true}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestToggleFavoriteNotFound(t *testing.T) {
	f := &fakeStore{favErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).ToggleFavorite(rec, req(http.MethodPost, "/", `{"uid":"x","favorite":true}`, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestListTrashOK(t *testing.T) {
	f := &fakeStore{trashItems: []store.TrashItem{{UID: "u", FormattedName: "Borrado"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListTrash(rec, req(http.MethodGet, "/?limit=10", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["items"]; !ok {
		t.Error("falta clave items")
	}
}

func TestRestoreContactOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).RestoreContact(rec, req(http.MethodPost, "/", "", id()))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestRestoreContactNotFound(t *testing.T) {
	f := &fakeStore{deleteErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).RestoreContact(rec, req(http.MethodPost, "/", "", id()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestPurgeContactOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).PurgeContact(rec, req(http.MethodDelete, "/", "", id()))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

const googleCSV = "Name,Given Name,Family Name,E-mail 1 - Type,E-mail 1 - Value,Phone 1 - Type,Phone 1 - Value\r\n" +
	"Ada Lovelace,Ada,Lovelace,Home,ada@example.com,Mobile,+34600000000\r\n"

func TestImportCSVGoogleOK(t *testing.T) {
	f := &fakeStore{
		book:     domain.AddressBook{OwnerSub: "user-1"}, // req() autentica como user-1
		emailSet: map[string]struct{}{},
	}
	rec := httptest.NewRecorder()
	newHandlers(f).ImportCSV(rec, req(http.MethodPost, "/", googleCSV, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["imported"] != float64(1) || body["skipped"] != float64(0) {
		t.Fatalf("recuento inesperado: %+v", body)
	}
	if len(f.created) != 1 {
		t.Fatalf("CreateContact llamado %d veces, want 1", len(f.created))
	}
	got := f.created[0].Contact
	if got.FamilyName != "Lovelace" || got.PrimaryEmail() != "ada@example.com" {
		t.Errorf("mapeo CSV->contacto incorrecto: %+v", got)
	}
}

func TestImportCSVForeignBook(t *testing.T) {
	f := &fakeStore{book: domain.AddressBook{OwnerSub: "someone-else"}, emailSet: map[string]struct{}{}}
	rec := httptest.NewRecorder()
	newHandlers(f).ImportCSV(rec, req(http.MethodPost, "/", googleCSV, id()))
	// No se puede importar a la libreta de otro dueño.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
	if len(f.created) != 0 {
		t.Errorf("no debió crear contactos en libreta ajena: %d", len(f.created))
	}
}

func TestImportCSVDedupeByEmail(t *testing.T) {
	f := &fakeStore{
		book:     domain.AddressBook{OwnerSub: "user-1"},
		emailSet: map[string]struct{}{"ada@example.com": {}}, // ya existe
	}
	rec := httptest.NewRecorder()
	newHandlers(f).ImportCSV(rec, req(http.MethodPost, "/", googleCSV, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	body := decode(t, rec)
	if body["imported"] != float64(0) || body["skipped"] != float64(1) {
		t.Fatalf("dedupe no aplicado: %+v", body)
	}
	if len(f.created) != 0 {
		t.Errorf("no debió crear el duplicado: %d", len(f.created))
	}
}

func TestImportCSVUnrecognized(t *testing.T) {
	f := &fakeStore{book: domain.AddressBook{OwnerSub: "user-1"}, emailSet: map[string]struct{}{}}
	rec := httptest.NewRecorder()
	newHandlers(f).ImportCSV(rec, req(http.MethodPost, "/", "Col A,Col B\r\n1,2\r\n", id()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (cabeceras no reconocidas)", rec.Code)
	}
}

func TestExportCSV(t *testing.T) {
	f := &fakeStore{contacts: []domain.Contact{
		{FormattedName: "Ada Lovelace", Emails: []domain.Email{{Email: "ada@example.com", Type: domain.EmailHome}}},
	}}
	rec := httptest.NewRecorder()
	newHandlers(f).ExportCSV(rec, req(http.MethodGet, "/", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q, want text/csv", ct)
	}
	if !strings.Contains(rec.Body.String(), "ada@example.com") {
		t.Error("el CSV exportado no contiene el correo")
	}
	if !strings.Contains(rec.Body.String(), "Ada Lovelace") {
		t.Error("el CSV exportado no contiene el nombre")
	}
}

// --- Helpers puros -----------------------------------------------------------

func TestToAPIErrorMapping(t *testing.T) {
	if toAPIError(nil) != nil {
		t.Fatal("toAPIError(nil) debe ser nil")
	}
	cases := map[error]int{
		domain.ErrNotFound:           http.StatusNotFound,
		domain.ErrConflict:           http.StatusConflict,
		domain.ErrPreconditionFailed: http.StatusPreconditionFailed,
		domain.ErrInvalidName:        http.StatusBadRequest,
		domain.ErrInvalidContact:     http.StatusBadRequest,
		domain.ErrInvalidGroup:       http.StatusBadRequest,
		domain.ErrInvalidVCard:       http.StatusBadRequest,
		domain.ErrDefaultImmutable:   http.StatusBadRequest,
		context.DeadlineExceeded:     http.StatusInternalServerError,
	}
	for in, want := range cases {
		if got := httpx.AsError(toAPIError(in)).Status; got != want {
			t.Errorf("toAPIError(%v) status = %d, want %d", in, got, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	for _, in := range []string{"2026-01-02", "2026-01-02T10:00:00Z", "20260102"} {
		if _, err := parseDate(in); err != nil {
			t.Errorf("parseDate(%q): %v", in, err)
		}
	}
	if _, err := parseDate("no-date"); err == nil {
		t.Error("parseDate debe fallar con basura")
	}
}

func TestEmailAndPhoneType(t *testing.T) {
	if emailType("WORK") != domain.EmailWork {
		t.Error("emailType work")
	}
	if emailType("") != domain.EmailHome {
		t.Error("emailType default home")
	}
	if phoneType("fax") != domain.PhoneFax {
		t.Error("phoneType fax")
	}
	if phoneType("") != domain.PhoneCell {
		t.Error("phoneType default cell")
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", " b ", "a", "", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("dedupe = %v, want [a b]", got)
	}
}

func TestIfMatchFromHeader(t *testing.T) {
	valid := uuid.New()
	r := httptest.NewRequest(http.MethodPut, "/", http.NoBody)
	r.Header.Set("If-Match", `W/"`+valid.String()+`"`)
	got := ifMatchFromHeader(r)
	if got == nil || *got != valid {
		t.Errorf("ifMatchFromHeader = %v, want %v", got, valid)
	}

	r2 := httptest.NewRequest(http.MethodPut, "/", http.NoBody)
	r2.Header.Set("If-Match", "*")
	if ifMatchFromHeader(r2) != nil {
		t.Error("If-Match=* debe ser nil")
	}

	r3 := httptest.NewRequest(http.MethodPut, "/", http.NoBody)
	if ifMatchFromHeader(r3) != nil {
		t.Error("sin If-Match debe ser nil")
	}
}

func TestEnsureOnePrimary(t *testing.T) {
	emails := []domain.Email{{Email: "a"}, {Email: "b"}}
	ensureOnePrimaryEmails(emails)
	if !emails[0].IsPrimary {
		t.Error("primer email debe marcarse principal")
	}
	// Si ya hay uno principal, no se toca.
	emails2 := []domain.Email{{Email: "a"}, {Email: "b", IsPrimary: true}}
	ensureOnePrimaryEmails(emails2)
	if emails2[0].IsPrimary {
		t.Error("no debe cambiar si ya hay principal")
	}
	phones := []domain.Phone{{Phone: "1"}}
	ensureOnePrimaryPhones(phones)
	if !phones[0].IsPrimary {
		t.Error("primer phone debe marcarse principal")
	}
}
