// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// casiTexto es `cerca` con la holgura de una medida que sale de medir texto (±0,2).
func casiTexto(t *testing.T, que string, got, quiero float32) {
	t.Helper()
	if math.Abs(float64(got-quiero)) > 0.2 {
		t.Errorf("%s: %g; la web, %g", que, got, quiero)
	}
}

// documentoConCampos es el del arnés en `modo=campos`: los dos campos de siempre más un
// desplegable de etiqueta larga, un texto de varias líneas y una firma.
func documentoConCampos() *Contenido {
	c := documentoDePrueba()
	c.Campos = append(c.Campos,
		Campo{Nombre: "provincia", Etiqueta: "Provincia de residencia habitual del solicitante", Tipo: "desplegable", Opciones: []string{"Madrid", "Sevilla"}, Pagina: 1},
		Campo{Nombre: "notas", Etiqueta: "Observaciones", Tipo: "texto", Valor: "Sin cambios", Pagina: 1, VariasLineas: true},
		Campo{Nombre: "firma1", Etiqueta: "Firma", Tipo: "firma", Pagina: 2},
	)
	return c
}

// TestLosCamposSeColocanComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz16. Etiquetas de16, NIF en171 de271×26, casilla223,
// select263 de26, textarea313 de58 y firma401. Los botones encogen hasta min-content;
// su fila mide62 de alto y Aplanar se sale a la derecha, como en la web.
func TestLosCamposSeColocanComoEnLaWeb(t *testing.T) {
	p, w := pantallaConDocumento(t, documentoConCampos(), PanelCampos)
	defer w.Close()
	if p.panel.camposP == nil {
		t.Fatal("el panel de campos no se ha montado")
	}
	var entradas []*ui.Entrada
	var area *ui.AreaTexto
	var selector *ui.Selector
	var casilla *casillaNativa
	acciones := map[string][]*Accion{}
	var asterisco *ui.Rotulo
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Entrada:
			entradas = append(entradas, x)
		case *ui.AreaTexto:
			area = x
		case *ui.Selector:
			selector = x
		case *casillaNativa:
			casilla = x
		case *Accion:
			acciones[x.Texto] = append(acciones[x.Texto], x)
		case *ui.Rotulo:
			if x.Texto == "*" {
				asterisco = x
			}
		}
	})
	caja := func(que string, o fyne.CanvasObject, x, y, ancho, alto float32) {
		t.Helper()
		a := absoluta(o)
		cerca(t, que+" x", a.X, x)
		cerca(t, que+" y", a.Y, y)
		cerca(t, que+" ancho", o.Size().Width, ancho)
		cerca(t, que+" alto", o.Size().Height, alto)
	}
	if len(entradas) != 2 || area == nil || selector == nil || casilla == nil {
		t.Fatalf("faltan controles: %d entradas, área %v, selector %v, casilla %v", len(entradas), area != nil, selector != nil, casilla != nil)
	}
	caja("el NIF", entradas[0], 8, 171, 271, 26)
	caja("la firma", entradas[1], 8, 401, 271, 26)
	caja("el desplegable", selector, 8, 263, 271, 26)
	caja("el área", area, 8, 313, 271, 58)
	cerca(t, "la casilla x", absoluta(casilla).X, 8)
	cerca(t, "la casilla y", absoluta(casilla).Y, 223)
	if asterisco == nil {
		t.Fatal("el NIF es obligatorio y no lleva su asterisco")
	}
	casiTexto(t, "el asterisco x", absoluta(asterisco).X, 27.46875)

	if ir := acciones["Página 1"]; len(ir) != 2 {
		t.Errorf("hay %d «Página 1»; la web pone dos (NIF y acepto)", len(ir))
	} else {
		casiTexto(t, "«Página 1» x", absoluta(ir[0]).X, 239.125)
		cerca(t, "«Página 1» y", absoluta(ir[0]).Y, 155)
	}
	for _, b := range []struct {
		texto    string
		x, ancho float32
	}{
		{"Guardar las respuestas", 8, 74.71875},
		{"Exportar los datos", 90.71875, 63.60938},
		{"Importar datos…", 162.32813, 64.26563},
		{"Aplanar", 234.59375, 58.85938},
	} {
		a := acciones[b.texto]
		if len(a) != 1 {
			t.Errorf("no está «%s»", b.texto)
			continue
		}
		casiTexto(t, b.texto+" x", absoluta(a[0]).X, b.x)
		cerca(t, b.texto+" y", absoluta(a[0]).Y, 439)
		casiTexto(t, b.texto+" ancho", a[0].Size().Width, b.ancho)
		cerca(t, b.texto+" alto", a[0].Size().Height, 62)
	}
	if n := len(lineasAccion(acciones["Importar datos…"][0])); n != 2 {
		t.Errorf("«Importar datos…» va en %d renglones; en la web, dos", n)
	}
}

