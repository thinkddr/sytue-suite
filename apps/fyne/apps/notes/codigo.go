// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LOS NÚMEROS DE LÍNEA Y EL AJUSTE DE UN BLOQUE DE CÓDIGO (`pre[data-lineas]` y
// `pre[data-ajuste]`), y lo que pinta lo nuevo de las tablas y las columnas.
//
// Los números van en su columna, como `.notes-lineas` de app.css: `w-10` a la derecha,
// en la monoespaciada de `text-sm` y en `text-subtle`, y el texto empieza a `pl-13` del
// borde del bloque. Se numeran las líneas del CÓDIGO, no las de la pantalla: una línea
// larga que se parte lleva un solo número, en su primera fila.
//
// El ajuste: en la web, sin `data-ajuste` la línea larga no se parte y el bloque se
// desplaza de lado; con él se parte por cualquier letra (`overflow-wrap: anywhere`).
// ponytail: el maquetador de la nota no tiene desplazamiento de lado por bloque, así que
// aquí una línea larga se parte siempre —por los blancos sin ajuste, por cualquier letra
// con él—: se ve entera y se puede escribir en ella. El atributo se lee, se cambia desde
// la barra del bloque y se guarda igual, para la web.

// Medidas de los números de línea: `w-10` su columna y `pl-13` donde empieza el código.
var (
	anchoNumerosCodigo  = tema.Espacio(10)
	sangriaCodigoLineas = tema.Espacio(13)
)

// numerarCodigo pone el número de cada línea del código del bloque `i` (cuyas líneas
// maquetadas empiezan en `primera`), a la derecha de `derecha`.
func (m *Maquetado) numerarCodigo(i int, b Bloque, primera int, derecha float32, e estiloBloque) {
	texto := []rune(b.Texto())
	for _, l := range m.Lineas[primera:] {
		if l.Bloque != i {
			continue
		}
		if l.Desde > 0 && l.Desde <= len(texto) && texto[l.Desde-1] != '\n' {
			continue // la continuación de una línea partida no lleva número
		}
		n := strings.Count(string(texto[:min(l.Desde, len(texto))]), "\n") + 1
		m.Adornos = append(m.Adornos, Adorno{
			Tipo: adornoLineas, Bloque: i, X: derecha, Y: l.Y, Al: l.Alto,
			Texto: strconv.Itoa(n), escalon: e.escalon,
		})
	}
}

// letraALetra parte los pedazos de texto de un bloque en letras sueltas que se pueden
// cortar: el `overflow-wrap: anywhere` del código con ajuste.
func letraALetra(ps []pedazo) []pedazo {
	out := make([]pedazo, 0, len(ps))
	for _, p := range ps {
		if p.blanco || p.salto || p.img != nil {
			out = append(out, p)
			continue
		}
		k := 0
		for _, r := range p.texto {
			out = append(out, pedazo{texto: string(r), desde: p.desde + k, estilo: p.estilo, corte: true})
			k++
		}
	}
	return out
}

// pintarAdornoNuevo pinta la guía de una columna, el número de una fila de tabla y los
// números de línea del código.
func pintarAdornoNuevo(a Adorno, v tema.Variante) []fyne.CanvasObject {
	switch a.Tipo {
	case adornoGuia:
		g := ui.NuevoBordeDiscontinuo(tema.Borde, tema.RadioSM, "border")
		g.Move(fyne.NewPos(a.X, a.Y))
		g.Resize(fyne.NewSize(a.An, a.Al))
		return []fyne.CanvasObject{g}
	case adornoNumeroFila, adornoLineas:
		est := estiloTrozo{escalon: a.escalon, mono: a.Tipo == adornoLineas}
		cara, estilo := caraDe(est)
		t := canvas.NewText(a.Texto, tema.Color("text-subtle", v))
		t.TextSize, t.TextStyle, t.FontSource = a.escalon.Tam, estilo, cara
		w := medirTrozo(a.Texto, est)
		x := a.X - w
		if a.Tipo == adornoNumeroFila {
			x = a.X + a.An - pxCeldaTabla - w // `text-right` dentro de su `px-2`
		}
		t.Move(fyne.NewPos(x, a.Y+(a.escalon.Linea-altoFuenteDe(a.escalon.Tam))/2))
		t.Resize(t.MinSize())
		return []fyne.CanvasObject{t}
	case adornoVineta, adornoNumero, adornoCasilla, adornoFilo, adornoFondo, adornoRegla, adornoCaja,
		adornoPanel, adornoIcono, adornoCabecera, adornoDecision, adornoEtiqueta:
	}
	return nil
}
