// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type solicitudAlbumesBloqueada struct {
	iniciada, liberar        chan struct{}
	aviso, cierre            sync.Once
	trabajos                 sync.WaitGroup
	operaciones, inventarios atomic.Int32
}

// Reuse the Photos SDK transport used by the account-lifetime regressions. The
// blocked operation does not hold the inventory response or the UI goroutine.
func bloquearSolicitudAlbumes(t *testing.T, a *App, method, path string, status int, body string) *solicitudAlbumesBloqueada {
	t.Helper()
	b := &solicitudAlbumesBloqueada{iniciada: make(chan struct{}), liberar: make(chan struct{})}
	a.cli = api.NewPhotos("https://photos.invalid", nil, &http.Client{Transport: ownerPhotoTransport(func(r *http.Request) (*http.Response, error) {
		code, payload := http.StatusOK, `{"items":[]}`
		switch {
		case r.Method == method && r.URL.Path == path:
			b.operaciones.Add(1)
			b.aviso.Do(func() { close(b.iniciada) })
			select {
			case <-b.liberar:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			code, payload = status, body
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/albums":
			b.inventarios.Add(1)
		default:
			code = http.StatusNotFound
		}
		return &http.Response{StatusCode: code, Request: r, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	a.lanzar = func(f func()) { b.trabajos.Go(f) }
	t.Cleanup(func() { b.terminar() })
	return b
}

func (b *solicitudAlbumesBloqueada) esperarInicio(t *testing.T) {
	t.Helper()
	select {
	case <-b.iniciada:
	case <-time.After(5 * time.Second):
		t.Fatal("the album operation did not reach the Photos SDK")
	}
}

func (b *solicitudAlbumesBloqueada) terminar() {
	b.cierre.Do(func() { close(b.liberar) })
	b.trabajos.Wait()
	fyne.DoAndWait(func() {})
}

func comprobarRefrescoAlbumesDisponible(t *testing.T, a *App, b *solicitudAlbumesBloqueada) {
	t.Helper()
	if !a.puedeRefrescarLista() {
		t.Fatal("album refresh did not become available after the operation completed")
	}
	gen, inventories := a.gen, b.inventarios.Load()
	a.refrescarLista()
	b.terminar()
	if a.gen != gen+1 || b.inventarios.Load() != inventories+1 || a.alb.cargando {
		t.Fatal("refresh did not reload the album inventory after the operation completed")
	}
}

func TestGestoAlbumesConservaSugerenciasEnVuelo(t *testing.T) {
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 3})
	a.Ir(RutaAlbumes)
	b := bloquearSolicitudAlbumes(t, a, http.MethodGet, "/api/v1/themes", http.StatusOK,
		`{"themes":[{"items":[{"id":"suggested-photo"}]}]}`)
	a.buscarMontones()
	b.esperarInicio(t)
	gen := a.gen
	a.refrescarLista()
	b.terminar()
	if a.gen != gen || a.alb.buscando || len(a.alb.montones) != 1 || len(a.alb.montones[0]) != 1 || a.alb.montones[0][0].ID != "suggested-photo" {
		t.Fatalf("pull retired pending suggestions: generation %d→%d, searching=%v, suggestions=%v", gen, a.gen, a.alb.buscando, a.alb.montones)
	}
	if b.inventarios.Load() != 0 {
		t.Fatal("pull started another inventory request while suggestions were pending")
	}
	comprobarRefrescoAlbumesDisponible(t, a, b)
}

func TestGestoAlbumesConservaAltaEnVueloAunqueSeVacieElCampo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"success", http.StatusOK, `{"id":"created-album","title":"Viaje"}`},
		{"error", http.StatusInternalServerError, `{"code":"unexpected_error"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 3})
			a.Ir(RutaAlbumes)
			b := bloquearSolicitudAlbumes(t, a, http.MethodPost, "/api/v1/albums", tc.status, tc.body)
			a.campoAlbum.PonerTexto("Viaje")
			a.crearAlbum(a.campoAlbum.Texto())
			b.esperarInicio(t)
			gen := a.gen
			a.campoAlbum.PonerTexto("") // The pending request, not this field, owns busy state.
			a.refrescarLista()
			a.crearAlbum("Duplicado")
			b.terminar()
			if a.gen != gen || b.operaciones.Load() != 1 || b.inventarios.Load() != 0 {
				t.Fatalf("pull retired or repeated pending creation: generation %d→%d, creates=%d, inventories=%d", gen, a.gen, b.operaciones.Load(), b.inventarios.Load())
			}
			if tc.status == http.StatusOK {
				if len(a.alb.lista) != 1 || a.alb.lista[0].ID != "created-album" || a.campoAlbum.Texto() != "" {
					t.Fatal("completed creation was not adopted by its original album view")
				}
			} else if a.alb.error == "" {
				t.Fatal("the creation failure disappeared")
			}
			comprobarRefrescoAlbumesDisponible(t, a, b)
		})
	}
}

func TestGestoAlbumesAltaRetiradaNoBloqueaLaVistaNueva(t *testing.T) {
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 3})
	a.Ir(RutaAlbumes)
	b := bloquearSolicitudAlbumes(t, a, http.MethodPost, "/api/v1/albums", http.StatusOK,
		`{"id":"retired-album","title":"Anterior"}`)
	a.crearAlbum("Anterior")
	b.esperarInicio(t)
	a.Ir(RutaFotos)
	a.Ir(RutaAlbumes)
	b.terminar()
	if len(a.alb.lista) != 0 || !a.puedeRefrescarLista() {
		t.Fatal("a retired creation reached the new view or left its refresh disabled")
	}
	comprobarRefrescoAlbumesDisponible(t, a, b)
}
