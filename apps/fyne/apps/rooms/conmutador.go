// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"cmp"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Contrapartida de QuickSwitcher.tsx y client/conmutador.ts dentro de la pantalla
// principal declarada en pantalla.go.
//
// «Ir a un canal» (Ctrl+G): un diálogo `sm` con la caja, la lista de hasta doce canales
// y la pista de las teclas. Escribir filtra, las flechas mueven y Intro abre. Solo los
// canales a los que ya se pertenece; para otros está «Explorar canales».

// TopeConmutador es `MAX`: las filas que caben.
const TopeConmutador = 12

// Medidas de la HOJA: la lista es `rounded-md border divide-y divide-border/60
// max-h-[50dvh]`; cada fila `flex items-center gap-3 px-3 py-2 text-sm` con la marca en
// su `h-6 w-6`, el nombre (semi si hay sin leer) y «actual» o la pastilla de sin leer.
var (
	CanalonFilaConmutador = tema.Espacio(3)
	RellenoFilaConmutador = tema.Espacio(2)
	HuecoFilaConmutador   = tema.Espacio(3)
	CajaMarcaConmutador   = tema.Espacio(6)
	// AltoFilaConmutador manda la caja de la marca (21) sobre la línea (15,75).
	AltoFilaConmutador = 2*RellenoFilaConmutador + CajaMarcaConmutador
	HuecoConmutador    = tema.Espacio(2)
)

// altoFilaConmutador es AltoFilaConmutador con el suelo del dedo: 44 con
// `pointer-coarse:min-h-11`, como la web (medían 40).
func altoFilaConmutador() float32 {
	if ui.EsTactil() {
		return max(AltoFilaConmutador, ui.SueloTactilBarra)
	}
	return AltoFilaConmutador
}

// Normalizar es `normalizar`: sin acentos ni mayúsculas, y sin espacios a los lados.
// «facturacion» encuentra «Facturación».
func Normalizar(s string) string {
	return strings.TrimSpace(buscar.Plegar(s))
}

// OrdenarConmutador es `ordenarCanales`: con la caja vacía, lo que tiene algo pendiente
// primero y luego por nombre; escribiendo, primero lo que EMPIEZA por lo escrito,
// después lo que lo contiene (lo demás no sale), y dentro de cada grupo lo pendiente
// por delante. El nombre se compara como `localeCompare` con `sensitivity: 'base'`.
func OrdenarConmutador(l i18n.Locale, canales []Canal, q string) []Canal {
	texto := Normalizar(q)
	peso := func(c Canal) int {
		if texto == "" {
			return 0
		}
		n := Normalizar(c.Nombre)
		switch {
		case strings.HasPrefix(n, texto):
			return 2
		case strings.Contains(n, texto):
			return 1
		}
		return -1
	}
	pendiente := func(c Canal) int {
		if c.SinLeer+c.Menciones > 0 {
			return 1
		}
		return 0
	}
	col := collate.New(l.Tag(), collate.Loose)
	out := make([]Canal, 0, len(canales))
	for _, c := range canales {
		if peso(c) >= 0 {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b Canal) int {
		return cmp.Or(cmp.Compare(peso(b), peso(a)), cmp.Compare(pendiente(b), pendiente(a)),
			col.CompareString(a.Nombre, b.Nombre))
	})
	return out
}

// Conmutador es el contenido del diálogo: la caja, la lista y la pista.
type Conmutador struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale
	// Canales son los del carril; Actual el abierto (lleva «actual» en vez de la
	// pastilla).
	Canales []Canal
	Actual  string
	// Marca pinta el icono de cada canal, el MISMO que el del carril.
	Marca func(Canal) fyne.CanvasObject

	AlElegir func(Canal)

	caja    *ui.Entrada
	filas   []Canal
	indice  int
	lista   *fyne.Container
	vacio   *canvas.Text
	pista   *canvas.Text
	borde   *ui.Caja
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*Conmutador)(nil)

// NuevoConmutador crea el contenido con la caja vacía.
func NuevoConmutador(t *i18n.Translator, l i18n.Locale, canales []Canal, actual string) *Conmutador {
	c := &Conmutador{T: t, Locale: l, Canales: canales, Actual: actual}
	c.ExtendBaseWidget(c)
	c.caja = ui.NuevaEntrada(t.T("switcher.placeholder", nil))
	c.caja.AlCambiar(func(string) {
		c.indice = 0
		c.Refresh()
	})
	c.caja.AlTeclear = c.tecla
	c.filtrar()
	return c
}

// Caja es la caja de escribir, para darle el foco al abrir.
func (c *Conmutador) Caja() *ui.Entrada { return c.caja }

// Filas son las que se ven ahora.
func (c *Conmutador) Filas() []Canal { return c.filas }

// Indice es la fila señalada (`aria-selected`).
func (c *Conmutador) Indice() int { return c.indice }

