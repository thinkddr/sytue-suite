// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// Los números de estos tests salen de MEDIR la web: `PdfApp` montado con el BFF fingido
// (tres A4, un índice, un adjunto, dos comentarios, dos campos y una firma) en Chromium a
// 1280×800 con `--font-render-hinting=none`, y `getBoundingClientRect` de cada pieza. Se
// escriben con su valor y no con la cuenta que los produce: un guardián que sacara sus
// puntos de la misma constante que vigila saldría verde con el defecto puesto.

// todos recorre el árbol pintado, contenedores y widgets.
func todos(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, c := range x.Objects {
			todos(c, f)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(x).Objects() {
			todos(c, f)
		}
	}
}

func absoluta(o fyne.CanvasObject) fyne.Position {
	return fyne.CurrentApp().Driver().AbsolutePositionForObject(o)
}

func cerca32(a, b float32) bool { return math.Abs(float64(a-b)) < 0.02 }

// documentoDePrueba es el mismo contenido que se le dio a la web al medirla.
func documentoDePrueba() *Contenido {
	c := &Contenido{Total: 3}
	for n := 1; n <= 3; n++ {
		p := a4()
		p.Numero = n
		c.Paginas = append(c.Paginas, p)
	}
	c.Paginas[0].Extracto = new("Contrato de prestación de servicios entre las partes firmantes")
	c.Paginas[1].Extracto = new("Cláusulas")
	c.Indice = []Marcador{{Titulo: "1. Objeto del contrato", Pagina: 0}, {Titulo: "1.1 Alcance", Pagina: 1, Nivel: 1}, {Titulo: "2. Precio", Pagina: 2}}
	c.Adjuntos = []Adjunto{{Nombre: "factura-2026-0142.xml", Descripcion: "Factura electrónica", TamanoBytes: 18432, Tipo: "text/xml", Pagina: -1}}
	cero := 0
	c.Anotaciones = []AnotacionLeida{
		{Pagina: 0, Tipo: "Text", Rect: Rect{60, 700, 80, 720}, Texto: "Revisar el importe de la cláusula tercera", Autor: "Ada Lovelace"},
		{Pagina: 0, Tipo: "Highlight", Rect: Rect{60, 600, 300, 620}, Autor: "Grace Hopper", RespuestaA: &cero, Estado: "Accepted"},
	}
	c.Campos = []Campo{
		{Nombre: "nif", Etiqueta: "NIF", Tipo: "texto", Rect: Rect{60, 400, 260, 420}, Obligatorio: true},
		{Nombre: "acepto", Tipo: "casilla", Valor: "Off", Rect: Rect{60, 360, 72, 372}},
	}
	c.Firmas = []FirmaLeida{{Nombre: "Ana", CubreTodo: true}}
	c.Huella = Huella{Autor: "Ada", Productor: "Sytue", Titulo: "Contrato", ConAdjuntos: true}
	return c
}

