// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type directPhotoUploadData struct {
	api.PhotosData
	raw   []byte
	calls int
}

func (*directPhotoUploadData) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func (d *directPhotoUploadData) ImportarFoto(ctx context.Context, job, name, mime string, body io.Reader, size int64) (api.Photo, error) {
	if ctx.Err() != nil {
		return api.Photo{}, ctx.Err()
	}
	if _, err := uuid.Parse(job); err != nil || name != "original.png" || mime != "image/png" || size != int64(len(d.raw)) {
		return api.Photo{}, api.ErrPrivatePhotosOperation
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return api.Photo{}, err
	}
	if !bytes.Equal(raw, d.raw) {
		return api.Photo{}, api.ErrPrivatePhotosOperation
	}
	d.calls++
	return api.Photo{ID: job, Name: name}, nil
}

func (d *directPhotoUploadData) ReanudarImportaciones(ctx context.Context) ([]api.Photo, error) {
	return nil, ctx.Err()
}

func TestPrivatePhotoUploadUsesDeviceWithoutDriveOrLegacyHTTP(t *testing.T) {
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	data := &directPhotoUploadData{raw: pngDe(bandas(8, 4))}
	private, err := application.cli.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	application.cli = private
	application.Drive = nil
	path := filepath.Join(t.TempDir(), "original.png")
	if err := os.WriteFile(path, data.raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := application.subirUna(t.Context(), nil, path); err != nil {
		t.Fatal(err)
	}
	if data.calls != 1 || len(service.vistas()) != 0 {
		t.Fatal("OS import bypassed the private device", data.calls, service.vistas())
	}
	application.Cerrar()
}
