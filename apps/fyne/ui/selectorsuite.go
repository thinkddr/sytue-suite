// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # SelectorSuite: «elegir algo de otra app» (SuitePicker.tsx)
//
// UN selector para lo que se puede enganchar desde otra app de la suite: un fichero de
// Drive, un documento de Docs, un evento de Calendar, una sala de Rooms u otra nota. Drive
// se NAVEGA (carpetas y migas) porque su modelo es un árbol; los otros cuatro se BUSCAN,
// porque el suyo es una lista. La diferencia la marca el origen (`api.OrigenSuite`).
//
// Los datos los trae `Cargar`, que es `api.Elegir.Listar` con la sesión de la app: aquí
// no hay BFF, cada servicio aplica sus permisos al token de la persona. Lo que se elige
// es una REFERENCIA —`api.Elegible`—, nunca una copia.
//
// MEDIDO en el arnés (SuitePicker de Notepads pintado en Chromium con `/api/suite`
// simulado, 1280×800 y 360×740, 2026-09-23): diálogo `md` (560; 332 a 360), cabecera
// —el buscador `md` de 31,5 o las migas de 26,25— con `mb-2` (7), la caja `h-72` (252)
// con `border` y radio 6, y filas de 34,25 = `py-2` + 19,25 + su pelo `border-subtle`,
// `px-3` (10,5), glifo de 16 y `gap-2.5` (8,75). La meta es `sytue-meta` (10,5/14,
// `text-subtle`) con `max-w-32` (112); la carpeta lleva el galón de 14 en `text-subtle` y
// su glifo en `primary-text`, el resto en `text-muted`. «Cargar más» es `secondary sm`
// (28) centrado con `py-3`, y el pie, «Cancelar» fantasma `md`.
//
// ponytail: la ruta de Drive se ESTRECHA cuando no cabe (ui.Migas) y la web la parte en
// líneas (`flex-wrap`). Solo se nota con cinco o seis carpetas de hondo en el móvil.
var (
	// AltoCajaSelector es el `h-72` de la caja de la lista.
	AltoCajaSelector float32 = 72 * 0.25 * tema.Rem
	// AltoFilaSelector es una fila: `py-2`, la línea de `text-base` y su pelo.
	AltoFilaSelector = 2*tema.Espacio(2) + tema.TextoBase.Linea + tema.Borde
	// MaxMetaSelector es el `max-w-32` de la meta.
	MaxMetaSelector float32 = 32 * 0.25 * tema.Rem

	pxFilaSelector    = tema.Espacio(3)
	huecoFilaSelector = tema.Espacio(2.5)
	huecoCabSelector  = tema.Espacio(2) // mb-2 del buscador o de las migas
	pyMasSelector     = tema.Espacio(3) // py-3 alrededor de «Cargar más»
	margenEstadoSel   = tema.Espacio(4) // p-4 del vacío y del error
)

// Los tamaños fijos del selector.
const (
	LadoGlifoSelector  float32 = 16
	LadoGalonSelector  float32 = 14
	LadoRuedaSelector  float32 = 20
	LadoAvisoSelector  float32 = 36
	esperaBusquedaSelc         = 250 * time.Millisecond // el `setTimeout(…, 250)` de la web
)

// TextosSelector son los textos, YA traducidos del catálogo de quien abre el selector:
// Notepads los tiene en `picker.*`, y otra app los tendrá con otro nombre.
type TextosSelector struct {
	Titulo        string // picker.<origen>.title
	Buscar        string // picker.search: marca de agua y nombre del campo
	Migas         string // picker.breadcrumb: nombre de la ruta
	Raiz          string // picker.myDrive
	Vacio         string // picker.empty
	Fallo         string // picker.failed
	Reintentar    string // common.retry
	MasResultados string // picker.loadMore
	Cancelar      string // common.cancel
	// Cerrar es el nombre del aspa; vacío, el común de la suite (`ui.close`).
	Cerrar string
	// Pie es la nota bajo la lista (`<p class="text-text-subtle mt-2 text-sm">`): el
	// «se adjunta por referencia» del selector de Drive de Sytue Rooms. Vacío, no hay.
	Pie string
}

