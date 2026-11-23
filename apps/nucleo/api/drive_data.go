// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// DriveData is the personal data surface used by native screens and file tools.
// Private repositories implement it without sending application data to legacy
// HTTP endpoints. Account controls use the separate original Drive client.
type DriveData interface {
	PrivateStorageScope() storage.Scope
	Root(context.Context) (Folder, error)
	Children(context.Context, string, string) (Children, error)
	CreateFolder(context.Context, string, string) (Folder, error)
	File(context.Context, string) (File, error)
	Folder(context.Context, string) (Folder, error)
	Download(context.Context, string) (io.ReadCloser, error)
	Thumbnail(context.Context, string) (io.ReadCloser, error)
	Upload(context.Context, string, string, string, io.Reader, int64) (File, error)
	Rename(context.Context, DriveKind, string, string) error
	Move(context.Context, DriveKind, string, string) error
	Delete(context.Context, DriveKind, string) error
	Restore(context.Context, DriveKind, string) error
	CopyFile(context.Context, string) (File, error)
	Trash(context.Context) (DriveTrash, error)
	Search(context.Context, string) (Children, error)
	Recent(context.Context, string) (Compartidos, string, error)
	Favorites(context.Context) (Compartidos, error)
	SetFavorite(context.Context, DriveKind, string, bool) error
	Versions(context.Context, string, string) ([]DriveVersion, string, error)
	DownloadVersion(context.Context, string, int) (io.ReadCloser, error)
	RestoreVersion(context.Context, string, int) error
	Zip(context.Context, []string, []string, string) (io.ReadCloser, int64, error)
	SensitivityLabels(context.Context) ([]EtiquetaDrive, error)
	SetLabel(context.Context, string, string) error
}

// ErrPrivateDriveOperation prevents a missing private implementation from
// silently reading or publishing application data through the old service.
var ErrPrivateDriveOperation = errors.New("drive: operation unavailable through private data")

// WithPrivateData returns a personal data facade for existing native screens and
// OS file helpers. All remaining HTTP operations fail closed, including new API
// additions. Use the separate original client for account controls, migration
// source and ciphertext gateway. This never mutates that original client.
// The caller owns the repository lifetime and closes it when identity changes.
func (d *Drive) WithPrivateData(data DriveData) (*Drive, error) {
	if d == nil || d.c == nil || data == nil || d.data != nil {
		return nil, ErrPrivateDriveOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "drive" {
		return nil, ErrPrivateDriveOperation
	}
	return &Drive{c: privateDataClient(d.c, ErrPrivateDriveOperation), data: data}, nil
}

// Privado dice si este cliente va por los datos privados (WithPrivateData): lo que
// sube queda cifrado en el dispositivo y el servidor no puede leerlo, así que
// ningún servicio (Photos, por ejemplo) puede indexarlo por su id.
func (d *Drive) Privado() bool { return d != nil && d.data != nil }

// ErrUnidadCifrada: el Drive del servidor de esta persona ya no admite escrituras
// porque está (o está pasando) en almacenamiento privado; contesta 409 con el
// código `private_storage_migration_required`. Lo que se suba tiene que ir al
// Drive privado, cifrado en el dispositivo.
var ErrUnidadCifrada = errors.New("drive: el Drive del servidor está en almacenamiento privado")

// EsUnidadCifrada reconoce ese 409 por su código; cualquier otro 409 no lo es.
func EsUnidadCifrada(err error) bool {
	if errors.Is(err, ErrUnidadCifrada) {
		return true
	}
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusConflict && e.Code() == "private_storage_migration_required"
}
