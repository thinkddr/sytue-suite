// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
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
	books, err := s.Books()
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
	people, err := s.Contacts()
	if err != nil {
		return nil, err
	}
	result := make([]ContactView, 0, len(people))
	for _, person := range people {
		if !person.BookDeleted && person.DeletedAt == nil {
			result = append(result, person)
		}
	}
	return result, nil
}

// photoContact resolves the legacy UID-only photo API without selecting an
// arbitrary contact when the same UID occurs in more than one address book.
func (s *Snapshot) photoContact(uid string) (ContactView, error) {
	uid = card.CleanUID(uid)
	if uid == "" {
		return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	people, err := s.visiblePeople()
	if err != nil {
		return ContactView{}, err
	}
	var found ContactView
	matched := false
	for _, person := range people {
		if card.CleanUID(person.UID) != uid {
			continue
		}
		if matched {
			return ContactView{}, queryError(http.StatusConflict, "private_contact_uid_ambiguous")
		}
		found, matched = person, true
	}
	if !matched {
		return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	return found, nil
}

func (s *Snapshot) QueryAll(limit, offset int) ([]api.Contact, int, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return nil, 0, err
	}
	limit = pageLimit(limit, 100)
	offset = max(0, offset)
	result := []api.Contact{}
	if offset >= len(people) {
		return result, 0, nil
	}
	end := min(len(people), offset+limit)
	for _, person := range people[offset:end] {
		result = append(result, person.Contact)
	}
	if end == len(people) {
		end = 0
	}
	return result, end, nil
}

func (s *Snapshot) FindContact(id string) (ContactView, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return ContactView{}, err
	}
	for _, person := range people {
		if person.ID == id {
			return person, nil
		}
	}
	return ContactView{}, queryError(http.StatusNotFound, "private_contact_not_found")
}

func (s *Snapshot) QuerySearch(text string, limit int) ([]api.Contact, error) {
	query := buscar.Plegar(strings.TrimSpace(text))
	if query == "" {
		return nil, queryError(http.StatusBadRequest, "contact_query_required")
	}
	people, err := s.visiblePeople()
	if err != nil {
		return nil, err
	}
	result := []api.Contact{}
	limit = pageLimit(limit, 50)
	for _, person := range people {
		fields := []string{person.FormattedName, person.FamilyName, person.GivenName, person.Organization, person.Nickname, person.Note}
		for _, email := range person.Emails {
			fields = append(fields, email.Email)
		}
		for _, phone := range person.Phones {
			fields = append(fields, phone.Phone)
		}
		if strings.Contains(buscar.Plegar(strings.Join(fields, "\n")), query) {
			result = append(result, person.Contact)
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
	value, err := card.PhotoDataURI(person.VCard)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, queryError(http.StatusNotFound, "private_contact_photo_not_found")
	}
	raw, _, err := card.PhotoBytes(value)
	return raw, err
}

func (s *Snapshot) QueryFavorites() ([]string, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[string]bool{}
	for _, person := range people {
		if person.Favorite && !seen[person.UID] {
			result = append(result, person.UID)
			seen[person.UID] = true
		}
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
	groups, err := s.Groups()
	if err != nil {
		return nil, err
	}
	result := []api.ContactGroup{}
	for _, group := range groups {
		if group.AddressBookID == book && !group.BookDeleted && group.DeletedAt == nil {
			result = append(result, group.ContactGroup)
		}
	}
	return result, nil
}

func (s *Snapshot) QueryTrash(limit int) ([]api.TrashItem, error) {
	people, err := s.Contacts()
	if err != nil {
		return nil, err
	}
	result := []api.TrashItem{}
	for _, person := range people {
		if person.BookDeleted || person.DeletedAt == nil {
			continue
		}
		primary := ""
		for _, email := range person.Emails {
			if primary == "" || email.IsPrimary {
				primary = email.Email
			}
			if email.IsPrimary {
				break
			}
		}
		result = append(result, api.TrashItem{ID: person.ID, UID: person.UID, FormattedName: person.FormattedName, Organization: person.Organization, PrimaryEmail: primary, DeletedAt: *person.DeletedAt})
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
	people, err := s.visiblePeople()
	if err != nil {
		return "", nil, err
	}
	contacts := make([]card.Contact, 0, len(people))
	for _, person := range people {
		raw, err := json.Marshal(person.Contact)
		if err != nil {
			return "", nil, err
		}
		var contact card.Contact
		err = json.Unmarshal(raw, &contact)
		clear(raw)
		if err != nil {
			return "", nil, err
		}
		contacts = append(contacts, contact)
	}
	var output bytes.Buffer
	if err := card.WriteContactsCSV(&output, contacts); err != nil {
		return "", nil, err
	}
	return "contacts.csv", output.Bytes(), nil
}
