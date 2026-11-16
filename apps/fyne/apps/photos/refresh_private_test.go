// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateRefreshPhotoData struct {
	api.PhotosData
	mu      sync.Mutex
	photos  []api.Photo
	refresh func(context.Context) error
	reads   int
}

func (*privateRefreshPhotoData) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func (p *privateRefreshPhotoData) Timeline(ctx context.Context, _ string, _ int, _ bool, _ string, _ ...string) ([]api.Photo, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return append([]api.Photo(nil), p.photos...), "", ctx.Err()
}

func (p *privateRefreshPhotoData) RefreshPrivateData(ctx context.Context) error {
	return p.refresh(ctx)
}

func TestPrivatePhotosGesturePreservesProjectionOnFailureAndJoinsBeforeReload(t *testing.T) {
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	data := &privateRefreshPhotoData{photos: fotosDelServicio(1)}
	private, err := application.cli.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	application.cli = private
	application.Arrancar()
	if len(application.g.fotos) != 1 {
		t.Fatal("private UI did not load its initial projection")
	}
	original := application.g.fotos[0].Name
	var work sync.WaitGroup
	application.lanzar = func(fn func()) { work.Go(fn) }
	flush := func() { t.Helper(); work.Wait(); fyne.DoAndWait(func() {}); work.Wait(); fyne.DoAndWait(func() {}) }
	entered, release := make(chan struct{}), make(chan struct{})
	failure := errors.New("authenticated candidate is unavailable")
	data.refresh = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return failure
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	application.refrescarLista()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("private refresh gesture did not reach its data port")
	}
	application.refrescarLista()
	data.mu.Lock()
	reads := data.reads
	data.mu.Unlock()
	if !application.refrescando || application.puedeRefrescarLista() || application.g.fotos[0].Name != original || reads != 1 {
		t.Fatal("pending refresh replaced content, reloaded early or admitted another gesture")
	}
	close(release)
	flush()
	if application.refrescando || application.g.error == "" || application.g.fotos[0].Name != original {
		t.Fatal("failed private refresh discarded prior photos or stayed loading")
	}
	data.refresh = func(ctx context.Context) error {
		data.mu.Lock()
		data.photos[0].Name = "Changed by verified private repository"
		data.mu.Unlock()
		return ctx.Err()
	}
	application.refrescarLista()
	flush()
	if application.refrescando || len(application.g.fotos) != 1 || application.g.fotos[0].Name != "Changed by verified private repository" {
		t.Fatal("successful private refresh did not reload its published candidate")
	}
	data.mu.Lock()
	reads = data.reads
	data.mu.Unlock()
	if reads != 2 || len(service.vistas()) != 0 {
		t.Fatal("private gesture implicitly reloaded or reached legacy HTTP", reads, service.vistas())
	}
	application.Cerrar()
}
