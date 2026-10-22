// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"errors"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// glifoPrueba es un cuadrado: cualquier ruta vale, lo que se mide es dónde cae.
const glifoPrueba = "M4 4h16v16H4z"

func filaDePrueba(e api.Elegible) *FilaSelector {
	return (&SelectorSuite{GlifoCarpeta: glifoPrueba, GlifoElemento: glifoPrueba, Locale: "es"}).fila(e)
}

// conTinta dice si un píxel no es el blanco del fondo.
func conTinta(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 < 250 || g>>8 < 250 || b>>8 < 250
}

// Los números son los MEDIDOS en el arnés (SuitePicker en Chromium, 1280×800): la fila
// empieza en x = 383 y mide 514 × 34,25; el glifo va en 393,5, el nombre en 418,25, la
// meta termina en 886,5 con 112 de ancho como mucho, y el pelo es `border-subtle`.
func TestLaFilaDelSelectorEsLaDeLaWeb(t *testing.T) {
	// Los colores medidos son los de la casa: otra prueba del proceso pudo dejar puesto
	// el producto de su barra (ElegirApp) o un color, y con -test.shuffle primary-text
	// salía del acento ajeno según el orden en que corrieran.
	tema.ElegirColor("")
	tema.ElegirApp("")
	t.Cleanup(func() { tema.ElegirColor(""); tema.ElegirApp("") })
	// Los colores medidos, contra los tokens: si la hoja cambia, esto lo dice.
	for token, web := range map[string]color.NRGBA{
		"text": {15, 29, 32, 255}, "text-muted": {67, 85, 90, 255},
		"text-subtle": {88, 106, 111, 255}, "primary-text": {12, 99, 107, 255},
	} {
		if got := tema.Color(token, tema.Claro); got != web {
			t.Errorf("%s = %v y la web pinta %v", token, got, web)
		}
	}

	sala := filaDePrueba(api.Elegible{ID: "c1", Nombre: "General", Meta: "Anuncios de toda la organización", URL: "u"})
	if m := sala.MinSize(); m.Height != 39 {
		t.Errorf("la fila pide %v de alto y en la web mide 34,25", m.Height)
	}
	sala.Resize(fyne.NewSize(514, 39))
	img := pinta(t, 514, 40, sala)
	r := test.WidgetRenderer(sala).(*filaSelectorRenderer)
	if x := r.nombre.Position().X; x != 38 {
		t.Errorf("el nombre empieza en %v y en la web en 35,25", x)
	}
	if ancho := Medir(r.meta.Text, tema.TextoXS, Normal).Width; ancho > 128+0.5 { // max-w-32 medido: 112, no la variable que se vigila
		t.Errorf("la meta mide %v y el tope es 112", ancho)
	}
	if fin := r.meta.Position().X + Medir(r.meta.Text, tema.TextoXS, Normal).Width; fin < 501.5 || fin > 502.5 {
		t.Errorf("la meta termina en %v y en la web en 503,5 (886,5 − 383)", fin)
	}
	// El pelo de abajo: `rgba(15, 29, 32, .07)` sobre blanco es (238, 239, 239).
	if c := color.NRGBAModel.Convert(img.At(400, 77)).(color.NRGBA); !cercaDe(c, color.NRGBA{238, 239, 239, 255}, 2) {
		t.Errorf("el pelo de la fila pinta %v y la web (238, 239, 239)", c)
	}
	// El glifo cae entero entre 10,5 y 26,5 (×2: 21 y 53) y el nombre empieza en 35,25.
	minG, maxG, primeroNombre := 9999, -1, -1
	for x := 0; x < 200; x++ {
		for y := 0; y < 76; y++ {
			if !conTinta(img.At(x, y)) {
				continue
			}
			switch {
			case x < 60:
				minG, maxG = min(minG, x), max(maxG, x)
			case primeroNombre < 0:
				primeroNombre = x
			}
		}
	}
	if minG < 24 || maxG >= 56 {
		t.Errorf("el glifo pinta de %d a %d (×2) y tiene que caber en [21, 53)", minG, maxG)
	}
	if primeroNombre < 76 || primeroNombre > 82 {
		t.Errorf("la primera tinta del nombre cae en %d (×2) y la web la pone en 70,5", primeroNombre)
	}

	// La carpeta: galón de 14 pegado a la derecha, en 872,5 − 383 = 489,5.
	carpeta := filaDePrueba(api.Elegible{ID: "F1", Nombre: "Facturas", Carpeta: true})
	carpeta.Resize(fyne.NewSize(514, 39))
	img = pinta(t, 514, 40, carpeta)
	galon, fuera := false, false
	for x := 950; x < 1028; x++ {
		for y := 0; y < 76; y++ {
			if conTinta(img.At(x, y)) {
				galon = galon || x >= 979 && x < 1007
				fuera = fuera || x >= 1008
			}
		}
	}
	if !galon || fuera {
		t.Errorf("el galón de la carpeta no cae en [979, 1007) ×2 (dentro: %v, pasado el borde: %v)", galon, fuera)
	}
}

