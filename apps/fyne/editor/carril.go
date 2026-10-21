// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// itemsCarril son las cinco filas del carril, en el orden medido en producción
// (y = 211, 242,5, 274, 305,5, 337).
var itemsCarril = []carrilItem{
	{PanelBuscar, iconos.EdBuscar, "rail.search"},
	{PanelEsquema, iconos.EdEsquema, "rail.outline"},
	{PanelComentarios, iconos.EdComentarios, "rail.comments"},
	{PanelRevision, iconos.EdRevision, "rail.review"},
	{PanelIA, iconos.EdIA, "rail.ai"},
}

// Carril es la franja izquierda de 40 px con los cinco botones de panel.
//
// Medido: los botones son de 28×28 y caen en x=5,5, que es (39−28)/2 — el carril mide
// 40 con su `border-r` dentro, así que su contenido son 39. `py-2` arriba y `gap-1`
// entre botones.
type Carril struct {
	widget.BaseWidget

	ctx     *Contexto
	botones []*BotonCinta
}

var _ fyne.Widget = (*Carril)(nil)

// NuevoCarril monta la franja.
func NuevoCarril(ctx *Contexto) *Carril {
	c := &Carril{ctx: ctx}
	c.ExtendBaseWidget(c)
	for _, it := range itemsCarril {
		b := NuevoBotonCinta(it.icono, ctx.Traducir(it.rotulo), func() { ctx.AbrirPanel(it.p) })
		b.Activo = ctx.Panel == it.p
		c.botones = append(c.botones, b)
	}
	return c
}

// AccessibilityRole lo anuncia como contenedor con nombre.
func (c *Carril) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el nombre del carril.
func (c *Carril) AccessibilityLabel() string { return c.ctx.Traducir("rail.label") }

// MinSize es el ancho declarado; el alto lo pone el marco.
func (c *Carril) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(AnchoCarril, 0)
}

// CreateRenderer compone el fondo, la línea y los botones.
func (c *Carril) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &carrilRenderer{
		c:     c,
		fondo: canvas.NewRectangle(tema.Color("surface", v)),
		linea: canvas.NewRectangle(tema.Color("border", v)),
	}
}

type carrilRenderer struct {
	c     *Carril
	fondo *canvas.Rectangle
	linea *canvas.Rectangle
}

func (r *carrilRenderer) MinSize() fyne.Size { return r.c.MinSize() }

func (r *carrilRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, 2+len(r.c.botones))
	objs = append(objs, r.fondo, r.linea)
	for _, b := range r.c.botones {
		objs = append(objs, b)
	}
	return objs
}

func (r *carrilRenderer) Destroy() {}

func (r *carrilRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.linea.Move(fyne.NewPos(t.Width-tema.Borde, 0))
	r.linea.Resize(fyne.NewSize(tema.Borde, t.Height))

	contenido := t.Width - tema.Borde
	y := RellenoCarril
	for _, b := range r.c.botones {
		b.Move(fyne.NewPos((contenido-LadoBotonCarril)/2, y))
		b.Resize(fyne.NewSize(LadoBotonCarril, LadoBotonCarril))
		y += LadoBotonCarril + HuecoCarril
	}
}

func (r *carrilRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.fondo.Refresh()
	r.linea.Refresh()
	for i, b := range r.c.botones {
		b.Activo = r.c.ctx.Panel == itemsCarril[i].p
		b.Refresh()
	}
}

// ---- el panel desplegable ----

// PanelLateral es el panel de 288 px que el carril despliega.
//
// Los cuerpos que esta ola trae son BUSCAR, ESQUEMA y COMENTARIOS, que son los tres
// del blueprint original (§6.4); revisión e IA enseñan su estado vacío con el motivo.
// La web tiene ocho paneles: los cinco restantes están en el informe.
type PanelLateral struct {
	widget.BaseWidget

	ctx    *Contexto
	titulo string
	cuerpo fyne.CanvasObject
	cerrar *ui.BotonIcono
	// Lado es "izq" o "der": el de ajustes lleva la línea al otro lado.
	derecha bool
}

var _ fyne.Widget = (*PanelLateral)(nil)

