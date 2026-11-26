// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	fynestorage "fyne.io/fyne/v2/storage"
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

// conLineaVacia es directPhotoUploadData con una línea de tiempo vacía: tras subir, la
// galería se vuelve a pedir.
type conLineaVacia struct{ *directPhotoUploadData }

func (conLineaVacia) Timeline(context.Context, string, int, bool, string, ...string) ([]api.Photo, string, error) {
	return nil, "", nil
}

// Con App.ImportarVideo (Sytue Office, FO-09), «Añadir fotos» y soltar admiten además los
// vídeos: cada uno va a ImportarVideo, las imágenes siguen por ImportarFoto y lo demás se
// queda fuera. Sin él, como en la conectada, un vídeo no entra y la galería dice que solo
// imágenes.
func TestAnadirYSoltarAdmitenVideosSoloConImportarVideo(t *testing.T) {
	dir := t.TempDir()
	escribir := func(nombre string, datos []byte) string {
		t.Helper()
		r := filepath.Join(dir, nombre)
		if err := os.WriteFile(r, datos, 0o600); err != nil {
			t.Fatal(err)
		}
		return r
	}
	data := &directPhotoUploadData{raw: pngDe(bandas(8, 4))}
	foto := escribir("original.png", data.raw)
	video := escribir("cumple.MP4", []byte("un vídeo"))
	webm := escribir("perro.webm", []byte("otro vídeo"))
	texto := escribir("notas.txt", []byte("hola"))
	montar := func() *App {
		application, _ := appDePrueba(t, &servicioFalso{})
		private, err := application.cli.WithPrivateData(conLineaVacia{data})
		if err != nil {
			t.Fatal(err)
		}
		application.cli, application.Drive = private, nil
		t.Cleanup(application.Cerrar)
		return application
	}

	sinVideos := montar()
	sinVideos.subir([]string{video})
	if sinVideos.g.aviso != sinVideos.v.Traduce("gallery.onlyImages") || data.calls != 0 {
		t.Fatal("sin ImportarVideo un vídeo no entra:", sinVideos.g.aviso, data.calls)
	}

	conVideos := montar()
	var importados []string
	conVideos.ImportarVideo = func(ctx context.Context, ruta string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		importados = append(importados, filepath.Base(ruta))
		return nil
	}
	uris := []fyne.URI{fynestorage.NewFileURI(foto), fynestorage.NewFileURI(video), fynestorage.NewFileURI(texto), fynestorage.NewFileURI(webm)}
	conVideos.soltado(fyne.Position{}, uris)
	if data.calls != 1 || len(importados) != 2 || importados[0] != "cumple.MP4" || importados[1] != "perro.webm" {
		t.Fatal("soltar: la foto por ImportarFoto y los dos vídeos por ImportarVideo:", data.calls, importados)
	}
}
