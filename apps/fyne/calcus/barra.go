// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Barra es la barra de fórmulas de SheetCanvas.tsx: el rótulo de la celda activa, la
// «fx» y el campo con el texto crudo de esa celda.
//
// Refleja la celda ancla de la rejilla y escribe en ella: Intro guarda y baja una
// fila, que es lo que hace la web.
type Barra struct {
	widget.BaseWidget

	R        *Rejilla
	ed       *ui.EditorPlano
	enfocado bool
	// AlEnfocar avisa de que el campo gana o pierde el foco: es cuando empieza y acaba
	// la edición de una fórmula en la barra.
	AlEnfocar func(bool)
	// campo es dónde está el campo, para colgar debajo la lista del autocompletado.
	campo fyne.Position
	rend  *barraRenderer
}

var _ fyne.Widget = (*Barra)(nil)

// NuevaBarra la ata a su rejilla.
func NuevaBarra(r *Rejilla) *Barra {
	b := &Barra{R: r}
	b.ExtendBaseWidget(b)
	// Desplazable: una fórmula más larga que el campo corre como en el `<input>` de la
	// web, con el cursor siempre a la vista.
	b.ed = ui.NuevoEditorPlanoDesplazable(EscalonFx, func(f bool) {
		b.enfocado = f
		if b.AlEnfocar != nil {
			b.AlEnfocar(f)
		}
		b.Refresh()
	})
	b.ed.Tecla = b.tecla
	// La rueda corre el texto: el tinte va con él. Lo que se corre mientras la barra se
	// coloca o se repinta lo tiñe ella misma, y repintar desde ahí no acabaría nunca.
	// Solo el tinte: repintar el editor lo devolvería al cursor y la rueda no correría.
	b.ed.AlDesplazar(func() {
		if r := b.rend; r != nil && !r.colocando && r.anchoCampo > 0 && b.ed.Desplazamiento() != r.tinteX {
			r.tintar(b.campo, r.anchoCampo)
			r.recorte.Refresh()
		}
	})
	b.Sincronizar()
	return b
}

// Sincronizar trae a la barra lo que hay en la celda ancla. La llama la rejilla cada
// vez que la selección se mueve o el contenido cambia.
func (b *Barra) Sincronizar() {
	s := b.R.Seleccion()
	texto := b.R.L.Crudo(s.C, s.F)
	if b.R.L.FormulaOculta(s.C, s.F) {
		texto = "" // la fórmula oculta de una hoja protegida no se enseña
	}
	b.ed.PonerTexto(texto)
	// La celda que la protección no deja editar deja la barra cerrada: es el
	// `readOnly={!canEditSel}` del campo de la web.
	b.ed.SoloLectura(b.R.SoloLectura || !b.R.L.Editable(s.C, s.F))
	b.Refresh()
}

// campoX es dónde empieza el campo de la fórmula y dónde acaba por abajo, en las
// coordenadas del lienzo (la barra va arriba del todo).
func (b *Barra) campoX() (x, abajo float32) {
	return b.campo.X, b.campo.Y + AltoEntradaFx
}

// Rotulo es el `refLabel`: la celda ancla, o el rango con sus dos esquinas.
func (b *Barra) Rotulo() string {
	c1, f1, c2, f2 := b.R.Rango()
	if c1 == c2 && f1 == f2 {
		return Ref(c1, f1)
	}
	return Ref(c1, f1) + ":" + Ref(c2, f2)
}

// tecla: Intro escribe y baja; Escape devuelve el campo a lo que hay en la celda.
func (b *Barra) tecla(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		// Lo que una validación estricta rechaza se queda en la barra, sin bajar: es el
		// `return` de `onFxKeyDown` tras el aviso.
		s := b.R.Seleccion()
		if !b.R.Escribir(b.ed.Texto()) && !b.R.SoloLectura {
			return true
		}
		b.R.Seleccionar(s.C, s.F+1, false)
		b.R.recuperarFoco() // `gridRef.current?.focus()`: la fórmula se da por acabada
		return true
	case fyne.KeyEscape:
		b.Sincronizar()
		return true
	default:
		return false
	}
}

