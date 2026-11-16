// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func (r *Repository) CreateAddressBook(ctx context.Context, display string) (api.AddressBook, error) {
	return r.CreateAddressBookAt(ctx, api.RutaDeLibreta(display), display, "")
}

func (r *Repository) CreateAddressBookAt(ctx context.Context, name, display, color string) (api.AddressBook, error) {
	input := struct{ Name, Display, Color string }{name, display, color}
	change, err := r.mutateAggregate(ctx, "create-book", input, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.NewBook(name, display, color, time.Now().UTC())
	})
	if err != nil {
		return api.AddressBook{}, err
	}
	return aggregateBook(change)
}

func (s *Snapshot) NewBook(name, display, color string, now time.Time) (Aggregate, error) {
	if name == "" {
		name = "book-" + uuid.NewString()
	}
	name, display = strings.TrimSpace(name), strings.TrimSpace(display)
	if !card.ValidName(name) || display == "" {
		return Aggregate{}, queryError(http.StatusBadRequest, "invalid_address_book_name")
	}
	if color == "" {
		color = "#3B82F6"
	}
	if !validBookColor(color) {
		return Aggregate{}, queryError(http.StatusBadRequest, "invalid_address_book_color")
	}
	books, err := s.VisibleBooks()
	if err != nil {
		return Aggregate{}, err
	}
	for _, book := range books {
		if strings.EqualFold(book.Name, name) {
			return Aggregate{}, queryError(http.StatusConflict, "address_book_name_exists")
		}
	}
	id := uuid.NewString()
	row, err := newDomainRow(s.Scope(), "book", id, map[string]any{"owner_sub": s.Scope().Subject, "name": name, "display_name": display, "description": "", "color": color, "is_default": len(books) == 0, "ctag": uuid.NewString(), "sync_token": 1, "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	result := Aggregate{Version: 1, Scope: s.Scope(), Kind: "book_state", ID: id, Rows: []Row{row}}
	return result, result.Validate(s.Scope())
}

func validBookColor(color string) bool {
	if (len(color) != 7 && len(color) != 9) || color[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(color[1:])
	return err == nil
}

func aggregateBook(change Aggregate) (api.AddressBook, error) {
	if change.Kind != "book_state" || len(change.Rows) != 1 {
		return api.AddressBook{}, ErrSnapshot
	}
	var book api.AddressBook
	if json.Unmarshal(change.Rows[0].JSON, &book) != nil || book.ID != change.ID {
		return api.AddressBook{}, ErrSnapshot
	}
	return book, nil
}

func (r *Repository) RenameAddressBook(ctx context.Context, id, name string) (api.AddressBook, error) {
	change, err := r.mutateAggregate(ctx, "rename-book", struct{ ID, Name string }{id, name}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.RenameBook(id, name, time.Now().UTC())
	})
	if err != nil {
		return api.AddressBook{}, err
	}
	return aggregateBook(change)
}

func (s *Snapshot) RenameBook(id, display string, now time.Time) (Aggregate, error) {
	display = strings.TrimSpace(display)
	if display == "" {
		return Aggregate{}, queryError(http.StatusBadRequest, "invalid_address_book_name")
	}
	books, err := s.VisibleBooks()
	if err != nil {
		return Aggregate{}, err
	}
	found := false
	for _, book := range books {
		if book.ID == id {
			found = true
			break
		}
	}
	if !found {
		return Aggregate{}, queryError(http.StatusNotFound, "private_address_book_not_found")
	}
	change, err := editAggregate(s, "book_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	change.Rows[0], err = patchRow(change.Rows[0], map[string]any{"display_name": display, "updated_at": now, "ctag": uuid.NewString()})
	return change, err
}

func (r *Repository) DeleteAddressBook(ctx context.Context, id string) error {
	_, err := r.mutateAggregate(ctx, "delete-book", struct{ ID string }{id}, func(snapshot *Snapshot) (Aggregate, error) { return snapshot.DeleteBook(id, time.Now().UTC()) })
	return err
}

func (s *Snapshot) DeleteBook(id string, now time.Time) (Aggregate, error) {
	books, err := s.VisibleBooks()
	if err != nil {
		return Aggregate{}, err
	}
	found := false
	for _, book := range books {
		if book.ID == id {
			if book.IsDefault {
				return Aggregate{}, queryError(http.StatusBadRequest, "default_address_book_immutable")
			}
			found = true
			break
		}
	}
	if !found {
		return Aggregate{}, queryError(http.StatusNotFound, "private_address_book_not_found")
	}
	change, err := editAggregate(s, "book_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	if err := aggregateTimestamp(&change, now); err != nil {
		return Aggregate{}, err
	}
	return change, nil
}
