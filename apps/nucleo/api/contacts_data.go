// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ContactsData is the personal address book surface, implemented on verified
// devices. Account controls and retained migration sources keep their separate
// original client. A missing private operation never reads legacy HTTP data.
type ContactsData interface {
	PrivateStorageScope() storage.Scope
	AddressBooks(context.Context) ([]AddressBook, error)
	All(context.Context, int, int) ([]Contact, int, error)
	Search(context.Context, string, int) ([]Contact, error)
	Create(context.Context, string, ContactInput) (Contact, error)
	ImportCard(context.Context, string, string) (Contact, error)
	Update(context.Context, string, ContactInput, string) (Contact, error)
	Contact(context.Context, string) (Contact, error)
	Delete(context.Context, string) error
	InBookPage(context.Context, string, string) (ContactPage, error)
	VCard(context.Context, string) ([]byte, error)
	Photo(context.Context, string) ([]byte, error)
	SetPhoto(context.Context, string, string) error
	RemovePhoto(context.Context, string) error
	Favorites(context.Context) ([]string, error)
	SetFavorite(context.Context, string, bool) error
	BatchFavorite(context.Context, []string, bool) error
	BatchTrash(context.Context, []string) error
	Trash(context.Context, int) ([]TrashItem, error)
	Restore(context.Context, string) error
	Purge(context.Context, string) error
	Groups(context.Context, string) ([]ContactGroup, error)
	CreateGroup(context.Context, string, string) (ContactGroup, error)
	RenameGroup(context.Context, string, string, []string) (ContactGroup, error)
	DeleteGroup(context.Context, string) error
	BatchGroupMembers(context.Context, string, []string, bool) error
	DeleteAddressBook(context.Context, string) error
	ImportCSV(context.Context, string, []byte) (int, int, error)
	ExportCSV(context.Context) (string, []byte, error)
	CreateAddressBook(context.Context, string) (AddressBook, error)
	CreateAddressBookAt(context.Context, string, string, string) (AddressBook, error)
	RenameAddressBook(context.Context, string, string) (AddressBook, error)
}

var ErrPrivateContactsOperation = errors.New("contacts: operation unavailable through private data")

// WithPrivateData preserves existing screens' typed API while removing their
// original transport and session. The caller owns the private repository's
// lifetime and must drain it before closing the SDK or changing identity.
func (c *Contacts) WithPrivateData(data ContactsData) (*Contacts, error) {
	if c == nil || c.c == nil || data == nil || c.data != nil {
		return nil, ErrPrivateContactsOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "contacts" {
		return nil, ErrPrivateContactsOperation
	}
	return &Contacts{c: privateDataClient(c.c, ErrPrivateContactsOperation), data: data}, nil
}