// La caja de la lista deja ver el fondo del panel y solo el contorno lleva el color del
// borde: en la web es `border rounded-md` sin fondo. Se vio gris entera en el retrato
// porque `Caja` pinta el borde como relleno por debajo.
func TestLaCajaDelSelectorNoSePintaDelColorDelBorde(t *testing.T) {
	p := nuevoSelectorDePrueba(t, api.ElegirRooms)
	p.respuesta = func(llamada) (api.PaginaSuite, error) { return api.PaginaSuite{}, nil }
	p.soltar(0) // vacía: «Aquí no hay nada»
	// En el origen: `pinta` no coloca, y la caja trae la posición que le dio el diálogo.
	// Sin esto el píxel de dentro caía FUERA de la caja y el guardián no veía nada.
	p.s.marco.Move(fyne.NewPos(0, 0))
	p.s.marco.Resize(fyne.NewSize(516, AltoCajaSelector))
	img := pinta(t, 516, AltoCajaSelector, p.s.marco)
	dentro := color.NRGBAModel.Convert(img.At(40, 40)).(color.NRGBA)
	if !cercaDe(dentro, tema.Color("panel-surface", tema.Claro), 1) {
		t.Errorf("el interior de la caja pinta %v y tiene que ser el fondo del panel", dentro)
	}
	if borde := color.NRGBAModel.Convert(img.At(0, 200)).(color.NRGBA); cercaDe(borde, dentro, 3) {
		t.Errorf("el contorno de la caja no se ve: %v", borde)
	}
}

