// SPDX-License-Identifier: Apache-2.0
package web

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type (
	ListBooks struct{}
	ListAll   struct{ Limit, Offset int }
	Search    struct {
		Query string
		Limit int
	}
)

type (
	GetContact    struct{ ID string }
	ListBook      struct{ Book, Cursor string }
	ExportCard    struct{ ID string }
	GetPhoto      struct{ UID string }
	ListFavorites struct{}
	ListTrash     struct{ Limit int }
	ListGroups    struct{ Book string }
	ExportCSV     struct{}
)

type ContactList struct {
	Contacts   []api.Contact `json:"contacts"`
	NextOffset int           `json:"next_offset"`
}
type CSVExport struct {
	Filename string `json:"filename"`
}

func (ListBooks) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.AddressBooks(ctx)
	return Result{Value: v}, e
}

func (c ListAll) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, n, e := d.All(ctx, c.Limit, c.Offset)
	return Result{Value: ContactList{v, n}}, e
}

func (c Search) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Search(ctx, c.Query, c.Limit)
	return Result{Value: v}, e
}

func (c GetContact) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Contact(ctx, c.ID)
	return Result{Value: v}, e
}

func (c ListBook) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.InBookPage(ctx, c.Book, c.Cursor)
	return Result{Value: v}, e
}

func (c ExportCard) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.VCard(ctx, c.ID)
	return Result{Body: v}, e
}

func (c GetPhoto) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Photo(ctx, c.UID)
	return Result{Body: v}, e
}

func (ListFavorites) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Favorites(ctx)
	return Result{Value: v}, e
}

func (c ListTrash) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Trash(ctx, c.Limit)
	return Result{Value: v}, e
}

func (c ListGroups) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Groups(ctx, c.Book)
	return Result{Value: v}, e
}

func (ExportCSV) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	name, v, e := d.ExportCSV(ctx)
	return Result{Value: CSVExport{name}, Body: v}, e
}
