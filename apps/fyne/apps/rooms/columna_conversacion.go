// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// columnaConversacion conserva las piezas vivas del canal o hilo. Sólo cuando
// sus franjas no caben introduce un scroll de la columna completa: ni el editor
// ni los controles táctiles tienen que encoger para entrar en horizontal.
type columnaConversacion struct {
	minimo      func(ancho float32) float32
	colocar     func(fyne.Size)
	ancho       float32
	contenido   *fyne.Container
	desborde    *container.Scroll
	desbordando bool
}

func (c *columnaConversacion) repartir(t fyne.Size, objetos []fyne.CanvasObject) {
	c.ancho = t.Width
	antes := c.desbordando
	c.desbordando = t.Height < c.minimo(t.Width)
	if !c.desbordando {
		c.colocar(t)
		return
	}
	if c.desborde == nil {
		c.contenido = container.New(c)
		c.desborde = container.NewVScroll(c.contenido)
	}
	c.contenido.Objects = objetos
	c.contenido.Refresh()
	c.desborde.Move(fyne.Position{})
	c.desborde.Resize(t)
	c.desborde.Refresh()
	if !antes {
		c.desborde.ScrollToBottom()
	}
}

func (c *columnaConversacion) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, c.minimo(c.ancho))
}

func (c *columnaConversacion) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.colocar(t)
}
