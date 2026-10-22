// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los `<button>` a mano del editor. No son `Button` de la web —cada uno lleva sus
// clases—, así que no son `ui.Boton`: son una caja con borde y radio, un glifo opcional
// y un rótulo, y los colores los decide su Estilo según el estado.

// BotonEditor es uno de esos botones: las proporciones del recorte, los fondos del panel
// y los preajustes de Sytue AI.
type BotonEditor struct {
	ui.Pulsable

	Rotulo string
	// Glifo va delante del rótulo, de LadoGlifo de lado y con Hueco hasta él.
	Glifo            fyne.CanvasObject
	LadoGlifo, Hueco float32
	Escalon          tema.Escalon
	Peso             ui.Peso
	MargenX, MargenY float32
	Radio            float32
	// Elegido es el `aria-pressed`; TintaNormal, el token del rótulo sin elegir; Debajo,
	// el de la superficie sobre la que va (vacío, `bg`).
	Elegido             bool
	TintaNormal, Debajo string
	// Estilo da fondo, borde y tinta según el estado.
	Estilo func(v tema.Variante, b *BotonEditor) (fondo, borde, tinta color.NRGBA)

	caja  *ui.Caja
	texto *canvas.Text
	aro   *ui.Aro
}

var _ fyne.Widget = (*BotonEditor)(nil)

// NuevoBotonEditor crea el botón con su rótulo en `text-xs`.
func NuevoBotonEditor(rotulo string, al func()) *BotonEditor {
	b := &BotonEditor{Rotulo: rotulo, Escalon: tema.TextoXS, Peso: ui.Normal, Estilo: estiloOpcion}
	b.Nombre, b.AlPulsar = rotulo, al
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es el rótulo (y el glifo) con su relleno y el borde.
func (b *BotonEditor) MinSize() fyne.Size {
	m := ui.Medir(b.Rotulo, b.Escalon, b.Peso)
	ancho := m.Width + 2*(b.MargenX+tema.Borde)
	if b.Glifo != nil {
		ancho += b.LadoGlifo + b.Hueco
	}
	return fyne.NewSize(ancho, b.Escalon.Linea+2*(b.MargenY+tema.Borde))
}

// CreateRenderer: el aro de foco, la caja, el glifo y el rótulo.
func (b *BotonEditor) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// La caja se construye YA con su borde: `ui.Caja` decide al nacer si lleva uno, y
	// una construida con el borde transparente no lo pinta nunca.
	fondo, borde, _ := b.Estilo(v, b)
	b.caja = ui.NuevaCaja(fondo, borde, b.Radio)
	b.texto = ui.Texto(b.Rotulo, b.Escalon, b.Peso, color.NRGBA{})
	b.aro = ui.NuevoAro(b.Radio, ui.SinHueco, v)
	objs := append(b.aro.Objetos(), b.caja.Objetos()...)
	if b.Glifo != nil {
		objs = append(objs, b.Glifo)
	}
	r := &botonEditorRenderer{b: b, objs: append(objs, b.texto)}
	r.Refresh()
	return r
}

type botonEditorRenderer struct {
	b    *BotonEditor
	objs []fyne.CanvasObject
}

func (r *botonEditorRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonEditorRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *botonEditorRenderer) Destroy()                     {}

func (r *botonEditorRenderer) Layout(t fyne.Size) {
	b := r.b
	b.aro.Colocar(t)
	b.caja.Redimensionar(t)
	x := tema.Borde + b.MargenX
	if b.Glifo != nil {
		b.Glifo.Move(fyne.NewPos(x, (t.Height-b.LadoGlifo)/2))
		b.Glifo.Resize(fyne.NewSize(b.LadoGlifo, b.LadoGlifo))
		x += b.LadoGlifo + b.Hueco
	}
	// `truncate`: «Damero (transparente)» no cabe en media columna de 20rem.
	ui.Truncar(b.texto, b.Rotulo, b.Escalon, b.Peso, t.Width-x-tema.Borde-b.MargenX)
	ui.ColocarTexto(b.texto, b.Escalon, x, (t.Height-b.Escalon.Linea)/2)
}

func (r *botonEditorRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := b.Estilo(v, b)
	b.caja.Pintar(fondo, borde)
	b.texto.Color = tinta
	b.texto.Refresh()
	b.aro.Pintar(v)
	b.aro.Ver(b.Enfocado())
	r.Layout(b.Size())
}

