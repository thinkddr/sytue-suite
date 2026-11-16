// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// botonesDeLote son los rótulos de los botones de la barra de selección.
func botonesDeLote(e *Explorador) []string {
	var out []string
	recorrer(e.Objeto(testVariante()), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && (b.Variante == ui.Secundario || b.Variante == ui.Peligro) {
			out = append(out, b.Texto)
		}
	})
	return out
}

// TestCombinarSoloConDosOMasPDFYNadaMas es el `puedeCombinar` de la web: «Combinar PDF»
// sale delante de «Mover» con dos o más PDF marcados y nada más; con un .docx o una
// carpeta en la selección, con un solo PDF, sin red o sin cliente de PDF, no sale.
//
// VISTO EN ROJO (2026-11-15): sin el botón, la barra solo lleva «Mover», «Descargar» y
// «Papelera» con tres PDF marcados.
func TestCombinarSoloConDosOMasPDFYNadaMas(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	combinar := tr.T("pdf.combine", nil)
	nuevo := func(marcar ...string) *Explorador {
		e := NuevoExplorador(tr)
		e.Fase, e.Ventana = Exito, 1280
		e.AlCombinarPDF = func() {}
		e.Elementos = []Elemento{
			{ID: "c", Nombre: "Facturas", Carpeta: true, Tamano: -1},
			{ID: "p1", Nombre: "a.pdf", Tamano: 10},
			{ID: "p2", Nombre: "B.PDF", Tamano: 10},
			{ID: "p3", Nombre: "c.pdf", Tamano: 10},
			{ID: "d", Nombre: "notas.docx", Tamano: 10},
		}
		for _, id := range marcar {
			e.Seleccion[id] = true
		}
		return e
	}

	e := nuevo("p3", "p1", "p2")
	if !e.PuedeCombinar() {
		t.Fatal("con tres PDF marcados no se puede combinar")
	}
	// «Nuevo», de la cabecera, también es secundario: lo que cuenta es que vaya
	// delante de «Mover».
	b := botonesDeLote(e)
	if i, j := slices.Index(b, combinar), slices.Index(b, tr.T("files.selection.move", nil)); i < 0 || i > j {
		t.Errorf("«Combinar PDF» no va delante de «Mover» en la barra: %q", b)
	}
	marcados := e.PDFMarcados()
	ids := make([]string, 0, len(marcados))
	for _, el := range marcados {
		ids = append(ids, el.ID)
	}
	if !slices.Equal(ids, []string{"p1", "p2", "p3"}) {
		t.Errorf("se combinarían en el orden %q, y no en el de la lista", ids)
	}

	for nombre, e := range map[string]*Explorador{
		"un solo PDF":       nuevo("p1"),
		"un PDF y un .docx": nuevo("p1", "p2", "d"),
		"PDF y una carpeta": nuevo("p1", "p2", "c"),
	} {
		if e.PuedeCombinar() || slices.Contains(botonesDeLote(e), combinar) {
			t.Errorf("con %s se ofrece combinar", nombre)
		}
	}
	sinRed := nuevo("p1", "p2")
	sinRed.Desconectado = true
	if sinRed.PuedeCombinar() {
		t.Error("sin red se ofrece combinar")
	}
	sinCliente := nuevo("p1", "p2")
	sinCliente.AlCombinarPDF = nil
	if sinCliente.PuedeCombinar() || slices.Contains(botonesDeLote(sinCliente), combinar) {
		t.Error("sin cliente de PDF se ofrece combinar")
	}
}

// motorDoble es Drive y el motor de PDF de mentira en el mismo servidor: sirve cada PDF
// por su id, apunta en qué orden le llegan al motor y qué se sube.
type motorDoble struct {
	mu       sync.Mutex
	pedidas  []string
	alMotor  []string
	op       string
	subido   string
	cuerpo   string
	carpetas map[string]string // id → contenido
}

