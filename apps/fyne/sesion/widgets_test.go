// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

type widgetApp struct{ fyne.App }

func (widgetApp) Metadata() fyne.AppMetadata {
	return fyne.AppMetadata{ID: "eu.sytue.docus", Custom: map[string]string{"esquema": "sytue-docs"}}
}

func TestWidgetBusquedaSoloAceptaLaAccionLocalSinTexto(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s := &Suite{app: widgetApp{a}}
	for _, destino := range []string{"sytue-docs:/?action=search", "sytue-docs:?action=search"} {
		u, _ := url.Parse(destino)
		if !s.EsBusqueda(u) {
			t.Fatalf("no reconoce la búsqueda de esta app: %s", destino)
		}
	}
	for _, destino := range []string{
		"sytue-mail:/?action=search", "https://docs.sytue.eu/?action=search",
		"sytue-docs://external/?action=search", "sytue-docs:/document?asset=one&action=search",
		"sytue-docs:/?action=search&q=private", "sytue-docs:/?action=search&action=search",
		"sytue-docs:/?action=search#private", "sytue-docs:private?action=search",
		"sytue-docs:/?action=compose", "sytue-docs:/?action=search;secret=private",
	} {
		u, _ := url.Parse(destino)
		if s.EsBusqueda(u) {
			t.Fatalf("acepta un destino o texto que el widget no declara: %s", destino)
		}
	}
	if s.EsBusqueda(nil) || (&Suite{}).EsBusqueda(&url.URL{}) {
		t.Fatal("busca sin enlace ni aplicación")
	}
}

func TestWidgetBusquedaTempranaNoAbsorbeRutasDesconocidas(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s := &Suite{app: widgetApp{a}, oidc: &auth.OIDC{}}
	u, _ := url.Parse("sytue-docs:/?action=search")
	s.recibirURL(u)
	if !s.busquedaPendiente || len(s.sinDueno) != 0 {
		t.Fatal("la acción temprana se perdió o se entregó como ruta de documento")
	}
	llamadas := 0
	s.AlBuscar(func() { llamadas++ })
	fyne.DoAndWait(func() {})
	if llamadas != 1 || s.busquedaPendiente || len(s.alAbrirURL) != 0 {
		t.Fatal("el receptor de búsqueda alteró las rutas de producto o repitió la acción")
	}
	u, _ = url.Parse("sytue-docs:/does-not-exist")
	s.recibirURL(u)
	if !s.finArranque() {
		t.Fatal("registrar la búsqueda oculta una ruta no encontrada")
	}
}

func TestWidgetUsesAdoptedIdentityBeforeDataFetch(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	ajustes.MarcarSesion()
	s := &Suite{
		app: widgetApp{app}, Sesion: auth.NewSession(auth.Tokens{Subject: "first"}, nil, nil),
		generacionSesion: ajustes.Leer().Sesion,
	}
	old := s.CapturarWidget()
	ctx := context.Background()
	if err := old.Publicar(ctx, widgets.Counter{Kind: "documents", Value: 2}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := widgets.Read("eu.sytue.docus")
	if err != nil || snapshot.Product != "docs" {
		t.Fatalf("product must come from schema: %+v %v", snapshot, err)
	}
	// Another app connects a second account. This Suite hasn't adopted it yet:
	// reading the new disk marker here would mislabel the first person's data.
	ajustes.MarcarSesion()
	if err := s.CapturarWidget().Publicar(ctx, widgets.Counter{Kind: "documents", Value: 99}); !errors.Is(err, widgets.ErrPrivate) {
		t.Fatalf("unadopted account marker used: %v", err)
	}
	s.mu.Lock()
	s.Sesion.Sincronizar(auth.Tokens{Subject: "second"})
	s.generacionSesion = ajustes.Leer().Sesion
	s.mu.Unlock()
	if err := s.CapturarWidget().Publicar(ctx, widgets.Counter{Kind: "documents", Value: 3}); err != nil {
		t.Fatal(err)
	}
	if err := old.Publicar(ctx, widgets.Counter{Kind: "documents", Value: 99}); !errors.Is(err, widgets.ErrPrivate) {
		t.Fatalf("late first account response used: %v", err)
	}
	snapshot, err = widgets.Read("eu.sytue.docus")
	if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0].Value != 3 {
		t.Fatalf("second account snapshot changed: %+v %v", snapshot, err)
	}
	s.Sesion.Sincronizar(auth.Tokens{})
	if err := s.CapturarWidget().Publicar(ctx); !errors.Is(err, widgets.ErrPrivate) {
		t.Fatalf("closed session published: %v", err)
	}
}
