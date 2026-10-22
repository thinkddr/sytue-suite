// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las piezas pulsables de los diálogos de Notepads.
//
// En la web NO son componentes de `packages/ui`: son `<button>` sueltos con sus clases,
// escritos en NotebookSettings.tsx, MoverNotaModal.tsx, TagsModal.tsx y el modal de
// exportar de NotesApp.tsx. Por eso viven aquí y no en el catálogo: lo que comparten es
// la forma —una caja con borde opcional, un fondo al pasar por encima y otro cuando
// está elegida—, y eso es un widget con cuatro parámetros y no cinco copias.

// Opcion es un `<button>` de forma libre: una caja, y dentro lo que se le dé.
type Opcion struct {
	ui.Pulsable

	// Contenido es lo que va dentro, ya con su layout.
	Contenido fyne.CanvasObject
	// Radio es el redondeo; por defecto, el `rounded-control` compartido.
	Radio float32
	// PX y PY son el relleno horizontal y vertical, sin contar el borde.
	PX, PY float32
	// Borde pinta el `border` de un píxel: con `box-sizing: border-box` ocupa sitio.
	Borde bool
	// Fila es una fila de lista (mover, exportar, abrir rápido): con el dedo mide 44,
	// como en la web. Medían 34 y 30.
	Fila bool
	// Activa es `aria-pressed`: pinta BordeActivo y FondoActivo, y no el de encima.
	// FondoActivo puede ser translúcido (`bg-primary/5`): se compone sobre la
	// superficie como lo hace el navegador.
	Activa      bool
	BordeActivo color.NRGBA
	FondoActivo color.NRGBA
	// Superficie es el token de lo que hay DEBAJO: el botón es transparente y se ve
	// su padre. Vacío es `surface-raised`, el panel de un diálogo.
	Superficie string
	// Rotulo es el nombre accesible.
	Rotulo string
}

var _ fyne.Widget = (*Opcion)(nil)

// NuevaOpcion crea la caja pulsable.
func NuevaOpcion(rotulo string, contenido fyne.CanvasObject, alPulsar func()) *Opcion {
	o := &Opcion{Contenido: contenido, Rotulo: rotulo, Radio: tema.Forma("radius-control")}
	o.Nombre, o.AlPulsar = rotulo, alPulsar
	o.ExtendBaseWidget(o)
	return o
}

// AccessibilityLabel es el rótulo.
func (o *Opcion) AccessibilityLabel() string { return o.Rotulo }

// grosor es el borde que ocupa sitio.
func (o *Opcion) grosor() float32 {
	if o.Borde {
		return tema.Borde
	}
	return 0
}

// MedidaOpcion es el contenido con su relleno y su borde.
func MedidaOpcion(o *Opcion) fyne.Size {
	m := o.Contenido.MinSize()
	g := o.grosor()
	return fyne.NewSize(m.Width+2*(o.PX+g), o.suelo(m.Height+2*(o.PY+g)))
}

// suelo es el `pointer-coarse:min-h-11` de una fila de lista: 44 con el dedo.
func (o *Opcion) suelo(alto float32) float32 {
	if o.Fila && ui.EsTactil() {
		return max(alto, ui.SueloTactilBarra)
	}
	return alto
}

// AltoPara mide el contenido al ancho que realmente recibe la tarjeta. Lo usan
// tanto las columnas como la rejilla de disposiciones al encoger el diálogo.
func (o *Opcion) AltoPara(ancho float32) float32 {
	g := o.grosor()
	return o.suelo(ui.AltoA(o.Contenido, max(ancho-2*(o.PX+g), 0)) + 2*(o.PY+g))
}

// CreateRenderer arma aro, caja y contenido. Termina en Refresh (regla 7).
func (o *Opcion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Con borde, la caja se construye CON él: `ui.Caja` decide al nacer si lleva el
	// rectángulo de fuera, y uno transparente la dejaría sin filete para siempre.
	borde := color.NRGBA{}
	if o.Borde {
		borde = tema.Color("border", v)
	}
	r := &opcionRenderer{
		o:    o,
		aro:  ui.NuevoAro(o.Radio, ui.SinHueco, v),
		caja: ui.NuevaCaja(color.NRGBA{}, borde, o.Radio),
	}
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.objetos = append(r.objetos, o.Contenido)
	r.Refresh()
	return r
}

type opcionRenderer struct {
	o       *Opcion
	aro     *ui.Aro
	caja    *ui.Caja
	objetos []fyne.CanvasObject
}

func (r *opcionRenderer) MinSize() fyne.Size           { return MedidaOpcion(r.o) }
func (r *opcionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *opcionRenderer) Destroy()                     {}

func (r *opcionRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	g := r.o.grosor()
	x, y := r.o.PX+g, r.o.PY+g
	r.o.Contenido.Move(fyne.NewPos(x, y))
	r.o.Contenido.Resize(fyne.NewSize(max(t.Width-2*x, 0), max(t.Height-2*y, 0)))
}

func (r *opcionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	o := r.o
	superficie := o.Superficie
	if superficie == "" {
		superficie = "surface-raised"
	}
	// El relleno va OPACO: la caja pinta el borde como un rectángulo lleno por debajo,
	// así que un relleno translúcido dejaría ver el color del borde en toda la caja.
	fondo, borde := tema.Color(superficie, v), tema.Color("border", v)
	switch {
	case o.Activa:
		fondo, borde = tokens.SobreSRGB(o.FondoActivo, fondo), o.BordeActivo
	case o.Encima() && !o.Inerte:
		fondo = tema.Color("bg-subtle", v) // hover:bg-bg-subtle
	}
	r.caja.Pintar(fondo, borde)
	r.aro.Ver(o.Enfocado())
	r.aro.Pintar(v)
	o.Contenido.Refresh()
}

