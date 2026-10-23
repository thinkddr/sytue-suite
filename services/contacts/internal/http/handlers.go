// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// contactStore es la superficie del store que consume la capa HTTP. Se declara
// como interfaz (satisfecha por *store.Store) para poder inyectar dobles en los
// tests sin depender de una base de datos.
type contactStore interface {
	// Todo el CRUD va owner-scoped: el subject del JWT baja al store, que es la
	// verja unica de dueno (JOIN address_books por owner_sub) para REST y CardDAV.
	EnsureDefaultAddressBook(ctx context.Context, ownerSub string) (domain.AddressBook, error)
	ListAddressBooks(ctx context.Context, ownerSub string) ([]domain.AddressBook, error)
	CreateAddressBook(ctx context.Context, na store.NewAddressBook) (domain.AddressBook, error)
	GetAddressBook(ctx context.Context, ownerSub string, id uuid.UUID) (domain.AddressBook, error)
	UpdateAddressBook(ctx context.Context, ownerSub string, id uuid.UUID, p store.AddressBookPatch) (domain.AddressBook, error)
	DeleteAddressBook(ctx context.Context, ownerSub string, id uuid.UUID) error

	ListContacts(ctx context.Context, subject string, bookID uuid.UUID, limit int) ([]domain.Contact, error)
	ListContactsPage(ctx context.Context, subject string, bookID uuid.UUID, after *store.ContactCursor, limit int) (store.ContactPage, error)
	CreateContact(ctx context.Context, subject string, in store.UpsertContact) (domain.Contact, error)
	GetContact(ctx context.Context, subject string, id uuid.UUID) (domain.Contact, error)
	UpdateContact(ctx context.Context, subject string, id uuid.UUID, in store.UpsertContact, ifMatch *uuid.UUID) (domain.Contact, error)
	DeleteContact(ctx context.Context, subject string, id uuid.UUID) error
	SearchContacts(ctx context.Context, subject, query string, bookID *uuid.UUID, limit int) ([]domain.Contact, error)
	GetCardObjectByResource(ctx context.Context, bookID uuid.UUID, resourceName string) (domain.CardObject, error)

	// Favoritos / papelera / import-export (paridad Google Contacts).
	SetFavorite(ctx context.Context, subject, uid string, on bool) error
	ListFavoriteUIDs(ctx context.Context, subject string) ([]string, error)
	ListTrash(ctx context.Context, subject string, limit int) ([]store.TrashItem, error)
	RestoreContact(ctx context.Context, subject string, id uuid.UUID) error
	PurgeContact(ctx context.Context, subject string, id uuid.UUID) error
	ContactEmailSet(ctx context.Context, bookID uuid.UUID) (map[string]struct{}, error)
	ListOwnerContacts(ctx context.Context, subject string) ([]domain.Contact, error)
	// ListCardObjects da los vCard GUARDADOS de una libreta (para exportarlos tal cual).
	ListCardObjects(ctx context.Context, bookID uuid.UUID) ([]domain.CardObject, error)
	// ListOwnerContactsPage es el listado paginado de TODAS las libretas del
	// usuario (lo que la capa de compatibilidad entiende por «mis contactos»).
	ListOwnerContactsPage(ctx context.Context, subject string, limit, offset int) ([]domain.Contact, error)

	// Acciones en lote (multiseleccion), owner-scoped, una consulta por accion.
	TrashContacts(ctx context.Context, subject string, uids []string) (int, error)
	SetFavorites(ctx context.Context, subject string, uids []string, on bool) (int, error)
	ModifyGroupMembers(ctx context.Context, subject string, groupID uuid.UUID, uids []string, add bool) (domain.Group, error)

	// Foto de contacto (PHOTO embebida en la vCard), owner-scoped por UID.
	SetContactPhoto(ctx context.Context, subject, uid, dataURI string) error
	RemoveContactPhoto(ctx context.Context, subject, uid string) error
	GetContactPhoto(ctx context.Context, subject, uid string) (string, uuid.UUID, error)
	// SyncAvatarByEmail refleja el avatar propio de Sytue ID en TODAS las fichas
	// que coincidan por correo (no owner-scoped: ver internal.go).
	SyncAvatarByEmail(ctx context.Context, emails []string, dataURI string) (int, error)

	ListGroups(ctx context.Context, subject string, bookID uuid.UUID) ([]domain.Group, error)
	CreateGroup(ctx context.Context, subject string, in store.UpsertGroup) (domain.Group, error)
	GetGroup(ctx context.Context, subject string, id uuid.UUID) (domain.Group, error)
	UpdateGroup(ctx context.Context, subject string, id uuid.UUID, in store.UpsertGroup, ifMatch *uuid.UUID) (domain.Group, error)
	DeleteGroup(ctx context.Context, subject string, id uuid.UUID) error
}

