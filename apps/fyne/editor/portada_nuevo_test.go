// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// creaciones es services/docs de mentira para «Documento nuevo»: la lista de la portada y
// el título de cada documento que se crea.
type creaciones struct {
	servicio
	mu      sync.Mutex
	titulos []string
}

func (c *creaciones) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/documents" {
		var cuerpo struct{ Title string }
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &cuerpo)
		c.mu.Lock()
		c.titulos = append(c.titulos, cuerpo.Title)
		c.mu.Unlock()
	}
	c.servicio.ServeHTTP(w, r)
}

func (c *creaciones) creados() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.titulos...)
}

// TestDocumentoNuevoDelWidgetPreguntaAntesDeCrear: la acción «Documento nuevo» del widget
// nativo (`sytue-docs:/?action=create`) no escribe nada sola, porque el enlace lo abre
// cualquiera (ADR 0078). Espera a que la portada tenga su lista, pregunta «Nuevo
// documento» con el título, y solo con el sí crea —con ese título— y lo abre. Cancelar no
// crea nada, y dos enlaces seguidos no abren dos preguntas.
func TestDocumentoNuevoDelWidgetPreguntaAntesDeCrear(t *testing.T) {
	s := &creaciones{servicio: servicio{lista: dosDocs}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	for _, tipo := range []Tipo{Documento, Hoja, Presentacion} {
		p := NuevaPortada(tipo, nil, "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
		p.Lanzar = func(f func()) { f() }
		w := test.NewWindow(p.Objeto())
		w.Resize(fyne.NewSize(900, 700))
		var abierto api.Document
		p.AlAbrir = func(d api.Document) { abierto = d }

		// Antes de la lista (sin la vista montada): se apunta y no pregunta.
		p.PreguntarCrear(w.Canvas())
		if d := dialogoArriba(w.Canvas()); d != nil || !p.NuevoPendiente() {
			t.Fatalf("%v: sin lista ya pregunta (%v) o no se apunta", tipo, d)
		}
		p.Cargar()
		d := dialogoArriba(w.Canvas())
		if d == nil || d.Titulo != claveNuevo(tipo) || p.NuevoPendiente() {
			t.Fatalf("%v: al cargar la lista no pregunta «%s»: %+v", tipo, claveNuevo(tipo), d)
		}
		p.PreguntarCrear(w.Canvas())
		if otro := dialogoArriba(w.Canvas()); otro != d {
			t.Fatalf("%v: un segundo enlace abre otra pregunta", tipo)
		}
		if len(s.creados()) != 0 {
			t.Fatalf("%v: crea antes de contestar: %v", tipo, s.creados())
		}
		pie := d.Pie.(*fyne.Container).Objects
		pie[0].(*ui.Boton).Tapped(nil)
		p.atenderNuevo()
		if dialogoArriba(w.Canvas()) != nil || len(s.creados()) != 0 || abierto.ID != "" {
			t.Fatalf("%v: cancelar crea %v o abre %q", tipo, s.creados(), abierto.ID)
		}

		p.PreguntarCrear(w.Canvas())
		campo := w.Canvas().Focused().(*ui.Entrada)
		campo.PonerTexto("  Presupuesto 2027 ")
		campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
		if got := s.creados(); len(got) != 1 || got[0] != "Presupuesto 2027" || abierto.ID != "nuevo" {
			t.Fatalf("%v: con el sí crea %v y abre %q", tipo, got, abierto.ID)
		}
		if dialogoArriba(w.Canvas()) != nil || p.Creando {
			t.Fatalf("%v: la pregunta sigue abierta o la portada en «Creando…»", tipo)
		}
		if !strings.Contains(s.ultimo(), "POST /api/v1/documents") {
			t.Fatalf("%v: lo último no es crear: %s", tipo, s.ultimo())
		}
		w.Close()
		s.mu.Lock()
		s.titulos = nil
		s.mu.Unlock()
	}
}