// PestanaEditor es una de las dos del panel: `flex-1 border-b-2 py-2.5 text-xs
// font-medium`, la activa con `border-primary text-text font-semibold` y las otras en
// `text-text-muted` con `hover:bg-bg-subtle hover:text-text`.
type PestanaEditor struct {
	ui.Pulsable
	Rotulo string
	Icono  iconos.Icono
	Activa bool

	fondo, raya *canvas.Rectangle
	texto       *canvas.Text
	glifo       *iconos.Dibujo
	aro         *ui.Aro
}

var _ fyne.Widget = (*PestanaEditor)(nil)

// NuevaPestanaEditor crea la pestaña.
func NuevaPestanaEditor(rotulo string, ic iconos.Icono, activa bool, al func()) *PestanaEditor {
	p := &PestanaEditor{Rotulo: rotulo, Icono: ic, Activa: activa}
	p.Nombre, p.AlPulsar = rotulo, al
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityRole: un botón. En la web son `<button>` sueltos, no un `tablist`, y el
// fork no tiene rol de pestaña.
func (p *PestanaEditor) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// ladoIconoPestana es el `size-3.5` del icono de Sytue AI.
const ladoIconoPestana = 3.5 * 0.25 * tema.Rem

func (p *PestanaEditor) peso() ui.Peso {
	if p.Activa {
		return ui.SemiNegrita
	}
	return ui.Medio
}

// CreateRenderer: el fondo del `hover`, el rótulo, el icono, la raya de abajo y el aro.
func (p *PestanaEditor) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo, p.raya = canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(color.NRGBA{})
	p.texto = ui.Texto(p.Rotulo, tema.TextoXS, p.peso(), color.NRGBA{})
	p.aro = ui.NuevoAro(0, ui.SinHueco, v)
	objs := append(p.aro.Objetos(), p.fondo, p.raya)
	if p.Icono != iconos.Ninguno {
		p.glifo = iconos.Trazo(p.Icono, ladoIconoPestana, "text-muted", v)
		objs = append(objs, p.glifo.Objeto())
	}
	r := &pestanaEditorRenderer{p: p, objs: append(objs, p.texto)}
	r.Refresh()
	return r
}

type pestanaEditorRenderer struct {
	p    *PestanaEditor
	objs []fyne.CanvasObject
}

func (r *pestanaEditorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ui.Medir(r.p.Rotulo, tema.TextoXS, r.p.peso()).Width, AltoPestanaEditor)
}
func (r *pestanaEditorRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *pestanaEditorRenderer) Destroy()                     {}

// Layout centra el rótulo (con el icono delante, `gap-1.5`) sobre la raya de 2.
func (r *pestanaEditorRenderer) Layout(t fyne.Size) {
	p := r.p
	p.aro.Colocar(t)
	p.fondo.Resize(t)
	p.raya.Move(fyne.NewPos(0, t.Height-2))
	p.raya.Resize(fyne.NewSize(t.Width, 2))
	ancho := ui.Medir(p.Rotulo, tema.TextoXS, p.peso()).Width
	if p.glifo != nil {
		ancho += ladoIconoPestana + tema.Espacio(1.5)
	}
	x := (t.Width - ancho) / 2
	arriba := tema.Espacio(2.5)
	if p.glifo != nil {
		p.glifo.Colocar(x, arriba+(tema.TextoXS.Linea-ladoIconoPestana)/2, ladoIconoPestana)
		x += ladoIconoPestana + tema.Espacio(1.5)
	}
	ui.ColocarTexto(p.texto, tema.TextoXS, x, arriba)
}

func (r *pestanaEditorRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, fondo, raya := tema.Color("text-muted", v), color.NRGBA{}, color.NRGBA{}
	switch {
	case p.Activa:
		tinta, raya = tema.Color("text", v), tema.Color("primary", v)
	case p.Encima():
		tinta, fondo = tema.Color("text", v), tema.Color("bg-subtle", v)
	}
	p.fondo.FillColor, p.raya.FillColor = fondo, raya
	p.texto.Color = tinta
	if p.glifo != nil {
		p.glifo.TenirCon(tinta)
	}
	p.aro.Pintar(v)
	p.aro.Ver(p.Enfocado())
	r.Layout(p.Size())
	for _, o := range r.objs {
		o.Refresh()
	}
}