// Handlers agrupa los manejadores de la API REST de Contacts.
type Handlers struct {
	store contactStore
	cfg   config.Config
}

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("A resource with that identifier or name already exists.")
	case errors.Is(err, domain.ErrPreconditionFailed):
		return httpx.NewError(http.StatusPreconditionFailed, "precondition_failed", "El recurso ha cambiado (ETag no coincide).")
	case errors.Is(err, domain.ErrInvalidName):
		return httpx.ErrBadRequest.WithMessage("Invalid name.")
	case errors.Is(err, domain.ErrInvalidContact):
		return httpx.ErrBadRequest.WithMessage("Invalid contact: a name is required.")
	case errors.Is(err, domain.ErrInvalidGroup):
		return httpx.ErrBadRequest.WithMessage("Invalid group: a name is required.")
	case errors.Is(err, domain.ErrInvalidVCard):
		return httpx.ErrBadRequest.WithMessage("Invalid vCard.")
	case errors.Is(err, domain.ErrInvalidCSV):
		return httpx.ErrBadRequest.WithMessage("Unrecognised CSV: use an export from Google Contacts or Outlook.")
	case errors.Is(err, domain.ErrDefaultImmutable):
		return httpx.ErrBadRequest.WithMessage("The default address book cannot be deleted.")
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// parseID extrae y valida el UUID del parámetro de ruta "id".
func parseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid id.")
	}
	return id, nil
}

// subFromCtx devuelve el subject del usuario autenticado.
func subFromCtx(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// ifMatchFromHeader extrae el ETag (UUID) de la cabecera If-Match, si la hay.
func ifMatchFromHeader(r *http.Request) *uuid.UUID {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" || raw == "*" {
		return nil
	}
	if len(raw) >= 2 && raw[0] == 'W' && raw[1] == '/' {
		raw = raw[2:]
	}
	raw = strings.Trim(raw, `"`)
	if id, err := uuid.Parse(raw); err == nil {
		return &id
	}
	return nil
}

// ---------------------------------------------------------------------------
// Libretas de direcciones
// ---------------------------------------------------------------------------

