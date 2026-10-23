// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestWidgetLibretasCompletasSinConfundirContactosPaginados(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	for _, caso := range []string{"completas", "fallo", "cambia sesión", "copia sin conexión"} {
		t.Run(caso, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			ajustes.MarcarSesion()
			listados := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/address-books":
					listados++
					if caso == "cambia sesión" {
						ajustes.MarcarSesion()
					}
					if caso == "fallo" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = io.WriteString(w, `{"address_books":[{"id":"one","name":"Private"},{"id":"two"},{"id":"two"}]}`)
				default:
					_, _ = io.WriteString(w, `{"contacts":[],"favorites":[]}`)
				}
			}))
			defer srv.Close()
			a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
			a.enFondo = func(f func()) { f() }
			a.red = &offline.Monitor{}
			if caso == "copia sin conexión" {
				a.red.Report(offline.Offline)
			}
			a.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.contacts", "contacts", widgets.Capture(ajustes.Leer().Sesion))
			}
			a.Cargar()
			if listados != 1 {
				t.Fatalf("el widget añadió consultas: %d", listados)
			}
			snapshot, err := widgets.Read("eu.sytue.contacts")
			if caso != "completas" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("publicó datos sin confirmar la sesión o frescura: %+v, %v", snapshot, err)
				}
				return
			}
			if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "address_books", Value: 2}) {
				t.Fatalf("debe contar libretas únicas, no contactos de una página: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestWidgetIncluyeLibretaCreadaPorSincronizacionTardia(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ajustes.MarcarSesion()
	var created atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/address-books":
			created.Store(true)
			_, _ = io.WriteString(w, `{"id":"org","name":"organizacion"}`)
		case r.URL.Path == "/api/v1/address-books":
			if created.Load() {
				_, _ = io.WriteString(w, `{"address_books":[{"id":"one","is_default":true},{"id":"org","name":"organizacion"}]}`)
			} else {
				_, _ = io.WriteString(w, `{"address_books":[{"id":"one","is_default":true}]}`)
			}
		default:
			_, _ = io.WriteString(w, `{"contacts":[],"items":[],"uids":[]}`)
		}
	}))
	defer srv.Close()
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	a.enFondo = func(f func()) { f() }
	a.Directorio = api.NewDirectorio(srv.URL, nil, nil)
	a.Widget = func() widgets.View {
		return widgets.NewView("eu.sytue.contacts", "contacts", widgets.Capture(ajustes.Leer().Sesion))
	}
	a.Cargar()
	snapshot, err := widgets.Read("eu.sytue.contacts")
	if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0].Value != 2 || a.Pantalla.Activa == nil || a.Pantalla.Activa.ID != "one" {
		t.Fatalf("contador tardío/selección: %+v error=%v activa=%+v", snapshot, err, a.Pantalla.Activa)
	}
}
