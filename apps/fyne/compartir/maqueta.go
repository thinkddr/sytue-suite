// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compartir

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

var transparente = color.NRGBA{}

// pila apila los bloques a todo el ancho, cada uno con su separación por encima.
type pila struct{ separas []float32 }

func (p *pila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		ancho, alto = max(ancho, m.Width), alto+p.separas[i]+m.Height
	}
	return fyne.NewSize(ancho, alto)
}

func (p *pila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := float32(0)
	for i, o := range objs {
		y += p.separas[i]
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// filaForm es `flex flex-wrap items-start gap-2`: los grupos de campo (el de ancho 0
// crece, con `min-w-56`) y el botón con `mt-6`, a la altura del campo y no de su etiqueta.
//
// ponytail: sin `flex-wrap`, como la de Drive. A 360 la web baja el botón y el nivel a
// otra línea; aquí el correo encoge hasta su `min-w-56`.
type filaForm struct {
	campos []fyne.CanvasObject
	anchos []float32
	boton  fyne.CanvasObject
}

func (f *filaForm) MinSize([]fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, c := range f.campos {
		w := f.anchos[i]
		if w == 0 {
			w = MinCorreo
		}
		ancho += w + tema.Espacio(2)
		alto = max(alto, c.MinSize().Height)
	}
	m := f.boton.MinSize()
	return fyne.NewSize(ancho+m.Width, max(alto, tema.Espacio(6)+m.Height))
}

func (f *filaForm) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	mb := f.boton.MinSize()
	fijo := mb.Width + float32(len(f.campos))*tema.Espacio(2)
	flexibles := 0
	for _, w := range f.anchos {
		fijo += w
		if w == 0 {
			flexibles++
		}
	}
	libre := float32(0)
	if flexibles > 0 {
		libre = max((t.Width-fijo)/float32(flexibles), MinCorreo)
	}
	x := float32(0)
	for i, c := range f.campos {
		w := f.anchos[i]
		if w == 0 {
			w = libre
		}
		c.Move(fyne.NewPos(x, 0))
		c.Resize(fyne.NewSize(w, c.MinSize().Height))
		x += w + tema.Espacio(2)
	}
	f.boton.Move(fyne.NewPos(x, tema.Espacio(6)))
	f.boton.Resize(mb)
}

// filaPersona coloca una persona. El filete va ARRIBA (`divide-y`).
type filaPersona struct {
	glifo        *iconos.Dibujo
	nombre, meta *canvas.Text
	quitar       *ui.Boton
	linea        *canvas.Rectangle
}

func (f *filaPersona) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(2)+max(tema.TextoBase.Linea+tema.TextoXS.Linea, f.quitar.MinSize().Height))
}

func (f *filaPersona) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.linea.Move(fyne.NewPos(0, 0))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	x := tema.Espacio(3)
	f.glifo.Colocar(x, (t.Height-16)/2, 16)
	x += 16 + tema.Espacio(3)
	m := f.quitar.MinSize()
	derecha := t.Width - tema.Espacio(3) - m.Width
	f.quitar.Move(fyne.NewPos(derecha, (t.Height-m.Height)/2))
	f.quitar.Resize(m)
	cabe := max(derecha-tema.Espacio(3)-x, 0)
	y := (t.Height - tema.TextoBase.Linea - tema.TextoXS.Linea) / 2
	ui.Truncar(f.nombre, f.nombre.Text, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	ui.Truncar(f.meta, f.meta.Text, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.meta, tema.TextoXS, x, y+tema.TextoBase.Linea)
}

// enLinea envuelve un texto en su caja de LÍNEA, que es lo que ocupa en la web.
func enLinea(t *canvas.Text, e tema.Escalon) fyne.CanvasObject {
	return container.New(&linea{t: t, e: e}, t)
}

type linea struct {
	t *canvas.Text
	e tema.Escalon
}

func (l *linea) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(l.t.Text, l.e, ui.Normal).Width, l.e.Linea)
}

func (l *linea) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.Truncar(l.t, l.t.Text, l.e, ui.Normal, t.Width)
	ui.ColocarTexto(l.t, l.e, 0, 0)
}

// enVersal es el `sytue-overline` del encabezado, con su caja de línea de `text-xs`.
func enVersal(v *ui.Versal) fyne.CanvasObject { return container.New(&versal{v: v}, v.Objetos()...) }

type versal struct{ v *ui.Versal }

func (e *versal) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(e.v.Ancho(), tema.TextoXS.Linea)
}

func (e *versal) Layout([]fyne.CanvasObject, fyne.Size) { e.v.Colocar(0, 0) }

// margen pone los primeros objetos (la caja) a tamaño completo y el último dentro, con
// ese relleno.
type margen struct{ x, y float32 }

func (c *margen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*c.x, m.Height+2*c.y)
}

func (c *margen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(c.x, c.y))
			o.Resize(fyne.NewSize(max(t.Width-2*c.x, 0), max(t.Height-2*c.y, 0)))
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// altoFijo es un contenedor de un alto dado (`h-16`) con todo a tamaño completo.
type altoFijo struct{ alto float32 }

func (a *altoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, a.alto) }

func (a *altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}