func (c *Conmutador) filtrar() {
	c.filas = OrdenarConmutador(c.Locale, c.Canales, c.caja.Texto())
	if len(c.filas) > TopeConmutador {
		c.filas = c.filas[:TopeConmutador]
	}
	c.indice = min(c.indice, max(len(c.filas)-1, 0))
}

// tecla son las del `onKeyDown` de la caja: flechas y Intro. Escape lo atiende el
// diálogo.
func (c *Conmutador) tecla(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyDown:
		c.indice = min(c.indice+1, max(len(c.filas)-1, 0))
	case fyne.KeyUp:
		c.indice = max(c.indice-1, 0)
	case fyne.KeyReturn, fyne.KeyEnter:
		if c.indice < len(c.filas) && c.AlElegir != nil {
			c.AlElegir(c.filas[c.indice])
		}
		return true
	default:
		return false
	}
	c.Refresh()
	return true
}

// CreateRenderer compone la caja, la lista y la pista.
func (c *Conmutador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.vacio = ui.Texto(c.T.T("switcher.empty", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	c.pista = ui.Texto(c.T.T("switcher.hint", nil), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	c.borde = ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	r := &conmutadorRenderer{c: c}
	r.Refresh()
	return r
}

// MedidaConmutador es lo que ocupa con `n` filas. Es la única cuenta.
func MedidaConmutador(n int) fyne.Size {
	caja := ui.MedidaEntrada(ui.CampoMD, 0, false).Height
	lista := tema.TextoSM.Linea + 2*tema.Espacio(2) // el `px-1 py-2` de «Ningún canal coincide»
	if n > 0 {
		lista = 2*tema.Borde + float32(n)*altoFilaConmutador() + float32(n-1)*tema.Borde
	}
	return fyne.NewSize(0, caja+HuecoConmutador+lista+HuecoConmutador+escalonMeta.Linea)
}

type conmutadorRenderer struct{ c *Conmutador }

func (r *conmutadorRenderer) MinSize() fyne.Size           { return MedidaConmutador(len(r.c.filas)) }
func (r *conmutadorRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *conmutadorRenderer) Destroy()                     {}

func (r *conmutadorRenderer) Layout(t fyne.Size) {
	c := r.c
	mc := ui.MedidaEntrada(ui.CampoMD, 0, false)
	c.caja.Move(fyne.NewPos(0, 0))
	c.caja.Resize(fyne.NewSize(t.Width, mc.Height))
	y := mc.Height + HuecoConmutador
	if len(c.filas) == 0 {
		ui.ColocarTexto(c.vacio, tema.TextoSM, tema.Espacio(1), y+tema.Espacio(2))
		y += tema.TextoSM.Linea + 2*tema.Espacio(2)
	} else {
		alto := 2*tema.Borde + float32(len(c.filas))*altoFilaConmutador() + float32(len(c.filas)-1)*tema.Borde
		ui.MoverCaja(c.borde, fyne.NewPos(0, y), fyne.NewSize(t.Width, alto))
		c.lista.Move(fyne.NewPos(tema.Borde, y+tema.Borde))
		c.lista.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), alto-2*tema.Borde))
		y += alto
	}
	ui.ColocarTexto(c.pista, escalonMeta, tema.Espacio(1), y+HuecoConmutador)
}

func (r *conmutadorRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.filtrar()
	filas := make([]fyne.CanvasObject, 0, 2*len(c.filas))
	for i, canal := range c.filas {
		if i > 0 {
			// `divide-border/60`: el filete de entre filas, compuesto sobre la lista.
			sep := canvas.NewRectangle(atenuado("border", v, 0.6))
			sep.SetMinSize(fyne.NewSize(0, tema.Borde))
			filas = append(filas, sep)
		}
		var marca fyne.CanvasObject
		if c.Marca != nil {
			marca = c.Marca(canal)
		}
		f := NuevaFilaConmutador(c.T, canal, marca, i == c.indice, canal.ID == c.Actual)
		f.AlEncima = func() {
			if c.indice != i {
				c.indice = i
				c.Refresh()
			}
		}
		f.AlPulsar = func() {
			if c.AlElegir != nil {
				c.AlElegir(canal)
			}
		}
		filas = append(filas, f)
	}
	c.lista = container.New(ui.Columna{}, filas...)
	c.objetos = []fyne.CanvasObject{c.caja}
	if len(c.filas) == 0 {
		c.objetos = append(c.objetos, c.vacio)
	} else {
		c.objetos = append(c.objetos, c.borde.Objetos()...)
		c.objetos = append(c.objetos, c.lista)
	}
	c.objetos = append(c.objetos, c.pista)
	c.vacio.Color = tema.Color("text-muted", v)
	c.pista.Color = tema.Color("text-subtle", v)
	r.Layout(c.Size())
}