// SelectorSuite es el diálogo. Se rellena y se abre con Mostrar.
type SelectorSuite struct {
	Origen api.OrigenSuite
	Textos TextosSelector
	// GlifoCarpeta y GlifoElemento son las rutas SVG (`d`) de los dibujos de la fila:
	// cada app pone los suyos, como en la web (Notepads, su `Glyph`).
	GlifoCarpeta, GlifoElemento string
	// Locale escribe el tamaño de los ficheros de Drive.
	Locale i18n.Locale
	// Cargar trae una página: `api.Elegir.Listar` con el origen ya puesto.
	Cargar func(ctx context.Context, carpeta, q, cursor string) (api.PaginaSuite, error)
	// AlElegir recibe lo elegido; el diálogo ya se ha cerrado.
	AlElegir func(api.Elegible)

	d        *Dialogo
	cabecera *fyne.Container
	marco    *marcoSelector
	buscar   *Entrada
	// lista se conserva entre páginas: «Cargar más» no devuelve el desplazamiento
	// arriba, como no lo hace el navegador.
	lista *container.Scroll
	rueda *Rueda
	// lanzar ejecuta la carga; nil es en su propia goroutine. Las pruebas la hacen en
	// el acto, porque su `fyne.Do` no pasa al hilo de la interfaz.
	lanzar func(func())

	mu     sync.Mutex
	vuelta int // lo que llega de una vuelta vieja no se pinta
	reloj  *time.Timer
	// Estado de la lista: nil y sin error es «cargando».
	items   []api.Elegible
	cursor  string
	fallo   bool
	cargado bool
	// pila es la ruta de Drive: id y nombre de cada carpeta desde «Mi unidad».
	pila []carpetaSelector
	q    string
}

type carpetaSelector struct{ id, nombre string }

// Mostrar abre el selector sobre el lienzo y empieza a cargar.
func (s *SelectorSuite) Mostrar(c fyne.Canvas) {
	s.marco = &marcoSelector{s: s}
	s.marco.ExtendBaseWidget(s.marco)
	s.cabecera = container.NewStack()
	if s.Origen.Navegable() {
		s.pintarMigas()
	} else {
		s.buscar = NuevaEntrada(s.Textos.Buscar)
		s.buscar.AlCambiar(func(q string) {
			s.mu.Lock()
			s.q = q
			if s.reloj != nil {
				s.reloj.Stop()
			}
			s.reloj = time.AfterFunc(esperaBusquedaSelc, func() { fyne.Do(func() { s.cargar(false) }) })
			s.mu.Unlock()
		})
		s.cabecera.Objects = []fyne.CanvasObject{s.buscar}
	}
	var lista fyne.CanvasObject = s.marco
	if s.Textos.Pie != "" {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		lista = container.New(Columna{Hueco: tema.Espacio(2)}, s.marco, NuevoParrafo(s.Textos.Pie, tema.TextoSM, Normal, "text-subtle", v))
	}
	cuerpo := container.New(Columna{Hueco: huecoCabSelector}, s.cabecera, lista)
	cancelar := NuevoBoton(s.Textos.Cancelar, nil).Con(Fantasma, TallaMD)
	s.d = NuevoDialogo(s.Textos.Titulo, cuerpo).Con(DialogoMD, cancelar)
	cancelar.AlPulsar = s.d.Descartar
	if s.Textos.Cerrar != "" {
		s.d.RotuloCerrar = s.Textos.Cerrar
	}
	s.d.AlCerrar = s.cerrar
	if s.buscar != nil {
		s.d.Foco = s.buscar
	}
	s.d.Mostrar(c)
	s.cargar(false)
}

// cerrar retira el diálogo y deja caer lo que estuviera en vuelo.
func (s *SelectorSuite) cerrar() {
	s.mu.Lock()
	s.vuelta++
	if s.reloj != nil {
		s.reloj.Stop()
	}
	s.mu.Unlock()
	s.d.Cerrar()
}