func (m *motorDoble) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		ruta := r.Method + " " + r.URL.Path
		m.pedidas = append(m.pedidas, ruta)
		switch {
		case ruta == "POST /api/v1/pdf/pages":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			m.op = r.FormValue("op")
			for _, f := range r.MultipartForm.File["file"] {
				m.alMotor = append(m.alMotor, f.Filename)
			}
			_, _ = io.WriteString(w, "%PDF-unido")
		case ruta == "POST /api/v1/folders/carpeta/files":
			m.subido = r.URL.Query().Get("name")
			b, _ := io.ReadAll(r.Body)
			m.cuerpo = string(b)
			_, _ = io.WriteString(w, `{"id":"nuevo","folder_id":"carpeta","name":"`+m.subido+`"}`)
		case strings.HasPrefix(ruta, "GET /api/v1/files/") && strings.HasSuffix(ruta, "/content"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/files/"), "/content")
			datos, ok := m.carpetas[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, datos)
		case ruta == "GET /api/v1/folders/carpeta/children":
			// «a (combinado).pdf» ya existe: el nuevo no lo pisa.
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"p1","name":"a.pdf"},{"id":"v","name":"a (combinado).pdf"}]}`)
		case strings.HasPrefix(ruta, "GET "):
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (m *motorDoble) pedido(prefijo string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.ContainsFunc(m.pedidas, func(r string) bool { return strings.HasPrefix(r, prefijo) })
}

// TestCombinarSubeUnPDFNuevoJuntoALosOriginales: con tres PDF marcados, «Combinar»
// los baja en el orden de la lista, el motor los une con `combinar` y el resultado se
// sube a su carpeta con la palabra del idioma y un nombre que no pisa a ninguno. El
// aviso dice cómo se llama y junto a cuál está, y la selección se suelta.
//
// VISTO EN ROJO (2026-11-15): sin `combinarPDF`, la pantalla no pide nada al motor.
func TestCombinarSubeUnPDFNuevoJuntoALosOriginales(t *testing.T) {
	test.NewTempApp(t)
	m := &motorDoble{carpetas: map[string]string{"p1": "%PDF-1", "p2": "%PDF-2", "p3": "%PDF-3"}}
	srv := m.servidor(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.aparte = func(f func()) { f() }
	p.ConPDF(api.NewPDF(srv.URL, srv.URL, sesionDoble{}, nil))
	p.explor.Fase, p.explor.Ventana = Exito, 1280
	p.explor.Ruta = []Elemento{{ID: "carpeta", Nombre: "Contratos", Carpeta: true}}
	p.explor.Elementos = []Elemento{
		{ID: "p1", Nombre: "a.pdf", Tamano: 6, Padre: "carpeta"},
		{ID: "d", Nombre: "notas.docx", Tamano: 6, Padre: "carpeta"},
		{ID: "p2", Nombre: "b.pdf", Tamano: 6, Padre: "carpeta"},
		{ID: "p3", Nombre: "c.pdf", Tamano: 6, Padre: "carpeta"},
	}
	for _, id := range []string{"p3", "p2", "p1"} {
		p.explor.Seleccion[id] = true
	}
	if p.explor.AlCombinarPDF == nil {
		t.Fatal("ConPDF no engancha «Combinar PDF»")
	}
	p.explor.AlCombinarPDF()
	for i := 0; i < 200 && len(p.avisos.Avisos()) == 0; i++ {
		esperar()
	}

	m.mu.Lock()
	op, alMotor, subido, cuerpo := m.op, slices.Clone(m.alMotor), m.subido, m.cuerpo
	m.mu.Unlock()
	if op != api.PDFCombinar || !slices.Equal(alMotor, []string{"a.pdf", "b.pdf", "c.pdf"}) {
		t.Errorf("al motor llega op=%q con %q", op, alMotor)
	}
	if subido != "a (combinado 2).pdf" || cuerpo != "%PDF-unido" {
		t.Errorf("se ha subido %q con %q", subido, cuerpo)
	}
	avisos := p.avisos.Avisos()
	if len(avisos) == 0 {
		t.Fatal("combinar no avisa")
	}
	a := avisos[len(avisos)-1]
	if a.Tono != ui.AvisoExito || a.Titulo != p.tr.T("pdf.done", i18n.Vars{"name": "a (combinado 2).pdf"}) ||
		a.Descripcion != p.tr.T("pdf.doneWhere", i18n.Vars{"name": "a.pdf"}) {
		t.Errorf("el aviso es %+v", a)
	}
	esperar()
	if n := p.explor.Marcados(); n != 0 {
		t.Errorf("tras combinar siguen marcados %d", n)
	}
	if p.explor.Ocupado || p.explor.Combinando {
		t.Error("la barra se queda ocupada tras combinar")
	}
}

// TestCombinarPasadoDelTopeNoBajaNada: el tope del motor es de la petición ENTERA, así
// que tres PDF que juntos pasan de PDFMaxBytes se paran antes de bajar nada, con su
// aviso propio y no con el genérico.
func TestCombinarPasadoDelTopeNoBajaNada(t *testing.T) {
	test.NewTempApp(t)
	m := &motorDoble{carpetas: map[string]string{}}
	srv := m.servidor(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.ConPDF(api.NewPDF(srv.URL, srv.URL, sesionDoble{}, nil))
	tercio := int64(api.PDFMaxBytes/3 + 1)
	pdfs := []Elemento{
		{ID: "p1", Nombre: "a.pdf", Tamano: tercio},
		{ID: "p2", Nombre: "b.pdf", Tamano: tercio},
		{ID: "p3", Nombre: "c.pdf", Tamano: tercio},
	}
	p.Combinar(pdfs, "carpeta")
	esperar()
	if m.pedido("GET /api/v1/files/") || m.pedido("POST ") {
		t.Errorf("pasado el tope se ha pedido %q", m.pedidas)
	}
	avisos := p.avisos.Avisos()
	if len(avisos) == 0 || avisos[len(avisos)-1].Titulo != p.tr.T("pdf.combineTooLarge", nil) {
		t.Errorf("pasado el tope no se avisa con su texto: %+v", avisos)
	}
}

// TestCombinarQueFallaLoDice: si el motor rechaza uno, se avisa con `pdf.failed` y no se
// sube nada; la selección se queda para volver a intentarlo.
func TestCombinarQueFallaLoDice(t *testing.T) {
	test.NewTempApp(t)
	m := &motorDoble{carpetas: map[string]string{"p1": "%PDF-1"}} // p2 no está: 404
	srv := m.servidor(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.ConPDF(api.NewPDF(srv.URL, srv.URL, sesionDoble{}, nil))
	p.explor.Seleccion["p1"], p.explor.Seleccion["p2"] = true, true
	p.Combinar([]Elemento{{ID: "p1", Nombre: "a.pdf"}, {ID: "p2", Nombre: "b.pdf"}}, "carpeta")
	esperar()
	if m.pedido("POST ") {
		t.Errorf("con un original que no baja se ha seguido: %q", m.pedidas)
	}
	avisos := p.avisos.Avisos()
	if len(avisos) == 0 || avisos[len(avisos)-1].Tono != ui.AvisoPeligro ||
		avisos[len(avisos)-1].Titulo != p.tr.T("pdf.failed", nil) {
		t.Errorf("el fallo no se dice: %+v", avisos)
	}
	if p.explor.Marcados() != 2 {
		t.Error("tras fallar se suelta la selección")
	}
}