// TestLaCintaMideLoQueLaWeb: la fila de 40 (el `pt-1` y la salida de `h-9`), la banda de
// 71 (un `RibbonGroup` de 67 más el `py-0.5`) y el `border-b`. La primera
// versión de esta cinta usaba la del editor, de 100, y sus pestañas `text-sm` subrayadas.
func TestLaCintaMideLoQueLaWeb(t *testing.T) {
	if got := AltoBarra(); got != 112 {
		t.Errorf("la cinta mide %g de alto; la web, 112", got)
	}
	tv := traductorES(t)
	b := NuevaBarra(func(c string) string { return tv(c, nil) })
	w := ventanaSinRelleno(b.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, AltoBarra()))

	var pestanas []*pestanaBarra
	var grupos []*editor.Grupo
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *pestanaBarra:
			pestanas = append(pestanas, x)
		case *editor.Grupo:
			grupos = append(grupos, x)
		}
	})
	if len(pestanas) != 6 {
		t.Fatalf("la tira tiene %d pestañas", len(pestanas))
	}
	// `px-1`, salida `w-9` y `gap-1` dejan Inicio en x=44; centrado en la fila de
	// 36, `py-1.5 text-xs` mide 28 y empieza en y=8. El texto histórico de Chromium
	// tenía 25,63 px a rem14: a rem16, más `px-3`, el botón mide 53,29.
	p0, p1 := absoluta(pestanas[0]), absoluta(pestanas[1])
	if !cerca32(p0.X, 44) || !cerca32(p0.Y, 8) || !cerca32(pestanas[0].Size().Height, 28) {
		t.Errorf("«Inicio» está en %v y mide %v; la web la pone en 44,8 con 28 de alto", p0, pestanas[0].Size())
	}
	if d := p1.X - (p0.X + pestanas[0].Size().Width); !cerca32(d, 2) {
		t.Errorf("entre pestañas hay %g; la web pone el `gap-0.5` (2)", d)
	}
	if math.Abs(float64(pestanas[0].Size().Width-53.29)) > 0.2 {
		t.Errorf("«Inicio» mide %g de ancho; la web, 53,29", pestanas[0].Size().Width)
	}
	if len(grupos) != 2 {
		t.Fatalf("la pestaña de Inicio tiene %d grupos; la web, Herramientas y Trazo", len(grupos))
	}
	// `RibbonGroup`: 6 + min-h-11 (44) + gap-0.5 (2) + leyenda leading-none (11)
	// + pb-1 (4) = 67. Cinco `w-8` con gap-0.5 y px-1.5 suman 180 de ancho.
	g0, g1 := absoluta(grupos[0]), absoluta(grupos[1])
	if !cerca32(g0.X, 4) || !cerca32(g0.Y, 42) || !cerca32(grupos[0].Size().Height, 67) {
		t.Errorf("Herramientas está en %v y mide %v; la web: 4,42 y 67 de alto", g0, grupos[0].Size())
	}
	if !cerca32(grupos[0].Size().Width, 180) {
		t.Errorf("Herramientas mide %g de ancho; la web, 180", grupos[0].Size().Width)
	}
	if !cerca32(g1.X, 188) {
		t.Errorf("Trazo empieza en x=%g; la web, 188", g1.X)
	}
}

// TestElVisorColocaLasHojasComoLaWeb: al ancho (el ajuste con el que nace) un A4 en
// 1280 mide 944 —el hueco del documento, 992, menos los 48 LITERALES de la web— y va
// centrado bajo la cinta, a 24 de ella. La segunda hoja, detrás del `gap-6`.
func TestElVisorColocaLasHojasComoLaWeb(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.PonerContenido(documentoDePrueba())
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))

	if len(p.hojas) != 3 {
		t.Fatalf("hay %d hojas", len(p.hojas))
	}
	h0, h1 := absoluta(p.hojas[0]), absoluta(p.hojas[1])
	ancho, alto := MedidaPapel(p.hojas[0].pagina, p.hojas[0].zoom)
	if !cerca32(h0.X, 312) || !cerca32(h0.Y, 136) {
		t.Errorf("la primera hoja está en %v; la web la pone en 312,136", h0)
	}
	// A4: 842 × 944 / 595 = 1335,879; la siguiente hoja suma gap-1.5 (6),
	// text-xs (16) y gap-6 (24). Cifras independientes de MedidaHoja y del tema Go.
	if !cerca32(ancho, 944) || math.Abs(float64(alto-1335.88)) > 0.02 {
		t.Errorf("el papel mide %g×%g; la web, 944×1335,88", ancho, alto)
	}
	if math.Abs(float64(h1.Y-1517.88)) > 0.02 {
		t.Errorf("la segunda hoja empieza en y=%g; la web, 1517,88", h1.Y)
	}
	// Panel w-72 menos border-r y p-2: 271. La miniatura w-16 tiene la razón A4,
	// 64 × 842 / 595, más p-1.5 a ambos lados: fila de 102,57.
	var filas []*filaMiniatura
	todos(w.Content(), func(o fyne.CanvasObject) {
		if f, ok := o.(*filaMiniatura); ok {
			filas = append(filas, f)
		}
	})
	if len(filas) != 3 {
		t.Fatalf("el panel de Páginas tiene %d filas", len(filas))
	}
	f0 := absoluta(filas[0])
	if !cerca32(f0.X, 8) || !cerca32(f0.Y, 155) {
		t.Errorf("la primera miniatura está en %v; la web, 8,155", f0)
	}
	if m := filas[0].Size(); !cerca32(m.Width, 271) || math.Abs(float64(m.Height-102.57)) > 0.02 {
		t.Errorf("la fila de miniatura mide %v; la web, 271×102,57", m)
	}
	if !filas[0].actual || filas[1].actual {
		t.Error("la miniatura marcada no es la de la página que se mira")
	}
}

