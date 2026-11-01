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
			t.Setenv("HOME", t.TempDir())
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

func TestWidgetCalendarioPublicaRejillaYProximosEventos(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ajustes.MarcarSesion()

	ahora := time.Now().UTC().Truncate(time.Minute)
	proximo := ahora.Add(45 * time.Minute)
	finProximo := proximo.Add(30 * time.Minute)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/calendars":
			_, _ = io.WriteString(w, `{"calendars":[{"id":"work","display_name":"Equipo","color":"#953be7"}]}`)
		case "/api/v1/calendars/shared-with-me":
			_, _ = io.WriteString(w, `{"calendars":[]}`)
		case "/api/v1/calendars/work/instances":
			_, _ = io.WriteString(w, `{"instances":[`+
				`{"start":"`+proximo.Format(time.RFC3339)+`","end":"`+finProximo.Format(time.RFC3339)+`","event":{"id":"e1","title":"Revisión de diseño","location":"Sala Norte","status":"CONFIRMED"}},`+
				`{"start":"`+proximo.Format(time.RFC3339)+`","end":"`+finProximo.Format(time.RFC3339)+`","event":{"id":"e2","title":"enc:v1:secreto","status":"CONFIRMED"}},`+
				`{"start":"`+proximo.Format(time.RFC3339)+`","end":"`+finProximo.Format(time.RFC3339)+`","event":{"id":"e3","title":"Cancelada","status":"CANCELLED"}}`+
				`]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &Pantalla{T: traductor(t), Cursor: ahora, Zona: time.UTC}
	a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
	a.lanzar = func(f func()) { f() }
	a.enUI = func(f func()) { f() }
	a.Widget = func() widgets.View {
		return widgets.NewView("eu.sytue.calendar", "calendar", widgets.Capture(ajustes.Leer().Sesion))
	}
	a.cargarCalendarios()

	snapshot, err := widgets.Read("eu.sytue.calendar")
	if err != nil {
		t.Fatalf("lectura del snapshot: %v", err)
	}
	if snapshot.Calendar == nil || snapshot.Calendar.Year != proximo.Year() || snapshot.Calendar.Month != int(proximo.Month()) {
		t.Fatalf("falta la rejilla mensual en el snapshot: %+v", snapshot.Calendar)
	}
	if len(snapshot.Calendar.BusyDays) == 0 {
		t.Fatalf("faltan los días ocupados del mes: %+v", snapshot.Calendar)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Title != "Revisión de diseño" || snapshot.Items[0].Subtitle != "Sala Norte" {
		t.Fatalf("debe publicar próximos eventos filtrando cancelados y cifrados: %+v", snapshot.Items)
	}
	if len(snapshot.Counters) != 2 || snapshot.Counters[1] != (widgets.Counter{Kind: "events", Value: 2}) {
		t.Fatalf("contadores inesperados: %+v", snapshot.Counters)
	}
}
