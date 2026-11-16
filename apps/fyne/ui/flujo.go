// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
)

// Flujo es `flex flex-wrap` con su `gap`: los hijos visibles por orden, de izquierda a
// derecha, y a la línea siguiente el que no cabe. Nació en Mail para la barra de la
// cabecera del lector, que en la web parte en dos o tres filas cuando el panel se
// estrecha —y la cuenta del alto de la cabecera tiene que saber cuántas—, y subió aquí
// cuando la ficha de Contactos lo pidió para sus etiquetas.
//
// Con `UltimoADerecha`, el último hijo se empuja al borde derecho de SU línea, que es
// el `ms-auto` del grupo de triaje: en ancho está al final de la misma fila, y si no
// cabe baja a la siguiente y se queda a la derecha.
//
// Un hijo que sea a su vez un Flujo más ancho que la línea entera no se sale: se queda
// con el ancho de la línea y se parte por dentro, que es lo que hace el navegador con
// un elemento `flex-wrap` que encoge (`min-width: auto` es su hijo más ancho).
//
// Con `PrimeroEntero`, el primer hijo visible es `w-full`: ocupa su línea entera (el
// rótulo de una sección de la vista rápida de Calendar). Con `Repartir`, la línea es
// `justify-between`: lo que sobra de cada línea con más de un hijo va entre ellos.
//
// Calendar (`envolver`) y Bases (`flujo`) tenían cada una su propio reparto por filas;
// las dos usan ya este, que es el único `flex-wrap` de las nativas.
type Flujo struct {
	Hueco          float32
	UltimoADerecha bool
	PrimeroEntero  bool
	Repartir       bool
	// Ancho, si se sabe, hace que el mínimo sea el del flujo YA PARTIDO a ese ancho, que
	// es lo que necesita un bloque `min-w-0` dentro de una columna que suma mínimos.
	Ancho float32
}

var _ fyne.Layout = Flujo{}

// Colocar reparte los hijos en `ancho`. Devuelve la caja de cada uno (nil los
// ocultos) y el alto total. Es la ÚNICA cuenta: la usan MinSize, Layout y quien
// necesita medir el alto a un ancho (AltoFlujo).
func (f Flujo) Colocar(objs []fyne.CanvasObject, ancho float32) (cajas []fyne.Position, tams []fyne.Size, alto float32) {
	cajas = make([]fyne.Position, len(objs))
	tams = make([]fyne.Size, len(objs))
	ultimo := -1
	for i, o := range objs {
		if o.Visible() {
			ultimo = i
		}
	}
	x, y, linea := float32(0), float32(0), float32(0)
	primero := true
	var enLinea []int // índices de la línea en curso, para `Repartir`
	repartir := func() {
		if !f.Repartir || len(enLinea) < 2 || ancho <= 0 {
			return
		}
		u := enLinea[len(enLinea)-1]
		extra := max(ancho-(cajas[u].X+tams[u].Width), 0) / float32(len(enLinea)-1)
		for k, j := range enLinea {
			cajas[j].X += extra * float32(k)
		}
	}
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		t := f.medida(o, ancho, primero)
		primero = false
		if x > 0 && x+t.Width > ancho {
			repartir()
			x, y, linea, enLinea = 0, y+linea+f.Hueco, 0, enLinea[:0]
		}
		cajas[i], tams[i] = fyne.NewPos(x, y), t
		enLinea = append(enLinea, i)
		x += t.Width + f.Hueco
		linea = max(linea, t.Height)
	}
	repartir()
	if ultimo >= 0 && f.UltimoADerecha && ancho > 0 {
		// `ms-auto`: se lleva el sobrante de su línea.
		cajas[ultimo].X = max(ancho-tams[ultimo].Width, cajas[ultimo].X)
	}
	if ultimo < 0 {
		return cajas, tams, 0
	}
	return cajas, tams, y + linea
}

// medida es lo que ocupa un hijo en su línea: su mínimo, la línea entera si es el
// primero con `PrimeroEntero`, y si es un Flujo más ancho que la línea, ese Flujo
// partido al ancho de la línea.
func (f Flujo) medida(o fyne.CanvasObject, ancho float32, primero bool) fyne.Size {
	t := o.MinSize()
	if ancho <= 0 {
		return t
	}
	if primero && f.PrimeroEntero {
		t.Width = ancho
	}
	if c, es := o.(*fyne.Container); es {
		if g, es := c.Layout.(Flujo); es && t.Width > ancho {
			_, _, h := g.Colocar(c.Objects, ancho)
			t = fyne.NewSize(ancho, h)
		}
	}
	return t
}

// MinSize es la fila entera sin partir: lo que pide el flujo cuando cabe todo. Con
// `ancho`, lo que ocupa partido a ese ancho.
func (f Flujo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if f.Ancho > 0 {
		_, _, h := f.Colocar(objs, f.Ancho)
		return fyne.NewSize(f.Ancho, h)
	}
	ancho, alto, n := float32(0), float32(0), 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		t := o.MinSize()
		if n > 0 {
			ancho += f.Hueco
		}
		ancho += t.Width
		alto = max(alto, t.Height)
		n++
	}
	return fyne.NewSize(ancho, alto)
}

// Layout coloca a los hijos con el reparto de Colocar. En cada línea van centrados en
// vertical sobre su propio alto (`items-center`).
func (f Flujo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	cajas, tams, _ := f.Colocar(objs, t.Width)
	linea := map[float32]float32{} // y de la línea → su alto
	for i, o := range objs {
		if o.Visible() {
			linea[cajas[i].Y] = max(linea[cajas[i].Y], tams[i].Height)
		}
	}
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		o.Move(cajas[i].AddXY(0, (linea[cajas[i].Y]-tams[i].Height)/2))
		o.Resize(tams[i])
	}
}

// AltoFlujo es el alto de un contenedor con Flujo a ese ancho; si no lleva Flujo, su
// alto mínimo.
func AltoFlujo(c *fyne.Container, ancho float32) float32 {
	f, es := c.Layout.(Flujo)
	if !es {
		return c.MinSize().Height
	}
	_, _, h := f.Colocar(c.Objects, ancho)
	return h
}