func cercaDe(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// llamada es lo que el selector le pide a `Cargar`.
type llamada struct{ carpeta, q, cursor string }

// selectorDePrueba monta el selector con la carga EN DIFERIDO: cada petición se queda en
// `pendientes` hasta que el test la suelta, que es como se ve el estado «cargando» y una
// respuesta que llega tarde.
type selectorDePrueba struct {
	s          *SelectorSuite
	mu         sync.Mutex
	llamadas   []llamada
	pendientes []func()
	respuesta  func(llamada) (api.PaginaSuite, error)
	elegido    *api.Elegible
}

func nuevoSelectorDePrueba(t *testing.T, origen api.OrigenSuite) *selectorDePrueba {
	p := &selectorDePrueba{}
	p.s = &SelectorSuite{
		Origen: origen, GlifoCarpeta: glifoPrueba, GlifoElemento: glifoPrueba, Locale: "es",
		Textos: TextosSelector{
			Titulo: "Elige un fichero de Sytue Drive", Buscar: "Buscar", Migas: "Ruta", Raiz: "Mi unidad",
			Vacio: "Aquí no hay nada", Fallo: "No se ha podido cargar.", Reintentar: "Reintentar",
			MasResultados: "Cargar más", Cancelar: "Cancelar",
		},
		Cargar: func(_ context.Context, carpeta, q, cursor string) (api.PaginaSuite, error) {
			l := llamada{carpeta, q, cursor}
			p.mu.Lock()
			p.llamadas = append(p.llamadas, l)
			p.mu.Unlock()
			return p.respuesta(l)
		},
		AlElegir: func(e api.Elegible) { p.elegido = &e },
		lanzar: func(f func()) {
			p.mu.Lock()
			p.pendientes = append(p.pendientes, f)
			p.mu.Unlock()
		},
	}
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	p.s.Mostrar(w.Canvas())
	return p
}

// soltar deja pasar la petición i-ésima de las pendientes.
func (p *selectorDePrueba) soltar(i int) {
	p.mu.Lock()
	f := p.pendientes[i]
	p.mu.Unlock()
	f()
}

func (p *selectorDePrueba) ultima() llamada {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.llamadas[len(p.llamadas)-1]
}

// dentro es lo que la caja enseña ahora.
func (p *selectorDePrueba) dentro() fyne.CanvasObject {
	return test.WidgetRenderer(p.s.marco).(*marcoRenderer).contenido
}

func (p *selectorDePrueba) filas() []*FilaSelector {
	sc, ok := p.dentro().(*container.Scroll)
	if !ok {
		return nil
	}
	var out []*FilaSelector
	for _, o := range sc.Content.(*fyne.Container).Objects {
		if f, ok := o.(*FilaSelector); ok {
			out = append(out, f)
		}
	}
	return out
}

// botonDe busca un botón por su texto dentro de un objeto.
func botonDe(o fyne.CanvasObject, texto string) *Boton {
	switch x := o.(type) {
	case *Boton:
		if x.Texto == texto {
			return x
		}
	case *fyne.Container:
		for _, h := range x.Objects {
			if b := botonDe(h, texto); b != nil {
				return b
			}
		}
	case *container.Scroll:
		return botonDe(x.Content, texto)
	case *Vacio:
		return botonDe(x.Accion, texto)
	}
	return nil
}

// Drive se navega: carga, entra en una carpeta, vuelve por las migas, pagina sin perder
// el desplazamiento, se recupera de un error y elige un fichero.
func TestElSelectorNavegaDriveYElige(t *testing.T) {
	p := nuevoSelectorDePrueba(t, api.ElegirDrive)
	acta := api.Elegible{ID: "f1", Nombre: "acta.pdf", URL: "https://drive.sytue.eu/drive/files/f1", Bytes: 2048}
	fallar := false
	p.respuesta = func(l llamada) (api.PaginaSuite, error) {
		switch {
		case fallar:
			return api.PaginaSuite{}, errors.New("502")
		case l.carpeta == "F1":
			return api.PaginaSuite{}, nil
		case l.cursor == "c2":
			return api.PaginaSuite{Elegibles: []api.Elegible{{ID: "z", Nombre: "z.txt", URL: "u"}}}, nil
		}
		return api.PaginaSuite{Elegibles: []api.Elegible{{ID: "F1", Nombre: "Facturas", Carpeta: true}, acta}, Cursor: "c2"}, nil
	}

	// Recién abierto, CARGANDO: la rueda de 20.
	if c, ok := p.dentro().(*fyne.Container); !ok || len(c.Objects) != 1 {
		t.Fatalf("recién abierto la caja enseña %T y tiene que ser la rueda centrada", p.dentro())
	} else if _, ok := c.Objects[0].(*Rueda); !ok {
		t.Fatalf("recién abierto la caja enseña %T y tiene que ser la rueda", c.Objects[0])
	}
	p.soltar(0)
	if l := p.ultima(); l != (llamada{}) {
		t.Errorf("la primera página se pide con %+v y es «Mi unidad» sin cursor", l)
	}
	fs := p.filas()
	if len(fs) != 2 || !fs[0].Elegible.Carpeta || fs[1].Meta != "2,0 KB" {
		t.Fatalf("la lista enseña %d filas (meta del fichero %q)", len(fs), fs[len(fs)-1].Meta)
	}
	lista := p.s.lista

	// «Cargar más» AÑADE, y el desplazamiento es el mismo: no vuelve arriba.
	test.Tap(botonDe(p.dentro(), "Cargar más"))
	p.soltar(1)
	if l := p.ultima(); l.cursor != "c2" {
		t.Errorf("«Cargar más» pide %+v y tiene que llevar el cursor c2", l)
	}
	if n := len(p.filas()); n != 3 || botonDe(p.dentro(), "Cargar más") != nil {
		t.Errorf("tras la segunda página hay %d filas y sigue «Cargar más»: %v", n, botonDe(p.dentro(), "Cargar más") != nil)
	}
	if p.s.lista != lista {
		t.Error("«Cargar más» ha rehecho el desplazamiento: la lista vuelve arriba")
	}

	// Entrar en la carpeta: la ruta crece y se pide ESA carpeta. Vacía: «Aquí no hay nada».
	test.Tap(p.filas()[0])
	p.soltar(2)
	migas := p.s.cabecera.Objects[0].(*Migas)
	if l := p.ultima(); l.carpeta != "F1" || len(migas.Ruta) != 2 || migas.Ruta[1] != "Facturas" {
		t.Errorf("al entrar se pide %+v y la ruta es %v", l, migas.Ruta)
	}
	if c, ok := p.dentro().(*fyne.Container); !ok {
		t.Errorf("una carpeta vacía enseña %T", p.dentro())
	} else if txt, ok := c.Objects[0].(*fcanvas.Text); !ok || txt.Text != "Aquí no hay nada" {
		t.Errorf("una carpeta vacía no dice «Aquí no hay nada»")
	}

	// Volver por la miga de la raíz, y que falle: el error ofrece reintentar.
	fallar = true
	migas.AlIr(0)
	p.soltar(3)
	if l := p.ultima(); l.carpeta != "" {
		t.Errorf("la miga de la raíz pide %+v", l)
	}
	reintentar := botonDe(p.dentro(), "Reintentar")
	if reintentar == nil {
		t.Fatal("el error no ofrece «Reintentar»")
	}
	fallar = false
	test.Tap(reintentar)
	p.soltar(4)

	// Elegir un fichero cierra y lo entrega entero.
	test.Tap(p.filas()[1])
	if p.elegido == nil || *p.elegido != acta {
		t.Errorf("se ha elegido %+v", p.elegido)
	}
	if p.s.d.capa != nil {
		t.Error("elegir no ha cerrado el diálogo")
	}
}

// Una respuesta que llega TARDE —la de una búsqueda que ya no es la que se ve— no pisa
// a la buena.
func TestElSelectorNoPintaRespuestasViejas(t *testing.T) {
	p := nuevoSelectorDePrueba(t, api.ElegirRooms)
	p.respuesta = func(l llamada) (api.PaginaSuite, error) {
		return api.PaginaSuite{Elegibles: []api.Elegible{{ID: l.q, Nombre: "sala " + l.q, URL: "u"}}}, nil
	}
	p.s.q = "vie"
	p.s.cargar(false)
	p.s.q = "nueva"
	p.s.cargar(false)
	p.soltar(2) // la nueva llega primero
	p.soltar(1) // y la vieja, después
	filas := p.filas()
	ids := make([]string, 0, len(filas))
	for _, f := range filas {
		ids = append(ids, f.Elegible.ID)
	}
	if len(ids) != 1 || ids[0] != "nueva" {
		t.Errorf("la lista enseña %v: la respuesta vieja ha pisado a la nueva", ids)
	}
}

// Escribir no pregunta a cada tecla: se espera 250 ms desde la última.
func TestElSelectorEsperaAQueSeDejeDeEscribir(t *testing.T) {
	p := nuevoSelectorDePrueba(t, api.ElegirRooms)
	p.respuesta = func(llamada) (api.PaginaSuite, error) { return api.PaginaSuite{}, nil }
	for _, s := range []string{"v", "ve", "ven"} {
		p.s.buscar.editor.SetText(s)
	}
	cuantas := func() int {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.pendientes)
	}
	// El reloj de la búsqueda llega con `fyne.Do`, que el driver del fork (sytue.4) deja
	// encolado hasta que la rutina del test sincroniza: por eso el `DoAndWait` en cada
	// vuelta. Sin él, esto no ve nunca la búsqueda.
	esperar(t, func() bool {
		fyne.DoAndWait(func() {})
		return cuantas() > 1
	})
	if n := cuantas(); n != 2 { // la de abrir y UNA por «ven»
		t.Errorf("tres teclas han pedido %d búsquedas y tiene que ser una", n-1)
	}
	p.s.mu.Lock()
	q := p.s.q
	p.s.mu.Unlock()
	if q != "ven" {
		t.Errorf("se busca %q", q)
	}
}

