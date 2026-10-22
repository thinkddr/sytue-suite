// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioEtiquetas hace de Drive con una taxonomía (o sin ella) y de Sytue AI, y apunta
// lo que no es de lectura.
type servicioEtiquetas struct {
	mu        sync.Mutex
	pedidas   []string
	etiquetas string
	contenido string
}

func (s *servicioEtiquetas) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/sensitivity-labels":
			if s.etiquetas == "" {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = io.WriteString(w, s.etiquetas)
			return
		case "GET /api/v1/files/f1/content":
			_, _ = io.WriteString(w, s.contenido)
			return
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
			return
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		case "POST /api/v1/assist":
			var q map[string]any
			_ = json.Unmarshal(cuerpo, &q)
			texto := "**Acta** de la junta: se aprueba el presupuesto."
			if q["action"] == "puntos" {
				texto = "- Primero\n- Segundo"
			}
			s.mu.Lock()
			s.pedidas = append(s.pedidas, "assist "+q["action"].(string)+" "+q["kind"].(string)+" "+q["modo"].(string)+" "+
				string(rune('0'+len(q["context"].(string))/1000%10)))
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"text": texto})
			return
		}
		s.mu.Lock()
		s.pedidas = append(s.pedidas, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
		s.mu.Unlock()
		_, _ = io.WriteString(w, `{"id":"f1"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioEtiquetas) lista() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pedidas)
}

// TestClasificarDiceLoQueImplicaYAplica: cada etiqueta dice si bloquea el enlace público
// o es informativa, la actual va marcada, y pulsar una la pone por su ID; «Quitar» solo
// sale si ya hay una, y quita con un DELETE.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `label.informational` en las dos ramas
// del `if l.BlockExternalShare`, la etiqueta que bloquea no lo dice.
func TestClasificarDiceLoQueImplicaYAplica(t *testing.T) {
	s := &servicioEtiquetas{etiquetas: `{"labels":[{"id":"l1","name":"Confidencial","block_external_share":true},{"id":"l2","name":"Interna"}]}`}
	p := pantallaDePrueba(t, s.servidor(t))
	el := Elemento{ID: "f1", Nombre: "acta.odt", Etiqueta: &api.DriveLabel{Name: "Interna"}}
	dc := p.pedirClasificar(el)
	if dc == nil || len(p.taxonomia) != 2 {
		t.Fatalf("la taxonomía no llega: %+v", p.taxonomia)
	}
	var filas []*filaEtiqueta
	recorrer(dc.cuerpo, func(o fyne.CanvasObject) {
		if f, es := o.(*filaEtiqueta); es {
			filas = append(filas, f)
		}
	})
	if len(filas) != 3 {
		t.Fatalf("hay %d filas y tiene que haber dos etiquetas y «Quitar»", len(filas))
	}
	if filas[0].detalle != p.tr.T("label.blocks", nil) || filas[1].detalle != p.tr.T("label.informational", nil) {
		t.Errorf("lo que implica cada una: %q / %q", filas[0].detalle, filas[1].detalle)
	}
	if filas[0].marcada || !filas[1].marcada || !filas[2].peligro {
		t.Error("la marca va en la actual y «Quitar» en rojo")
	}
	filas[0].Tapped(nil)
	dc2 := p.pedirClasificar(el)
	recorrer(dc2.cuerpo, func(o fyne.CanvasObject) {
		if f, es := o.(*filaEtiqueta); es && f.peligro {
			f.Tapped(nil)
		}
	})
	quiero := []string{`PUT /api/v1/files/f1/label {"label_id":"l1"}`, "DELETE /api/v1/files/f1/label"}
	if got := s.lista(); !slices.Equal(got, quiero) {
		t.Errorf("al servicio ha llegado %q y se esperaba %q", got, quiero)
	}
}

// TestClasificarSinTaxonomiaDiceQueNoHay: un fallo al leer la taxonomía no es un error
// en pantalla, es «tu organización todavía no clasifica», que es lo que sirve.
func TestClasificarSinTaxonomiaDiceQueNoHay(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioEtiquetas{}).servidor(t))
	dc := p.pedirClasificar(Elemento{ID: "f1", Nombre: "acta.odt"})
	if p.taxonomia == nil || len(p.taxonomia) != 0 {
		t.Fatalf("un fallo deja la taxonomía en %+v", p.taxonomia)
	}
	// El título se busca en el estado vacío y no como un rótulo suelto: es largo y parte en
	// dos líneas en el diálogo de talla sm.
	dice := false
	recorrer(dc.cuerpo, func(o fyne.CanvasObject) {
		if x, es := o.(*ui.Vacio); es && x.Titulo == p.tr.T("label.emptyTitle", nil) {
			dice = true
		}
	})
	if !dice {
		t.Error("sin taxonomía no dice que la organización no ha definido clasificaciones")
	}
}

// TestResumirMandaLoDeLaWeb: el cuerpo que llega a Sytue AI es el del BFF de la web
// —`kind: file`, `modo: calidad` y la acción— y el texto se corta a 200 KB. Cambiar a
// «puntos» vuelve a pedir y pinta la lista con sus viñetas.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `io.LimitReader`, el contexto de un
// fichero de 300 KB llega entero (un 7 en los miles módulo 10, en vez del 4).
func TestResumirMandaLoDeLaWeb(t *testing.T) {
	s := &servicioEtiquetas{contenido: strings.Repeat("a", 300*1024)}
	srv := s.servidor(t)
	p := pantallaDePrueba(t, srv)
	p.IA = api.NewIA(srv.URL, sesionDoble{}, nil)
	dr := p.pedirResumen(Elemento{ID: "f1", Nombre: "acta.md"})
	if dr == nil || dr.fallo || len(dr.bloques) != 1 {
		t.Fatalf("el resumen no llega: %+v", dr)
	}
	dr.accion = "puntos"
	dr.pedir()
	if len(dr.bloques) != 2 || !contieneTexto(dr.cuerpo, "•") {
		t.Errorf("los puntos no se pintan como lista: %+v", dr.bloques)
	}
	// 200 KB = 204.800 bytes: los miles módulo 10 son 4.
	quiero := []string{"assist resumir file calidad 4", "assist puntos file calidad 4"}
	if got := s.lista(); !slices.Equal(got, quiero) {
		t.Errorf("a Sytue AI ha llegado %q y se esperaba %q", got, quiero)
	}
}

// TestClasificarYResumirSePintanEnLosDosTemas: un token mal escrito entra en pánico AL
// PINTAR.
func TestClasificarYResumirSePintanEnLosDosTemas(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		s := &servicioEtiquetas{etiquetas: `{"labels":[{"id":"l1","name":"Confidencial","block_external_share":true}]}`, contenido: "hola"}
		srv := s.servidor(t)
		p := pantallaDePrueba(t, srv)
		p.IA = api.NewIA(srv.URL, sesionDoble{}, nil)
		p.pedirClasificar(Elemento{ID: "f1", Nombre: "acta.odt", Etiqueta: &api.DriveLabel{Name: "Confidencial"}})
		p.win.Canvas().Capture()
		p.pedirResumen(Elemento{ID: "f1", Nombre: "acta.md"})
		p.win.Canvas().Capture()
	}
}
