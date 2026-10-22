// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los mismos vectores que `normalizeHex` de slides/temas.ts.
func TestNormalizarHexEsElDeLaWeb(t *testing.T) {
	for entrada, quiere := range map[string]string{
		"#fff": "#FFFFFF", "abc": "#AABBCC", "1f2937": "#1F2937", "#1f2937": "#1F2937",
	} {
		if got, ok := NormalizarHex(entrada); !ok || got != quiere {
			t.Errorf("%q: %q %v, quiere %q", entrada, got, ok, quiere)
		}
	}
	for _, malo := range []string{"", "#ff", "#fffff", "rojo", "#GGGGGG", "#1234567"} {
		if got, ok := NormalizarHex(malo); ok {
			t.Errorf("%q no es un color y salió %q", malo, got)
		}
	}
}

type servidorDeTemas struct {
	mu      sync.Mutex
	cambio  bool
	cuerpos []string
}

func (s *servidorDeTemas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/themes":
		_, _ = io.WriteString(w, `{"themes":[{"id":"papel"},{"id":"carbon"}],"brand":{"version":3,"name":"Acme","bg":"#FFFFFF","ink":"#0A0A0A","accent":"#1F2937"},"versions":[{"version":3,"name":"Acme","bg":"#FFFFFF","ink":"#0A0A0A","accent":"#1F2937"}],"can_manage":true}`)
	case "POST /api/v1/documents/d1/theme":
		s.mu.Lock()
		s.cuerpos = append(s.cuerpos, string(b))
		s.mu.Unlock()
		if s.cambio {
			_, _ = io.WriteString(w, `{"changed":true}`)
		} else {
			_, _ = io.WriteString(w, `{"changed":false}`)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func contextoDeTemas(t *testing.T) (*Contexto, *[]string) {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	var avisos []string
	c := &Contexto{Tipo: Hoja, T: tr.T, Refrescar: func() {}}
	c.Notificar = func(_ ui.TonoAviso, titulo string) { avisos = append(avisos, titulo) }
	return c, &avisos
}

// El grupo «Tema» no existe hasta que llegan los temas, y entonces ofrece el
// placeholder, la marca, el catálogo y —solo a quien administra— «Editar la marca…».
func TestElGrupoTemaOfreceMarcaCatalogoYEdicion(t *testing.T) {
	s := &servidorDeTemas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	c, _ := contextoDeTemas(t)
	temas := &TemasDoc{Svc: api.NewDocs(srv.URL, sesionFija{}, srv.Client()), Lanzar: func(f func()) { f() }, DocID: "d1"}
	c.Temas = temas
	// La primera llamada pide los temas (síncrono en la prueba) y no devuelve nada aún.
	temas.cargar(nil)
	g := temas.Grupo(c)
	if len(g) != 2 {
		t.Fatalf("con datos hay separador y grupo: %d", len(g))
	}
	var sel *SelectorCinta
	var buscar func(fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		if x, ok := o.(*SelectorCinta); ok {
			sel = x
		}
		if gr, ok := o.(*Grupo); ok {
			for _, h := range gr.Controles {
				buscar(h)
			}
		}
	}
	buscar(g[1])
	if sel == nil {
		t.Fatal("el grupo lleva un selector")
	}
	rotulos := make([]string, 0, len(sel.Opciones))
	for _, o := range sel.Opciones {
		rotulos = append(rotulos, o.Rotulo)
	}
	quiero := "Elige un tema…|Marca: Acme (v3)|Actualizar a la marca actual…|Papel (predeterminado)|Carbón|Editar la marca de la organización…"
	if strings.Join(rotulos, "|") != quiero {
		t.Errorf("opciones:\n  %q\nquiero:\n  %q", strings.Join(rotulos, "|"), quiero)
	}
	// Sin permiso de administrar no se ofrece editar la marca.
	temas.datos.CanManage = false
	g = temas.Grupo(c)
	buscar(g[1])
	if n := len(sel.Opciones); n != 5 {
		t.Errorf("sin administrar sobra la edición de la marca: %d opciones", n)
	}
}

// Aplicar un tema que cambia algo reabre el documento; uno que no, solo avisa.
func TestAplicarTemaRecargaSoloSiCambia(t *testing.T) {
	s := &servidorDeTemas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	c, avisos := contextoDeTemas(t)
	recargas, guardados := 0, 0
	temas := &TemasDoc{
		Svc: api.NewDocs(srv.URL, sesionFija{}, srv.Client()), Lanzar: func(f func()) { f() }, DocID: "d1",
		Guardar: func() { guardados++ }, Recargar: func() { recargas++ },
	}
	temas.Aplicar(c, "carbon")
	if recargas != 0 || guardados != 1 || len(*avisos) != 1 || (*avisos)[0] != "El documento ya tiene ese tema" {
		t.Errorf("sin cambio: recargas %d, guardados %d, avisos %v", recargas, guardados, *avisos)
	}
	s.cambio = true
	temas.Aplicar(c, api.IDTemaMarca)
	if recargas != 1 || guardados != 2 || (*avisos)[1] != "Tema aplicado" {
		t.Errorf("con cambio: recargas %d, guardados %d, avisos %v", recargas, guardados, *avisos)
	}
	if len(s.cuerpos) != 2 || s.cuerpos[1] != `{"theme":"marca"}` {
		t.Errorf("cuerpos: %v", s.cuerpos)
	}
}

// Las líneas de la previa dicen lo mismo que `lineaDeCambio` de la web.
func TestLineaDeCambioEsLaDeLaWeb(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	casos := map[string]api.DocThemeChange{
		"Títulos: 3 · color predeterminado → #1F2937":                             {Scope: "headings", Count: 3, To: "#1F2937"},
		"Fila de encabezado de la hoja Ventas: 4 celdas · relleno #FFF → #1F2937": {Scope: "sheet_header", Sheet: "Ventas", Count: 4, From: "#FFF", To: "#1F2937"},
		"Logotipo de la organización: añadido o actualizado":                      {Scope: "logo", To: "abc"},
		"Logotipo de la organización: quitado":                                    {Scope: "logo"},
	}
	for quiere, k := range casos {
		if got := LineaDeCambio(tr.T, k); got != quiere {
			t.Errorf("%+v: %q, quiere %q", k, got, quiere)
		}
	}
}
