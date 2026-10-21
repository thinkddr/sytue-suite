// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fyne.io/fyne/v2"
)

// flujo es `flex flex-wrap` con su `gap`: los hijos visibles por orden, de izquierda a
// derecha, y a la línea siguiente el que no cabe. Es la barra de la cabecera del
// lector, que en la web parte en dos o tres filas cuando el panel se estrecha —y la
// cuenta del alto de la cabecera tiene que saber cuántas—.
//
// Con `ultimoADerecha`, el último hijo se empuja al borde derecho de SU línea, que es
// el `ms-auto` del grupo de triaje: en ancho está al final de la misma fila, y si no
// cabe baja a la siguiente y se queda a la derecha.
//
// Un hijo que sea a su vez un flujo más ancho que la línea entera no se sale: se queda
// con el ancho de la línea y se parte por dentro, que es lo que hace el navegador con
// un elemento `flex-wrap` que encoge (`min-width: auto` es su hijo más ancho).
//
// ponytail: no hay `ui.Flujo` porque ninguna otra pantalla lo ha pedido todavía
// (`ui.CabeceraApp` lo dejó escrito como hueco). Si una segunda lo necesita, sube a
// `apps/fyne/ui` tal cual.
type flujo struct {
	hueco          float32
	ultimoADerecha bool
	// ancho, si se sabe, hace que el mínimo sea el del flujo YA PARTIDO a ese ancho, que
	// es lo que necesita un bloque `min-w-0` dentro de una columna que suma mínimos.
	ancho float32
}

// colocar reparte los hijos en `ancho`. Devuelve la caja de cada uno (nil los
// ocultos) y el alto total. Es la ÚNICA cuenta: la usan MinSize, Layout y la medida
// de la cabecera.
func (f flujo) colocar(objs []fyne.CanvasObject, ancho float32) (cajas []fyne.Position, tams []fyne.Size, alto float32) {
	cajas = make([]fyne.Position, len(objs))
	tams = make([]fyne.Size, len(objs))
	ultimo := -1
	for i, o := range objs {
		if o.Visible() {
			ultimo = i
		}
	}
	x, y, linea := float32(0), float32(0), float32(0)
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		t := o.MinSize()
		if c, es := o.(*fyne.Container); es && ancho > 0 {
			if g, es := c.Layout.(flujo); es && t.Width > ancho {
				_, _, h := g.colocar(c.Objects, ancho)
				t = fyne.NewSize(ancho, h)
			}
		}
		if x > 0 && x+t.Width > ancho {
			x, y, linea = 0, y+linea+f.hueco, 0
		}
		cajas[i], tams[i] = fyne.NewPos(x, y), t
		x += t.Width + f.hueco
		linea = max(linea, t.Height)
	}
	if ultimo >= 0 && f.ultimoADerecha && ancho > 0 {
		// `ms-auto`: se lleva el sobrante de su línea.
		cajas[ultimo].X = max(ancho-tams[ultimo].Width, cajas[ultimo].X)
	}
	if ultimo < 0 {
		return cajas, tams, 0
	}
	return cajas, tams, y + linea
}

// MinSize es la fila entera sin partir: lo que pide el flujo cuando cabe todo. Con
// `ancho`, lo que ocupa partido a ese ancho.
func (f flujo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if f.ancho > 0 {
		_, _, h := f.colocar(objs, f.ancho)
		return fyne.NewSize(f.ancho, h)
	}
	ancho, alto, n := float32(0), float32(0), 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		t := o.MinSize()
		if n > 0 {
			ancho += f.hueco
		}
		ancho += t.Width
		alto = max(alto, t.Height)
		n++
	}
	return fyne.NewSize(ancho, alto)
}

// Layout coloca a los hijos con el reparto de colocar. En cada línea van centrados en
// vertical sobre su propio alto: todos los de la cabecera miden 28, así que no se nota.
func (f flujo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	cajas, tams, _ := f.colocar(objs, t.Width)
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

// altoFlujo es el alto de un contenedor con `flujo` a ese ancho.
func altoFlujo(c *fyne.Container, ancho float32) float32 {
	f, es := c.Layout.(flujo)
	if !es {
		return c.MinSize().Height
	}
	_, _, h := f.colocar(c.Objects, ancho)
	return h
}
