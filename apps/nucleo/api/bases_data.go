// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BasesData is the device-owned Bases surface. Missing capabilities must return
// an error; the private facade never falls back to the legacy transport.
type BasesData interface {
	PrivateStorageScope() storage.Scope
	ListBases(context.Context) ([]BasesBase, error)
	CreateBase(context.Context, string) (BasesBase, error)
	GetBase(context.Context, string) (BasesBase, error)
	UpdateBase(context.Context, string, string, string) error
	DeleteBase(context.Context, string) error
	ListTables(context.Context, string) ([]BasesTable, error)
	CreateTable(context.Context, string, string) (BasesTable, error)
	RenameTable(context.Context, string, string) error
	DeleteTable(context.Context, string) error
	CreateField(context.Context, string, BasesNewField) (BasesField, error)
	RenameField(context.Context, string, string) error
	DeleteField(context.Context, string) error
	ListRecords(context.Context, string, string, string, int) (BasesRecordPage, error)
	CreateRecord(context.Context, string, map[string]any, map[string][]string) (BasesRecord, error)
	GetRecord(context.Context, string) (BasesRecord, error)
	WriteCell(context.Context, string, string, BasesCell, string) (BasesCellResult, error)
	DeleteRecord(context.Context, string, bool) error
	ListViews(context.Context, string) ([]BasesView, error)
	CreateView(context.Context, string, string, string, BasesViewConfig) (BasesView, error)
	DeleteView(context.Context, string) error
	ListFormLinks(context.Context, string) ([]BasesFormLink, error)
	CreateFormLink(context.Context, string, int) (BasesFormLink, error)
	RevokeFormLink(context.Context, string) error
	ListPermissions(context.Context, string) ([]BasesPermission, error)
	GrantPermission(context.Context, string, BasesGrant) (BasesPermission, error)
	RevokePermission(context.Context, string, string) error
	MyGroups(context.Context) ([]string, error)
	TransferBase(context.Context, string, string) error
	Export(context.Context, string, string) (io.ReadCloser, error)
}

var ErrPrivateBasesOperation = errors.New("bases: operation unavailable through private data")

// WithPrivateData keeps the typed API used by existing screens. The caller owns
// the repository and drains its work before closing the SDK or changing account.
func (b *Bases) WithPrivateData(data BasesData) (*Bases, error) {
	if b == nil || b.c == nil || data == nil || b.private != nil {
		return nil, ErrPrivateBasesOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "bases" {
		return nil, ErrPrivateBasesOperation
	}
	return &Bases{c: privateDataClient(b.c, ErrPrivateBasesOperation), private: data}, nil
}
