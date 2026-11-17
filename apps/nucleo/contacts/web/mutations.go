// SPDX-License-Identifier: Apache-2.0
package web

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type CreateContact struct {
	Book  string
	Input api.ContactInput
}
type (
	ImportCard    struct{ Book, Card string }
	UpdateContact struct {
		ID    string
		Input api.ContactInput
		ETag  string
	}
)

type (
	DeleteContact struct{ ID string }
	SetPhoto      struct{ UID, DataURI string }
	RemovePhoto   struct{ UID string }
	SetFavorite   struct {
		UID     string
		Enabled bool
	}
)

type BatchFavorite struct {
	UIDs    []string
	Enabled bool
}
type (
	BatchTrash  struct{ UIDs []string }
	Restore     struct{ ID string }
	Purge       struct{ ID string }
	CreateGroup struct{ Book, Name string }
	RenameGroup struct {
		ID, Name string
		Members  []string
	}
)

type (
	DeleteGroup       struct{ ID string }
	BatchGroupMembers struct {
		ID   string
		UIDs []string
		Add  bool
	}
)

type (
	DeleteBook struct{ ID string }
	ImportCSV  struct {
		Book string
		Data []byte
	}
)

type (
	CreateBook   struct{ Display string }
	CreateBookAt struct{ Name, Display, Color string }
	RenameBook   struct{ ID, Name string }
)

type ImportCount struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

func (c CreateContact) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Create(ctx, c.Book, c.Input)
	return Result{Value: v}, e
}

func (c ImportCard) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.ImportCard(ctx, c.Book, c.Card)
	return Result{Value: v}, e
}

func (c UpdateContact) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.Update(ctx, c.ID, c.Input, c.ETag)
	return Result{Value: v}, e
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c DeleteContact) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.Delete(ctx, c.ID)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c SetPhoto) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.SetPhoto(ctx, c.UID, c.DataURI)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c RemovePhoto) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.RemovePhoto(ctx, c.UID)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c SetFavorite) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.SetFavorite(ctx, c.UID, c.Enabled)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c BatchFavorite) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.BatchFavorite(ctx, c.UIDs, c.Enabled)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c BatchTrash) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.BatchTrash(ctx, c.UIDs)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c Restore) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.Restore(ctx, c.ID)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c Purge) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.Purge(ctx, c.ID)
}

func (c CreateGroup) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.CreateGroup(ctx, c.Book, c.Name)
	return Result{Value: v}, e
}

func (c RenameGroup) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.RenameGroup(ctx, c.ID, c.Name, c.Members)
	return Result{Value: v}, e
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c DeleteGroup) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.DeleteGroup(ctx, c.ID)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c BatchGroupMembers) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.BatchGroupMembers(ctx, c.ID, c.UIDs, c.Add)
}

//nolint:unparam // execute implements the shared mutation interface; write-only mutations have no Result value
func (c DeleteBook) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	return Result{}, d.DeleteAddressBook(ctx, c.ID)
}

func (c ImportCSV) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	n, skipped, err := d.ImportCSV(ctx, c.Book, c.Data)
	if err != nil {
		return Result{}, &PartialImportError{Imported: n, Skipped: skipped, cause: err}
	}
	return Result{Value: ImportCount{n, skipped}}, nil
}

func (c CreateBook) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.CreateAddressBook(ctx, c.Display)
	return Result{Value: v}, e
}

func (c CreateBookAt) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.CreateAddressBookAt(ctx, c.Name, c.Display, c.Color)
	return Result{Value: v}, e
}

func (c RenameBook) execute(ctx context.Context, d api.ContactsData) (Result, error) {
	v, e := d.RenameAddressBook(ctx, c.ID, c.Name)
	return Result{Value: v}, e
}
