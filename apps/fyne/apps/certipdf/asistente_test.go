// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// Los casos de `ocr.test.ts`, uno a uno.
func TestLasPaginasALeerSonLasDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		total, actual, tope int
		quiero              []int
	}{
		{100, 40, 5, []int{40, 41, 42, 43, 44}},
		{10, 9, 5, []int{6, 7, 8, 9, 10}},
		{3, 2, maxPaginasOCR, []int{1, 2, 3}},
		{0, 1, maxPaginasOCR, nil},
		{5, 99, 3, []int{3, 4, 5}},
		{5, -4, 2, []int{1, 2}},
	} {
		if got := paginasALeer(c.total, c.actual, c.tope); !slices.Equal(got, c.quiero) {
			t.Errorf("paginasALeer(%d, %d, %d) = %v; la web, %v", c.total, c.actual, c.tope, got, c.quiero)
		}
	}
	if n := len(paginasALeer(500, 1, maxPaginasOCR)); n != 12 {
		t.Errorf("el tope por defecto lee %d páginas; la web, 12", n)
	}
	if got := juntar([]paginaLeida{{3, " tres "}, {1, "uno"}}); got != "[página 1]\nuno\n\n[página 3]\ntres" {
		t.Errorf("juntar = %q", got)
	}
	if got := juntar([]paginaLeida{{1, "   "}}); got != "" {
		t.Errorf("una página sin texto sale: %q", got)
	}
	if esLegible("a b c") || !esLegible(strings.Repeat("x", 40)) {
		t.Error("esLegible no es el de la web")
	}
}

// respuestaDePrueba es la del arnés de la web.
const respuestaDePrueba = "**Resumen**\nEl contrato obliga a *las partes* a prestar el servicio durante doce meses por **1.200 €** (página 2).\n- Precio: 1.200 € al año (página 2)\n- Plazo: doce meses prorrogables (página 3)"

// TestElAsistenteSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y rem16. Aviso en155 de32; botones en195 de32; pregunta235 de36.
// Respuesta: título279, párrafo301 con cursiva en114,64063 y negrita en141,17188
// en la segunda línea317; puntos339/361 con texto en19,875; Copiar383.
func TestElAsistenteSeColocaComoEnLaWeb(t *testing.T) {
	p, w := pantallaConPanel(t, PanelAsistente)
	defer w.Close()
	a := p.panel.asistenteP
	if a == nil {
		t.Fatal("el asistente no se ha montado")
	}
	botones := map[string]*ui.Boton{}
	todos(w.Content(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok {
			botones[b.Texto] = b
		}
	})
	caja := func(que string, o fyne.CanvasObject, x, y, ancho, alto float32) {
		t.Helper()
		if o == nil {
			t.Fatalf("no está %s", que)
		}
		casiTexto(t, que+" x", absoluta(o).X, x)
		cerca(t, que+" y", absoluta(o).Y, y)
		casiTexto(t, que+" ancho", o.Size().Width, ancho)
		cerca(t, que+" alto", o.Size().Height, alto)
	}
	caja("«Resumir»", botones["Resumir"], 8, 195, 74.04688, 32)
	caja("«Puntos clave»", botones["Puntos clave"], 86.04688, 195, 100.95313, 32)
	caja("el campo", a.pregunta, 8, 235, 184.45313, 36)
	caja("«Preguntar»", botones["Preguntar"], 196.45313, 235, 82.54688, 32)
	if !botones["Preguntar"].Inerte {
		t.Error("«Preguntar» sin pregunta tiene que estar apagado")
	}

	a.respuesta = ia.BloquesDe(respuestaDePrueba)
	a.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	palabras := map[string]*canvas.Text{}
	var copiar *ui.Boton
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *canvas.Text:
			if _, ya := palabras[x.Text]; !ya {
				palabras[x.Text] = x
			}
		case *ui.Boton:
			if x.Texto == "Copiar" {
				copiar = x
			}
		}
	})
	// Las palabras se colocan en su caja de línea: la línea empieza donde su texto menos
	// lo que [ui.ColocarTexto] lo baja para poner la línea base donde Chromium.
	linea := func(x *canvas.Text) float32 {
		return absoluta(x).Y - ui.DesfaseTexto(x, 16)
	}
	for _, c := range []struct {
		palabra string
		x, y    float32
	}{
		{"Resumen", 8, 279},
		{"El", 8, 301},
		{"las", 114.64063, 301},
		{"1.200", 141.17188, 317},
		{"Precio:", 19.875, 339},
		{"Plazo:", 19.875, 361},
	} {
		x := palabras[c.palabra]
		if x == nil {
			t.Errorf("no se pinta «%s»", c.palabra)
			continue
		}
		casiTexto(t, "«"+c.palabra+"» x", absoluta(x).X, c.x)
		cerca(t, "«"+c.palabra+"» y", linea(x), c.y)
	}
	if palabras["las"] != nil && palabras["las"].FontSource != tema.Cursiva {
		t.Error("«las partes» va en cursiva en la web")
	}
	caja("«Copiar»", copiar, 8, 383, 63.5, 32)
}

