// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestWidgetCalendariosCompletosSinPublicarListaParcial(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	for _, caso := range []string{"completos", "compartidos fallan", "propios fallan", "cambia sesión", "sin conexión"} {
		t.Run(caso, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			ajustes.MarcarSesion()
			var listados atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/calendars":
					listados.Add(1)
					if caso == "cambia sesión" {
						ajustes.MarcarSesion()
					}
					if caso == "propios fallan" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = io.WriteString(w, `{"calendars":[{"id":"own","display_name":"Private title"},{"id":"same"}]}`)
				case "/api/v1/calendars/shared-with-me":
					listados.Add(1)
					if caso == "compartidos fallan" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = io.WriteString(w, `{"calendars":[{"id":"shared","shared_by":"private@example.test"},{"id":"same"}]}`)
				default:
					_, _ = io.WriteString(w, `{"instances":[]}`)
				}
			}))
			defer srv.Close()
			p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC, SinConexion: caso == "sin conexión"}
			a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
			a.lanzar = func(f func()) { f() }
			a.enUI = func(f func()) { f() }
			a.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.calendar", "calendar", widgets.Capture(ajustes.Leer().Sesion))
			}
			a.cargarCalendarios()
			if got := listados.Load(); got != 2 {
				t.Fatalf("el widget añadió lecturas: %d", got)
			}
			snapshot, err := widgets.Read("eu.sytue.calendar")
			if caso != "completos" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("publicó datos parciales, sin red o de otra sesión: %+v, %v", snapshot, err)
				}
				return
			}
			if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "calendars", Value: 3}) {
				t.Fatalf("debe contar calendarios únicos completos, no eventos: %+v, %v", snapshot, err)
			}
		})
	}
}