// FilaConmutador es una fila de la lista.
type FilaConmutador struct {
	widget.BaseWidget

	Canal     Canal
	Marca     fyne.CanvasObject
	Senalada  bool
	EsActual  bool
	AlPulsar  func()
	AlEncima  func()
	actual    string
	fondo     *canvas.Rectangle
	nombre    *canvas.Text
	cola      *canvas.Text
	pastilla  *ui.Caja
	objetos   []fyne.CanvasObject
	anchoCola float32
}

var (
	_ fyne.Tappable     = (*FilaConmutador)(nil)
	_ desktop.Hoverable = (*FilaConmutador)(nil)
)

// NuevaFilaConmutador crea la fila.
func NuevaFilaConmutador(t *i18n.Translator, c Canal, marca fyne.CanvasObject, senalada, actual bool) *FilaConmutador {
	f := &FilaConmutador{
		Canal: c, Marca: marca, Senalada: senalada, EsActual: actual,
		actual: t.T("switcher.current", nil),
	}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el nombre del canal (`role="option"`).
func (f *FilaConmutador) AccessibilityLabel() string { return f.Canal.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaConmutador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// Tapped abre el canal.
func (f *FilaConmutador) Tapped(*fyne.PointEvent) {
	if f.AlPulsar != nil {
		f.AlPulsar()
	}
}

// MouseIn es el `onMouseEnter` que mueve la señal a esta fila.
func (f *FilaConmutador) MouseIn(*desktop.MouseEvent) {
	if f.AlEncima != nil {
		f.AlEncima()
	}
}

// MouseMoved no hace nada.
func (f *FilaConmutador) MouseMoved(*desktop.MouseEvent) {}

// MouseOut no hace nada: la señal se queda donde estaba, como en la web.
func (f *FilaConmutador) MouseOut() {}

// CreateRenderer compone la fila.
func (f *FilaConmutador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	if f.Senalada {
		f.fondo.FillColor = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	}
	peso := ui.Normal
	if f.Canal.SinLeer > 0 {
		peso = ui.SemiNegrita
	}
	f.nombre = ui.Texto(f.Canal.Nombre, tema.TextoSM, peso, tema.Color("text", v))
	f.objetos = []fyne.CanvasObject{f.fondo}
	if f.Marca != nil {
		f.objetos = append(f.objetos, f.Marca)
	}
	f.objetos = append(f.objetos, f.nombre)
	switch {
	case f.EsActual:
		f.cola = ui.Texto(f.actual, escalonMeta, ui.Normal, tema.Color("text-subtle", v))
		f.anchoCola = ui.Medir(f.actual, escalonMeta, ui.Normal).Width
		f.objetos = append(f.objetos, f.cola)
	case f.Canal.SinLeer > 0:
		// `bg-primary rounded-full px-1.5 text-xs font-bold tabular-nums`.
		n := Contador(f.Canal.SinLeer)
		f.cola = ui.Texto(n, tema.TextoXS, ui.Negrita, tema.Color("on-primary", v))
		f.anchoCola = ui.Medir(n, tema.TextoXS, ui.Negrita).Width + 2*tema.Espacio(1.5)
		f.pastilla = ui.NuevaCaja(tema.Color("primary", v), color.NRGBA{}, tema.RadioPill)
		f.objetos = append(f.objetos, f.pastilla.Objetos()...)
		f.objetos = append(f.objetos, f.cola)
	}
	return &filaConmutadorRenderer{f: f}
}

type filaConmutadorRenderer struct{ f *FilaConmutador }

func (r *filaConmutadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, altoFilaConmutador())
}
func (r *filaConmutadorRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaConmutadorRenderer) Destroy()                     {}

func (r *filaConmutadorRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := CanalonFilaConmutador
	if f.Marca != nil {
		m := f.Marca.MinSize()
		f.Marca.Move(fyne.NewPos(x+(CajaMarcaConmutador-m.Width)/2, (t.Height-m.Height)/2))
		f.Marca.Resize(m)
	}
	x += CajaMarcaConmutador + HuecoFilaConmutador
	der := t.Width - CanalonFilaConmutador
	if f.cola != nil {
		der -= f.anchoCola
		if f.pastilla != nil {
			ui.MoverCaja(f.pastilla, fyne.NewPos(der, (t.Height-tema.TextoXS.Linea)/2),
				fyne.NewSize(f.anchoCola, tema.TextoXS.Linea))
			ui.ColocarTexto(f.cola, tema.TextoXS, der+tema.Espacio(1.5), (t.Height-tema.TextoXS.Linea)/2)
		} else {
			ui.ColocarTexto(f.cola, escalonMeta, der, (t.Height-escalonMeta.Linea)/2)
		}
		der -= HuecoFilaConmutador
	}
	peso := ui.Normal
	if f.Canal.SinLeer > 0 {
		peso = ui.SemiNegrita
	}
	ui.Truncar(f.nombre, f.Canal.Nombre, tema.TextoSM, peso, max(der-x, 0))
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filaConmutadorRenderer) Refresh() { r.Layout(r.f.Size()) }