// NuevoPanelLateral monta el panel con su título YA traducido y su cuerpo.
func NuevoPanelLateral(ctx *Contexto, titulo string, cuerpo fyne.CanvasObject, derecha bool) *PanelLateral {
	p := &PanelLateral{ctx: ctx, titulo: titulo, cuerpo: cuerpo, derecha: derecha}
	p.ExtendBaseWidget(p)
	// CLOSE_BTN de LeftRail.tsx: `h-7 w-7 rounded-sm text-header-fg hover:bg-chrome-hover
	// active:bg-chrome-active`, el aspa `md` (16) y el anillo en `header-fg`.
	p.cerrar = ui.NuevoBotonIcono(iconos.Cerrar, ctx.Traducir("rail.close"), func() {
		if derecha {
			ctx.Ajustes = false
		} else {
			ctx.Panel = SinPanel
		}
		ctx.refrescar()
	}).SobreBanda()
	p.cerrar.Radio, p.cerrar.TamIcono, p.cerrar.TokenAro = tema.RadioSM, IconoCerrarPanel, "header-fg"
	return p
}

// IconoCerrarPanel es el `size="md"` de EditorIcon: 16.
const IconoCerrarPanel float32 = 16

// DerechaCabeceraPanel es el `pr-1.5` de la cabecera del panel.
var DerechaCabeceraPanel = tema.Espacio(1.5)

// AccessibilityRole es `role="complementary"`; Fyne no lo tiene, así que se anuncia
// como contenedor con su nombre, que es lo que un lector necesita para saltar a él.
func (p *PanelLateral) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// AccessibilityLabel es el título del panel.
func (p *PanelLateral) AccessibilityLabel() string { return p.titulo }

// MinSize es el ancho declarado.
func (p *PanelLateral) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(AnchoPanel, 0)
}

// CreateRenderer compone el fondo, la línea, la cabecera y el cuerpo.
func (p *PanelLateral) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &panelRenderer{
		p:      p,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		linea:  canvas.NewRectangle(tema.Color("border", v)),
		banda:  canvas.NewRectangle(tema.Color("header-bg", v)),
		titulo: ui.Texto(p.titulo, tema.TextoSM, ui.SemiNegrita, tema.Color("header-fg", v)),
	}
}

// panelRenderer pinta el `<aside>`: la cabecera es una BANDA de cromo (`bg-header
// text-header-fg h-10`), como la de un diálogo, y no una fila más sobre `surface`.
type panelRenderer struct {
	p      *PanelLateral
	fondo  *canvas.Rectangle
	linea  *canvas.Rectangle
	banda  *canvas.Rectangle
	titulo *canvas.Text
}

func (r *panelRenderer) MinSize() fyne.Size { return r.p.MinSize() }

func (r *panelRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo, r.linea, r.banda, r.titulo, r.p.cerrar}
	if r.p.cuerpo != nil {
		objs = append(objs, r.p.cuerpo)
	}
	return objs
}

func (r *panelRenderer) Destroy() {}

func (r *panelRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	x := t.Width - tema.Borde
	if r.p.derecha {
		x = 0
	}
	r.linea.Move(fyne.NewPos(x, 0))
	r.linea.Resize(fyne.NewSize(tema.Borde, t.Height))
	// La caja del contenido es la del `<aside>` sin su borde (a la derecha en el del
	// carril, a la izquierda en el de ajustes).
	izq, ancho := float32(0), max(t.Width-tema.Borde, 0)
	if r.p.derecha {
		izq = tema.Borde
	}
	r.banda.Move(fyne.NewPos(izq, 0))
	r.banda.Resize(fyne.NewSize(ancho, AltoCabeceraPanel))

	ui.ColocarTexto(r.titulo, tema.TextoSM, izq+MargenPanel,
		(AltoCabeceraPanel-tema.TextoSM.Linea)/2)
	m := r.p.cerrar.MinSize()
	r.p.cerrar.Move(fyne.NewPos(izq+ancho-DerechaCabeceraPanel-m.Width, (AltoCabeceraPanel-m.Height)/2))
	r.p.cerrar.Resize(m)

	// El cuerpo ocupa el panel entero: cada cuerpo lleva su propio relleno, que no es
	// el mismo en todos (el esquema es `px-1 py-3`, buscar `p-3`).
	if r.p.cuerpo != nil {
		r.p.cuerpo.Move(fyne.NewPos(izq, AltoCabeceraPanel))
		r.p.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-AltoCabeceraPanel, 0)))
	}
}

func (r *panelRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.banda.FillColor = tema.Color("header-bg", v)
	r.titulo.Text = r.p.titulo
	r.titulo.Color = tema.Color("header-fg", v)
	for _, o := range []fyne.CanvasObject{r.fondo, r.linea, r.banda, r.titulo} {
		o.Refresh()
	}
}
