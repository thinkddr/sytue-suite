// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// ARRASTRAR UN BLOQUE POR SU ASA (⋮⋮), como el asa de Confluence y la de la web
// (`llevarBloque` de mover-bloques.ts): se coge el asa, una raya de 2 px en el color de
// marca dice dónde caerá —encima o debajo del bloque bajo el puntero, según la mitad— y
// al soltar el bloque va allí. Una tabla o unas columnas van enteras, y dentro de ellas
// no se suelta nada (rompería la rejilla). Soltar entre dos bloques de un panel lo mete
// en el panel; fuera de él, lo saca.
//
// Solo con ratón: con el dedo no hay asa (el menú de mantener pulsado tiene «Subir» y
// «Bajar»), como en la web, donde el arrastre de HTML no existe en una pantalla táctil.

// enAsa dice si un punto de la zona cae en el asa del bloque bajo el ratón.
func (z *zonaEdicion) enAsa(pos fyne.Position) bool {
	if z.encima < 0 || z.encima >= len(z.e.doc.Bloques) {
		return false
	}
	p, ok := z.cajaAsa(z.encima)
	return ok && pos.X >= p.X-2 && pos.X <= p.X+PXZonaEd && pos.Y >= p.Y-4 && pos.Y <= p.Y+ladoAsaBloque+4
}

// unidadSoltar es un sitio donde soltar: los bloques [i, j) que van juntos y su caja.
type unidadSoltar struct {
	i, j          int
	arriba, abajo float32
}

// unidadesSoltar son los bloques del documento agrupados como se mueven (una tabla
// entera), con la caja que ocupan en lo maquetado.
func (z *zonaEdicion) unidadesSoltar() []unidadSoltar {
	d, m := z.e.doc, z.maquetado()
	var out []unidadSoltar
	for i := 0; i < len(d.Bloques) && i < len(m.Bloques); {
		_, j := d.unidad(i)
		u := unidadSoltar{i: i, j: j, arriba: m.Bloques[i].Y, abajo: m.Bloques[i].Y + m.Bloques[i].Alto}
		for k := i + 1; k < j && k < len(m.Bloques); k++ {
			u.arriba, u.abajo = min(u.arriba, m.Bloques[k].Y), max(u.abajo, m.Bloques[k].Y+m.Bloques[k].Alto)
		}
		out = append(out, u)
		i = j
	}
	return out
}

// destinoSoltar es dónde caería lo que se arrastra con el puntero a la altura `y` de la
// zona: delante de qué bloque (len: al final) y la altura de la raya.
func (z *zonaEdicion) destinoSoltar(y float32) (int, float32) {
	us := z.unidadesSoltar()
	if len(us) == 0 {
		return -1, 0
	}
	y -= PYZonaEd
	mejor := us[len(us)-1]
	for _, u := range us {
		if y <= u.abajo {
			mejor = u
			break
		}
	}
	if y < (mejor.arriba+mejor.abajo)/2 {
		return mejor.i, mejor.arriba
	}
	return mejor.j, mejor.abajo
}

// arrastrarAsa sigue el arrastre de un bloque: apunta dónde caería y repinta la raya.
func (z *zonaEdicion) arrastrarAsa(pos fyne.Position) {
	z.soltar, z.ySoltar = z.destinoSoltar(pos.Y)
	z.Refresh()
}

// soltarAsa remata el arrastre: el bloque va donde dice la raya, en un paso de deshacer.
func (z *zonaEdicion) soltarAsa() {
	desde, hasta := z.llevando, z.soltar
	z.llevando, z.soltar = -1, -1
	if desde >= 0 && hasta >= 0 {
		e := z.e
		e.hacer(func() { e.cur = e.doc.LlevarBloque(desde, hasta); e.ancla = e.cur })
	}
	z.Refresh()
}

// rayaSoltar es la raya de dónde caerá el bloque (`.notes-soltar`: `inset-x-3 -mt-px
// h-0.5 rounded-full bg-primary`), o nil si no se arrastra nada.
func (z *zonaEdicion) rayaSoltar(ancho float32, v tema.Variante) fyne.CanvasObject {
	if z.llevando < 0 || z.soltar < 0 {
		return nil
	}
	alto := tema.Espacio(0.5)
	return rect(tema.Color("primary", v), PXZonaEd, PYZonaEd+z.ySoltar-alto/2, max(ancho-2*PXZonaEd, 0), alto, alto/2)
}

// LlevarBloque lleva el bloque `b` —o la tabla, o las columnas, de las que es— delante
// del bloque `destino` (len: al final) y devuelve el cursor al principio de lo llevado.
// Soltarlo sobre sí mismo, o dentro de una tabla, no hace nada. Lo que no es de una tabla
// toma la caja de donde cae: la de los dos bloques entre los que queda, o ninguna.
func (d *Documento) LlevarBloque(b, destino int) Pos {
	if b < 0 || b >= len(d.Bloques) || destino < 0 || destino > len(d.Bloques) {
		return Pos{max(b, 0), 0}
	}
	i, j := d.unidad(b)
	enTabla := func(k int) bool {
		return k > 0 && k < len(d.Bloques) && d.Bloques[k].Celda.EnTabla() && d.Bloques[k-1].Celda.Tabla == d.Bloques[k].Celda.Tabla
	}
	if destino >= i && destino <= j || enTabla(destino) {
		return Pos{b, inicioEditable(d.Bloques[b])}
	}
	var caja *Caja
	if destino > 0 && destino < len(d.Bloques) && d.Bloques[destino-1].Caja == d.Bloques[destino].Caja &&
		!d.Bloques[destino].Celda.EnTabla() {
		caja = d.Bloques[destino].Caja
	}
	llevados := slices.Clone(d.Bloques[i:j])
	if !llevados[0].Celda.EnTabla() {
		for k := range llevados {
			llevados[k].Caja = caja
		}
	}
	d.Bloques = slices.Delete(d.Bloques, i, j)
	if destino > j {
		destino -= j - i
	}
	d.Bloques = slices.Insert(d.Bloques, destino, llevados...)
	return Pos{destino, inicioEditable(d.Bloques[destino])}
}
