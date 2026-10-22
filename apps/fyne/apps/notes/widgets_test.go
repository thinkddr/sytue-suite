// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestWidgetNotasUsaConteosACLDelArbolCompleto(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	for _, caso := range []string{"completo", "fallo", "cambia sesión", "copia sin conexión"} {
		t.Run(caso, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			ajustes.MarcarSesion()
			listados := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/notebooks" {
					t.Errorf("el widget añadió una consulta: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				listados++
				if caso == "cambia sesión" {
					ajustes.MarcarSesion()
				}
				if caso == "fallo" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, `{"notebooks":[{"id":"own","note_count":8,"title":"Private title"},{"id":"shared","parent_id":"own","note_count":3},{"id":"shared","note_count":3},{"id":"daily","note_count":2,"is_daily":true},{"id":"archived","note_count":99,"archived_at":"2026-10-19T00:00:00Z"}]}`)
			}))
			defer srv.Close()
			red := &offline.Monitor{}
			p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), red, nil)
			p.activo.ID = "already-open"
			if caso == "copia sin conexión" {
				red.Report(offline.Offline)
				// El almacén habilita leer la última copia a través del transporte.
				p.Almacen = &offline.Store{}
			}
			p.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.notes", "notes", widgets.Capture(ajustes.Leer().Sesion))
			}
			p.cargarCuadernos()
			if listados != 1 {
				t.Fatalf("el widget añadió consultas: %d", listados)
			}
			snapshot, err := widgets.Read("eu.sytue.notes")
			if caso != "completo" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("publicó datos sin confirmar la sesión o frescura: %+v, %v", snapshot, err)
				}
				return
			}
			if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "notes", Value: 13}) {
				t.Fatalf("debe sumar ACL sin duplicar ni contar archivados: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestWidgetNotasRechazaConteosInvalidos(t *testing.T) {
	for _, n := range []int{-1, 1_000_000_000_001} {
		if total, valido := notasParaWidget([]api.Notebook{{ID: "one", NoteCount: n}}); valido {
			t.Fatalf("aceptó un contador inválido: %d", total)
		}
	}
	if _, valido := notasParaWidget([]api.Notebook{{ID: "one", NoteCount: 1_000_000_000_000}, {ID: "two", NoteCount: 1}}); valido {
		t.Fatal("aceptó una suma fuera del límite")
	}
	archivado := time.Now()
	if total, valido := notasParaWidget([]api.Notebook{{ID: "archive", ArchivedAt: &archivado, NoteCount: -1}}); !valido || total != 0 {
		t.Fatalf("incluyó un cuaderno archivado: %d, %v", total, valido)
	}
}
