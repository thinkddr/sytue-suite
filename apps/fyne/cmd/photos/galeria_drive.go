// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// galeriaDriveData delega en el repositorio Drive oculto de esta app lo único
// que la galería necesita para subir fotos y para su galería cifrada: «/Fotos», el
// original, los ficheros de su índice y la versión nueva de una foto editada. El resto
// de la fachada falla cerrado —esta no es la app de Drive— y el resolve es el
// mismo del proveedor de pins: pide el Drive oculto y devuelve ErrNotReady si
// todavía no está montado.
type galeriaDriveData struct {
	resolve func(context.Context) (*privateDrive.Repository, error)
	scope   storage.Scope
}

func (d galeriaDriveData) PrivateStorageScope() storage.Scope { return d.scope }

func (d galeriaDriveData) Root(ctx context.Context) (api.Folder, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return api.Folder{}, err
	}
	return repository.Root(ctx)
}

func (d galeriaDriveData) Children(ctx context.Context, folder, cursor string) (api.Children, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return api.Children{}, err
	}
	return repository.Children(ctx, folder, cursor)
}

func (d galeriaDriveData) CreateFolder(ctx context.Context, folder, name string) (api.Folder, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return api.Folder{}, err
	}
	return repository.CreateFolder(ctx, folder, name)
}

func (d galeriaDriveData) Upload(ctx context.Context, folder, name, contentType string, input io.Reader, size int64) (api.File, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return api.File{}, err
	}
	return repository.Upload(ctx, folder, name, contentType, input, size)
}

// Download descifra un fichero del Drive privado: el índice de la galería cifrada, sus
// miniaturas y el original que se abre (ADR 0086). Solo en el dispositivo.
func (d galeriaDriveData) Download(ctx context.Context, id string) (io.ReadCloser, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return repository.Download(ctx, id)
}

// ReplaceFile guarda una edición de la galería cifrada como versión nueva de la foto
// vista (ADR 0086): el repositorio rechaza una versión que otro dispositivo ya cambió
// en vez de pisarla. Solo para eso; no sube ficheros nuevos.
func (d galeriaDriveData) ReplaceFile(ctx context.Context, original api.File, contentType string, input io.Reader, size int64) (api.File, error) {
	repository, err := d.resolve(ctx)
	if err != nil {
		return api.File{}, err
	}
	return repository.ReplaceFile(ctx, original, contentType, input, size)
}

var _ api.DriveFileReplacer = galeriaDriveData{}

// Lo que la galería no usa queda cerrado: nada de esta fachada gestiona
// carpetas ajenas, papelera, versiones ni etiquetas.

func (galeriaDriveData) File(context.Context, string) (api.File, error) {
	return api.File{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Folder(context.Context, string) (api.Folder, error) {
	return api.Folder{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Thumbnail(context.Context, string) (io.ReadCloser, error) {
	return nil, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Rename(context.Context, api.DriveKind, string, string) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Move(context.Context, api.DriveKind, string, string) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Delete(context.Context, api.DriveKind, string) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Restore(context.Context, api.DriveKind, string) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) CopyFile(context.Context, string) (api.File, error) {
	return api.File{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Trash(context.Context) (api.DriveTrash, error) {
	return api.DriveTrash{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Search(context.Context, string) (api.Children, error) {
	return api.Children{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Recent(context.Context, string) (api.Compartidos, string, error) {
	return api.Compartidos{}, "", api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Favorites(context.Context) (api.Compartidos, error) {
	return api.Compartidos{}, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) SetFavorite(context.Context, api.DriveKind, string, bool) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Versions(context.Context, string, string) ([]api.DriveVersion, string, error) {
	return nil, "", api.ErrPrivateDriveOperation
}

func (galeriaDriveData) DownloadVersion(context.Context, string, int) (io.ReadCloser, error) {
	return nil, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) RestoreVersion(context.Context, string, int) error {
	return api.ErrPrivateDriveOperation
}

func (galeriaDriveData) Zip(context.Context, []string, []string, string) (io.ReadCloser, int64, error) {
	return nil, 0, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) SensitivityLabels(context.Context) ([]api.EtiquetaDrive, error) {
	return nil, api.ErrPrivateDriveOperation
}

func (galeriaDriveData) SetLabel(context.Context, string, string) error {
	return api.ErrPrivateDriveOperation
}