// Muestra es el botón redondo de un color: la paleta de un cuaderno (`h-7 w-7`) y la de
// un estado de etiqueta (`h-5 w-5`).
//
// El elegido lleva `ring-2 ring-text ring-offset-N`: un anillo de dos píxeles del color
// del texto, separado de la muestra por un hueco del color del FONDO de la página —no de
// la superficie del diálogo—. Como el aro de foco, no ocupa sitio.
type Muestra struct {
	ui.Pulsable
	Color  color.NRGBA
	Lado   float32
	Hueco  float32
	Activa bool
	Rotulo string
	// Celda es el blanco, si es mayor que la muestra: con el dedo, 44 con el círculo de
	// `Lado` en su centro (la web: `after:-inset-*` y el paso de 44 entre muestras).
	Celda float32
}

var _ fyne.Widget = (*Muestra)(nil)

// conCeldaTactil da a una paleta de muestras el blanco de 44 con el dedo y devuelve el
// hueco que va entre ellas: ninguno, porque los blancos ya se tocan.
func conCeldaTactil(muestras []fyne.CanvasObject, hueco float32) float32 {
	if !ui.EsTactil() {
		return hueco
	}
	for _, o := range muestras {
		if m, es := o.(*Muestra); es {
			m.Celda = ui.SueloTactilBarra
		}
	}
	return 0
}

// NuevaMuestra crea la muestra de un color de datos.
func NuevaMuestra(c color.NRGBA, lado, hueco float32, rotulo string, alPulsar func()) *Muestra {
	m := &Muestra{Color: c, Lado: lado, Hueco: hueco, Rotulo: rotulo}
	m.Nombre, m.AlPulsar = rotulo, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityLabel es el hexadecimal, que es lo que la web pone en `aria-label`.
func (m *Muestra) AccessibilityLabel() string { return m.Rotulo }

// CreateRenderer arma el anillo, el hueco y el círculo.
func (m *Muestra) CreateRenderer() fyne.WidgetRenderer {
	r := &muestraRenderer{
		m:      m,
		anillo: canvas.NewRectangle(color.NRGBA{}),
		hueco:  canvas.NewRectangle(color.NRGBA{}),
		punto:  canvas.NewRectangle(m.Color),
		aro:    ui.NuevoAro(m.Lado/2, ui.SinHueco, tema.Claro),
	}
	r.Refresh()
	return r
}

type muestraRenderer struct {
	m                    *Muestra
	anillo, hueco, punto *canvas.Rectangle
	aro                  *ui.Aro
}

func (r *muestraRenderer) MinSize() fyne.Size {
	l := max(r.m.Lado, r.m.Celda)
	return fyne.NewSize(l, l)
}

func (r *muestraRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.anillo, r.hueco, r.punto)
}
func (r *muestraRenderer) Destroy() {}

func (r *muestraRenderer) Layout(t fyne.Size) {
	lado := min(t.Width, t.Height)
	if r.m.Celda > 0 {
		lado = r.m.Lado // el círculo no crece con su blanco
	}
	pos := fyne.NewPos((t.Width-lado)/2, (t.Height-lado)/2)
	colocarRedondo(r.punto, pos, lado, 0)
	colocarRedondo(r.hueco, pos, lado, r.m.Hueco)
	colocarRedondo(r.anillo, pos, lado, r.m.Hueco+tema.AnilloFoco)
	r.aro.ColocarEn(pos, fyne.NewSize(lado, lado))
}

// colocarRedondo pone un círculo de `lado + 2·fuera` centrado sobre la caja.
func colocarRedondo(c *canvas.Rectangle, pos fyne.Position, lado, fuera float32) {
	c.Move(fyne.NewPos(pos.X-fuera, pos.Y-fuera))
	c.Resize(fyne.NewSize(lado+2*fuera, lado+2*fuera))
	c.CornerRadius = lado/2 + fuera
}

func (r *muestraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.punto.FillColor = r.m.Color
	r.anillo.FillColor, r.hueco.FillColor = tema.Color("text", v), tema.Color("bg", v)
	for _, o := range []*canvas.Rectangle{r.anillo, r.hueco} {
		if r.m.Activa {
			o.Show()
		} else {
			o.Hide()
		}
		o.Refresh()
	}
	r.punto.Refresh()
	r.aro.Ver(r.m.Enfocado())
	r.aro.Pintar(v)
}

// linea es un texto de UNA línea que ocupa la caja de línea de CSS —19,25 para
// `text-base`, no los 14,70 de la fuente, que es lo que mide un `canvas.Text`— y hace el
// `truncate` de la hoja: si le dan menos ancho que el suyo, recorta con elipsis.
//
// Sin esto, una fila de `py-2 text-sm` medía 27,66 donde la hoja da 29,75: la
// diferencia era justo la de la caja de la fuente contra la de línea.
func linea(s string, e tema.Escalon, p ui.Peso, c color.NRGBA) fyne.CanvasObject {
	return container.New(&cajaLinea{s: s, e: e, p: p}, ui.Texto(s, e, p, c))
}

// lineaTok es linea con el color de un token.
func lineaTok(s string, e tema.Escalon, p ui.Peso, token string, v tema.Variante) fyne.CanvasObject {
	return linea(s, e, p, tema.Color(token, v))
}

type cajaLinea struct {
	s string
	e tema.Escalon
	p ui.Peso
}

func (c *cajaLinea) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(c.s, c.e, c.p).Width, c.e.Linea)
}

func (c *cajaLinea) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		if x, ok := o.(*canvas.Text); ok {
			ui.Truncar(x, c.s, c.e, c.p, t.Width)
			ui.ColocarTexto(x, c.e, 0, (t.Height-c.e.Linea)/2)
		}
	}
}
