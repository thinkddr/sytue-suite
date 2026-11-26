// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

// ContactView keeps the original vCard alongside its API projection. Editing a
// managed field can therefore retain embedded photos and unknown properties.
type ContactView struct {
	api.Contact
	ObjectID    string
	VCard       string
	Favorite    bool
	DeletedAt   *time.Time
	BookDeleted bool
}

type BookView struct {
	api.AddressBook
	DeletedAt *time.Time `json:"deleted_at"`
}

type GroupView struct {
	api.ContactGroup
	ObjectID    string
	VCard       string
	DeletedAt   *time.Time
	BookDeleted bool
}

func (s *Snapshot) calcularLibros() ([]BookView, error) {
	books := make([]BookView, 0)
	for _, row := range s.Rows() {
		if row.Kind != "book" {
			continue
		}
		var book BookView
		if json.Unmarshal(row.JSON, &book) != nil {
			return nil, ErrSnapshot
		}
		books = append(books, book)
	}
	slices.SortFunc(books, compararLibros)
	return books, nil
}

func compararLibros(a, b BookView) int {
	return compararClaves(ClaveDeLibreta(a.AddressBook), ClaveDeLibreta(b.AddressBook))
}

// compararClaves compara dos claves de orden campo a campo.
func compararClaves[K ~[2]string | ~[3]string | ~[4]string](a, b K) int {
	for i := range len(a) {
		if n := strings.Compare(a[i], b[i]); n != 0 {
			return n
		}
	}
	return 0
}

