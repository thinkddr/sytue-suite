// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"strings"
	"unicode"
)

// Ecuaciones de Office (OMML, ECMA-376 Parte 1 §22.1). El editor no tiene fórmulas, pero su texto
// es del documento: cada m:oMath entra como una línea de texto en el formato lineal de las
// ecuaciones de Word (el que se teclea en Word para escribir una: «(a+b)/2», «x^2», «√(x)»,
// «∑_(i=1)^n a_i»), de modo que se lee y, pegado en Word, vuelve a ser una ecuación. Una m:oMathPara
// es una ecuación aparte: va centrada salvo que el párrafo diga otra cosa.
//
// Clean-room sobre la especificación; nada se ha portado de otra implementación.

// nodoMath es un elemento de OMML ya leído: su nombre, sus atributos val de las propiedades que
// importan y sus hijos, o su texto si es un m:t.
type nodoMath struct {
	nombre string
	val    string
	conVal bool // lleva val, aunque sea vacío (begChr="" es que no hay delimitador)
	hijos  []*nodoMath
	texto  string
}

func nuevoNodoMath(se xml.StartElement) *nodoMath {
	n := &nodoMath{nombre: se.Name.Local}
	for _, a := range se.Attr {
		if a.Name.Local == "val" {
			n.val, n.conVal = a.Value, true
		}
	}
	return n
}

// familiaEcuacion es la letra de una ecuación: la de Word y, sin ella, una con serifa. Es la que
// el importador escribe para Cambria Math con su genérica, así que vuelve igual de una ida y
// vuelta por .docx.
const familiaEcuacion = "'Cambria Math',serif"

// alineacionEcuacion es la alineación de una m:oMathPara (oMathParaPr/jc, §22.1.2.51): centrada
// salvo que diga izquierda o derecha.
func alineacionEcuacion(n *nodoMath) string {
	switch n.prop("oMathParaPr", "jc", "centerGroup") {
	case "left":
		return "left"
	case "right":
		return "right"
	}
	return "center"
}

// leerMath lee el elemento se de OMML con lo que tiene dentro. Iterativo sobre una pila y con
// tope de profundidad: el fichero viene de fuera.
func leerMath(dec *xml.Decoder, se xml.StartElement) *nodoMath {
	raiz := nuevoNodoMath(se)
	pila := []*nodoMath{raiz}
	for len(pila) > 0 {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		cima := pila[len(pila)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			if len(pila) >= 64 {
				_ = dec.Skip()
				continue
			}
			n := nuevoNodoMath(t)
			cima.hijos = append(cima.hijos, n)
			pila = append(pila, n)
		case xml.CharData:
			if cima.nombre == "t" {
				cima.texto += string(t)
			}
		case xml.EndElement:
			pila = pila[:len(pila)-1]
		}
	}
	return raiz
}

// hijo es el primer hijo con ese nombre, o nil.
func (n *nodoMath) hijo(nombre string) *nodoMath {
	if n == nil {
		return nil
	}
	for _, h := range n.hijos {
		if h.nombre == nombre {
			return h
		}
	}
	return nil
}

// prop es el val de la propiedad nombre dentro de las propiedades pr (dPr, naryPr…), o defecto.
func (n *nodoMath) prop(pr, nombre, defecto string) string {
	if p := n.hijo(pr).hijo(nombre); p != nil {
		if !p.conVal {
			return "on" // un interruptor sin val está encendido
		}
		return p.val
	}
	return defecto
}