// Elegir cierra y entrega. Lo llama la fila de un elemento.
func (s *SelectorSuite) elegir(e api.Elegible) {
	s.cerrar()
	if s.AlElegir != nil {
		s.AlElegir(e)
	}
}

// entrar navega a una carpeta de Drive.
func (s *SelectorSuite) entrar(e api.Elegible) {
	s.pila = append(s.pila, carpetaSelector{e.ID, e.Nombre})
	s.pintarMigas()
	s.cargar(false)
}

// pintarMigas rehace la ruta: «Mi unidad» y una miga por carpeta.
func (s *SelectorSuite) pintarMigas() {
	ruta := make([]string, 0, 1+len(s.pila))
	ruta = append(ruta, s.Textos.Raiz)
	for _, c := range s.pila {
		ruta = append(ruta, c.nombre)
	}
	m := NuevasMigas(s.Textos.Migas, ruta, func(i int) {
		s.pila = s.pila[:i]
		s.pintarMigas()
		s.cargar(false)
	})
	m.Estilo = MigasDelSelector
	s.cabecera.Objects = []fyne.CanvasObject{m}
	s.cabecera.Refresh()
}

// cargar pide una página. Con `mas`, la siguiente, que se AÑADE a lo que hay.
func (s *SelectorSuite) cargar(mas bool) {
	s.mu.Lock()
	s.vuelta++
	mia, q, cursor := s.vuelta, s.q, ""
	carpeta := ""
	if n := len(s.pila); n > 0 {
		carpeta = s.pila[n-1].id
	}
	if mas {
		cursor = s.cursor
	} else {
		s.items, s.cargado = nil, false
	}
	s.fallo = false
	s.mu.Unlock()
	s.marco.Refresh()
	if s.Cargar == nil {
		return
	}
	lanzar := s.lanzar
	if lanzar == nil {
		lanzar = func(f func()) { go f() }
	}
	lanzar(func() {
		p, err := s.Cargar(context.Background(), carpeta, q, cursor)
		fyne.Do(func() {
			s.mu.Lock()
			if mia != s.vuelta {
				s.mu.Unlock()
				return
			}
			if err != nil {
				s.fallo = true
			} else {
				s.items, s.cursor, s.cargado = append(s.items, p.Elegibles...), p.Cursor, true
			}
			s.mu.Unlock()
			s.marco.Refresh()
		})
	})
}

// marcoSelector es la caja de la lista con sus cuatro estados: cargando, error, vacío y
// la lista.
type marcoSelector struct {
	widget.BaseWidget
	s *SelectorSuite
}

// MedidaMarcoSelector es la caja: alto fijo (`h-72`) y el ancho que le den.
func MedidaMarcoSelector() fyne.Size { return fyne.NewSize(0, AltoCajaSelector) }

func (m *marcoSelector) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El relleno es el del panel del diálogo (`panel-surface`): la caja de la web no
	// tiene fondo propio y deja ver el del Modal. Transparente NO vale: `Caja` pinta el
	// borde como un relleno por debajo, y se vería el color del borde en toda la caja.
	r := &marcoRenderer{m: m, borde: NuevaCaja(tema.Color("panel-surface", v), tema.Color("border", v), tema.RadioMD)}
	r.Refresh()
	return r
}

type marcoRenderer struct {
	m         *marcoSelector
	borde     *Caja
	contenido fyne.CanvasObject
	objetos   []fyne.CanvasObject
}

func (r *marcoRenderer) MinSize() fyne.Size           { return MedidaMarcoSelector() }
func (r *marcoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *marcoRenderer) Destroy()                     {}

func (r *marcoRenderer) Layout(t fyne.Size) {
	r.borde.Redimensionar(t)
	if r.contenido != nil {
		r.contenido.Move(fyne.NewPos(tema.Borde, tema.Borde))
		r.contenido.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde, 0)))
	}
}