// MuestraFondo es uno de los cuatro círculos de la barra flotante: `h-6 w-6 rounded-full
// border`, la elegida con `ring-2 ring-primary ring-offset-1`, un punto dentro y
// `scale-110`, y las demás al 80 % de opacidad y con `hover:scale-105`.
//
// ponytail: el `transition-transform` no se anima; el tamaño cambia de golpe. Se anima
// con `fyne.NewAnimation` sobre EscalaMuestra si algún día se echa en falta.
type MuestraFondo struct {
	ui.Pulsable
	Fondo   FondoLienzo
	Elegida bool

	muestra fyne.CanvasObject
	anillo  *ui.Aro
	punto   *canvas.Circle
	velo    *canvas.Circle
	aro     *ui.Aro
}

var _ fyne.Widget = (*MuestraFondo)(nil)

// LadoMuestraFondo es el `h-6 w-6`.
const LadoMuestraFondo = 6 * 0.25 * tema.Rem

// EscalaMuestra es el `transform: scale()` de la muestra: `scale-110` la elegida y
// `hover:scale-105` las demás al pasar por encima. Escala la caja entera —el círculo,
// el anillo y el punto— alrededor de su centro, y como es un transform, sin mover a
// las vecinas: la caja de la muestra sigue midiendo 21.
func EscalaMuestra(elegida, encima bool) float32 {
	switch {
	case elegida:
		return 1.1
	case encima:
		return 1.05
	}
	return 1
}

// NuevaMuestraFondo crea la muestra, con el nombre del fondo como nombre accesible.
func NuevaMuestraFondo(v Vista, f FondoLienzo, elegida bool, al func()) *MuestraFondo {
	m := &MuestraFondo{Fondo: f, Elegida: elegida}
	m.Nombre, m.AlPulsar = v.Traduce(f.clave()), al
	m.ExtendBaseWidget(m)
	return m
}

// MinSize es el círculo.
func (m *MuestraFondo) MinSize() fyne.Size { return fyne.NewSize(LadoMuestraFondo, LadoMuestraFondo) }

// CreateRenderer: el anillo de la elegida (1 de hueco y 2 de primario), la muestra, el
// velo del 80 %, el punto y el aro de foco.
func (m *MuestraFondo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m.anillo = ui.NuevoAroCon("primary", tema.RadioPill, 1, v)
	m.muestra = muestraFondo(m.Fondo, LadoMuestraFondo, 8).Objeto()
	m.velo = canvas.NewCircle(color.NRGBA{})
	m.punto = canvas.NewCircle(color.NRGBA{})
	m.aro = ui.NuevoAro(tema.RadioPill, ui.SinHueco, v)
	objs := append(append(m.anillo.Objetos(), m.aro.Objetos()...), m.muestra, m.velo, m.punto)
	r := &muestraFondoRenderer{m: m, objs: objs}
	r.Refresh()
	return r
}

type muestraFondoRenderer struct {
	m    *MuestraFondo
	objs []fyne.CanvasObject
}

func (r *muestraFondoRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraFondoRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *muestraFondoRenderer) Destroy()                     {}

func (r *muestraFondoRenderer) Layout(t fyne.Size) {
	m := r.m
	k := EscalaMuestra(m.Elegida, m.Encima())
	c := fyne.NewSize(t.Width*k, t.Height*k)
	o := fyne.NewPos((t.Width-c.Width)/2, (t.Height-c.Height)/2)
	m.anillo.ColocarEn(o, c)
	m.aro.ColocarEn(o, c)
	m.muestra.Move(o)
	m.muestra.Resize(c)
	m.velo.Move(o)
	m.velo.Resize(c)
	lado := 1.5 * 0.25 * tema.Rem * k // `h-1.5 w-1.5`, escalado con la caja
	m.punto.Move(fyne.NewPos((t.Width-lado)/2, (t.Height-lado)/2))
	m.punto.Resize(fyne.NewSize(lado, lado))
}

func (r *muestraFondoRenderer) Refresh() {
	m := r.m
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m.anillo.Pintar(v)
	m.anillo.Ver(m.Elegida)
	m.aro.Pintar(v)
	m.aro.Ver(m.Enfocado() && !m.Elegida)
	// `opacity-80 hover:opacity-100` en las que no están elegidas: un velo del color de
	// la barra al 20 % hace lo mismo sin componer la muestra entera.
	m.velo.FillColor = color.NRGBA{}
	if !m.Elegida && !m.Encima() {
		m.velo.FillColor = conAlfa(tema.Color("surface", v), 0.2)
	}
	m.punto.Hidden = !m.Elegida
	m.punto.FillColor = tema.Color("primary", v)
	if m.Fondo == FondoOscuro {
		m.punto.FillColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	r.Layout(m.Size())
	for _, o := range r.objs {
		o.Refresh()
	}
}