// linealMath es la ecuación n en formato lineal.
//
//nolint:gocyclo // una rama por estructura de OMML: la ramificación ES la tabla
func linealMath(n *nodoMath) string {
	if n == nil {
		return ""
	}
	e := func() string { return linealMath(n.hijo("e")) }
	switch n.nombre {
	case "t":
		return n.texto
	case "f":
		// Sin raya (un coeficiente binomial) es «¦»; inclinada, «⁄»; las demás, «/».
		raya := map[string]string{"noBar": "¦", "skw": "⁄"}[n.prop("fPr", "type", "bar")]
		return grupoMath(linealMath(n.hijo("num"))) + cmpOr(raya, "/") + grupoMath(linealMath(n.hijo("den")))
	case "sSup":
		return e() + "^" + grupoMath(linealMath(n.hijo("sup")))
	case "sSub":
		return e() + "_" + grupoMath(linealMath(n.hijo("sub")))
	case "sSubSup":
		return e() + "_" + grupoMath(linealMath(n.hijo("sub"))) + "^" + grupoMath(linealMath(n.hijo("sup")))
	case "sPre":
		return "_" + grupoMath(linealMath(n.hijo("sub"))) + "^" + grupoMath(linealMath(n.hijo("sup"))) + e()
	case "rad":
		return raizMath(n)
	case "d":
		return delimitadoMath(n)
	case "nary":
		return naryMath(n)
	case "acc":
		return e() + n.prop("accPr", "chr", "̂")
	case "bar":
		if n.prop("barPr", "pos", "bot") == "top" {
			return e() + "̅"
		}
		return e() + "̲"
	case "groupChr":
		return e() + n.prop("groupChrPr", "chr", "⏟")
	case "limLow":
		return e() + "_" + grupoMath(linealMath(n.hijo("lim")))
	case "limUpp":
		return e() + "^" + grupoMath(linealMath(n.hijo("lim")))
	case "func":
		nombre, arg := linealMath(n.hijo("fName")), e()
		if strings.HasPrefix(arg, "(") {
			return nombre + arg
		}
		return nombre + " " + arg
	case "m":
		return "■(" + filasMath(n, "mr", "&") + ")"
	case "eqArr":
		return "█(" + filasMath(n, "e", "") + ")"
	}
	if strings.HasSuffix(n.nombre, "Pr") {
		return "" // propiedades: no son texto
	}
	var b strings.Builder
	for _, h := range n.hijos {
		b.WriteString(linealMath(h))
	}
	return b.String()
}

// raizMath es una raíz: «√x», o con su índice, «√(3&x)».
func raizMath(n *nodoMath) string {
	grado, e := linealMath(n.hijo("deg")), linealMath(n.hijo("e"))
	if grado == "" || encendido(n.prop("radPr", "degHide", "")) {
		return "√" + grupoMath(e)
	}
	return "√(" + grado + "&" + e + ")"
}

// delimitadoMath son sus partes entre sus delimitadores (paréntesis si no dice otros).
func delimitadoMath(n *nodoMath) string {
	var partes []string
	for _, h := range n.hijos {
		if h.nombre == "e" {
			partes = append(partes, linealMath(h))
		}
	}
	return n.prop("dPr", "begChr", "(") + strings.Join(partes, n.prop("dPr", "sepChr", "│")) + n.prop("dPr", "endChr", ")")
}

// naryMath es un operador n-ario (suma, integral…) con sus límites: «∑_(i=1)^n a_i».
func naryMath(n *nodoMath) string {
	out := n.prop("naryPr", "chr", "∫")
	if sub := linealMath(n.hijo("sub")); sub != "" && !encendido(n.prop("naryPr", "subHide", "")) {
		out += "_" + grupoMath(sub)
	}
	if sup := linealMath(n.hijo("sup")); sup != "" && !encendido(n.prop("naryPr", "supHide", "")) {
		out += "^" + grupoMath(sup)
	}
	return out + " " + linealMath(n.hijo("e"))
}

// filasMath son las filas de una matriz (mr, con sus celdas e separadas por sep) o de un sistema
// de ecuaciones (e), separadas por «@».
func filasMath(n *nodoMath, fila, sep string) string {
	var filas []string
	for _, h := range n.hijos {
		if h.nombre != fila {
			continue
		}
		if sep == "" {
			filas = append(filas, linealMath(h))
			continue
		}
		var celdas []string
		for _, c := range h.hijos {
			if c.nombre == "e" {
				celdas = append(celdas, linealMath(c))
			}
		}
		filas = append(filas, strings.Join(celdas, sep))
	}
	return strings.Join(filas, "@")
}

// encendido dice si el val de un interruptor de OMML está encendido.
func encendido(v string) bool { return v == "on" || v == "1" || v == "true" }

// grupoMath pone entre paréntesis lo que no es un solo operando (un número, un nombre o un
// carácter): «x^10», pero «x^(n+1)».
func grupoMath(s string) string {
	r := []rune(s)
	if len(r) <= 1 {
		return s
	}
	for _, c := range r {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return "(" + s + ")"
		}
	}
	return s
}

// ecuaciones son las líneas de un m:oMath (una) o de un m:oMathPara (una por cada m:oMath).
func ecuaciones(n *nodoMath) []string {
	if n.nombre == "oMath" {
		return []string{linealMath(n)}
	}
	var out []string
	for _, h := range n.hijos {
		if h.nombre == "oMath" {
			out = append(out, linealMath(h))
		}
	}
	return out
}