// TestRecortarPaginaEsExactoALaEscala: un dibujo pedido a la escala de la pantalla sale
// RECORTADO —un píxel de borde por lado— y no remuestreado, así que cada píxel es el del
// motor. Se comprueba con un tablero de un píxel: cualquier filtro lo convierte en gris.
func TestRecortarPaginaEsExactoALaEscala(t *testing.T) {
	t.Parallel()
	const w, h = 40, 60
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if (x+y)%2 == 0 {
				src.Set(x, y, color.Black)
			} else {
				src.Set(x, y, color.White)
			}
		}
	}
	// El papel mide 40×60 px y el dibujo va por dentro del borde: 38×58.
	got := RecortarPagina(src, w, h, w-2, h-2)
	if b := got.Bounds(); b.Dx() != w-2 || b.Dy() != h-2 {
		t.Fatalf("el recorte mide %v", b)
	}
	for y := range h - 2 {
		for x := range w - 2 {
			if got.At(x, y) != color.RGBAModel.Convert(src.At(x+1, y+1)) {
				t.Fatalf("el píxel %d,%d no es el del motor: se ha remuestreado", x, y)
			}
		}
	}
	// A otra escala (tras un zoom) se remuestrea, pero al tamaño pedido.
	if b := RecortarPagina(src, w, h, 18, 28).Bounds(); b.Dx() != 18 || b.Dy() != 28 {
		t.Errorf("remuestreado mide %v", b)
	}
}

// TestLaHojaPintaElDibujoDelMotor: con la página dibujada, lo que sale en la ventana son
// los píxeles del PNG en su sitio, y el aviso de «cargando» se va.
func TestLaHojaPintaElDibujoDelMotor(t *testing.T) {
	p := Pagina{Numero: 1, Ancho: 100, Alto: 140, Caja: Rect{0, 0, 100, 140}}
	img := image.NewRGBA(image.Rect(0, 0, 100, 140))
	for y := range 140 {
		for x := range 100 {
			img.Set(x, y, color.White)
		}
	}
	for y := 40; y < 50; y++ {
		for x := 30; x < 40; x++ {
			img.Set(x, y, color.Black)
		}
	}
	h := NuevaHoja(p, 1, HojaCargando, "Cargando…", nil)
	w := ventanaSinRelleno(h)
	defer w.Close()
	w.Resize(MedidaHoja(p, 1))
	h.PonerImagen(img, "", "")
	c := w.Canvas().Capture()
	for _, caso := range []struct {
		x, y  int
		negro bool
	}{{30, 40, true}, {39, 49, true}, {29, 40, false}, {40, 49, false}, {35, 50, false}} {
		r, _, _, _ := c.At(caso.x, caso.y).RGBA()
		if negro := r>>8 < 20; negro != caso.negro {
			t.Errorf("el píxel %d,%d es %v; el dibujo del motor dice negro=%v", caso.x, caso.y, c.At(caso.x, caso.y), caso.negro)
		}
	}
	if r, ok := test.WidgetRenderer(h).(*hojaRenderer); !ok || len(r.aviso) != 0 {
		t.Error("con la página dibujada la hoja sigue diciendo que carga")
	}
}