// TestResumirMandaElDocumentoConSusPaginas: «Resumir» pide al servicio `resumir` en el
// modo calidad, con la capa de texto y su número de página delante, y pinta lo que vuelve.
func TestResumirMandaElDocumentoConSusPaginas(t *testing.T) {
	var pedido struct {
		Action, Kind, Modo, Context string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&pedido)
		_ = json.NewEncoder(w).Encode(map[string]string{"text": respuestaDePrueba})
	}))
	defer srv.Close()
	p, w := pantallaConPanel(t, PanelAsistente)
	defer w.Close()
	listo := make(chan struct{}, 4)
	p.trasRed = func() { listo <- struct{}{} }
	p.IA = api.NewIA(srv.URL, nil, nil)
	p.textos = [][]Fragmento{{{Texto: "Contrato"}, {Texto: "de obra"}}, {}, {{Texto: "Precio"}}}
	a := p.panel.asistenteP
	a.preguntar(instruccionResumen, "resumir")
	esperarRed(t, listo)
	if pedido.Action != "resumir" || pedido.Kind != "pdf" || pedido.Modo != "calidad" {
		t.Errorf("se pidió %+v", pedido)
	}
	if pedido.Context != "\n\n[página 1]\nContrato de obra\n\n[página 3]\nPrecio" {
		t.Errorf("el contexto es %q", pedido.Context)
	}
	if len(a.respuesta) != 4 || a.ocupado {
		t.Errorf("la respuesta son %d bloques (ocupado %v)", len(a.respuesta), a.ocupado)
	}
}

// TestEnUnEscaneoSeLeePrimero: sin capa de texto, Paneles.tsx real en Chromium a
// rem16 muestra la caja en8,195 de271×90. Leer ocupa17,244 de253×32 y Resumir baja a293.
func TestEnUnEscaneoSeLeePrimero(t *testing.T) {
	c := documentoDePrueba()
	for i := range c.Paginas {
		c.Paginas[i].Extracto = new("")
	}
	_, w := pantallaConDocumento(t, c, PanelAsistente)
	defer w.Close()
	botones := map[string]*ui.Boton{}
	todos(w.Content(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok {
			botones[b.Texto] = b
		}
	})
	leer := botones["Leer el escaneo con IA"]
	if leer == nil {
		t.Fatal("un escaneo no ofrece leerlo")
	}
	cerca(t, "«Leer» x", absoluta(leer).X, 17)
	cerca(t, "«Leer» y", absoluta(leer).Y, 244)
	cerca(t, "«Leer» ancho", leer.Size().Width, 253)
	cerca(t, "«Resumir» y", absoluta(botones["Resumir"]).Y, 293)
}
