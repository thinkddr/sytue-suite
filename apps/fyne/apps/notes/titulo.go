// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Paridad web: renombrar una nota EN LÍNEA —el título de NoteCard.tsx y la fila de
// NoteTree.tsx—. «En línea y no en un diálogo: el rótulo ya está donde hay que escribir,
// y un modal para una palabra es tres clics más».

// Rellenos del campo de renombrar, de la hoja.
const (
	// PXCampoTitulo es el `px-1.5` de los dos campos.
	PXCampoTitulo = 1.5 * 0.25 * tema.Rem
	// PYCampoTarjeta es el `py-0.5` del de la tarjeta y PYCampoArbol el `py-1` del árbol.
	PYCampoTarjeta = 0.5 * 0.25 * tema.Rem
	PYCampoArbol   = 1 * 0.25 * tema.Rem
	// TopeTitulo es el `maxLength={200}` del campo de la tarjeta.
	TopeTitulo = 200
	// PXBotonTitulo es el `-mx-1 px-1` del botón del título: se pinta 3,5 más allá del
	// texto a cada lado sin moverlo.
	PXBotonTitulo = 1 * 0.25 * tema.Rem
)

// CampoEnLinea es el `<input>` suelto con el que se renombra: `rounded-md border px-1.5`
// con la letra de `text-base`. No es `ui.Entrada`, que es el `<Input>` de Form.tsx con
// sus tallas (24 o 31,5): este mide la línea más su relleno, 24,75 en la tarjeta y 28,25
// en el árbol, y cambiarlo por una talla movería la cabecera al empezar a escribir.
//
// Intro y salir del campo GUARDAN; Escape descarta. Es la regla de las dos webs.
type CampoEnLinea struct {
	widget.BaseWidget

	ed    *ui.EditorPlano
	py    float32
	fondo string
	// enfocado enciende el aro; hecho dice que ya se confirmó o se canceló, para que el
	// FocusLost que llega DESPUÉS de Intro no vuelva a guardar.
	enfocado, hecho bool

	AlConfirmar func(string)
	AlCancelar  func()
}

var _ fyne.Widget = (*CampoEnLinea)(nil)

// NuevoCampoEnLinea monta el campo con su texto, su marca de agua, su relleno vertical y
// el token de su fondo. `tope` es el `maxLength` (0 sin tope).
func NuevoCampoEnLinea(texto, marca string, py float32, fondo string, tope int) *CampoEnLinea {
	c := &CampoEnLinea{py: py, fondo: fondo}
	c.ExtendBaseWidget(c)
	c.ed = ui.NuevoEditorPlano(tema.TextoBase, func(f bool) {
		c.enfocado = f
		c.Refresh()
		if !f {
			c.confirmar()
		}
	})
	c.ed.PlaceHolder = marca
	c.ed.PonerTexto(texto)
	if tope > 0 {
		c.ed.AlCambiar(func(s string) {
			if r := []rune(s); len(r) > tope {
				c.ed.PonerTexto(string(r[:tope]))
			}
		})
	}
	c.ed.Tecla = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			c.confirmar()
			return true
		case fyne.KeyEscape:
			c.cancelar()
			return true
		default:
		}
		return false
	}
	return c
}

// Editor es lo que recibe el foco: el campo se abre para escribir.
func (c *CampoEnLinea) Editor() fyne.Focusable { return c.ed }

// SeleccionarTodo deja el texto entero marcado: renombrar suele ser sustituir, no
// añadir al final (el `campo.current?.select()` del árbol).
func (c *CampoEnLinea) SeleccionarTodo() { c.ed.TypedShortcut(&fyne.ShortcutSelectAll{}) }

// Texto es lo escrito.
func (c *CampoEnLinea) Texto() string { return c.ed.Texto() }

func (c *CampoEnLinea) confirmar() {
	if c.hecho {
		return
	}
	c.hecho = true
	llamar1(c.AlConfirmar, c.ed.Texto())
}

func (c *CampoEnLinea) cancelar() {
	if c.hecho {
		return
	}
	c.hecho = true
	llamar0(c.AlCancelar)
}

// AltoCampoEnLinea es la línea de `text-base` con el relleno y el borde.
func AltoCampoEnLinea(py float32) float32 { return tema.TextoBase.Linea + 2*(py+tema.Borde) }

// CreateRenderer: caja, aro y el editor dentro. Termina en Refresh (regla 7).
func (c *CampoEnLinea) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &campoEnLineaRenderer{
		c:    c,
		caja: ui.NuevaCaja(tema.Color(c.fondo, v), tema.Color("border", v), tema.RadioMD),
		aro:  ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
	}
	r.objetos = append(r.aro.Antes(r.caja.Objetos()...), c.ed.Objeto())
	r.Refresh()
	return r
}

type campoEnLineaRenderer struct {
	c       *CampoEnLinea
	caja    *ui.Caja
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *campoEnLineaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*(PXCampoTitulo+tema.Borde), AltoCampoEnLinea(r.c.py))
}
func (r *campoEnLineaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *campoEnLineaRenderer) Destroy()                     {}