// TestExtractoLegible es `extracto.test.ts`.
func TestExtractoLegible(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		entra string
		sale  string
		ok    bool
	}{
		{"IMPRESO DE MATRÍCULA", "IMPRESO DE MATRÍCULA", true},
		{",035(62 '( 0$75Ê&8/$", "", false},
		{"  abc ", "", false},
		{"", "", false},
		{"Cláusula 3.ª", "Cláusula 3.ª", true},
	} {
		s, ok := ExtractoLegible(c.entra)
		if ok != c.ok || (ok && s != c.sale) {
			t.Errorf("ExtractoLegible(%q) = %q, %v", c.entra, s, ok)
		}
	}
}

// TestSinDocumentoDiceQuePasaYReintenta: lo que no llega a ser respuesta es la RED —no el
// documento—, un 401 es la sesión, y en los dos está «Reintentar».
func TestSinDocumentoDiceQuePasaYReintenta(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(900, 600))
	reintentos := 0
	p.AlReintentar = func() { reintentos++ }
	for _, c := range []struct {
		err    error
		estado editor.Estado
		frase  string
	}{
		{errors.New("dial tcp: connection refused"), editor.SinConexion, "pdf.error.offline"},
		{&api.Error{Status: http.StatusUnauthorized}, editor.SinSesion, "pdf.error.session"},
		// La frase del motor está en inglés: no se enseña, se dice lo que se intentaba.
		{&api.Error{Status: http.StatusBadRequest, Body: `{"message":"Page 9 does not exist"}`}, editor.Error, "pdf.error.open"},
		// Con código, el suyo: «no es un PDF» no se confunde con «está dañado».
		{&api.Error{Status: http.StatusBadRequest, Body: `{"error":{"code":"pdf_not_pdf","message":"x"}}`}, editor.Error, "pdf.error.notPdf"},
	} {
		p.FalloAlAbrir(c.err)
		if p.Estado != c.estado {
			t.Errorf("%v deja el estado %d y no %d", c.err, p.Estado, c.estado)
		}
		quiero := traductorES(t)(c.frase, nil)
		if p.Error != quiero {
			t.Errorf("%v dice %q y no %q", c.err, p.Error, quiero)
		}
		var boton *Accion
		todos(w.Content(), func(o fyne.CanvasObject) {
			if a, ok := o.(*Accion); ok {
				boton = a
			}
		})
		if boton == nil {
			t.Fatal("sin documento no hay «Reintentar»")
		}
		boton.Tapped(nil)
	}
	if reintentos != 4 {
		t.Errorf("«Reintentar» se pulsó cuatro veces y reintentó %d", reintentos)
	}
}

// motorDePrueba finge el motor: el contenido de tres A4 y, al dibujar, un PNG blanco
// del tamaño exacto que se pide. Apunta la escala a la que se pidió.
type motorDePrueba struct {
	escalas []string
	lotes   []string
}

func (m *motorDePrueba) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	switch r.URL.Path {
	case "/api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":100,"alto":140,"caja":[0,0,100,140]},
			{"numero":2,"ancho":100,"alto":140,"caja":[0,0,100,140]},{"numero":3,"ancho":100,"alto":140,"caja":[0,0,100,140]}],
			"total":3,"permisos":{"Anotar":true}}`)
	case "/api/v1/pdf/render":
		m.escalas = append(m.escalas, r.FormValue("escala"))
		m.lotes = append(m.lotes, r.FormValue("paginas"))
		var buf bytes.Buffer
		_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 10, 14)))
		b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
		// La tercera página «no se pudo dibujar»: no viene en el lote.
		_, _ = io.WriteString(w, `{"paginas":{"1":"`+b64+`","2":"`+b64+`"},"total":3}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestLaPantallaAbreYDibujaPorLotes es la cadena entera contra un motor fingido: abrir