// Refresh rehace lo de dentro según el estado.
func (r *marcoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.borde.Pintar(tema.Color("panel-surface", v), tema.Color("border", v))
	r.contenido = r.m.s.contenido(v)
	r.objetos = append(r.borde.Objetos(), r.contenido)
	r.Layout(r.m.Size())
	canvas.Refresh(r.m)
}

// contenido es lo que va dentro de la caja en el estado actual.
func (s *SelectorSuite) contenido(v tema.Variante) fyne.CanvasObject {
	s.mu.Lock()
	items, cursor, fallo, cargado := s.items, s.cursor, s.fallo, s.cargado
	s.mu.Unlock()
	if s.rueda != nil && (fallo || cargado) {
		s.rueda.Parar() // una animación que nadie ve sigue gastando
		s.rueda = nil
	}
	switch {
	case fallo:
		vac := NuevoVacio(s.Textos.Fallo, "")
		vac.Ilustracion = iconos.Trazo(iconos.Aviso, LadoAvisoSelector, "danger-text", v).Objeto()
		vac.Accion = NuevoBoton(s.Textos.Reintentar, func() { s.cargar(false) }).Con(Secundario, TallaSM)
		return container.New(Margen{margenEstadoSel, margenEstadoSel, margenEstadoSel, margenEstadoSel},
			container.NewCenter(vac))
	case !cargado:
		s.lista = nil
		if s.rueda == nil {
			s.rueda = NuevaRueda(LadoRuedaSelector, "text", v)
		}
		return container.NewCenter(s.rueda)
	case len(items) == 0:
		return container.NewCenter(Texto(s.Textos.Vacio, tema.TextoBase, Normal, tema.Color("text-muted", v)))
	}
	filas := make([]fyne.CanvasObject, 0, len(items)+1)
	for _, e := range items {
		filas = append(filas, s.fila(e))
	}
	if cursor != "" {
		mas := NuevoBoton(s.Textos.MasResultados, func() { s.cargar(true) }).Con(Secundario, TallaSM)
		filas = append(filas, container.New(Margen{Arr: pyMasSelector, Abj: pyMasSelector}, container.NewCenter(mas)))
	}
	columna := container.New(Columna{}, filas...)
	if s.lista == nil {
		s.lista = container.NewVScroll(columna)
	} else {
		s.lista.Content = columna
		s.lista.Refresh()
	}
	return s.lista
}

// fila monta la de un elemento: las carpetas entran, lo demás se elige.
func (s *SelectorSuite) fila(e api.Elegible) *FilaSelector {
	f := &FilaSelector{Elegible: e, Glifo: s.GlifoElemento}
	if e.Carpeta {
		f.Glifo = s.GlifoCarpeta
		f.AlPulsar = func() { s.entrar(e) }
	} else {
		f.Meta = e.Meta
		if e.Bytes > 0 && f.Meta == "" {
			// La web escribe aquí los bytes en crudo (`String(r.size)`); las otras dos
			// listas de Drive de la suite los escriben con `formatBytes`, que es lo que
			// se lee.
			f.Meta = i18n.FormatBytes(s.Locale, float64(e.Bytes))
		}
		f.AlPulsar = func() { s.elegir(e) }
		f.Inerte = e.URL == "" // `disabled={!it.isDirectory && !it.url}`
	}
	f.Nombre = e.Nombre
	f.ExtendBaseWidget(f)
	return f
}

// FilaSelector es una fila de la lista: el `<button>` de SuitePicker.tsx.
type FilaSelector struct {
	pulsable
	Elegible api.Elegible
	// Glifo es la ruta SVG del dibujo de delante.
	Glifo string
	// Meta es lo de la derecha; en una carpeta va el galón en su lugar.
	Meta string
}

var _ fyne.Widget = (*FilaSelector)(nil)

// AccessibilityLabel es el nombre del elemento.
func (f *FilaSelector) AccessibilityLabel() string { return f.Elegible.Nombre }

