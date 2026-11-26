// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// «Ordenar párrafos» de la pestaña Inicio (`ordenarParrafos` de `canvas/exec.ts`): los
// bloques elegidos, de la A a la Z o al revés por su texto, con la colación del idioma
// del documento. Los dos recorren los mismos vectores
// (`packages/office/test/vectores/docus-herramientas.json`).

// OrdenDeTextos es `ordenDeTextos`: los índices de `textos` en el orden de «Ordenar
// párrafos». Estable: los iguales quedan como estaban, también de la Z a la A.
func OrdenDeTextos(textos []string, desc bool, idioma language.Tag) []int {
	c := collate.New(idioma)
	signo := 1
	if desc {
		signo = -1
	}
	orden := make([]int, len(textos))
	for i := range orden {
		orden[i] = i
	}
	slices.SortStableFunc(orden, func(a, b int) int { return signo * c.CompareString(textos[a], textos[b]) })
	return orden
}

// textoDeOrden es el texto de un bloque como lo compara la web: sin objetos y con el
// blanco junto (`textContent` con `\s+` → « » y recortado).
func textoDeOrden(t string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(t, string(ObjetoRuna), "")), " ")
}

// OrdenarBloques ordena los bloques [desde, hasta] y devuelve dónde queda el primero. Solo
// si son HERMANOS —párrafos seguidos o elementos de una misma lista—, cada uno su
// elemento entero y en el cuerpo: mezclar una lista con los párrafos de alrededor, partir
// una celda o mover un encabezado rompería el documento, y entonces no se toca nada. false
// también si ya estaban en ese orden.
func (d *Documento) OrdenarBloques(bloques []Bloque, desde, hasta int, desc bool, idioma language.Tag) (Lugar, bool) {
	if desde < 0 || hasta >= len(bloques) || desde >= hasta {
		return Lugar{}, false
	}
	var padre *html.Node
	nodos := make([]*html.Node, 0, hasta-desde+1)
	textos := make([]string, 0, hasta-desde+1)
	for i := desde; i <= hasta; i++ {
		b := bloques[i]
		if !b.Tipo.EsTexto() || !b.cubre() || esCelda(b.cont) || b.Region != "" || b.Celda != bloques[desde].Celda {
			return Lugar{}, false
		}
		if i == desde {
			padre = b.cont.Parent
		}
		if padre == nil || b.cont.Parent != padre {
			return Lugar{}, false
		}
		nodos = append(nodos, b.cont)
		textos = append(textos, textoDeOrden(b.Editar().Texto()))
	}
	orden := OrdenDeTextos(textos, desc, idioma)
	if slices.IsSorted(orden) {
		return Lugar{}, false
	}
	ref := nodos[len(nodos)-1].NextSibling
	for _, n := range nodos {
		padre.RemoveChild(n)
	}
	for _, i := range orden {
		padre.InsertBefore(nodos[i], ref)
	}
	return bloques[desde+orden[0]].Lugar, true
}
