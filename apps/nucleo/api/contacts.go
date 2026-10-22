// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Contacts es el cliente de Sytue Contacts, escrito CONTRA EL SERVICIO
// (services/contacts/internal/http) y no contra el SDK de la web, que llegó a pedir
// rutas que el servicio no tiene.
type Contacts struct{ c *Client }

// NewContacts crea el cliente sobre la raíz del servicio (ServiceURL(apex, "contacts")).
func NewContacts(servicio string, s Session, h *http.Client) *Contacts {
	return &Contacts{c: New(servicio+"/api/v1", s, h)}
}

// AddressBook es una libreta de direcciones.
type AddressBook struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default"`
}

// ContactEmail es un correo de un contacto.
type ContactEmail struct {
	Email     string `json:"email"`
	Type      string `json:"type,omitempty"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

// ContactPhone es un teléfono de un contacto.
type ContactPhone struct {
	Phone     string `json:"phone"`
	Type      string `json:"type,omitempty"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

// ContactAddress es una dirección postal.
type ContactAddress struct {
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

// Line es la dirección en una línea, con lo que tenga.
func (a ContactAddress) Line() string {
	partes := make([]string, 0, 5)
	for _, p := range []string{a.Street, a.Extended, strings.TrimSpace(a.PostalCode + " " + a.Locality), a.Region, a.Country} {
		if p != "" {
			partes = append(partes, p)
		}
	}
	return strings.Join(partes, ", ")
}

// ContactInput es lo que se manda al crear o actualizar. El PUT SUSTITUYE el contacto
// entero —lo que no vaya aquí se pierde—, así que se parte siempre de Contact.Input.
type ContactInput struct {
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
	Birthday        string           `json:"birthday,omitempty"` // YYYY-MM-DD
	PhotoURL        string           `json:"photo_url,omitempty"`
	Emails          []ContactEmail   `json:"emails,omitempty"`
	Phones          []ContactPhone   `json:"phones,omitempty"`
	Addresses       []ContactAddress `json:"addresses,omitempty"`
}

// Contact es un contacto tal y como lo devuelve el servicio.
type Contact struct {
	ID              string           `json:"id"`
	AddressBookID   string           `json:"address_book_id"`
	UID             string           `json:"uid"`
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
	Birthday        *time.Time       `json:"birthday,omitempty"`
	PhotoURL        string           `json:"photo_url,omitempty"`
	HasPhoto        bool             `json:"has_photo"` // lleva foto embebida: la que da Photo
	Emails          []ContactEmail   `json:"emails,omitempty"`
	Phones          []ContactPhone   `json:"phones,omitempty"`
	Addresses       []ContactAddress `json:"addresses,omitempty"`
	ETag            string           `json:"etag"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// DisplayName es `domain.Contact.DisplayName` del servicio: FN, o nombre y apellido, o
// la organización.
func (c Contact) DisplayName() string {
	if fn := strings.TrimSpace(c.FormattedName); fn != "" {
		return fn
	}
	if n := strings.TrimSpace(strings.TrimSpace(c.GivenName) + " " + strings.TrimSpace(c.FamilyName)); n != "" {
		return n
	}
	return strings.TrimSpace(c.Organization)
}

// Input es el contacto ENTERO en forma de petición: el punto de partida de toda edición.
func (c Contact) Input() ContactInput {
	in := ContactInput{
		FormattedName: c.FormattedName, FamilyName: c.FamilyName, GivenName: c.GivenName,
		AdditionalName: c.AdditionalName, HonorificPrefix: c.HonorificPrefix, HonorificSuffix: c.HonorificSuffix,
		Organization: c.Organization, Title: c.Title, Nickname: c.Nickname, Note: c.Note, PhotoURL: c.PhotoURL,
		Emails: c.Emails, Phones: c.Phones, Addresses: c.Addresses,
	}
	if c.Birthday != nil {
		in.Birthday = c.Birthday.Format(time.DateOnly)
	}
	return in
}

// AddressBooks lista las libretas; el servicio crea la predeterminada si no hay ninguna.
func (c *Contacts) AddressBooks(ctx context.Context) ([]AddressBook, error) {
	r, err := JSON[struct {
		AddressBooks []AddressBook `json:"address_books"`
	}](ctx, c.c, Request{Path: "/address-books"})
	return r.AddressBooks, err
}

// All devuelve una página de TODOS los contactos de la persona. siguiente es el offset
// de la página que viene, o 0 si esta era la última.
func (c *Contacts) All(ctx context.Context, limite, offset int) (contactos []Contact, siguiente int, err error) {
	r, err := JSON[struct {
		Contacts   []Contact `json:"contacts"`
		NextOffset int       `json:"next_offset"`
	}](ctx, c.c, Request{Path: "/contacts/all", Query: url.Values{
		"limit": {strconv.Itoa(limite)}, "offset": {strconv.Itoa(offset)},
	}})
	return r.Contacts, r.NextOffset, err
}

// Search busca por nombre, correo u organización. q no puede ir vacío: el servicio
// contesta 400.
func (c *Contacts) Search(ctx context.Context, q string, limite int) ([]Contact, error) {
	r, err := JSON[struct {
		Contacts []Contact `json:"contacts"`
	}](ctx, c.c, Request{Path: "/contacts", Query: url.Values{"q": {q}, "limit": {strconv.Itoa(limite)}}})
	return r.Contacts, err
}

// Create da de alta un contacto en una libreta.
func (c *Contacts) Create(ctx context.Context, libreta string, in ContactInput) (Contact, error) {
	return PostJSON[Contact](ctx, c.c, "/address-books/"+url.PathEscape(libreta)+"/contacts", in)
}

// ImportCard da de alta una tarjeta de fuera ENTERA (`text/vcard`): el servicio conserva
// lo que la ficha no enseña —la foto, los X-* de otros clientes— y le da un UID nuevo.
// Es lo que hace la web mandando la tarjeta como `baseVcard` a su BFF.
func (c *Contacts) ImportCard(ctx context.Context, libreta, tarjeta string) (Contact, error) {
	return JSON[Contact](ctx, c.c, Request{
		Method: http.MethodPost, Path: "/address-books/" + url.PathEscape(libreta) + "/contacts",
		Header: http.Header{"Content-Type": {"text/vcard; charset=utf-8"}}, Body: []byte(tarjeta),
	})
}

// Update sustituye un contacto. Con etag, el servicio rechaza (412) si alguien lo ha
// cambiado entretanto —otro dispositivo, una sincronización CardDAV— en vez de pisarlo.
func (c *Contacts) Update(ctx context.Context, id string, in ContactInput, etag string) (Contact, error) {
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return Contact{}, err
	}
	cabeceras := http.Header{"Content-Type": {"application/json"}}
	if etag != "" {
		cabeceras.Set("If-Match", `"`+etag+`"`)
	}
	return JSON[Contact](ctx, c.c, Request{Method: http.MethodPut, Path: "/contacts/" + url.PathEscape(id), Header: cabeceras, Body: cuerpo})
}

// Contact es un contacto por su id, de cualquier libreta: lo que resuelve el enlace
// canónico `/contacts/{id}` del buscador de la suite.
func (c *Contacts) Contact(ctx context.Context, id string) (Contact, error) {
	return JSON[Contact](ctx, c.c, Request{Path: "/contacts/" + url.PathEscape(id)})
}

// Delete manda un contacto a la papelera.
func (c *Contacts) Delete(ctx context.Context, id string) error {
	return c.borrar(ctx, "/contacts/"+url.PathEscape(id))
}

// borrar es un DELETE sin cuerpo en ninguno de los dos sentidos.
func (c *Contacts) borrar(ctx context.Context, ruta string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodDelete, Path: ruta})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// InBook lista los contactos de UNA libreta, que es lo que enseña la lista: cambiar de
// libreta en el menú cambia lo que se ve. All() es otra cosa —toda la agenda, paginada,
// para llevársela— y no sabe de qué libreta viene cada uno.
func (c *Contacts) InBook(ctx context.Context, libreta string) ([]Contact, error) {
	r, err := JSON[struct {
		Contacts []Contact `json:"contacts"`
	}](ctx, c.c, Request{
		// Sin `limit` el servicio da 100, y una libreta más larga se veía cortada sin
		// avisar. 500 es su tope.
		Path: "/address-books/" + url.PathEscape(libreta) + "/contacts", Query: url.Values{"limit": {"500"}},
	})
	return r.Contacts, err
}

// VCard es la vCard 4.0 de un contacto, tal y como la sirve el servicio
// (`GET /contacts/{id}/vcard`, `text/vcard`): lo que «Exportar» deja en la carpeta de
// descargas. Es la canónica del servicio y no una armada aquí, así que un campo que
// esta app no pinta —la foto, un X-* de otro cliente— viaja igual.
func (c *Contacts) VCard(ctx context.Context, id string) ([]byte, error) {
	resp, err := c.c.Do(ctx, Request{
		Path:   "/contacts/" + url.PathEscape(id) + "/vcard",
		Header: http.Header{"Accept": {"text/vcard"}},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// CleanUID es el UID desnudo: el servicio guarda y devuelve los favoritos sin el
// prefijo `urn:uuid:`, pero una vCard importada puede traerlo. Sin quitarlo, el cruce
// entre la lista y los destacados no encuentra nada.
func CleanUID(uid string) string {
	if len(uid) >= 9 && strings.EqualFold(uid[:9], "urn:uuid:") {
		return uid[9:]
	}
	return uid
}

// Photo son los bytes de la foto de un contacto (la PHOTO embebida de su vCard), por
// UID. Sin foto, el servicio contesta 404 y aquí sale `nil, nil`: el avatar se queda
// con las iniciales, que es lo que hace la web cuando su `<img>` no carga.
func (c *Contacts) Photo(ctx context.Context, uid string) ([]byte, error) {
	resp, err := c.c.Do(ctx, Request{Path: "/contacts/photo", Query: url.Values{"uid": {CleanUID(uid)}}})
	if err != nil {
		var e *Error
		if errors.As(err, &e) && e.Status == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// SetPhoto pone la foto de un contacto: un data URI raster (JPEG, PNG, WebP o GIF de
// hasta 256 KiB), que el servicio valida e incrusta en la PHOTO de su vCard.
func (c *Contacts) SetPhoto(ctx context.Context, uid, dataURI string) error {
	cuerpo, err := json.Marshal(map[string]string{"uid": CleanUID(uid), "data": dataURI})
	if err != nil {
		return err
	}
	resp, err := c.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/contacts/photo", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// RemovePhoto quita la foto de un contacto.
func (c *Contacts) RemovePhoto(ctx context.Context, uid string) error {
	resp, err := c.c.Do(ctx, Request{
		Method: http.MethodDelete, Path: "/contacts/photo", Query: url.Values{"uid": {CleanUID(uid)}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Favorites son los UID de los contactos destacados de la persona. Son SUYOS, no de la
// libreta: la lista los cruza por UID para pintar la estrella y para filtrar.
func (c *Contacts) Favorites(ctx context.Context) ([]string, error) {
	r, err := JSON[struct {
		UIDs []string `json:"uids"`
	}](ctx, c.c, Request{Path: "/favorites"})
	return r.UIDs, err
}

// SetFavorite destaca un contacto, o le quita el destacado.
func (c *Contacts) SetFavorite(ctx context.Context, uid string, destacado bool) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/favorites", map[string]any{"uid": uid, "favorite": destacado})
	return err
}

// BatchFavorite destaca o desdestaca varios de una vez (hasta 1000).
func (c *Contacts) BatchFavorite(ctx context.Context, uids []string, destacado bool) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/contacts/batch/favorite", map[string]any{"uids": uids, "favorite": destacado})
	return err
}

// BatchTrash manda varios a la papelera de una vez.
func (c *Contacts) BatchTrash(ctx context.Context, uids []string) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/contacts/batch/trash", map[string]any{"uids": uids})
	return err
}

// TrashItem es un contacto borrado, tal y como lo lista la papelera.
type TrashItem struct {
	ID            string    `json:"id"`
	UID           string    `json:"uid"`
	FormattedName string    `json:"formatted_name"`
	Organization  string    `json:"organization,omitempty"`
	PrimaryEmail  string    `json:"primary_email,omitempty"`
	DeletedAt     time.Time `json:"deleted_at"`
}

// Trash lista los contactos de la papelera, el más reciente primero.
func (c *Contacts) Trash(ctx context.Context, limite int) ([]TrashItem, error) {
	r, err := JSON[struct {
		Items []TrashItem `json:"items"`
	}](ctx, c.c, Request{Path: "/trash", Query: url.Values{"limit": {strconv.Itoa(limite)}}})
	return r.Items, err
}

// Restore saca un contacto de la papelera.
func (c *Contacts) Restore(ctx context.Context, id string) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/contacts/"+url.PathEscape(id)+"/restore", struct{}{})
	return err
}

// Purge elimina un contacto de la papelera para siempre.
func (c *Contacts) Purge(ctx context.Context, id string) error {
	return c.borrar(ctx, "/trash/"+url.PathEscape(id))
}

// ContactGroup es una etiqueta: un grupo de la libreta con sus miembros por UID.
type ContactGroup struct {
	ID            string   `json:"id"`
	AddressBookID string   `json:"address_book_id"`
	UID           string   `json:"uid"`
	Name          string   `json:"name"`
	Members       []string `json:"members"`
}

// Groups lista las etiquetas de una libreta.
func (c *Contacts) Groups(ctx context.Context, libreta string) ([]ContactGroup, error) {
	r, err := JSON[struct {
		Groups []ContactGroup `json:"groups"`
	}](ctx, c.c, Request{Path: "/address-books/" + url.PathEscape(libreta) + "/groups"})
	return r.Groups, err
}

// CreateGroup crea una etiqueta vacía en una libreta.
func (c *Contacts) CreateGroup(ctx context.Context, libreta, nombre string) (ContactGroup, error) {
	return PostJSON[ContactGroup](ctx, c.c, "/address-books/"+url.PathEscape(libreta)+"/groups",
		map[string]any{"name": nombre})
}

// RenameGroup cambia el nombre de una etiqueta. El PUT sustituye también la pertenencia,
// así que hay que reenviar SUS miembros: mandar una lista vacía la dejaría sin nadie.
func (c *Contacts) RenameGroup(ctx context.Context, id, nombre string, miembros []string) (ContactGroup, error) {
	cuerpo, err := json.Marshal(map[string]any{"name": nombre, "members": miembros})
	if err != nil {
		return ContactGroup{}, err
	}
	return JSON[ContactGroup](ctx, c.c, Request{
		Method: http.MethodPut, Path: "/groups/" + url.PathEscape(id), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteGroup elimina una etiqueta. Los contactos no se tocan: solo desaparece ella.
func (c *Contacts) DeleteGroup(ctx context.Context, id string) error {
	return c.borrar(ctx, "/groups/"+url.PathEscape(id))
}

// BatchGroupMembers añade (o quita) varios contactos, por UID, a una etiqueta de una vez.
func (c *Contacts) BatchGroupMembers(ctx context.Context, id string, uids []string, anadir bool) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/groups/"+url.PathEscape(id)+"/members",
		map[string]any{"uids": uids, "add": anadir})
	return err
}

// DeleteAddressBook elimina una libreta y sus contactos. La predeterminada no se puede:
// el servicio la protege.
func (c *Contacts) DeleteAddressBook(ctx context.Context, id string) error {
	return c.borrar(ctx, "/address-books/"+url.PathEscape(id))
}

// ImportCSV importa un CSV de Google Contacts o de Outlook a una libreta, en UNA
// petición: el servicio detecta el dialecto, se salta las filas inválidas y no repite
// correos que ya estén. Devuelve cuántos entraron y cuántos se omitieron.
func (c *Contacts) ImportCSV(ctx context.Context, libreta string, csv []byte) (importados, omitidos int, err error) {
	r, err := JSON[struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
	}](ctx, c.c, Request{
		Method: http.MethodPost, Path: "/address-books/" + url.PathEscape(libreta) + "/import-csv",
		Header: http.Header{"Content-Type": {"text/csv"}}, Body: csv,
	})
	return r.Imported, r.Skipped, err
}

// ExportCSV es la agenda ENTERA de la persona en CSV (formato Google Contacts), no solo
// la libreta que se ve: es lo que baja «Exportar CSV» en la web. `nombre` es el que da
// el servicio en `Content-Disposition`, que es el que pone el navegador al descargarlo.
func (c *Contacts) ExportCSV(ctx context.Context) (nombre string, csv []byte, err error) {
	resp, err := c.c.Do(ctx, Request{Path: "/export-csv", Header: http.Header{"Accept": {"text/csv"}}})
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if _, ps, e := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); e == nil {
		nombre = ps["filename"]
	}
	csv, err = io.ReadAll(resp.Body)
	return nombre, csv, err
}

// CreateAddressBook crea una libreta. El servicio usa `name` como segmento de ruta
// CardDAV y `display_name` como rótulo; con uno solo, el rótulo hace de los dos.
func (c *Contacts) CreateAddressBook(ctx context.Context, nombre string) (AddressBook, error) {
	return PostJSON[AddressBook](ctx, c.c, "/address-books",
		map[string]any{"name": RutaDeLibreta(nombre), "display_name": nombre})
}

// CreateAddressBookAt crea una libreta en una ruta CardDAV fija —la de «Organización»
// es `organizacion` en cualquier idioma—, con su rótulo y su color.
func (c *Contacts) CreateAddressBookAt(ctx context.Context, ruta, rotulo, color string) (AddressBook, error) {
	return PostJSON[AddressBook](ctx, c.c, "/address-books",
		map[string]any{"name": ruta, "display_name": rotulo, "color": color})
}

// RenameAddressBook cambia el rótulo de una libreta; su ruta CardDAV no se toca, que es
// por donde la tienen enganchada los teléfonos ya sincronizados.
func (c *Contacts) RenameAddressBook(ctx context.Context, id, nombre string) (AddressBook, error) {
	cuerpo, err := json.Marshal(map[string]any{"display_name": nombre})
	if err != nil {
		return AddressBook{}, err
	}
	return JSON[AddressBook](ctx, c.c, Request{
		Method: http.MethodPatch, Path: "/address-books/" + url.PathEscape(id), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// RutaDeLibreta convierte un rótulo en el segmento de ruta CardDAV que el servicio
// admite (domain.ValidName): sin separadores, sin control y no vacío.
func RutaDeLibreta(nombre string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(nombre)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 { // un rótulo entero en cirílico o en griego se queda sin ruta
		return "libreta-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return b.String()
}
