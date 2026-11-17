// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func contactInput(in api.ContactInput) (card.Contact, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return card.Contact{}, err
	}
	defer clear(raw)
	var value struct {
		card.Contact
		BirthdayText    string `json:"birthday"`
		AnniversaryText string `json:"anniversary"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return card.Contact{}, queryError(http.StatusBadRequest, "invalid_contact")
	}
	if value.BirthdayText != "" {
		birthday, err := time.Parse(time.DateOnly, value.BirthdayText)
		if err != nil {
			return card.Contact{}, queryError(http.StatusBadRequest, "invalid_contact_birthday")
		}
		value.Birthday = &birthday
	}
	if value.AnniversaryText != "" {
		anniversary, err := time.Parse(time.DateOnly, value.AnniversaryText)
		if err != nil {
			return card.Contact{}, queryError(http.StatusBadRequest, "invalid_contact_anniversary")
		}
		value.Anniversary = &anniversary
	}
	return value.Contact, nil
}

func (r *Repository) Create(ctx context.Context, book string, in api.ContactInput) (api.Contact, error) {
	change, err := r.mutateAggregate(ctx, "create-contact", struct {
		Book  string
		Input api.ContactInput
	}{book, in}, func(snapshot *Snapshot) (Aggregate, error) {
		person, err := contactInput(in)
		if err != nil {
			return Aggregate{}, err
		}
		person.UID = card.NewUID()
		return snapshot.NewPerson(book, person, "", time.Now().UTC())
	})
	if err != nil {
		return api.Contact{}, err
	}
	return r.Contact(ctx, change.ID)
}

func (r *Repository) ImportCard(ctx context.Context, book, data string) (api.Contact, error) {
	change, err := r.mutateAggregate(ctx, "import-card", struct{ Book, Data string }{book, data}, func(snapshot *Snapshot) (Aggregate, error) {
		person, canonical, err := card.ImportCard(data)
		if err != nil {
			return Aggregate{}, err
		}
		return snapshot.NewPerson(book, person, canonical, time.Now().UTC())
	})
	if err != nil {
		return api.Contact{}, err
	}
	return r.Contact(ctx, change.ID)
}

func (s *Snapshot) NewPerson(book string, person card.Contact, base string, now time.Time) (Aggregate, error) {
	books, err := s.VisibleBooks()
	if err != nil {
		return Aggregate{}, err
	}
	found := false
	for _, value := range books {
		if value.ID == book {
			found = true
			break
		}
	}
	if !found {
		return Aggregate{}, queryError(http.StatusNotFound, "private_address_book_not_found")
	}
	people, err := s.visiblePeople()
	if err != nil {
		return Aggregate{}, err
	}
	for _, value := range people {
		if value.AddressBookID == book && value.UID == person.UID {
			return Aggregate{}, queryError(http.StatusConflict, "contact_uid_exists")
		}
	}
	person.ID, person.ObjectID = uuid.New(), uuid.New()
	person.AddressBookID = uuid.MustParse(book)
	person.Tenant = s.Scope().Tenant
	person.CreatedAt, person.UpdatedAt = now, now
	person.ETag = uuid.New()
	data, err := card.EncodeContact(person, base)
	if err != nil {
		return Aggregate{}, err
	}
	primary, err := newDomainRow(s.Scope(), "contact", person.ID.String(), map[string]any{"address_book_id": book, "object_id": person.ObjectID.String(), "uid": person.UID, "created_at": now, "updated_at": now, "deleted_at": nil, "is_favorite": false})
	if err != nil {
		return Aggregate{}, err
	}
	object, err := newDomainRow(s.Scope(), "card", person.ObjectID.String(), map[string]any{"address_book_id": book, "uid": person.UID, "resource_name": card.ResourceName(person.UID), "kind": "individual", "etag": person.ETag.String(), "data": data, "size_bytes": len(data), "has_photo": card.HasEmbeddedPhoto(data), "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	change := Aggregate{Version: 1, Scope: s.Scope(), Kind: "contact_state", ID: person.ID.String(), Rows: []Row{primary, object}}
	return s.writePerson(change, person, data, now)
}

func (r *Repository) Update(ctx context.Context, id string, in api.ContactInput, etag string) (api.Contact, error) {
	_, err := r.mutateAggregate(ctx, "update-contact", struct {
		ID, ETag string
		Input    api.ContactInput
	}{id, etag, in}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.UpdatePerson(id, in, etag, time.Now().UTC())
	})
	if err != nil {
		return api.Contact{}, err
	}
	return r.Contact(ctx, id)
}

func (s *Snapshot) UpdatePerson(id string, in api.ContactInput, etag string, now time.Time) (Aggregate, error) {
	existing, err := s.FindContact(id)
	if err != nil {
		return Aggregate{}, err
	}
	if etag != "" && strings.Trim(etag, "\"") != existing.ETag {
		return Aggregate{}, queryError(http.StatusPreconditionFailed, "contact_etag_changed")
	}
	person, err := contactInput(in)
	if err != nil {
		return Aggregate{}, err
	}
	person.ID, person.ObjectID = uuid.MustParse(id), uuid.MustParse(existing.ObjectID)
	person.AddressBookID = uuid.MustParse(existing.AddressBookID)
	person.Tenant = s.Scope().Tenant
	person.UID = existing.UID
	person.ETag = uuid.New()
	person.UpdatedAt = now
	data, err := card.EncodeContact(person, existing.VCard)
	if err != nil {
		return Aggregate{}, err
	}
	change, err := editAggregate(s, "contact_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	return s.writePerson(change, person, data, now)
}

func (s *Snapshot) writePerson(change Aggregate, person card.Contact, data string, now time.Time) (Aggregate, error) {
	old := change.Rows
	used := map[string]bool{}
	result := make([]Row, 0, 2+len(person.Emails)+len(person.Phones))
	for _, row := range old {
		switch row.Kind {
		case "contact":
			updated, err := contactProjection(row, person)
			if err != nil {
				return Aggregate{}, err
			}
			updated, err = patchRow(updated, map[string]any{"updated_at": now})
			if err != nil {
				return Aggregate{}, err
			}
			result = append(result, updated)
		case "card":
			updated, err := patchRow(row, map[string]any{"data": data, "etag": person.ETag.String(), "size_bytes": len(data), "has_photo": card.HasEmbeddedPhoto(data), "updated_at": now})
			if err != nil {
				return Aggregate{}, err
			}
			result = append(result, updated)
		}
	}
	for i, email := range person.Emails {
		row, err := s.channelRow(old, used, "email", person.ID.String(), i, map[string]any{"email": email.Email, "type_label": string(email.Type), "is_primary": email.IsPrimary, "position": i})
		if err != nil {
			return Aggregate{}, err
		}
		result = append(result, row)
	}
	for i, phone := range person.Phones {
		row, err := s.channelRow(old, used, "phone", person.ID.String(), i, map[string]any{"phone": phone.Phone, "type_label": string(phone.Type), "is_primary": phone.IsPrimary, "position": i})
		if err != nil {
			return Aggregate{}, err
		}
		result = append(result, row)
	}
	change.Rows = result
	return change, change.Validate(s.Scope())
}

func contactProjection(row Row, person card.Contact) (Row, error) {
	fields, err := rowFields(row)
	if err != nil {
		return Row{}, err
	}
	inputType := reflect.TypeFor[api.ContactInput]()
	for i := 0; i < inputType.NumField(); i++ {
		key := strings.Split(inputType.Field(i).Tag.Get("json"), ",")[0]
		delete(fields, key)
	}
	raw, err := json.Marshal(person)
	if err != nil {
		return Row{}, err
	}
	defer clear(raw)
	var projection map[string]json.RawMessage
	if json.Unmarshal(raw, &projection) != nil {
		return Row{}, ErrSnapshot
	}
	for key, value := range projection {
		if key == "emails" || key == "phones" || key == "addresses" || key == "etag" || key == "has_photo" || key == "created_at" {
			continue
		}
		fields[key] = value
	}
	if person.Birthday != nil {
		fields["birthday"], _ = json.Marshal(person.Birthday.Format(time.DateOnly))
	}
	if person.Anniversary != nil {
		fields["anniversary"], _ = json.Marshal(person.Anniversary.Format(time.DateOnly))
	}
	row.JSON, err = json.Marshal(fields)
	return row, err
}

func (s *Snapshot) channelRow(old []Row, used map[string]bool, kind, contactID string, position int, fields map[string]any) (Row, error) {
	var positionMatch *Row
	for _, row := range old {
		if row.Kind != kind || used[row.ID] {
			continue
		}
		values, err := rowFields(row)
		if err != nil {
			return Row{}, err
		}
		var value string
		if json.Unmarshal(values[kind], &value) != nil {
			return Row{}, ErrSnapshot
		}
		if value == fields[kind] {
			used[row.ID] = true
			return patchRow(row, fields)
		}
		var prior int
		if json.Unmarshal(values["position"], &prior) != nil {
			return Row{}, ErrSnapshot
		}
		if prior == position {
			candidate := row
			positionMatch = &candidate
		}
	}
	if positionMatch != nil {
		used[positionMatch.ID] = true
		return patchRow(*positionMatch, fields)
	}
	fields["contact_id"] = contactID
	return newDomainRow(s.Scope(), kind, uuid.NewString(), fields)
}