// ListAddressBooks lista las libretas del usuario (asegura la por defecto).
func (h *Handlers) ListAddressBooks(w http.ResponseWriter, r *http.Request) {
	sub := subFromCtx(r)
	if _, err := h.store.EnsureDefaultAddressBook(r.Context(), sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	books, err := h.store.ListAddressBooks(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if books == nil {
		books = []domain.AddressBook{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"address_books": books})
}

type createAddressBookRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// CreateAddressBook crea una libreta de direcciones.
func (h *Handlers) CreateAddressBook(w http.ResponseWriter, r *http.Request) {
	var req createAddressBookRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}
	bk, err := h.store.CreateAddressBook(r.Context(), store.NewAddressBook{
		OwnerSub:    subFromCtx(r),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, bk)
}

// GetAddressBook devuelve una libreta por ID.
func (h *Handlers) GetAddressBook(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bk, err := h.store.GetAddressBook(r.Context(), subFromCtx(r), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, bk)
}

type updateAddressBookRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
}

// UpdateAddressBook aplica un patch parcial a una libreta.
func (h *Handlers) UpdateAddressBook(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateAddressBookRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bk, err := h.store.UpdateAddressBook(r.Context(), subFromCtx(r), id, store.AddressBookPatch{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, bk)
}

// DeleteAddressBook borra una libreta (no la por defecto).
func (h *Handlers) DeleteAddressBook(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteAddressBook(r.Context(), subFromCtx(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Contactos
// ---------------------------------------------------------------------------

// contactRequest es el cuerpo de creación/actualización de un contacto vía API.
type contactRequest struct {
	FormattedName   string           `json:"formatted_name"`
	FamilyName      string           `json:"family_name,omitempty"`
	GivenName       string           `json:"given_name,omitempty"`
	AdditionalName  string           `json:"additional_name,omitempty"`
	HonorificPrefix string           `json:"honorific_prefix,omitempty"`
	HonorificSuffix string           `json:"honorific_suffix,omitempty"`
	Organization    string           `json:"organization,omitempty"`
	Title           string           `json:"title,omitempty"`
	Nickname        string           `json:"nickname,omitempty"`
	Note            string           `json:"note,omitempty"`
	Birthday        string           `json:"birthday,omitempty"` // RFC3339 o YYYY-MM-DD
	PhotoURL        string           `json:"photo_url,omitempty"`
	Emails          []emailRequest   `json:"emails,omitempty"`
	Phones          []phoneRequest   `json:"phones,omitempty"`
	Addresses       []addressRequest `json:"addresses,omitempty"`
}

type emailRequest struct {
	Email     string `json:"email"`
	Type      string `json:"type,omitempty"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

type phoneRequest struct {
	Phone     string `json:"phone"`
	Type      string `json:"type,omitempty"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

type addressRequest struct {
	POBox      string `json:"po_box,omitempty"`
	Extended   string `json:"extended,omitempty"`
	Street     string `json:"street,omitempty"`
	Locality   string `json:"locality,omitempty"`
	Region     string `json:"region,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
	Type       string `json:"type,omitempty"`
	IsPrimary  bool   `json:"is_primary,omitempty"`
}

// toContact convierte el cuerpo de la API a un domain.Contact validado.
func (req contactRequest) toContact() (domain.Contact, error) {
	c := domain.Contact{
		FormattedName:   strings.TrimSpace(req.FormattedName),
		FamilyName:      strings.TrimSpace(req.FamilyName),
		GivenName:       strings.TrimSpace(req.GivenName),
		AdditionalName:  strings.TrimSpace(req.AdditionalName),
		HonorificPrefix: strings.TrimSpace(req.HonorificPrefix),
		HonorificSuffix: strings.TrimSpace(req.HonorificSuffix),
		Organization:    strings.TrimSpace(req.Organization),
		Title:           strings.TrimSpace(req.Title),
		Nickname:        strings.TrimSpace(req.Nickname),
		Note:            req.Note,
		PhotoURL:        strings.TrimSpace(req.PhotoURL),
	}
	if c.FormattedName == "" {
		c.FormattedName = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if c.FormattedName == "" {
		return domain.Contact{}, domain.ErrInvalidContact
	}
	if req.Birthday != "" {
		t, err := parseDate(req.Birthday)
		if err != nil {
			return domain.Contact{}, domain.ErrInvalidContact
		}
		c.Birthday = &t
	}
	for _, e := range req.Emails {
		if strings.TrimSpace(e.Email) == "" {
			continue
		}
		c.Emails = append(c.Emails, domain.Email{
			Email:     strings.TrimSpace(e.Email),
			Type:      emailType(e.Type),
			IsPrimary: e.IsPrimary,
		})
	}
	for _, p := range req.Phones {
		if strings.TrimSpace(p.Phone) == "" {
			continue
		}
		c.Phones = append(c.Phones, domain.Phone{
			Phone:     strings.TrimSpace(p.Phone),
			Type:      phoneType(p.Type),
			IsPrimary: p.IsPrimary,
		})
	}
	for _, a := range req.Addresses {
		addr := domain.Address{
			POBox:      strings.TrimSpace(a.POBox),
			Extended:   strings.TrimSpace(a.Extended),
			Street:     strings.TrimSpace(a.Street),
			Locality:   strings.TrimSpace(a.Locality),
			Region:     strings.TrimSpace(a.Region),
			PostalCode: strings.TrimSpace(a.PostalCode),
			Country:    strings.TrimSpace(a.Country),
			Type:       addressType(a.Type),
			IsPrimary:  a.IsPrimary,
		}
		if addr.IsZero() {
			continue
		}
		c.Addresses = append(c.Addresses, addr)
	}
	ensureOnePrimaryEmails(c.Emails)
	ensureOnePrimaryPhones(c.Phones)
	ensureOnePrimaryAddresses(c.Addresses)
	return c, nil
}

// ListContacts lista los contactos de una libreta (?limit=).
func (h *Handlers) ListContacts(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	contacts, err := h.store.ListContacts(r.Context(), subFromCtx(r), bookID, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if contacts == nil {
		contacts = []domain.Contact{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"contacts": contacts})
}

// CreateContact crea un contacto en una libreta (serializa su vCard).
func (h *Handlers) CreateContact(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, card, err := contactoDelAlta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	saved, err := h.store.CreateContact(r.Context(), subFromCtx(r), store.UpsertContact{
		AddressBookID: bookID,
		Contact:       c,
		CardData:      card,
		ResourceName:  domain.ResourceName(c.UID),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, saved.ETag)
	httpx.JSON(w, r, http.StatusCreated, saved)
}

// maxTarjeta acota una tarjeta importada: la foto admite 256 KiB, que en base64 son
// unos 342, y el resto de una ficha no llega a unos pocos KiB.
const maxTarjeta = 1 << 20

// contactoDelAlta lee el cuerpo de un alta. Con `Content-Type: text/vcard` es UNA
// tarjeta de fuera y se importa entera, con sus X-* y su foto (domain.ImportCard): es
// lo que la web consigue mandando la tarjeta como `baseVcard` a su BFF, y lo que un
// cliente sin BFF no tenía cómo hacer. Si no, es el JSON de la API.
func contactoDelAlta(w http.ResponseWriter, r *http.Request) (domain.Contact, string, error) {
	if tipo, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); tipo == "text/vcard" || tipo == "text/x-vcard" {
		datos, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTarjeta))
		var demasiado *http.MaxBytesError
		switch {
		case errors.As(err, &demasiado):
			return domain.Contact{}, "", httpx.NewError(http.StatusRequestEntityTooLarge, "vcard_too_large",
				"The vCard exceeds the maximum size (1 MiB).")
		case err != nil:
			return domain.Contact{}, "", httpx.ErrBadRequest.Wrap(err)
		}
		c, card, err := domain.ImportCard(string(datos))
		return c, card, toAPIError(err)
	}
	var req contactRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return domain.Contact{}, "", err
	}
	c, err := req.toContact()
	if err != nil {
		return domain.Contact{}, "", toAPIError(err)
	}
	c.UID = domain.NewUID()
	card, err := domain.EncodeContact(c, "")
	return c, card, toAPIError(err)
}

// GetContact devuelve un contacto por ID.
func (h *Handlers) GetContact(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.store.GetContact(r.Context(), subFromCtx(r), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, c.ETag)
	httpx.JSON(w, r, http.StatusOK, c)
}

// UpdateContact reemplaza un contacto (vCard regenerada preservando extras).
func (h *Handlers) UpdateContact(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req contactRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub := subFromCtx(r)
	current, err := h.store.GetContact(r.Context(), sub, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	c, err := req.toContact()
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	c.UID = current.UID // el UID es estable
	base, _ := h.store.GetCardObjectByResource(r.Context(), current.AddressBookID, domain.ResourceName(current.UID))
	card, err := domain.EncodeContact(c, base.Data)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	saved, err := h.store.UpdateContact(r.Context(), sub, id, store.UpsertContact{
		AddressBookID: current.AddressBookID,
		Contact:       c,
		CardData:      card,
		ResourceName:  base.ResourceName,
	}, ifMatchFromHeader(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, saved.ETag)
	httpx.JSON(w, r, http.StatusOK, saved)
}

// DeleteContact borra un contacto por ID.
func (h *Handlers) DeleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteContact(r.Context(), subFromCtx(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ExportContactVCard devuelve la vCard 4.0 canónica de un contacto (text/vcard).
func (h *Handlers) ExportContactVCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.store.GetContact(r.Context(), subFromCtx(r), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	obj, err := h.store.GetCardObjectByResource(r.Context(), c.AddressBookID, domain.ResourceName(c.UID))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8; version=4.0")
	w.Header().Set("Content-Disposition", `attachment; filename="`+c.UID+`.vcf"`)
	setETag(w, obj.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(obj.Data))
}

// ListAllContacts lista TODOS los contactos del usuario, página a página
// (?limit=&offset=). Devuelve `next_offset` cuando puede haber más.
//
// La búsqueda (`GET /contacts`) exige un término y el listado por libreta corta
// en 500 sin decirlo: ninguno de los dos sirve para llevarse la agenda entera.
func (h *Handlers) ListAllContacts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	contacts, err := h.store.ListOwnerContactsPage(r.Context(), subFromCtx(r), limit, offset)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if contacts == nil {
		contacts = []domain.Contact{}
	}
	out := map[string]any{"contacts": contacts}
	// Solo hay siguiente página si esta vino llena: pedir una página vacía de
	// más es una consulta a la base de datos por cada exportación del mundo.
	if len(contacts) == limit {
		out["next_offset"] = offset + limit
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// SearchContacts busca contactos del usuario (?q=&address_book_id=&limit=).
// Owner-scoped en el store: nunca sirve contactos de otros usuarios del tenant.
func (h *Handlers) SearchContacts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Search parameter 'q' is required."))
		return
	}
	var bookID *uuid.UUID
	if raw := r.URL.Query().Get("address_book_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid address_book_id."))
			return
		}
		bookID = &id
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	contacts, err := h.store.SearchContacts(r.Context(), subFromCtx(r), q, bookID, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if contacts == nil {
		contacts = []domain.Contact{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"contacts": contacts})
}

// ---------------------------------------------------------------------------
// Grupos
// ---------------------------------------------------------------------------

type groupRequest struct {
	Name    string   `json:"name"`
	Members []string `json:"members,omitempty"` // UID de contactos
}

// ListGroups lista los grupos de una libreta.
func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	groups, err := h.store.ListGroups(r.Context(), subFromCtx(r), bookID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if groups == nil {
		groups = []domain.Group{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"groups": groups})
}

// CreateGroup crea un grupo (vCard KIND=group) en una libreta.
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req groupRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	g := domain.Group{
		UID:     domain.NewUID(),
		Name:    strings.TrimSpace(req.Name),
		Members: dedupe(req.Members),
	}
	if g.Name == "" {
		httpx.Fail(w, r, toAPIError(domain.ErrInvalidGroup))
		return
	}
	card, err := domain.EncodeGroup(g, "")
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	saved, err := h.store.CreateGroup(r.Context(), subFromCtx(r), store.UpsertGroup{
		AddressBookID: bookID,
		Group:         g,
		CardData:      card,
		ResourceName:  domain.ResourceName(g.UID),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, saved.ETag)
	httpx.JSON(w, r, http.StatusCreated, saved)
}

// GetGroup devuelve un grupo por ID.
func (h *Handlers) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	g, err := h.store.GetGroup(r.Context(), subFromCtx(r), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, g.ETag)
	httpx.JSON(w, r, http.StatusOK, g)
}

// UpdateGroup reemplaza nombre y miembros de un grupo.
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req groupRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub := subFromCtx(r)
	current, err := h.store.GetGroup(r.Context(), sub, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	g := domain.Group{
		UID:     current.UID,
		Name:    strings.TrimSpace(req.Name),
		Members: dedupe(req.Members),
	}
	if g.Name == "" {
		g.Name = current.Name
	}
	base, _ := h.store.GetCardObjectByResource(r.Context(), current.AddressBookID, domain.ResourceName(current.UID))
	card, err := domain.EncodeGroup(g, base.Data)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	saved, err := h.store.UpdateGroup(r.Context(), sub, id, store.UpsertGroup{
		AddressBookID: current.AddressBookID,
		Group:         g,
		CardData:      card,
		ResourceName:  base.ResourceName,
	}, ifMatchFromHeader(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, saved.ETag)
	httpx.JSON(w, r, http.StatusOK, saved)
}

// DeleteGroup borra un grupo por ID.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteGroup(r.Context(), subFromCtx(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setETag fija la cabecera ETag fuerte del recurso.
func setETag(w http.ResponseWriter, etag uuid.UUID) {
	w.Header().Set("ETag", `"`+etag.String()+`"`)
}

// parseDate acepta RFC3339 o YYYY-MM-DD.
func parseDate(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{"2006-01-02", time.RFC3339, "20060102"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("fecha no parseable")
}

func emailType(t string) domain.EmailType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "work":
		return domain.EmailWork
	case "other":
		return domain.EmailOther
	default:
		return domain.EmailHome
	}
}

func phoneType(t string) domain.PhoneType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "home":
		return domain.PhoneHome
	case "work":
		return domain.PhoneWork
	case "fax":
		return domain.PhoneFax
	case "other":
		return domain.PhoneOther
	default:
		return domain.PhoneCell
	}
}

func addressType(t string) domain.AddressType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "work":
		return domain.AddressWork
	case "other":
		return domain.AddressOther
	default:
		return domain.AddressHome
	}
}

// ensureOnePrimaryEmails marca el primero como principal si ninguno lo es.
func ensureOnePrimaryEmails(emails []domain.Email) {
	for i := range emails {
		if emails[i].IsPrimary {
			return
		}
	}
	if len(emails) > 0 {
		emails[0].IsPrimary = true
	}
}

// ensureOnePrimaryPhones marca el primero como principal si ninguno lo es.
func ensureOnePrimaryPhones(phones []domain.Phone) {
	for i := range phones {
		if phones[i].IsPrimary {
			return
		}
	}
	if len(phones) > 0 {
		phones[0].IsPrimary = true
	}
}

// ensureOnePrimaryAddresses marca la primera como principal si ninguna lo es.
func ensureOnePrimaryAddresses(addrs []domain.Address) {
	for i := range addrs {
		if addrs[i].IsPrimary {
			return
		}
	}
	if len(addrs) > 0 {
		addrs[0].IsPrimary = true
	}
}

// dedupe elimina duplicados y vacíos preservando el orden.
func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
