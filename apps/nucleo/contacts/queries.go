// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func queryError(status int, code string) error { return &api.Error{Status: status, Body: code} }
func pageLimit(value, defaultValue int) int {
	if value < 1 || value > 500 {
		return defaultValue
	}
	return value
}

func (s *Snapshot) VisibleBooks() ([]api.AddressBook, error) {
	books, err := s.libros()
	if err != nil {
		return nil, err
	}
	result := make([]api.AddressBook, 0, len(books))
	for _, book := range books {
		if book.DeletedAt == nil {
			result = append(result, book.AddressBook)
		}
	}
	return result, nil
}

func (s *Snapshot) visiblePeople() ([]ContactView, error) {
	result := []ContactView{}
	err := s.cadaVisible(func(person *ContactView) bool {
		result = append(result, copiarContacto(*person))
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// cadaVisible llama a fn con cada contacto que se ve (ni en la papelera ni en una
// libreta borrada), en el orden de Contacts, hasta que fn devuelve false. Son los
// guardados de la instantánea (vistas.go): SOLO para leer; lo que salga fuera, con
// copiarContacto.
func (s *Snapshot) cadaVisible(fn func(*ContactView) bool) error {
	people, err := s.contactos()
	if err != nil {
		return err
	}
	for i := range people {
		if !people[i].BookDeleted && people[i].DeletedAt == nil && !fn(&people[i]) {
			break
		}
	}
	return nil
}

// photoContact resolves the legacy UID-only photo API without selecting an
// arbitrary contact when the same UID occurs in more than one address book.
func (s *Snapshot) photoContact(uid string) (ContactView, error) {
	uid = card.CleanUID(uid)
	if uid == "" {
		return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	var found *ContactView
	ambiguous := false
	err := s.cadaVisible(func(person *ContactView) bool {
		if card.CleanUID(person.UID) != uid {
			return true
		}
		if found != nil {
			ambiguous = true
			return false
		}
		found = person
		return true
	})
	if err != nil {
		return ContactView{}, err
	}
	if ambiguous {
		return ContactView{}, queryError(http.StatusConflict, "private_contact_uid_ambiguous")
	}
	if found == nil {
		return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	return copiarContacto(*found), nil
}

func (s *Snapshot) QueryAll(limit, offset int) ([]api.Contact, int, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return nil, 0, err
	}
	result, next := paginar(people, contactoDeVista, limit, offset)
	return result, next, nil
}

// ContactosVisibles son todos los contactos de QueryAll, en su orden, de una vez: para
// quien lee la libreta entera (el lector entre apps). Por páginas, cada una rehace la
// proyección de todos (descodificar cada vCard y ordenar), y con 20.000 contactos son
// cuarenta veces.
func (s *Snapshot) ContactosVisibles() ([]api.Contact, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return nil, err
	}
	out := make([]api.Contact, len(people))
	for i := range people {
		out[i] = people[i].Contact
	}
	return out, nil
}

// PaginarEn es la página de QueryAll (`All` del repositorio) sobre una lista ya leída,
// en su orden: hasta limit desde offset (con los mismos topes) y el desplazamiento de la
// siguiente, o 0 si no hay más. Es para quien lee la libreta entera una vez y la
// recorre sin volver al almacén (el lector entre apps, Mail).
func PaginarEn(lista []api.Contact, limit, offset int) ([]api.Contact, int) {
	return paginar(lista, func(c *api.Contact) *api.Contact { return c }, limit, offset)
}

func contactoDeVista(v *ContactView) *api.Contact { return &v.Contact }

func paginar[T any](lista []T, contacto func(*T) *api.Contact, limit, offset int) ([]api.Contact, int) {
	limit = pageLimit(limit, 100)
	offset = max(0, offset)
	result := []api.Contact{}
	if offset >= len(lista) {
		return result, 0
	}
	end := min(len(lista), offset+limit)
	for i := offset; i < end; i++ {
		result = append(result, *contacto(&lista[i]))
	}
	if end == len(lista) {
		end = 0
	}
	return result, end
}

func (s *Snapshot) FindContact(id string) (ContactView, error) {
	var found *ContactView
	err := s.cadaVisible(func(person *ContactView) bool {
		if person.ID == id {
			found = person
		}
		return found == nil
	})
	if err != nil {
		return ContactView{}, err
	}
	if found == nil {
		return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	return copiarContacto(*found), nil
}

func (s *Snapshot) QuerySearch(text string, limit int) ([]api.Contact, error) {
	if buscar.Plegar(strings.TrimSpace(text)) == "" {
		return nil, queryError(http.StatusBadRequest, "contact_query_required")
	}
	people, err := s.visiblePeople()
	if err != nil {
		return nil, err
	}
	return buscarEn(people, contactoDeVista, func(i int) string { return textoBuscable(&people[i].Contact) }, text, limit)
}

// BuscarEn es la búsqueda de QuerySearch (`Search` del repositorio) sobre una lista ya
// leída, en su orden: nombre, apellidos, organización, apodo, nota, correos y teléfonos,
// sin acentos ni mayúsculas, hasta limit (con los mismos topes). Un texto vacío es el
// mismo error. Es para quien lee la libreta entera una vez y busca en ella sin volver al
// almacén a cada letra (Mail, con el lector entre apps).
func BuscarEn(lista []api.Contact, text string, limit int) ([]api.Contact, error) {
	if buscar.Plegar(strings.TrimSpace(text)) == "" {
		return nil, queryError(http.StatusBadRequest, "contact_query_required")
	}
	return buscarEn(lista, func(c *api.Contact) *api.Contact { return c }, func(i int) string { return textoBuscable(&lista[i]) }, text, limit)
}

// TextosBuscables es, de cada contacto de lista, lo que mira la búsqueda, ya plegado.
// Quien busca muchas veces en la misma lista (la libreta en memoria de Mail y Calendar,
// a cada letra) lo calcula una vez: plegar 20.000 contactos en cada letra eran 100 ms y
// 170 MB de basura cuando nada casaba (contacts/lector, libreta_bench_test.go).
func TextosBuscables(lista []api.Contact) []string {
	out := make([]string, len(lista))
	for i := range lista {
		out[i] = textoBuscable(&lista[i])
	}
	return out
}

// BuscarEnTextos es BuscarEn con los textos de TextosBuscables(lista) ya calculados.
func BuscarEnTextos(lista []api.Contact, textos []string, text string, limit int) ([]api.Contact, error) {
	if buscar.Plegar(strings.TrimSpace(text)) == "" {
		return nil, queryError(http.StatusBadRequest, "contact_query_required")
	}
	if len(textos) != len(lista) {
		textos = TextosBuscables(lista)
	}
	return buscarEn(lista, func(c *api.Contact) *api.Contact { return c }, func(i int) string { return textos[i] }, text, limit)
}

// textoBuscable es lo que mira la búsqueda de un contacto, sin acentos ni mayúsculas.
func textoBuscable(person *api.Contact) string {
	fields := make([]string, 0, 6+len(person.Emails)+len(person.Phones))
	fields = append(fields, person.FormattedName, person.FamilyName, person.GivenName, person.Organization, person.Nickname, person.Note)
	for _, email := range person.Emails {
		fields = append(fields, email.Email)
	}
	for _, phone := range person.Phones {
		fields = append(fields, phone.Phone)
	}
	return buscar.Plegar(strings.Join(fields, "\n"))
}

func buscarEn[T any](lista []T, contacto func(*T) *api.Contact, texto func(int) string, text string, limit int) ([]api.Contact, error) {
	query := buscar.Plegar(strings.TrimSpace(text))
	result := []api.Contact{}
	limit = pageLimit(limit, 50)
	for i := range lista {
		if strings.Contains(texto(i), query) {
			result = append(result, *contacto(&lista[i]))
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

// QueryBook pins pagination to the identity, book and complete current visible
// projection. A concurrent change returns a conflict instead of a successful
// partial inventory. The cursor contains only an ID and a public fingerprint.
func (s *Snapshot) QueryBook(book, cursor string) (api.ContactPage, error) {
	books, err := s.VisibleBooks()
	if err != nil {
		return api.ContactPage{}, err
	}
	if !slices.ContainsFunc(books, func(value api.AddressBook) bool { return value.ID == book }) {
		return api.ContactPage{}, queryError(http.StatusNotFound, "private_address_book_not_found")
	}
	people, err := s.visiblePeople()
	if err != nil {
		return api.ContactPage{}, err
	}
	selected := []api.Contact{}
	for _, person := range people {
		if person.AddressBookID == book {
			selected = append(selected, person.Contact)
		}
	}
	fingerprint, err := bookFingerprint(s.Scope(), book, selected)
	if err != nil {
		return api.ContactPage{}, err
	}
	start := 0
	if cursor != "" {
		id, digest, found := strings.Cut(cursor, ".")
		if !found || len(digest) != 64 {
			return api.ContactPage{}, queryError(http.StatusBadRequest, "invalid_contact_cursor")
		}
		if digest != fingerprint {
			return api.ContactPage{}, queryError(http.StatusConflict, "private_contacts_changed")
		}
		anchor := slices.IndexFunc(selected, func(value api.Contact) bool { return value.ID == id })
		if anchor < 0 {
			return api.ContactPage{}, queryError(http.StatusConflict, "private_contacts_changed")
		}
		start = anchor + 1
	}
	end := min(len(selected), start+100)
	result := api.ContactPage{Contacts: slices.Clone(selected[start:end])}
	if end < len(selected) {
		result.NextCursor = selected[end-1].ID + "." + fingerprint
	}
	return result, nil
}

func bookFingerprint(scope storage.Scope, book string, people []api.Contact) (string, error) {
	raw, err := json.Marshal(struct {
		Scope  storage.Scope
		Book   string
		People []api.Contact
	}{Scope: scope, Book: book, People: people})
	if err != nil {
		return "", err
	}
	defer clear(raw)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Snapshot) QueryPhoto(uid string) ([]byte, error) {
	person, err := s.photoContact(uid)
	if err != nil {
		return nil, err
	}
	return FotoDeVCard(person.VCard)
}

func (s *Snapshot) QueryFavorites() ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	err := s.cadaVisible(func(person *ContactView) bool {
		if person.Favorite && !seen[person.UID] {
			result = append(result, person.UID)
			seen[person.UID] = true
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Snapshot) QueryGroups(book string) ([]api.ContactGroup, error) {
	books, err := s.VisibleBooks()
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(books, func(value api.AddressBook) bool { return value.ID == book }) {
		return nil, queryError(http.StatusNotFound, "private_address_book_not_found")
	}
	groups, err := s.grupos()
	if err != nil {
		return nil, err
	}
	result := []api.ContactGroup{}
	for _, group := range groups {
		if group.AddressBookID == book && !group.BookDeleted && group.DeletedAt == nil {
			value := group.ContactGroup
			value.Members = slices.Clone(value.Members)
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Snapshot) QueryTrash(limit int) ([]api.TrashItem, error) {
	// Los guardados, solo para leer: lo que sale son copias.
	people, err := s.contactos()
	if err != nil {
		return nil, err
	}
	result := []api.TrashItem{}
	for _, person := range people {
		if person.BookDeleted || person.DeletedAt == nil {
			continue
		}
		result = append(result, api.TrashItem{ID: person.ID, UID: person.UID, FormattedName: person.FormattedName, Organization: person.Organization, PrimaryEmail: CorreoPrincipal(person.Emails), DeletedAt: *person.DeletedAt})
	}
	slices.SortFunc(result, func(a, b api.TrashItem) int {
		if n := b.DeletedAt.Compare(a.DeletedAt); n != 0 {
			return n
		}
		return strings.Compare(b.ID, a.ID)
	})
	return result[:min(len(result), pageLimit(limit, 200))], nil
}

func (s *Snapshot) QueryCSV() (string, []byte, error) {
	contacts := []api.Contact{}
	err := s.cadaVisible(func(person *ContactView) bool {
		contacts = append(contacts, person.Contact)
		return true
	})
	if err != nil {
		return "", nil, err
	}
	return CSVDeContactos(contacts)
}