type sourceCard struct {
	Data      string     `json:"data"`
	ETag      string     `json:"etag"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type sourceContact struct {
	api.Contact
	BirthdayText    *string    `json:"birthday"`
	AnniversaryText *string    `json:"anniversary"`
	ObjectID        string     `json:"object_id"`
	Favorite        bool       `json:"is_favorite"`
	DeletedAt       *time.Time `json:"deleted_at"`
}

type sourceEmail struct {
	ID        string `json:"id"`
	ContactID string `json:"contact_id"`
	Email     string `json:"email"`
	Type      string `json:"type_label"`
	IsPrimary bool   `json:"is_primary"`
	Position  int    `json:"position"`
}

type sourcePhone struct {
	ID        string `json:"id"`
	ContactID string `json:"contact_id"`
	Phone     string `json:"phone"`
	Type      string `json:"type_label"`
	IsPrimary bool   `json:"is_primary"`
	Position  int    `json:"position"`
}

func (s *Snapshot) deletedBooks() (map[string]bool, error) {
	books, err := s.calcularLibros()
	if err != nil {
		return nil, err
	}
	deleted := make(map[string]bool, len(books))
	for _, book := range books {
		deleted[book.ID] = book.DeletedAt != nil
	}
	return deleted, nil
}

func (s *Snapshot) cards() (map[string]sourceCard, error) {
	cards := map[string]sourceCard{}
	for _, row := range s.Rows() {
		if row.Kind != "card" {
			continue
		}
		var value sourceCard
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, ErrSnapshot
		}
		cards[row.ID] = value
	}
	return cards, nil
}

func (s *Snapshot) calcularContactos() ([]ContactView, error) {
	books, err := s.deletedBooks()
	if err != nil {
		return nil, err
	}
	cards, err := s.cards()
	if err != nil {
		return nil, err
	}
	emails, phones, err := s.channels()
	if err != nil {
		return nil, err
	}
	contacts := make([]ContactView, 0)
	for _, row := range s.Rows() {
		if row.Kind != "contact" {
			continue
		}
		value, err := projectContact(row.JSON, cards, emails, phones, books)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, value)
	}
	claves := make([]claveContacto, len(contacts))
	for i := range contacts {
		claves[i] = claveDe(&contacts[i])
	}
	return ordenarContactos(contacts, claves), nil
}

// claveContacto es por lo que se ordenan los contactos: apellidos, nombre, nombre
// completo e identificador, sin mayúsculas.
type claveContacto [4]string

func claveDe(c *ContactView) claveContacto { return ClaveDeOrden(&c.Contact) }

// ordenarContactos ordena contacts por sus claves (claves[i] es la de contacts[i]).
func ordenarContactos(contacts []ContactView, claves []claveContacto) []ContactView {
	orden := make([]int, len(contacts))
	for i := range orden {
		orden[i] = i
	}
	slices.SortFunc(orden, func(a, b int) int { return compararClaves(claves[a], claves[b]) })
	out := make([]ContactView, len(contacts))
	for i, j := range orden {
		out[i] = contacts[j]
	}
	return out
}

func projectContact(raw []byte, cards map[string]sourceCard, emails map[string][]api.ContactEmail, phones map[string][]api.ContactPhone, books map[string]bool) (ContactView, error) {
	var source sourceContact
	if json.Unmarshal(raw, &source) != nil {
		return ContactView{}, ErrSnapshot
	}
	object, found := cards[source.ObjectID]
	if !found {
		return ContactView{}, ErrSnapshot
	}
	decoded, err := card.DecodeContact(object.Data)
	if err != nil {
		return ContactView{}, err
	}
	value := ContactView{Contact: source.Contact, ObjectID: source.ObjectID, VCard: object.Data, Favorite: source.Favorite, DeletedAt: source.DeletedAt, BookDeleted: books[source.AddressBookID]}
	value.ETag = object.ETag
	value.HasPhoto = card.HasEmbeddedPhoto(object.Data)
	if value.DeletedAt == nil {
		value.DeletedAt = object.DeletedAt
	}
	if source.BirthdayText != nil {
		birthday, err := time.Parse(time.DateOnly, *source.BirthdayText)
		if err != nil {
			return ContactView{}, ErrSnapshot
		}
		value.Birthday = &birthday
	}
	if source.AnniversaryText != nil {
		anniversary, err := time.Parse(time.DateOnly, *source.AnniversaryText)
		if err != nil {
			return ContactView{}, ErrSnapshot
		}
		value.Anniversary = &anniversary
	}
	// Una fila escrita antes de que la proyección entendiera el cumpleaños sin año, los
	// sitios web, las relaciones o el aniversario no los trae, pero la tarjeta sí. Sin
	// leerlos de ella, la edición parte de una entrada sin ellos y EncodeContact borra
	// lo que ya sabe proyectar: se perderían al editar cualquier otro campo.
	if value.Birthday == nil && value.BirthdayNoYear == "" {
		value.BirthdayNoYear = decoded.BirthdayNoYear
	}
	if value.Anniversary == nil {
		value.Anniversary = decoded.Anniversary
	}
	if len(value.URLs) == 0 {
		for _, u := range decoded.URLs {
			value.URLs = append(value.URLs, api.ContactURL{URL: u.URL, Type: u.Type})
		}
	}
	if len(value.Related) == 0 {
		for _, r := range decoded.Related {
			value.Related = append(value.Related, api.ContactRelated{Name: r.Name, Type: r.Type})
		}
	}
	value.Emails = slices.Clone(emails[value.ID])
	value.Phones = slices.Clone(phones[value.ID])
	for _, address := range decoded.Addresses {
		value.Addresses = append(value.Addresses, api.ContactAddress{POBox: address.POBox, Extended: address.Extended, Street: address.Street, Locality: address.Locality, Region: address.Region, PostalCode: address.PostalCode, Country: address.Country, Type: string(address.Type), IsPrimary: address.IsPrimary})
	}
	return value, nil
}

func (s *Snapshot) channels() (map[string][]api.ContactEmail, map[string][]api.ContactPhone, error) {
	emails, phones := []sourceEmail{}, []sourcePhone{}
	for _, row := range s.Rows() {
		switch row.Kind {
		case "email":
			var value sourceEmail
			if json.Unmarshal(row.JSON, &value) != nil {
				return nil, nil, ErrSnapshot
			}
			emails = append(emails, value)
		case "phone":
			var value sourcePhone
			if json.Unmarshal(row.JSON, &value) != nil {
				return nil, nil, ErrSnapshot
			}
			phones = append(phones, value)
		}
	}
	return agruparCorreos(emails), agruparTelefonos(phones), nil
}

// agruparCorreos son los correos de cada contacto, por posición y, a igualdad, por id.
func agruparCorreos(emails []sourceEmail) map[string][]api.ContactEmail {
	slices.SortFunc(emails, func(a, b sourceEmail) int {
		if a.Position != b.Position {
			return a.Position - b.Position
		}
		return strings.Compare(a.ID, b.ID)
	})
	byEmail := map[string][]api.ContactEmail{}
	for _, value := range emails {
		byEmail[value.ContactID] = append(byEmail[value.ContactID], api.ContactEmail{Email: value.Email, Type: value.Type, IsPrimary: value.IsPrimary})
	}
	return byEmail
}

// agruparTelefonos son los teléfonos de cada contacto, por posición y, a igualdad, por id.
func agruparTelefonos(phones []sourcePhone) map[string][]api.ContactPhone {
	slices.SortFunc(phones, func(a, b sourcePhone) int {
		if a.Position != b.Position {
			return a.Position - b.Position
		}
		return strings.Compare(a.ID, b.ID)
	})
	byPhone := map[string][]api.ContactPhone{}
	for _, value := range phones {
		byPhone[value.ContactID] = append(byPhone[value.ContactID], api.ContactPhone{Phone: value.Phone, Type: value.Type, IsPrimary: value.IsPrimary})
	}
	return byPhone
}

func (s *Snapshot) calcularGrupos() ([]GroupView, error) {
	books, err := s.deletedBooks()
	if err != nil {
		return nil, err
	}
	cards, err := s.cards()
	if err != nil {
		return nil, err
	}
	members := map[string][]string{}
	for _, row := range s.Rows() {
		if row.Kind != "member" {
			continue
		}
		var value sourceMember
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, ErrSnapshot
		}
		members[value.GroupID] = append(members[value.GroupID], value.UID)
	}
	groups := make([]GroupView, 0)
	for _, row := range s.Rows() {
		if row.Kind != "group" {
			continue
		}
		var value sourceGroup
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, ErrSnapshot
		}
		group, found := projectGroup(value, cards, members, books)
		if !found {
			return nil, ErrSnapshot
		}
		groups = append(groups, group)
	}
	slices.SortFunc(groups, compararGrupos)
	return groups, nil
}

type sourceMember struct {
	GroupID string `json:"group_id"`
	UID     string `json:"contact_uid"`
}

type sourceGroup struct {
	api.ContactGroup
	ObjectID  string     `json:"object_id"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// projectGroup es la vista de un grupo; false si no está su tarjeta.
func projectGroup(value sourceGroup, cards map[string]sourceCard, members map[string][]string, books map[string]bool) (GroupView, bool) {
	object, found := cards[value.ObjectID]
	if !found {
		return GroupView{}, false
	}
	uids := slices.Clone(members[value.ID])
	slices.Sort(uids)
	value.Members = uids
	if value.DeletedAt == nil {
		value.DeletedAt = object.DeletedAt
	}
	return GroupView{ContactGroup: value.ContactGroup, ObjectID: value.ObjectID, VCard: object.Data, DeletedAt: value.DeletedAt, BookDeleted: books[value.AddressBookID]}, true
}

func compararGrupos(a, b GroupView) int {
	return compararClaves(ClaveDeGrupo(a.ContactGroup), ClaveDeGrupo(b.ContactGroup))
}