func (r *campoEnLineaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x, y := PXCampoTitulo+tema.Borde, r.c.py+tema.Borde
	// El editor ocupa la caja de su FUENTE, centrada en la de línea de CSS: es lo que
	// hace `ColocarTexto` con un texto suelto.
	alto := r.c.ed.AltoFuente()
	r.c.ed.Colocar(fyne.NewPos(x, y+(tema.TextoBase.Linea-alto)/2),
		fyne.NewSize(max(t.Width-2*x, 0), alto))
}

func (r *campoEnLineaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color(r.c.fondo, v), tema.Color("border", v))
	r.aro.Pintar(v)
	r.aro.Ver(r.c.enfocado)
}

// BotonTitulo es el título de una nota cuando se puede cambiar: pulsarlo lo edita, que
// es donde todo el mundo lo intenta primero. `button` y no texto con un clic, para que
// llegue con el teclado y se anuncie como lo que es.
//
// `-mx-1 px-1 rounded-sm hover:bg-bg-subtle`: el fondo de encima sobresale 3,5 a cada
// lado del texto sin moverlo. Recorta con elipsis (`truncate`).
type BotonTitulo struct {
	ui.Pulsable

	// pesoPuesto distingue «peso normal» de «sin decir»: ui.Normal es el valor cero.
	pesoPuesto bool

	Texto string
	Tinta string
	// Tachada es el `line-through` del botón: la tarea hecha o cancelada.
	Tachada bool
	// Peso y Escalon son la letra del título: `font-medium text-base` en una tarjeta
	// (el valor cero), `font-normal` en una fila —ahí el título es el texto de la nota,
	// no un encabezado— y `text-2xl font-semibold` en la hoja de un árbol.
	Peso    ui.Peso
	Escalon tema.Escalon
}

// letra es el escalón y el peso con sus valores por defecto.
func (b *BotonTitulo) letra() (tema.Escalon, ui.Peso) {
	e, p := b.Escalon, b.Peso
	if e == (tema.Escalon{}) {
		e = tema.TextoBase
	}
	if p == ui.Normal && !b.pesoPuesto {
		p = ui.Medio
	}
	return e, p
}

var _ fyne.Widget = (*BotonTitulo)(nil)

// NuevoBotonTitulo crea el botón con su texto y el token de su tinta.
func NuevoBotonTitulo(texto, tinta, rotulo string, alPulsar func()) *BotonTitulo {
	b := &BotonTitulo{Texto: texto, Tinta: tinta}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// ConLetra fija el escalón y el peso del título (una fila, o la hoja de un árbol).
func (b *BotonTitulo) ConLetra(e tema.Escalon, p ui.Peso) *BotonTitulo {
	b.Escalon, b.Peso, b.pesoPuesto = e, p, true
	return b
}

// Cursor es la mano: es un botón.
func (b *BotonTitulo) Cursor() desktop.Cursor { return desktop.PointerCursor }

// CreateRenderer: fondo de encima, aro y el texto.
func (b *BotonTitulo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e, peso := b.letra()
	r := &botonTituloRenderer{
		b:     b,
		fondo: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		texto: ui.Texto(b.Texto, e, peso, tema.Color(b.Tinta, v)),
		raya:  canvas.NewRectangle(tema.Color(b.Tinta, v)),
	}
	r.fondo.CornerRadius = tema.RadioSM
	r.Refresh()
	return r
}

type botonTituloRenderer struct {
	b     *BotonTitulo
	fondo *canvas.Rectangle
	aro   *ui.Aro
	texto *canvas.Text
	raya  *canvas.Rectangle
}

// MinSize es el texto entero y la línea; quien lo coloca le da menos si no cabe.
func (r *botonTituloRenderer) MinSize() fyne.Size {
	e, peso := r.b.letra()
	return fyne.NewSize(ui.Medir(r.b.Texto, e, peso).Width, e.Linea)
}

func (r *botonTituloRenderer) Objects() []fyne.CanvasObject {
	return append(r.aro.Antes(r.fondo), r.texto, r.raya)
}
func (r *botonTituloRenderer) Destroy() {}

func (r *botonTituloRenderer) Layout(t fyne.Size) {
	pos := fyne.NewPos(-PXBotonTitulo, 0)
	tam := fyne.NewSize(t.Width+2*PXBotonTitulo, t.Height)
	r.fondo.Move(pos)
	r.fondo.Resize(tam)
	r.aro.ColocarEn(pos, tam)
	e, peso := r.b.letra()
	ui.Truncar(r.texto, r.b.Texto, e, peso, t.Width)
	ui.ColocarTexto(r.texto, e, 0, (t.Height-e.Linea)/2)
	// La raya a media altura de la x, del ancho de lo que se ve (como `tachar` del lateral).
	base := (t.Height-e.Linea)/2 + (e.Linea-1.2*e.Tam)/2 + 0.95*e.Tam
	r.raya.Move(fyne.NewPos(0, base-0.3*e.Tam))
	r.raya.Resize(fyne.NewSize(min(ui.Medir(r.texto.Text, e, peso).Width, t.Width), tema.Borde))
}

func (r *botonTituloRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.b.Encima() {
		r.fondo.Show()
	} else {
		r.fondo.Hide()
	}
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.texto.Color = tema.Color(r.b.Tinta, v)
	r.raya.FillColor = r.texto.Color
	if r.b.Tachada {
		r.raya.Show()
	} else {
		r.raya.Hide()
	}
	r.raya.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.fondo.Refresh()
	r.texto.Refresh()
}