// pide el contenido, el primer lote pide «1-3» (los que hay) en PNG, y la página que el
// motor no devolvió se DICE en vez de quedarse cargando para siempre.
func TestLaPantallaAbreYDibujaPorLotes(t *testing.T) {
	m := &motorDePrueba{}
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
	esperarRed(t, listo) // el primer lote
	if p.Contenido == nil || len(p.hojas) != 3 {
		t.Fatalf("tras abrir no hay documento: %+v", p.Contenido)
	}
	if strings.Join(m.lotes, "|") != "1-3" {
		t.Errorf("los lotes pedidos son %q; el primero es «1-3»", m.lotes)
	}
	if p.hojas[0].estado != HojaDibujada || p.hojas[1].estado != HojaDibujada {
		t.Error("las dos páginas que llegaron no se pintan")
	}
	if p.hojas[2].estado != HojaSinDibujar {
		t.Errorf("la página que el motor no dibujó está en %d; tiene que decir que no se pudo", p.hojas[2].estado)
	}
}

// TestLaCintaConservaLosGruposEnElMovil: Ribbon.tsx real en Chromium a 360×740,
// CSS1Compat, Figtree cargada y raíz 16, medido de nuevo el 2026-10-16 con el
// `RibbonGroup` de una fila: los grupos shrink-0 conservan 180 y 280,75 dentro del
// scroll horizontal, sin rótulo por debajo de md (52 de alto), y el quinto botón sigue
// en 146,52, sin envolver. La cinta mide 97.
func TestLaCintaConservaLosGruposEnElMovil(t *testing.T) {
	tv := traductorES(t)
	b := NuevaBarra(func(c string) string { return tv(c, nil) })
	b.Ancho = 360
	w := ventanaSinRelleno(b.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(360, b.Alto()))

	if got := b.Alto(); math.Abs(float64(got-97)) > 0.02 {
		t.Errorf("la cinta mide %g a 360; la web, 97", got)
	}
	var grupos []*editor.Grupo
	todos(w.Content(), func(o fyne.CanvasObject) {
		if g, ok := o.(*editor.Grupo); ok {
			grupos = append(grupos, g)
		}
	})
	if len(grupos) != 2 {
		t.Fatalf("hay %d grupos", len(grupos))
	}
	for i, quiero := range []float32{180, 280.75} {
		if got := grupos[i].Size().Width; math.Abs(float64(got-quiero)) > 0.05 {
			t.Errorf("el grupo %d mide %g de ancho; la web, %g", i, got, quiero)
		}
	}
	quinto := absoluta(grupos[0].Controles[4])
	if !cerca32(quinto.X, 146) || !cerca32(quinto.Y, 52) {
		t.Errorf("el quinto botón está en %v; la web lo conserva en 146,52", quinto)
	}
	r := test.WidgetRenderer(grupos[1])
	for _, o := range r.Objects() {
		if txt, ok := o.(*canvas.Text); ok && txt.Text == "Trazo" && txt.Visible() {
			t.Error("el rótulo «Trazo» se pinta en el móvil; la web no lo pinta por debajo de md")
		}
	}
}

// TestTrasPulsarLaCintaLasTeclasSueltasSiguenLlegando: pulsar una herramienta de la
// cinta rehace la cinta; el botón viejo no se queda con el foco, y Escape, sin nada
// enfocado, llega a la pantalla y vuelve a la mano, como el `keydown` de la web.
func TestTrasPulsarLaCintaLasTeclasSueltasSiguenLlegando(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() { p.PonerContenido(documentoDePrueba()); p.Atajos(w.Canvas()) })
	var lapiz *ui.BotonIcono
	todos(p.raiz, func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == p.t("pdf.tool.highlight") {
			lapiz = b
		}
	})
	if lapiz == nil {
		t.Fatal("no está el botón de resaltar en la cinta")
	}
	fyne.DoAndWait(func() { lapiz.Tapped(&fyne.PointEvent{}) })
	if p.barra.Herramienta != Resaltar {
		t.Fatalf("pulsar resaltar deja la herramienta %d", p.barra.Herramienta)
	}
	if f := w.Canvas().Focused(); f != nil {
		t.Errorf("el botón pulsado se queda con el foco (%T): las teclas sueltas no llegan", f)
	}
	fyne.DoAndWait(func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyEscape}) })
	if p.barra.Herramienta != Mano {
		t.Errorf("Escape tras pulsar la cinta deja la herramienta %d", p.barra.Herramienta)
	}
}