// La ruta del selector: «Mi unidad», ACTUAL, mide 67,75 × 26,25 en la web, que es su
// rótulo en `font-medium` más `px-2`. Con el semi-negrita de la ruta del explorador
// saldría más ancha.
func TestLaMigaDelSelectorNoCambiaDePeso(t *testing.T) {
	m := NuevasMigas("Ruta", []string{"Mi unidad"}, nil)
	m.Estilo = MigasDelSelector
	if got := MedidaMigas(m); got.Height != 30 || got.Width < 77.43-0.2 || got.Width > 77.43+0.2 {
		t.Errorf("la miga «Mi unidad» mide %v y en la web 67,75 × 26,25", got)
	}
}

// TestRetratoDelSelector guarda el selector abierto para MIRARLO al lado de la captura de
// la web (el SuitePicker en el arnés, con los mismos datos). No afirma nada: lo que se
// mide lo afirman los guardianes de arriba. Con `SYTUE_CAPTURA_SELECTOR=/carpeta`.
func TestRetratoDelSelector(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_SELECTOR")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_SELECTOR no se retrata")
	}
	salas := []api.Elegible{
		{ID: "c1", Nombre: "General", URL: "u", Meta: "Anuncios de toda la organización"},
		{ID: "c2", Nombre: "Ventas", URL: "u"},
		{ID: "c3", Nombre: "Ana García", URL: "u"},
	}
	drive := []api.Elegible{
		{ID: "F1", Nombre: "Facturas", Carpeta: true},
		{ID: "F2", Nombre: "Proyectos", Carpeta: true},
		{ID: "f1", Nombre: "Acta de la reunión.pdf", URL: "u", Bytes: 2048},
	}
	for _, c := range []struct {
		nombre string
		origen api.OrigenSuite
		w, h   float32
		pagina api.PaginaSuite
		err    error
		titulo string
	}{
		{"rooms-1280", api.ElegirRooms, 1280, 800, api.PaginaSuite{Elegibles: salas}, nil, "Elige una sala de Rooms"},
		{"rooms-360", api.ElegirRooms, 360, 740, api.PaginaSuite{Elegibles: salas}, nil, "Elige una sala de Rooms"},
		{"drive-1280", api.ElegirDrive, 1280, 800, api.PaginaSuite{Elegibles: drive, Cursor: "c2"}, nil, "Elige un fichero de Sytue Drive"},
		{"error-1280", api.ElegirRooms, 1280, 800, api.PaginaSuite{}, errors.New("502"), "Elige una sala de Rooms"},
	} {
		w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(c.w, c.h))
		s := &SelectorSuite{
			Origen: c.origen, GlifoCarpeta: glifoPrueba, GlifoElemento: glifoPrueba, Locale: "es",
			Textos: TextosSelector{
				Titulo: c.titulo, Buscar: "Buscar", Migas: "Ruta de carpetas", Raiz: "Mi unidad",
				Vacio: "Aquí no hay nada", Fallo: "No se ha podido cargar. Inténtalo otra vez.",
				Reintentar: "Reintentar", MasResultados: "Cargar más", Cancelar: "Cancelar",
			},
			Cargar: func(context.Context, string, string, string) (api.PaginaSuite, error) { return c.pagina, c.err },
			lanzar: func(f func()) { f() },
		}
		s.Mostrar(w.Canvas())
		f, err := os.Create(filepath.Join(dir, "fyne-"+c.nombre+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}

// La nota de pie va bajo la lista en `text-sm text-subtle`, con su `mt-2`; sin ella, la
// lista es lo último.
func TestElSelectorLlevaSuNotaDePie(t *testing.T) {
	nota := func(s *SelectorSuite) *Parrafo {
		var p *Parrafo
		var mirar func(o fyne.CanvasObject)
		mirar = func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *Parrafo:
				p = x
			case *fyne.Container:
				for _, h := range x.Objects {
					mirar(h)
				}
			}
		}
		mirar(s.d.Cuerpo)
		return p
	}
	sin := nuevoSelectorDePrueba(t, api.ElegirDrive)
	if nota(sin.s) != nil {
		t.Error("sin Pie ya hay nota")
	}
	con := &SelectorSuite{
		Origen: api.ElegirDrive, Textos: TextosSelector{Titulo: "Drive", Pie: "Se adjunta por referencia."},
		Cargar: func(context.Context, string, string, string) (api.PaginaSuite, error) { return api.PaginaSuite{}, nil },
		lanzar: func(func()) {},
	}
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	con.Mostrar(w.Canvas())
	if p := nota(con); p == nil || p.Texto != "Se adjunta por referencia." || p.Escalon != tema.TextoSM {
		t.Errorf("la nota de pie es %+v", p)
	}
}
