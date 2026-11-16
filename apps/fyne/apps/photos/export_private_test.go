// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privatePhotoExportData struct {
	api.PhotosData
	raw   []byte
	calls int
}

func (*privatePhotoExportData) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func (p *privatePhotoExportData) Export(ctx context.Context, id, format string) (io.ReadCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if id != "private-photo" || format != "png" {
		return nil, api.ErrPrivatePhotosOperation
	}
	p.calls++
	return io.NopCloser(bytes.NewReader(p.raw)), nil
}

func TestPrivatePhotoDownloadSavesDeviceExportWithoutLegacyHTTP(t *testing.T) {
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	data := &privatePhotoExportData{raw: pngDe(bandas(8, 4))}
	private, err := application.cli.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	application.cli = private
	var filename string
	var saved []byte
	application.Guardar = func(name string, raw []byte) (string, error) {
		filename = name
		saved = bytes.Clone(raw)
		return "/synthetic/private-export.png", nil
	}
	if err := application.bajar(t.Context(), api.Photo{ID: "private-photo", Name: "retained.jpeg"}, "png"); err != nil {
		t.Fatal(err)
	}
	if data.calls != 1 || filename != "retained.png" || !bytes.Equal(saved, data.raw) || len(service.vistas()) != 0 {
		t.Fatal("private download bypassed the device export or changed its bytes", data.calls, filename, service.vistas())
	}
	application.Cerrar()
}
