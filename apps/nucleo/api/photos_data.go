// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type PhotosData interface {
	PrivateStorageScope() storage.Scope
	Timeline(context.Context, string, int, bool, string, ...string) ([]Photo, string, error)
	Photo(context.Context, string) (Photo, error)
	SetFavorite(context.Context, string, bool) error
	SetDescription(context.Context, string, string, []string) error
	Trash(context.Context, string) error
	Papelera(context.Context) ([]Photo, error)
	Restaurar(context.Context, string) error
}

// PhotosMediaData streams retained authenticated media through the same private
// facade. Its absence is an unsupported private operation, never legacy HTTP.
type PhotosMediaData interface {
	VariantDe(context.Context, string, string, string) (io.ReadCloser, error)
}

// PhotosIngestData imports a Drive file already uploaded by the same device.
// The private repository retains the original and its metadata together.
type PhotosIngestData interface {
	Indexar(context.Context, string) (Photo, error)
}

// PhotosExportData keeps full-resolution exports within the verified device.
type PhotosExportData interface {
	Export(context.Context, string, string) (io.ReadCloser, error)
}

// EdicionPhotos retains the exact recipe and base of an unacknowledged save.
type EdicionPhotos struct {
	Receta Receta
	Base   string
}

// PhotosRecipeData keeps editor reads, conditional saves and recovery on device.
type PhotosRecipeData interface {
	LeerReceta(context.Context, string) (Receta, error)
	GuardarRecetaDesde(context.Context, string, Receta, string) (string, error)
	EdicionPendiente(context.Context, string) (*EdicionPhotos, error)
}

// PhotosAlbumData keeps personal album metadata and ordered reads on the device.
// Sharing requires a separate encrypted recipient capability.
type PhotosAlbumData interface {
	Albums(context.Context) ([]Album, error)
	Album(context.Context, string) (Album, error)
	AlbumPhotos(context.Context, string) ([]Photo, error)
	CreateAlbum(context.Context, string) (Album, error)
	RenameAlbum(context.Context, string, string) error
	DeleteAlbum(context.Context, string) error
}

// PhotosAlbumMembershipData commits a personal album's complete membership
// checkpoint through one authenticated revision.
type PhotosAlbumMembershipData interface {
	AddToAlbum(context.Context, string, []string) (int, error)
	RemoveFromAlbum(context.Context, string, string) error
}

var ErrPrivatePhotosOperation = errors.New("photos: operation unavailable through private data")

// WithPrivateData binds a separate facade to one verified device repository.
// Unsupported operations fail closed through the existing private transport.
func (p *Photos) WithPrivateData(data PhotosData) (*Photos, error) {
	if p == nil || p.c == nil || p.data != nil || data == nil {
		return nil, ErrPrivatePhotosOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "photos" {
		return nil, ErrPrivatePhotosOperation
	}
	return &Photos{c: privateDataClient(p.c, ErrPrivatePhotosOperation), data: data}, nil
}
