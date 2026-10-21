// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type appDeBusquedaWidget struct {
	fyne.App
	alAbrir func(*url.URL)
	entrada *url.URL
}

func (a *appDeBusquedaWidget) Metadata() fyne.AppMetadata {
	return fyne.AppMetadata{ID: "eu.sytue.docus", Custom: map[string]string{"esquema": "sytue-docs"}}
}

func (a *appDeBusquedaWidget) SetOnOpenURL(f func(*url.URL)) {
	a.alAbrir = f
	if a.entrada != nil {
		f(a.entrada)
	}
}

func TestWidgetBuscaTrasLoginYMontajeSinConsultaNiAtajoCedido(t *testing.T) {
	for _, frio := range []bool{false, true} {
		t.Run(fmt.Sprint("arranque frío=", frio), func(t *testing.T) {
			// Reutiliza la preparación aislada del llavero y configuración de la barra.
			_ = suiteConSesion(t)
			u, _ := url.Parse("sytue-docs:/?action=search")
			a := &appDeBusquedaWidget{App: fyne.CurrentApp()}
			if frio {
				a.entrada = u
			}
			s := sesion.Abrir(a, "sytue.eu")
			s.Sesion.Sincronizar(auth.Tokens{})
			s.Hub = nil
			b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Docs)
			t.Cleanup(b.Parar)
			cedido := false
			b.CederAtajo(func() { cedido = true })
			if !frio {
				a.alAbrir(u)
			}
			fyne.DoAndWait(func() {})
			if !b.buscarPendiente {
				t.Fatal("perdió la acción antes de iniciar sesión")
			}
			w := test.NewWindow(b)
			defer w.Close()
			w.Resize(fyne.NewSize(anchoPrueba, AltoBarraSuite()))
			if w.Canvas().Focused() == b.buscador.campo {
				t.Fatal("enfoca el buscador antes de entrar")
			}
			s.Sesion.Sincronizar(auth.Tokens{Subject: "user"})
			b.cambioBusqueda(s.Estado())
			test.WidgetRenderer(b).Layout(w.Canvas().Size())
			if b.buscarPendiente || w.Canvas().Focused() != b.buscador.campo || cedido {
				t.Fatalf("no abre el buscador global: pendiente=%v, foco=%T, cedido=%v", b.buscarPendiente, w.Canvas().Focused(), cedido)
			}
			if b.buscador.campo.Texto() != "" || b.buscador.estado != enReposo || b.buscador.espera != nil {
				t.Fatal("la acción lanzó una búsqueda o conservó texto anterior")
			}
			w.Canvas().Unfocus()
			test.WidgetRenderer(b).Layout(w.Canvas().Size())
			if w.Canvas().Focused() == b.buscador.campo {
				t.Fatal("una colocación posterior vuelve a robar el foco")
			}
		})
	}
}

func TestWidgetBusquedaDescartaRespuestaDeLaCuentaAnterior(t *testing.T) {
	s := suiteConSesion(t)
	consulta, continuar := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(consulta)
		select {
		case <-continuar:
			_, _ = fmt.Fprint(w, `{"hits":[{"id":"private","title":"private old account","source":"notes"}]}`)
		case <-time.After(5 * time.Second):
			http.Error(w, "test timeout", http.StatusGatewayTimeout)
		}
	}))
	defer srv.Close()
	b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	t.Cleanup(b.Parar)
	b.buscador.hub = api.NewHub(srv.URL, nil, nil)
	b.buscador.termino = "same words"
	fin := make(chan struct{})
	go func() { b.buscador.buscar("same words"); close(fin) }()
	select {
	case <-consulta:
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó la consulta")
	}
	s.Sesion.Sincronizar(auth.Tokens{Subject: "second"})
	// The second person can type the same words; query equality alone is unsafe.
	close(continuar)
	select {
	case <-fin:
	case <-time.After(5 * time.Second):
		t.Fatal("no terminó la respuesta")
	}
	fyne.DoAndWait(func() {})
	if len(b.buscador.hits) != 0 {
		t.Fatal("la respuesta anterior expuso resultados a otra cuenta")
	}
}
