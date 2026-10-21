// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// servidorDePlantillas contesta como services/docs: la galería con una plantilla de
// Sytue de cada tipo y una de la organización con dos versiones, y los temas con marca.
type servidorDePlantillas struct {
	mu      sync.Mutex
	cuerpos []string
}

func (s *servidorDePlantillas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents":
		_, _ = io.WriteString(w, `{"documents":[],"next_cursor":""}`)
	case "GET /api/v1/templates":
		_, _ = io.WriteString(w, `{"templates":[
{"key":"carta","kind":"document","name":"Carta","description":"Una carta formal con membrete."},
{"key":"presupuesto","kind":"spreadsheet","name":"Presupuesto","description":"Hoja"}],
"org_templates":[{"key":"org:d1","kind":"document","name":"Acta interna","description":"","versions":[{"no":2,"id":"v2"},{"no":1,"id":"v1"}]}],
"can_manage":true}`)
	case "GET /api/v1/themes":
		_, _ = io.WriteString(w, `{"themes":[{"id":"papel"},{"id":"carbon"}],"brand":{"version":3,"name":"Acme","bg":"#FFFFFF","ink":"#0A0A0A","accent":"#1F2937"},"versions":[],"can_manage":true}`)
	case "POST /api/v1/documents":
		s.mu.Lock()
		s.cuerpos = append(s.cuerpos, string(b))
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"n1","title":"Nuevo","kind":"document"},"role":"owner"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func portadaConPlantillas(t *testing.T) (*Portada, *servidorDePlantillas) {
	t.Helper()
	s := &servidorDePlantillas{}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPortada(Documento, tr.T, "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	p.Lanzar = func(f func()) { f() }
	p.ConPlantillas = true
	return p, s
}

// La galería trae solo las plantillas del tipo del producto, con la marca primero entre
// los temas, y se pinta con la fila de la organización antes que la de Sytue.
func TestLaGaleriaEnseñaLasPlantillasDelTipo(t *testing.T) {
	p, _ := portadaConPlantillas(t)
	p.Cargar()
	if len(p.Plantillas) != 1 || p.Plantillas[0].Key != "carta" {
		t.Fatalf("un documento solo ve plantillas de documento: %+v", p.Plantillas)
	}
	if len(p.PlantillasOrg) != 1 || len(p.PlantillasOrg[0].Versions) != 2 {
		t.Fatalf("la de la organización trae sus versiones: %+v", p.PlantillasOrg)
	}
	if len(p.Temas) != 3 || p.Temas[0].ID != api.IDTemaMarca || p.Temas[0].Rotulo != "Marca: Acme (v3)" || p.Temas[1].Rotulo != "Papel (predeterminado)" {
		t.Fatalf("temas: %+v", p.Temas)
	}
	if !p.PuedeMarca || p.Marca == nil || p.Marca.Version != 3 {
		t.Errorf("la marca y el permiso: %v %+v", p.PuedeMarca, p.Marca)
	}
	g := p.galeria()
	if g == nil {
		t.Fatal("con plantillas hay galería")
	}
	w := test.NewTempWindow(t, g)
	w.Resize(fyne.NewSize(900, 600))
	if MedidaGaleria(g) < 2*MedidaCromoPlantilla().Height {
		t.Errorf("la galería mide %v: no caben las dos filas", MedidaGaleria(g))
	}
}

// Sin plantillas no hay galería (la web no pinta ni el selector de tema), y sin
// ConPlantillas no se pide nada.
func TestSinPlantillasNoHayGaleria(t *testing.T) {
	p, _ := portadaConPlantillas(t)
	p.ConPlantillas = false
	p.Cargar()
	if p.galeria() != nil || p.Plantillas != nil {
		t.Error("sin ConPlantillas no se pide ni se pinta la galería")
	}
}

// Crear desde una plantilla de la organización manda la clave, el idioma, el tema y la
// versión SOLO si no es la vigente, y abre el documento que da el servicio.
func TestCrearDesdeLaPlantillaMandaVersionYTema(t *testing.T) {
	p, s := portadaConPlantillas(t)
	p.Cargar()
	org := p.PlantillasOrg[0]
	var abierto string
	p.AlAbrir = func(d api.Document) { abierto = d.ID }

	p.Tema = api.IDTemaMarca
	p.CrearDesde(org, "") // la vigente: sin template_version
	p.CrearDesde(org, "v1")
	if abierto != "n1" {
		t.Errorf("no se abrió el documento creado: %q", abierto)
	}
	quiero := []string{
		`{"template":"org:d1","locale":"es","theme":"marca"}`,
		`{"template":"org:d1","locale":"es","template_version":"v1","theme":"marca"}`,
	}
	if len(s.cuerpos) != 2 || s.cuerpos[0] != quiero[0] || s.cuerpos[1] != quiero[1] {
		t.Errorf("cuerpos:\n  %v\nquiero:\n  %v", s.cuerpos, quiero)
	}
	if p.Creando || p.CreandoClave != "" {
		t.Error("la portada se queda en «Creando…»")
	}
}
