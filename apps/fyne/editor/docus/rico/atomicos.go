// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Los BLOQUES ATÓMICOS de la web que este editor no sabe editar por dentro, pero tampoco
// pueden dejar el documento entero en solo lectura:
//
//   - El «Dibujo» (`editor/dibujo.ts`): `<figure data-dibujo data-escena="<JSON>">` con el
//     SVG de la escena y un `<img data-dibujo-respaldo hidden>` con su PNG.
//   - El informe de Power BI (`canvas/embedDom.ts`): `<div data-sytue-embed
//     data-provider="powerbi" data-report=…>` con su leyenda estática de respaldo.
//
// Los dos son `contenteditable="false"` también en la web: un objeto que se selecciona, se
// mueve y se borra entero. Aquí van como un bloque [Figura] más: su interior no se lee
// —la escena no tiene port Go, y el informe se hidrata con un iframe que nunca entra en el
// modelo—, se pinta con su respaldo y vuelve al servidor byte a byte.

// Las clases de los bloques atómicos ([Ilustracion.Clase]); son las de su `data-*`, como
// las de las figuras, y así [ClaseDe] da la misma con su HTML.
const (
	ClaseDibujo = "dibujo"
	ClaseEmbed  = "sytue-embed"
)

const (
	marcaDibujo = "data-dibujo"
	marcaEmbed  = "data-sytue-embed"
)

// Atomico es lo que se sabe de un bloque atómico para pintarlo: nada de él se edita.
type Atomico struct {
	Clase string
	// Titulo es el de la escena del dibujo (su `titulo`) o la referencia del informe (su
	// `data-report`); puede ir vacío.
	Titulo string
	// Proveedor es el `data-provider` del informe ("powerbi").
	Proveedor string
	// Respaldo es el `src` del PNG de respaldo del dibujo, tal cual; vacío si no lleva.
	// Quien lo pinta comprueba su tamaño y lo descodifica con su barrera.
	Respaldo string
	// Proporcion es alto/ancho del dibujo, de su escena (o del respaldo); 0 si no se sabe.
	Proporcion float64
}

// esAtomico dice si n es uno de los bloques atómicos de la web.
func esAtomico(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Namespace != "" {
		return false
	}
	return n.DataAtom == atom.Figure && tieneAtributo(n, marcaDibujo) ||
		n.DataAtom == atom.Div && tieneAtributo(n, marcaEmbed)
}

// Topes de la proporción de un dibujo: una escena de 1 × 10 000 no hace un bloque de un
// kilómetro, ni una de 10 000 × 1 una raya invisible.
const (
	proporcionMin = 0.05
	proporcionMax = 4
)

func atomicoDe(n *html.Node) *Atomico {
	if n.DataAtom == atom.Div {
		return &Atomico{Clase: ClaseEmbed, Titulo: strings.TrimSpace(atributo(n, "data-report")), Proveedor: atributo(n, "data-provider")}
	}
	a := &Atomico{Clase: ClaseDibujo}
	var escena struct {
		Ancho, Alto float64
		Titulo      string
	}
	if json.Unmarshal([]byte(atributo(n, "data-escena")), &escena) == nil {
		a.Titulo = strings.TrimSpace(escena.Titulo)
		a.Proporcion = proporcion(escena.Ancho, escena.Alto)
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type != html.ElementNode || h.DataAtom != atom.Img || !tieneAtributo(h, "data-dibujo-respaldo") {
			continue
		}
		a.Respaldo = atributo(h, "src")
		if a.Proporcion == 0 {
			w, _ := strconv.ParseFloat(atributo(h, "data-w"), 64)
			alto, _ := strconv.ParseFloat(atributo(h, "data-h"), 64)
			a.Proporcion = proporcion(w, alto)
		}
		break
	}
	return a
}

// ProporcionAcotada es alto/ancho acotado entre los topes del dibujo, o 0 si las medidas
// no valen; también para la del PNG de respaldo, cuando la escena no la dice.
func ProporcionAcotada(w, h float64) float64 { return proporcion(w, h) }

// proporcion es alto/ancho acotado, o 0 si las medidas no valen.
func proporcion(w, h float64) float64 {
	if !(w > 0) || !(h > 0) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return 0
	}
	return min(max(h/w, proporcionMin), proporcionMax)
}

// margenFigura es el margen de arriba y abajo de una figura: `margin: 1em auto` las de la
// web, y `margin: 12px 0` el bloque del informe.
func margenFigura(il *Ilustracion) (arriba, abajo float32) {
	if il != nil && il.Clase() == ClaseEmbed {
		return 12, 12
	}
	return 16, 16
}
