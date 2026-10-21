// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// LISTAS PLEGABLES, como en Twos y como en la web (client/en-nota.ts y app.css): un
// elemento de lista con sublista se pliega, el plegado se guarda con la nota
// (`<li class="plegada">`) y al LEERLA no se ve lo que cuelga de él, con «…» en su
// lugar. En el editor se sigue viendo entero, con la viñeta en el color de marca como
// aviso. Se pliega pulsando la viñeta al leer o con Ctrl+. al escribir.

// claseplegada es la clase del `<li>` plegado. La deja pasar el saneador del servicio.
const claseplegada = "plegada"

// tieneSublista dice si del bloque i cuelga una lista más honda: el siguiente es de lista,
// en la misma celda y de más nivel.
func (d *Documento) tieneSublista(i int) bool {
	if i < 0 || i+1 >= len(d.Bloques) || !esLista(d.Bloques[i].Tipo) {
		return false
	}
	s := d.Bloques[i+1]
	return esLista(s.Tipo) && s.Celda == d.Bloques[i].Celda && s.Nivel > d.Bloques[i].Nivel
}

// AlternarPlegado pliega o despliega el elemento i. Devuelve false si no hay qué plegar.
func (d *Documento) AlternarPlegado(i int) bool {
	if !d.tieneSublista(i) {
		return false
	}
	d.Bloques[i].Plegada = !d.Bloques[i].Plegada
	return true
}

// ocultos dice qué bloques no se ven por colgar de un elemento plegado. nil si se ve todo
// (la copia que se edita).
func (d *Documento) ocultos() []bool {
	if d.desplegado {
		return nil
	}
	var out []bool
	bajo := 0 // nivel del plegado del que se cuelga; 0, de ninguno
	for i, b := range d.Bloques {
		oculto := bajo > 0 && esLista(b.Tipo) && b.Nivel > bajo && i > 0 && b.Celda == d.Bloques[i-1].Celda
		if !oculto {
			bajo = 0
			if b.Plegada && d.tieneSublista(i) {
				bajo = b.Nivel
			}
		}
		if oculto && out == nil {
			out = make([]bool, len(d.Bloques))
		}
		if out != nil {
			out[i] = oculto
		}
	}
	return out
}

// conPuntos es el bloque plegado tal como se lee: su texto y «…» en `text-subtle`
// (el `::after` de app.css).
func conPuntos(b Bloque, v tema.Variante) Bloque {
	c := tema.Color("text-subtle", v)
	b.Tramos = append(slices.Clip(b.Tramos), Tramo{Texto: " …", Color: fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B), EnSpan: true})
	return b
}

// alternarPlegado es Ctrl+. en el editor: pliega o despliega la lista del cursor.
func (e *EditorNota) alternarPlegado() {
	if len(e.doc.Bloques) == 0 || !e.doc.tieneSublista(e.cur.B) {
		return
	}
	e.hacer(func() { e.doc.AlternarPlegado(e.cur.B) })
}

// tintaVineta es el token de la viñeta o el número de un elemento de lista.
func tintaVineta(a Adorno) string {
	if a.plegada {
		return "primary"
	}
	return "text"
}

// viñetaEn es el elemento de lista cuya viñeta (o número) está bajo (x, y) de la prosa:
// la viñeta va FUERA del texto, en la sangría, como el `::marker` de Chromium. -1 si no
// hay ninguna ahí.
func (m *Maquetado) viñetaEn(x, y float32) int {
	for _, a := range m.Adornos {
		if a.Tipo != adornoVineta && a.Tipo != adornoNumero {
			continue
		}
		if y >= a.Y && y <= a.Y+a.Al && x < a.X && x >= a.X-2*a.escalon.Tam {
			return a.Bloque
		}
	}
	return -1
}

// plegarLista es pulsar la viñeta de una lista al LEER una nota: pliega o despliega el
// elemento y guarda la nota, como la web (`alternarPlegado` y `onChange`).
func (p *Pantalla) plegarLista(n Nota, i int) {
	if n.Doc == nil {
		return
	}
	d := n.Doc.copia()
	if !d.AlternarPlegado(i) {
		return
	}
	html := d.ParaGuardar()
	lanzar(func() error { return p.editar(n.ID, Parche{Contenido: &html}, true) })
}