// TestConElFocoEnUnaMiniaturaLasTeclasSueltasLlegan: Av Pág con el foco en una miniatura
// del panel pasa de hoja, como el `keydown` del botón que sube al documento en la web.
func TestConElFocoEnUnaMiniaturaLasTeclasSueltasLlegan(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() { p.PonerContenido(documentoDePrueba()); p.Atajos(w.Canvas()) })
	var fila *filaMiniatura
	todos(p.raiz, func(o fyne.CanvasObject) {
		if f, ok := o.(*filaMiniatura); ok && fila == nil {
			fila = f
		}
	})
	if fila == nil {
		t.Fatal("no hay miniaturas en el panel")
	}
	fyne.DoAndWait(func() {
		w.Canvas().Focus(fila)
		fila.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	})
	if p.PaginaActual != 2 {
		t.Errorf("Av Pág con el foco en una miniatura deja la página %d", p.PaginaActual)
	}
}

// TestLasFlechasYEspacioCorrenElDocumento: ↓ corre 40, como una línea de Chromium, y
// Espacio el 87,5 % de lo que se ve; ↑ vuelve y arriba del todo no pasa de 0.
func TestLasFlechasYEspacioCorrenElDocumento(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() { p.PonerContenido(documentoDePrueba()) })
	d := p.desplazar
	tecla := func(k fyne.KeyName) { fyne.DoAndWait(func() { p.Tecla(&fyne.KeyEvent{Name: k}) }) }
	tecla(fyne.KeyDown)
	if !cerca32(d.Offset.Y, 40) {
		t.Errorf("↓ deja el documento en %g; una línea son 40", d.Offset.Y)
	}
	tecla(fyne.KeySpace)
	if quiero := 40 + 0.875*d.Size().Height; !cerca32(d.Offset.Y, quiero) {
		t.Errorf("Espacio deja el documento en %g; la página de Chromium lo deja en %g", d.Offset.Y, quiero)
	}
	for range 60 {
		tecla(fyne.KeyUp)
	}
	if d.Offset.Y != 0 {
		t.Errorf("↑ de sobra deja el documento en %g", d.Offset.Y)
	}
}