// TestDetectarCamposSeColocaComoEnLaWeb: DetectarCampos real en Chromium a rem16.
// Párrafos en159 y183, Buscar en12,239 de174,39063×30. Con tres propuestas:
// filas de24 en263/291/319 y botones en351, Crear131,60938 y Cancelar65,34375.
func TestDetectarCamposSeColocaComoEnLaWeb(t *testing.T) {
	c := documentoDePrueba()
	c.Campos = nil
	p, w := pantallaConDocumento(t, c, PanelCampos)
	defer w.Close()
	d := p.panel.camposP
	if d == nil || d.detectar == nil {
		t.Fatal("sin campos no se ofrece buscarlos")
	}
	if got := d.detectar.Texto; got != "Buscar campos en la página 1" {
		t.Errorf("el botón dice %q", got)
	}
	a := absoluta(d.detectar)
	cerca(t, "el botón x", a.X, 12)
	cerca(t, "el botón y", a.Y, 239)
	casiTexto(t, "el botón ancho", d.detectar.Size().Width, 174.39063)
	cerca(t, "el botón alto", d.detectar.Size().Height, 30)

	d.propuestas = []Propuesta{
		{Tipo: "texto", Etiqueta: "Nombre y apellidos"},
		{Tipo: "casilla"},
		{Tipo: "firma", Etiqueta: "Firma del solicitante, con fecha y lugar de expedición"},
	}
	d.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	acciones := map[string]*Accion{}
	var tipos []*ui.Rotulo
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *Accion:
			acciones[x.Texto] = x
		case *ui.Rotulo:
			if x.Texto == "Texto" || x.Texto == "Casilla" || x.Texto == "Firma" {
				tipos = append(tipos, x)
			}
		}
	})
	if len(tipos) != 3 {
		t.Fatalf("hay %d filas de propuesta; tres en la web", len(tipos))
	}
	// El rótulo del tipo va en la fila con su `py-1`: la fila empieza 4 px más arriba.
	for i, y := range []float32{263, 291, 319} {
		cerca(t, "la fila "+tipos[i].Texto, absoluta(tipos[i]).Y-4, y)
	}
	casiTexto(t, "«Firma» x", absoluta(tipos[2]).X, 239.78125)
	for _, b := range []struct {
		texto    string
		x, ancho float32
	}{{"Crear estos campos", 12, 131.60938}, {"Cancelar", 151.60938, 65.34375}} {
		x := acciones[b.texto]
		if x == nil {
			t.Fatalf("no está «%s»", b.texto)
		}
		casiTexto(t, b.texto+" x", absoluta(x).X, b.x)
		cerca(t, b.texto+" y", absoluta(x).Y, 351)
		casiTexto(t, b.texto+" ancho", x.Size().Width, b.ancho)
		cerca(t, b.texto+" alto", x.Size().Height, 30)
	}
}

// TestGuardarMandaSoloLoQueCambio: `op: rellenar` lleva los campos cuyo valor difiere
// del que trae el documento, y nada más; sin cambios, no hay nada que mandar.
func TestGuardarMandaSoloLoQueCambio(t *testing.T) {
	campos := documentoConCampos().Campos
	valores := map[string]string{"nif": "", "acepto": "Off", "provincia": "Sevilla", "notas": "Sin cambios", "firma1": ""}
	if got := cambiosDe(campos, valores); !maps.Equal(got, map[string]string{"provincia": "Sevilla"}) {
		t.Errorf("se mandaría %v; solo cambió la provincia", got)
	}
	valores["provincia"] = ""
	if got := cambiosDe(campos, valores); len(got) != 0 {
		t.Errorf("sin cambios se mandaría %v", got)
	}
}

// motorDeCampos finge `/pdf/form` con `op: detectar`: apunta qué se pidió y propone dos.
type motorDeCampos struct{ op, pagina string }

func (m *motorDeCampos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	switch r.URL.Path {
	case "/api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":100,"alto":140,"caja":[0,0,100,140]},
			{"numero":2,"ancho":100,"alto":140,"caja":[0,0,100,140]}],"total":2}`)
	case "/api/v1/pdf/form":
		m.op, m.pagina = r.FormValue("op"), r.FormValue("pagina")
		_, _ = io.WriteString(w, `{"propuestas":[{"tipo":"texto","pagina":1,"rect":[1,2,3,4],"etiqueta":"DNI"},{"tipo":"casilla","pagina":1,"rect":[5,6,7,8]}]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestDetectarPideLaPaginaQueSeMira: buscar manda `op: detectar` con la página que se
// está mirando, y lo que el motor propone se enseña con «Crear estos campos».
func TestDetectarPideLaPaginaQueSeMira(t *testing.T) {
	m := &motorDeCampos{}
	srv := httptest.NewServer(m)
	defer srv.Close()
	pdf := api.NewPDF(srv.URL, srv.URL, nil, nil)
	doc := &Documento{pdf: pdf, ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-")}

	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 8)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(doc)
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote, que el motor fingido no dibuja
	p.panel.Abrir(PanelCampos)
	p.panel.PaginaActual = 2
	p.panel.RefrescarActual()
	d := p.panel.camposP
	if d == nil || d.detectar == nil || d.detectar.Texto != "Buscar campos en la página 2" {
		t.Fatal("el botón no dice la página que se mira")
	}
	d.buscar()
	esperarRed(t, listo)
	for d.buscando {
		esperarRed(t, listo)
	}
	if m.op != "detectar" || m.pagina != "2" {
		t.Errorf("se pidió op=%q pagina=%q; tiene que ser detectar en la 2", m.op, m.pagina)
	}
	if len(d.propuestas) != 2 || d.propuestas[0].Etiqueta != "DNI" {
		t.Errorf("las propuestas son %+v", d.propuestas)
	}
}