// MedidaBarra es lo que ocupa la barra: el `h-8` de la web, y de ancho el mínimo en el
// que caben sus tres piezas. El ancho real lo pone quien la coloca (`flex-1`).
func MedidaBarra() fyne.Size {
	return fyne.NewSize(2*MargenBarra+AnchoRef+2*HuecoBarra+2*tema.Rem, AltoBarra)
}

// MinSize es la medida declarada.
func (b *Barra) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return MedidaBarra()
}

// CreateRenderer compone fondo, rótulo, «fx» y campo.
func (b *Barra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &barraRenderer{
		b:       b,
		fondo:   canvas.NewRectangle(tema.Color("surface", v)),
		linea:   canvas.NewRectangle(tema.Color("border", v)),
		rotulo:  ui.Texto("", tema.TextoXS, ui.Medio, tema.Color("text", v)),
		refCaja: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		// `.sytue-meta italic`: `text-xs` en `text-subtle`, en cursiva.
		fx:  ui.Texto("fx", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		// El fondo del campo. En la web el `<input>` es `bg-transparent` sobre la
		// barra, que es `bg-surface`, así que pintar la superficie aquí da el MISMO
		// píxel… y hace falta por otra razón: `ui.Aro` es un rectángulo RELLENO del
		// color del anillo, y cuenta con que el control tenga encima una caja opaca
		// que solo le deje ver el reborde. Un campo transparente no la tiene, y el
		// aro salía como un bloque azul que se comía la fórmula entera. Se vio
		// mirando el PNG; ningún test de medida lo habría cazado.
		campo: canvas.NewRectangle(tema.Color("surface", v)),
	}
	r.refCaja.CornerRadius = tema.RadioSM
	r.refCaja.StrokeWidth = tema.Borde
	r.refCaja.StrokeColor = tema.Color("border", v)
	r.fx.FontSource = tema.Cursiva
	r.campo.CornerRadius = tema.RadioSM // `rounded-sm`
	r.recorte = nuevoRecorte(nil)
	r.recomponer()
	b.rend = r
	return r
}

type barraRenderer struct {
	b              *Barra
	fondo          *canvas.Rectangle
	linea          *canvas.Rectangle
	rotulo, fx     *canvas.Text
	campo, refCaja *canvas.Rectangle
	aro            *ui.Aro
	tinte          tinte
	recorte        *recorte // el `overflow-hidden` del fondo tintado
	objetos        []fyne.CanvasObject
	ultimoAncho    float32
	// colocando es que el Layout o el Refresh están en marcha, y tinteX el
	// desplazamiento con el que se tiñó por última vez.
	colocando  bool
	tinteX     float32
	anchoCampo float32
}

// recomponer ordena los objetos: el tinte de la fórmula va entre el fondo del campo y el
// editor, que lo deja ver con su texto transparente.
func (r *barraRenderer) recomponer() {
	o := append([]fyne.CanvasObject{r.fondo, r.linea, r.refCaja, r.rotulo, r.fx}, r.aro.Objetos()...)
	r.recorte.objs = r.tinte.objetos()
	o = append(o, r.campo, r.recorte, r.b.ed.Objeto())
	r.objetos = o
}

func (r *barraRenderer) MinSize() fyne.Size           { return MedidaBarra() }
func (r *barraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraRenderer) Destroy()                     {}

func (r *barraRenderer) Layout(t fyne.Size) {
	defer r.ocupar()()
	r.ultimoAncho = t.Width
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	// `border-b`, que con `box-sizing: border-box` va DENTRO del `h-8`.
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	// `items-center` sobre el alto del contenido, que es el `h-8` menos su borde.
	contenido := t.Height - tema.Borde
	x := MargenBarra
	// El rótulo va centrado en su `w-16`.
	r.refCaja.Move(fyne.NewPos(x, (contenido-AltoRotulo)/2))
	r.refCaja.Resize(fyne.NewSize(AnchoRef, AltoRotulo))
	ancho := ui.Medir(r.rotulo.Text, tema.TextoXS, ui.Medio).Width
	ui.ColocarTexto(r.rotulo, tema.TextoXS, x+(AnchoRef-ancho)/2, (contenido-tema.TextoXS.Linea)/2)
	x += AnchoRef + HuecoBarra

	anchoFx := ui.Medir("fx", tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.fx, tema.TextoXS, x, (contenido-tema.TextoXS.Linea)/2)
	x += anchoFx + HuecoBarra

	// El campo: `h-5` centrado, y dentro su `px-1`. El editor ocupa solo la caja de
	// línea de su FUENTE, centrada en el `h-5`, que es lo que hace el navegador.
	caja := fyne.NewPos(x, (contenido-AltoEntradaFx)/2)
	anchoCampo := max(t.Width-MargenBarra-x, 0)
	r.campo.Move(caja)
	r.campo.Resize(fyne.NewSize(anchoCampo, AltoEntradaFx))
	r.aro.ColocarEn(caja, fyne.NewSize(anchoCampo, AltoEntradaFx))
	alto := r.b.ed.AltoFuente()
	r.b.ed.Colocar(fyne.NewPos(caja.X+MargenCelda, caja.Y+(AltoEntradaFx-alto)/2),
		fyne.NewSize(max(anchoCampo-2*MargenCelda, 0), alto))
	r.b.campo, r.anchoCampo = caja, anchoCampo
	r.tintar(caja, anchoCampo)
}

// tintar pinta la fórmula de la barra con sus referencias de colores (`fxIsFormula`):
// siempre que el texto empiece por «=», se esté editando o no. El texto del campo se
// vuelve transparente y el cursor conserva la tinta del tema.
//
// Los trozos van dentro de un recorte del tamaño del editor y corridos lo que él haya
// corrido su texto: es el `scrollLeft` que la web copia del `<input>` a su fondo. Así una
// fórmula más larga que el campo también se ve tintada, y la rueda sobre el campo, que
// corre el texto sin mover el cursor, lo repinta en el momento (`AlDesplazar`).
func (r *barraRenderer) tintar(caja fyne.Position, ancho float32) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto := r.b.ed.Texto()
	esc := tema.Escalon{Tam: EscalonFx.Tam, Linea: AltoEntradaFx}
	r.recorte.Move(fyne.NewPos(caja.X+MargenCelda, caja.Y))
	r.recorte.Resize(fyne.NewSize(max(ancho-2*MargenCelda, 0), AltoEntradaFx))
	pintado, crecio := false, false
	if strings.HasPrefix(texto, "=") {
		pintado, crecio = r.tinte.pintar(texto, formula.ExtractRefsCon(texto, r.b.R.L.ResolverParaResaltar(r.b.R.sel)),
			func(i int) color.NRGBA { return ColorRefFx(i, v) }, tema.Color("text", v),
			esc, -r.tinteDesde(), 0, math.MaxFloat32)
	} else {
		r.tinte.pintar("", nil, nil, color.NRGBA{}, esc, 0, 0, 0)
	}
	if pintado {
		r.b.ed.Tinta(color.NRGBA{}, tema.Color("text", v))
	} else {
		r.b.ed.Tinta(nil, nil)
	}
	if crecio {
		r.recomponer()
	}
}

// tinteDesde es el desplazamiento del editor, que queda apuntado para AlDesplazar.
func (r *barraRenderer) tinteDesde() float32 {
	r.tinteX = r.b.ed.Desplazamiento()
	return r.tinteX
}

// ocupar marca la barra ocupada y devuelve cómo soltarla; anidado, suelta el de fuera.
func (r *barraRenderer) ocupar() func() {
	antes := r.colocando
	r.colocando = true
	return func() { r.colocando = antes }
}

func (r *barraRenderer) Refresh() {
	defer r.ocupar()()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.rotulo.Text = r.b.Rotulo()
	r.rotulo.Color = tema.Color("text", v)
	r.refCaja.FillColor = tema.Color("bg-subtle", v)
	r.refCaja.StrokeColor = tema.Color("border", v)
	r.fx.Color = tema.Color("text-subtle", v)
	r.campo.FillColor = tema.Color("surface", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.enfocado)
	if r.ultimoAncho > 0 {
		r.Layout(fyne.NewSize(r.ultimoAncho, AltoBarra))
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
