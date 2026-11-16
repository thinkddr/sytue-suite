// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func readQuery[T any](ctx context.Context, r *Repository, query func(*Snapshot) (T, error)) (T, error) {
	snapshot, err := r.Snapshot(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	return query(snapshot)
}

func (r *Repository) AddressBooks(ctx context.Context) ([]api.AddressBook, error) {
	return readQuery(ctx, r, (*Snapshot).VisibleBooks)
}

func (r *Repository) Contact(ctx context.Context, id string) (api.Contact, error) {
	value, err := readQuery(ctx, r, func(s *Snapshot) (ContactView, error) { return s.FindContact(id) })
	return value.Contact, err
}

func (r *Repository) All(ctx context.Context, limit, offset int) ([]api.Contact, int, error) {
	snapshot, err := r.Snapshot(ctx)
	if err != nil {
		return nil, 0, err
	}
	return snapshot.QueryAll(limit, offset)
}

func (r *Repository) Search(ctx context.Context, text string, limit int) ([]api.Contact, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]api.Contact, error) { return s.QuerySearch(text, limit) })
}

func (r *Repository) InBookPage(ctx context.Context, book, cursor string) (api.ContactPage, error) {
	return readQuery(ctx, r, func(s *Snapshot) (api.ContactPage, error) { return s.QueryBook(book, cursor) })
}

func (r *Repository) VCard(ctx context.Context, id string) ([]byte, error) {
	value, err := readQuery(ctx, r, func(s *Snapshot) (ContactView, error) { return s.FindContact(id) })
	if err != nil {
		return nil, err
	}
	return []byte(value.VCard), nil
}

func (r *Repository) Photo(ctx context.Context, uid string) ([]byte, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]byte, error) { return s.QueryPhoto(uid) })
}

func (r *Repository) Favorites(ctx context.Context) ([]string, error) {
	return readQuery(ctx, r, (*Snapshot).QueryFavorites)
}

func (r *Repository) Groups(ctx context.Context, book string) ([]api.ContactGroup, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]api.ContactGroup, error) { return s.QueryGroups(book) })
}

func (r *Repository) Trash(ctx context.Context, limit int) ([]api.TrashItem, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]api.TrashItem, error) { return s.QueryTrash(limit) })
}

func (r *Repository) ExportCSV(ctx context.Context) (string, []byte, error) {
	snapshot, err := r.Snapshot(ctx)
	if err != nil {
		return "", nil, err
	}
	return snapshot.QueryCSV()
}