// MedidaFilaSelector es lo que pide una fila: su relleno y el glifo a lo ancho, y sus
// 34,25 de alto.
func MedidaFilaSelector() fyne.Size {
	return fyne.NewSize(2*pxFilaSelector+LadoGlifoSelector, AltoFilaSelector)
}

// CreateRenderer compone fondo, pelo, glifo, nombre y meta o galón.
func (f *FilaSelector) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaSelectorRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		pelo:   canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(f.Glifo, LadoGlifoSelector, color.NRGBA{}),
		nombre: Texto(f.Elegible.Nombre, tema.TextoBase, Normal, color.NRGBA{}),
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.pelo, r.glifo.Objeto(), r.nombre}
	switch {
	case f.Elegible.Carpeta:
		r.galon = iconos.Trazo(iconos.ChevronDerecha, LadoGalonSelector, "text-subtle", v)
		r.objetos = append(r.objetos, r.galon.Objeto())
	case f.Meta != "":
		r.meta = Texto(f.Meta, tema.TextoXS, Normal, color.NRGBA{})
		r.objetos = append(r.objetos, r.meta)
	}
	r.Refresh()
	return r
}

type filaSelectorRenderer struct {
	f            *FilaSelector
	fondo, pelo  *canvas.Rectangle
	glifo, galon *iconos.Dibujo
	nombre, meta *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *filaSelectorRenderer) MinSize() fyne.Size           { return MedidaFilaSelector() }
func (r *filaSelectorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaSelectorRenderer) Destroy()                     {}

// Layout: glifo a la izquierda, galón o meta pegados a la derecha y el nombre en medio,
// recortado (`flex-1 truncate`). Todo centrado en la caja de contenido, que es la fila
// sin su pelo de abajo.
func (r *filaSelectorRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.pelo.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.pelo.Resize(fyne.NewSize(t.Width, tema.Borde))
	caja := t.Height - tema.Borde
	r.glifo.Colocar(pxFilaSelector, (caja-LadoGlifoSelector)/2, LadoGlifoSelector)
	x := pxFilaSelector + LadoGlifoSelector + huecoFilaSelector
	derecha := t.Width - pxFilaSelector
	switch {
	case r.galon != nil:
		derecha -= LadoGalonSelector
		r.galon.Colocar(derecha, (caja-LadoGalonSelector)/2, LadoGalonSelector)
		derecha -= huecoFilaSelector
	case r.meta != nil:
		ancho := Truncar(r.meta, r.f.Meta, tema.TextoXS, Normal, MaxMetaSelector)
		derecha -= ancho
		ColocarTexto(r.meta, tema.TextoXS, derecha, (caja-tema.TextoXS.Linea)/2)
		derecha -= huecoFilaSelector
	}
	Truncar(r.nombre, r.f.Elegible.Nombre, tema.TextoBase, Normal, max(derecha-x, 0))
	ColocarTexto(r.nombre, tema.TextoBase, x, (caja-tema.TextoBase.Linea)/2)
}

// Refresh pinta los estados: `hover` y `focus-visible` en `bg-subtle`, y a media tinta
// si no se puede elegir (`disabled:opacity-50`).
func (r *filaSelectorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := func(token string) color.NRGBA {
		c := tema.Color(token, v)
		if r.f.Inerte {
			c = medioTono(c)
		}
		return c
	}
	r.fondo.FillColor = color.NRGBA{}
	if (r.f.encima || r.f.enfocado) && !r.f.Inerte {
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	r.pelo.FillColor = tema.Color("border-subtle", v)
	glifo := "text-muted"
	if r.f.Elegible.Carpeta {
		glifo = "primary-text"
	}
	r.glifo.TenirCon(tinta(glifo))
	r.nombre.Color = tinta("text")
	if r.meta != nil {
		r.meta.Color = tinta("text-subtle")
	}
	if r.galon != nil {
		r.galon.TenirCon(tinta("text-subtle"))
	}
	r.Layout(r.f.Size())
	canvas.Refresh(r.f)
}
