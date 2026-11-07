// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # La hoja de atajos de teclado (`ShortcutsModal.tsx`)
//
// Existe porque los atajos que nadie enseña no los usa nadie. El contenido lo declara
// cada app —son sus teclas y sus palabras—; aquí vive la forma: el diálogo `md`, los
// grupos y las teclas dibujadas como teclas. Nació en Sytue Mail y se subió aquí cuando
// Calendar necesitó la suya: la misma hoja no se copia.

// AtajoDeTeclado es una fila de la hoja: las teclas que hacen lo mismo («n» y «j»),
// ya en el idioma de quien lee («Ctrl+↵»), y lo que hacen.
type AtajoDeTeclado struct {
	Teclas      []string
	Descripcion string
}

// GrupoDeAtajos es un apartado de la hoja, con su rótulo.
type GrupoDeAtajos struct {
	Titulo string
	Atajos []AtajoDeTeclado
}

// MostrarHojaAtajos abre la hoja sobre el lienzo: un diálogo `md` con los grupos y el
// botón `cerrar` («Cerrar», ya traducido) en el pie. Devuelve el diálogo.
func MostrarHojaAtajos(lienzo fyne.Canvas, titulo, cerrar string, grupos []GrupoDeAtajos) *Dialogo {
	ancho := AnchoCuerpoTalla(DialogoMD, fyne.NewSize(lienzo.Size().Width, 0))
	secciones := make([]fyne.CanvasObject, 0, len(grupos))
	for _, g := range grupos {
		secciones = append(secciones, nuevaSeccionAtajos(g, ancho))
	}
	var d *Dialogo
	boton := NuevoBoton(cerrar, func() { d.Cerrar() })
	d = NuevoDialogo(titulo, container.New(Columna{Hueco: huecoGruposAtajos}, secciones...)).Con(DialogoMD, boton)
	d.RotuloCerrar = cerrar
	d.Mostrar(lienzo)
	return d
}

// Las medidas de la hoja: grupos con `gap-5`, el rótulo en `sytue-overline mb-2`, filas
// `flex items-center gap-4 py-1.5` separadas por `divide-y` a `border/60`, y cada tecla
// `h-6 min-w-6 px-1.5 rounded border bg-bg-subtle text-xs` con `gap-1` entre ellas.
const (
	huecoGruposAtajos float32 = 5 * 0.25 * tema.Rem
	bajoRotuloAtajos  float32 = 2 * 0.25 * tema.Rem
	rellenoFilaAtajo  float32 = 1.5 * 0.25 * tema.Rem
	huecoFilaAtajo    float32 = 4 * 0.25 * tema.Rem
	ladoTecla         float32 = 6 * 0.25 * tema.Rem
	huecoTeclas       float32 = 1 * 0.25 * tema.Rem
)

// AltoFilaAtajo es una fila: su `py-1.5` y la tecla de 21 (que manda sobre el texto).
var AltoFilaAtajo = 2*rellenoFilaAtajo + ladoTecla

// seccionAtajos es un grupo de la hoja.
type seccionAtajos struct {
	widget.BaseWidget
	g     GrupoDeAtajos
	ancho float32
}

func nuevaSeccionAtajos(g GrupoDeAtajos, ancho float32) *seccionAtajos {
	s := &seccionAtajos{g: g, ancho: ancho}
	s.ExtendBaseWidget(s)
	return s
}

func (s *seccionAtajos) MinSize() fyne.Size {
	n := float32(len(s.g.Atajos))
	return fyne.NewSize(s.ancho, tema.TextoXS.Linea+bajoRotuloAtajos+n*AltoFilaAtajo+max(n-1, 0)*tema.Borde)
}

func (s *seccionAtajos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &seccionAtajosRenderer{s: s, rotulo: NuevoVersal(s.g.Titulo, tema.TextoXS, SemiNegrita, tema.Color("text-subtle", v))}
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	for i, a := range s.g.Atajos {
		if i > 0 {
			f := canvas.NewRectangle(veloAtajos(tema.Color("border", v)))
			r.filetes = append(r.filetes, f)
			r.objetos = append(r.objetos, f)
		}
		fila := make([]*Tecla, 0, len(a.Teclas))
		for _, k := range a.Teclas {
			tecla := NuevaTecla(k, tema.TextoXS, Normal)
			tecla.Fondo, tecla.Tinta, tecla.Borde = "bg-subtle", "text-muted", "border"
			tecla.RellenoX, tecla.Radio = tema.Espacio(1.5), tema.RadioSM
			tecla.Minimo = fyne.NewSize(ladoTecla, ladoTecla)
			fila = append(fila, tecla)
			r.objetos = append(r.objetos, tecla)
		}
		texto := Texto(a.Descripcion, tema.TextoSM, Normal, tema.Color("text", v))
		r.teclas, r.textos = append(r.teclas, fila), append(r.textos, texto)
		r.objetos = append(r.objetos, texto)
	}
	r.Refresh()
	return r
}

type seccionAtajosRenderer struct {
	s       *seccionAtajos
	rotulo  *Versal
	filetes []*canvas.Rectangle
	teclas  [][]*Tecla // las de cada fila
	textos  []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *seccionAtajosRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *seccionAtajosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *seccionAtajosRenderer) Destroy()                     {}
func (r *seccionAtajosRenderer) Layout(t fyne.Size) {
	r.rotulo.Colocar(0, 0)
	y := tema.TextoXS.Linea + bajoRotuloAtajos
	for i := range r.teclas {
		if i > 0 {
			r.filetes[i-1].Move(fyne.NewPos(0, y))
			r.filetes[i-1].Resize(fyne.NewSize(t.Width, tema.Borde))
			y += tema.Borde
		}
		x := float32(0)
		for j, k := range r.teclas[i] {
			if j > 0 {
				x += huecoTeclas
			}
			m := k.MinSize()
			k.Move(fyne.NewPos(x, y+rellenoFilaAtajo))
			k.Resize(m)
			x += m.Width
		}
		ColocarTexto(r.textos[i], tema.TextoSM, x+huecoFilaAtajo, y+(AltoFilaAtajo-tema.TextoSM.Linea)/2)
		y += AltoFilaAtajo
	}
}

func (r *seccionAtajosRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Pintar(tema.Color("text-subtle", v))
	for _, f := range r.filetes {
		f.FillColor = veloAtajos(tema.Color("border", v))
	}
	for _, x := range r.textos {
		x.Color = tema.Color("text", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// veloAtajos es el `border/60` de los filetes: el color con su opacidad al 60 %.
func veloAtajos(c color.NRGBA) color.NRGBA {
	c.A = uint8(float32(c.A) * 0.6)
	return c
}
