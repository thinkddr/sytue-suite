// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fmt"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// flotantes son los gráficos y las segmentaciones de la hoja activa, colocados en las
// coordenadas del CONTENIDO de la rejilla (`left: x; top: y` dentro del contenedor que
// se desplaza) y escalados con su zoom. Las tarjetas se reutilizan por id: una tarjeta
// abierta en edición no se pierde porque la hoja se repinte.
func (c *Lienzo) flotantes(vw vista) []fyne.CanvasObject {
	l := c.Libro
	out := make([]fyne.CanvasObject, 0, len(l.Graficos())+len(l.Segmentaciones()))
	vivos := map[string]bool{}
	for _, g := range l.Graficos() {
		t := c.tarjetas[g.ID]
		if t == nil {
			t = nuevaTarjetaGrafico(c, g.ID)
			c.tarjetas[g.ID] = t
		}
		vivos[g.ID] = true
		x, y := g.Posicion()
		if t.modo == gestoMover {
			x, y = x+t.desp.DX, y+t.desp.DY
		}
		c.colocarFlotante(t, &t.clave, x, y, vw, fmt.Sprint(l.version, vw.z, t.editando, t.modo, t.medid))
		out = append(out, t)
	}
	for _, s := range l.Segmentaciones() {
		t := c.segmentos[s.ID]
		if t == nil {
			t = nuevaTarjetaSegmentacion(c, s.ID)
			c.segmentos[s.ID] = t
		}
		vivos[s.ID] = true
		x, y := s.Posicion()
		if t.moviendo {
			x, y = x+t.desp.DX, y+t.desp.DY
		}
		z := vw.z
		t.Move(fyne.NewPos(x*z-vw.desX, y*z-vw.desY))
		m := t.medida()
		t.Resize(fyne.NewSize(m.Width*z, m.Height*z))
		t.Refresh()
		out = append(out, t)
	}
	if chip := c.chipEnlace(vw); chip != nil {
		out = append(out, chip)
	}
	for id := range c.tarjetas {
		if !vivos[id] {
			delete(c.tarjetas, id)
		}
	}
	for id := range c.segmentos {
		if !vivos[id] {
			delete(c.segmentos, id)
		}
	}
	return out
}

// colocarFlotante mueve la tarjeta y, si ha cambiado lo que la pinta (`clave`), la
// rehace: una tarjeta que solo se desplaza con la hoja no recalcula su gráfico.
func (c *Lienzo) colocarFlotante(t *TarjetaGrafico, ultima *string, x, y float32, vw vista, clave string) {
	z := vw.z
	t.Move(fyne.NewPos(x*z-vw.desX, y*z-vw.desY))
	if *ultima != clave || t.tam.IsZero() {
		*ultima = clave
		t.Refresh()
	}
	t.Resize(fyne.NewSize(t.tam.Width*z, t.tam.Height*z))
}

// refrescarTarjetas obliga a rehacer las tarjetas en el próximo pintado (su medida ha
// cambiado sin que cambie el libro: el editor ha crecido o encogido).
func (c *Lienzo) refrescarTarjetas() {
	for _, t := range c.tarjetas {
		t.clave = ""
	}
	c.Rejilla.Refresh()
}

// SeparacionChipEnlace es el aire entre la celda y el chip de su enlace (`top + 4`).
const SeparacionChipEnlace = 4

// chipEnlace es el chip del enlace de la celda activa (`linkChip` de la web, un `Button`
// secundario `xs` con `href`): debajo de la celda, con la máquina o el correo, y abre el
// destino al pulsarlo. Va con los flotantes
// porque vive en las coordenadas de la rejilla y tiene que seguirla al desplazarse. nil si
// la celda no tiene enlace o se está escribiendo en ella.
func (c *Lienzo) chipEnlace(vw vista) *ui.Boton {
	rj := c.Rejilla
	if rj.editando {
		return nil
	}
	href := c.Libro.Enlace(rj.sel.C, rj.sel.F)
	if href == "" {
		return nil
	}
	if c.chip == nil {
		c.chip = ui.NuevoBoton("", func() {
			rj.AbrirEnlace(rj.sel.C, rj.sel.F)
			rj.recuperarFoco() // tras abrir, las flechas siguen moviendo la selección
		}).Con(ui.Secundario, ui.TallaXS)
		c.chip.Icono = iconos.Enlace
	}
	if texto := rotuloEnlace(href); c.chip.Texto != texto || c.chipHref != href {
		c.chip.Texto, c.chipHref = texto, href
		c.chip.Rotulo = c.t("sheet.link.open", i18n.Vars{"url": href})
		c.chip.Refresh()
	}
	c.chip.Move(fyne.NewPos(vw.x(rj.sel.C), vw.y(rj.sel.F)+vw.alto(rj.sel.F)+SeparacionChipEnlace))
	c.chip.Resize(c.chip.MinSize())
	return c.chip
}
