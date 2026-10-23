// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestCambiarCuentaRetiraFotosAlbumesYTrabajoPendiente(t *testing.T) {
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 3})
	a.CambiarSesion("issuer\x00ada\x00acme")
	a.Arrancar()
	a.ruta, a.uno.id = RutaAlbum, "old-album"
	a.albumes = []api.Album{{ID: "old-album"}}
	a.rec = estadoRecuerdos{pedidos: true, lista: []api.Memory{{}}}
	a.huellas.Store("same-id", "old-recipe")
	a.v.Teca.Poner("same-id", []byte("old-private-photo"))
	a.v.TecaGrande = NuevaFototeca()
	a.v.TecaGrande.Poner("same-id", []byte("old-private-original"))
	vida := a.vida
	var pendientes []func()
	a.lanzar = func(f func()) { pendientes = append(pendientes, f) }
	seEjecuto := false
	a.enSesion(func(context.Context) { seEjecuto = true })
	a.CambiarSesion("issuer\x00ada\x00other") // Dentro sigue siendo true.
	if vida.Err() != context.Canceled || a.Ruta() != RutaFotos || a.uno.id != "" || a.albumes != nil || a.Huella("same-id") != "" {
		t.Fatal("the previous owner's route, inventory or lifetime survived")
	}
	for _, teca := range []*Fototeca{a.v.Teca, a.v.TecaGrande} {
		teca.mu.Lock()
		bytes, pixeles, datos := teca.pesoC, teca.pixeles, len(teca.crudas)+len(teca.pintadas)
		teca.mu.Unlock()
		if bytes != 0 || pixeles != 0 || datos != 0 {
			t.Fatal("private image cache survived owner change")
		}
	}
	if len(a.g.fotos) != 0 || len(a.rec.lista) != 0 || !a.g.cargando {
		t.Fatal("new account did not start with an empty, loading gallery")
	}
	pendientes[0]()
	if seEjecuto {
		t.Fatal("queued work started after adopting another account")
	}
	gen, vida := a.gen, a.vida
	a.CambiarSesion("issuer\x00ada\x00other")
	if a.gen != gen || a.vida != vida {
		t.Fatal("same account reset its screen on token refresh")
	}
	a.CambiarSesion("issuer\x00grace\x00other")
	if vida.Err() != context.Canceled {
		t.Fatal("subject change did not cancel its predecessor")
	}
}

func TestCambioDeCuentaDescartaRecuerdosTardiosAunqueCambieRuta(t *testing.T) {
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 3})
	a.CambiarSesion("ada/acme")
	entered, release, finished := make(chan context.Context, 1), make(chan struct{}), make(chan struct{})
	a.cli = api.NewPhotos("https://photos.invalid", nil, &http.Client{Transport: ownerPhotoTransport(func(r *http.Request) (*http.Response, error) {
		entered <- r.Context()
		<-release
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"memories":[{"year":2020,"items":[{"id":"old-private-photo"}]}]}`))}, nil
	})})
	a.lanzar = func(f func()) { go func() { f(); close(finished) }() }
	a.pedirRecuerdos()
	request := <-entered
	a.lanzar = func(func()) {} // El inventario nuevo se carga por separado.
	a.CambiarSesion("grace/acme")
	close(release)
	<-finished
	if request.Err() == nil || len(a.rec.lista) != 0 {
		t.Fatal("previous account's memories escaped retirement")
	}
}

type ownerPhotoTransport func(*http.Request) (*http.Response, error)

func (f ownerPhotoTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVaciarFototecaNoAceptaDescargaAnteriorNiConservaErrores(t *testing.T) {
	f := NuevaFototeca()
	f.Poner("same-id", []byte("old"))
	f.mu.Lock()
	viejo := f.trabajo("same-id")
	f.fallidas["failed-id"] = true
	f.mu.Unlock()
	f.Vaciar()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.vigente("same-id", viejo) || len(f.crudas) != 0 || len(f.fallidas) != 0 || f.pesoC != 0 || f.pixeles != 0 {
		t.Fatal("cache reset retained data or accepted an old worker")
	}
}