// TestLosAtajosSonLosDeLaWeb: Av Pág pasa de hoja, Fin va a la última, Escape suelta la
// herramienta antes de cerrar el cajón, y Ctrl+F abre Buscar.
func TestLosAtajosSonLosDeLaWeb(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.PonerContenido(documentoDePrueba())
	p.Atajos(w.Canvas())

	p.Tecla(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	if p.PaginaActual != 2 {
		t.Errorf("Av Pág deja la página %d", p.PaginaActual)
	}
	p.Tecla(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if p.PaginaActual != 3 {
		t.Errorf("Fin deja la página %d", p.PaginaActual)
	}
	p.barra.Herramienta, p.PanelAbierto = Lapiz, true
	p.Tecla(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if p.barra.Herramienta != Mano || !p.PanelAbierto {
		t.Errorf("el primer Escape suelta la herramienta y deja el cajón: %d, %v", p.barra.Herramienta, p.PanelAbierto)
	}
	p.Tecla(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if p.PanelAbierto {
		t.Error("el segundo Escape cierra el cajón")
	}
	p.atajos[fyne.KeyF]()
	if p.panel.Abierto() != PanelBuscar {
		t.Errorf("Ctrl+F deja el panel en %d y no en Buscar", p.panel.Abierto())
	}

	// Ctrl+P manda a imprimir el PDF abierto, con su nombre. VISTO EN ROJO sin el atajo.
	impreso := make(chan string, 1)
	imprimirPDF = func(nombre string, datos []byte) error {
		impreso <- nombre + " " + string(datos)
		return nil
	}
	defer func() { imprimirPDF = ficheros.ImprimirDatos }()
	p.doc = &Documento{fichero: api.File{Name: "acta.pdf"}, datos: []byte("%PDF-1.7")}
	if f := p.atajos[fyne.KeyP]; f != nil {
		f()
	}
	select {
	case got := <-impreso:
		if got != "acta.pdf %PDF-1.7" {
			t.Errorf("Ctrl+P ha mandado a imprimir %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("Ctrl+P no ha mandado nada a imprimir")
	}
}

// esperarRed espera a que la pantalla termine de atender una respuesta del motor
// (`trasRed`). La respuesta se pinta con `fyne.Do`, y el driver de pruebas del fork lo
// deja para la rutina del test: sin el `DoAndWait` de cada vuelta no llegaría nunca.
// TestOtroDocumentoEmpiezaDeCero: abrir otro PDF es, en la web, cargar otra página. Ni la
// herramienta, ni la pestaña de la cinta, ni el grosor, ni el panel con su pestaña pasan
// al nuevo.
func TestOtroDocumentoEmpiezaDeCero(t *testing.T) {
	p, _, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	viejo := p.panel
	fyne.DoAndWait(func() {
		p.barra.Herramienta, p.barra.activa, p.barra.Grosor = Censura, PestFirmar, GrosorPorDefecto+3
		p.panel.Abrir(PanelBuscar)
		p.PanelAbierto, p.DosPaginas = true, true
	})
	otro := &Documento{pdf: p.doc.pdf, ruta: "/PDF/b.pdf", fichero: api.File{Name: "b.pdf"}, datos: []byte("%PDF-")}
	fyne.DoAndWait(func() { p.Abrir(otro) })
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote
	b := p.barra
	if b.Herramienta != Mano || b.activa != PestInicio || b.Grosor != GrosorPorDefecto {
		t.Errorf("la cinta sigue con la herramienta %d en la pestaña %d y grosor %d", b.Herramienta, b.activa, b.Grosor)
	}
	if p.panel == viejo || p.panel.Abierto() != PanelPaginas || p.PanelAbierto || p.DosPaginas {
		t.Errorf("el panel del documento anterior pasa al nuevo (en %d, abierto %v, dos páginas %v)", p.panel.Abierto(), p.PanelAbierto, p.DosPaginas)
	}
}

func esperarRed(t *testing.T, listo <-chan struct{}) {
	t.Helper()
	plazo := time.Now().Add(10 * time.Second)
	for {
		fyne.DoAndWait(func() {})
		select {
		case <-listo:
			return
		case <-time.After(5 * time.Millisecond):
		}
		if time.Now().After(plazo) {
			t.Fatal("el motor no contestó")
		}
	}
}

// TestElGrosorEsElRangoDeChromium: el `<input type="range">` del grosor va sin estilo,
// así que es el de Chromium (carril de 8, pulgar de 16) y no el de la suite. Medido en
// Ribbon.tsx real a rem16 y Figtree cargada:398,75,62 de64×16, centrado en la fila
// de44 px de su grupo. w-16 mide64 y el alto UA permanece16.
func TestElGrosorEsElRangoDeChromium(t *testing.T) {
	tv := traductorES(t)
	b := NuevaBarra(func(c string) string { return tv(c, nil) })
	w := ventanaSinRelleno(b.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, AltoBarra()))
	var d *ui.Deslizador
	todos(w.Content(), func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Deslizador); ok {
			d = x
		}
	})
	if d == nil || !d.Nativo {
		t.Fatal("el grosor no es el rango nativo de Chromium")
	}
	a := absoluta(d)
	cerca(t, "el grosor x", a.X, 398.75)
	cerca(t, "el grosor y", a.Y, 62)
	cerca(t, "el grosor ancho", d.Size().Width, 64)
	cerca(t, "el grosor alto", d.Size().Height, 16)
}
