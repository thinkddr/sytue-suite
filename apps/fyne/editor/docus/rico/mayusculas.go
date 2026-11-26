// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"strings"
	"unicode"
)

// «Cambiar mayúsculas y minúsculas» de la pestaña Inicio. Es el gemelo de
// `packages/office/src/editor/mayusculas.ts` y los dos recorren los mismos vectores
// (`packages/office/test/vectores/docus-mayusculas.json`): el mismo documento se edita en
// las dos y una frase no puede salir distinta según dónde se pulse.
//
// Runa a runa y sin cambiar la longitud: cada runa conserva su cadena de formato, así que
// el formato no se desplaza y la selección sigue abarcando lo mismo. `unicode.ToUpper` ya
// es así (el mapeo simple: «ß» se queda «ß», «İ» baja a «i»); la web descarta lo que su
// `toUpperCase` alargaría y repone a mano esos pocos del mapeo simple.

// ModoMayusculas es una de las cuatro transformaciones.
type ModoMayusculas string

// Las cuatro, en el orden del menú. Son los valores del comando `changeCase`.
const (
	TipoOracion ModoMayusculas = "sentence"
	Minusculas  ModoMayusculas = "lower"
	Mayusculas  ModoMayusculas = "upper"
	CadaPalabra ModoMayusculas = "title"
)

// ModosMayusculas son las cuatro, en el orden del menú.
var ModosMayusculas = []ModoMayusculas{TipoOracion, Minusculas, Mayusculas, CadaPalabra}

// LeerModoMayusculas dice qué modo es un valor del comando, o false si no es ninguno.
func LeerModoMayusculas(v string) (ModoMayusculas, bool) {
	for _, m := range ModosMayusculas {
		if string(m) == v {
			return m, true
		}
	}
	return "", false
}

// SiguienteModoMayusculas es Mayús+F3 de Word (`siguienteModoMayusculas`): el modo
// siguiente para el texto elegido en el ciclo minúsculas → MAYÚSCULAS → Cada Palabra →
// minúsculas. Lo que ya está todo en minúscula pasa a mayúsculas; todo en mayúscula, a
// cada palabra; lo demás, a minúsculas. Se mide con las mismas transformaciones.
func SiguienteModoMayusculas(texto string) ModoMayusculas {
	if NuevoCambioMayusculas(Minusculas).Tramo(texto) == texto {
		return Mayusculas
	}
	if NuevoCambioMayusculas(Mayusculas).Tramo(texto) == texto {
		return CadaPalabra
	}
	return Minusculas
}

// CambioMayusculas transforma los trozos de un párrafo seguidos: el estado (¿empieza
// frase?, ¿estamos dentro de una palabra?) cruza los trozos y se reinicia con [Parrafo].
type CambioMayusculas struct {
	modo                              ModoMayusculas
	inicioDeFrase, trasFin, enPalabra bool
}

// NuevoCambioMayusculas empieza un cambio en el principio de un párrafo.
func NuevoCambioMayusculas(m ModoMayusculas) *CambioMayusculas {
	return &CambioMayusculas{modo: m, inicioDeFrase: true}
}

// Parrafo empieza un párrafo nuevo: frase y palabra nuevas.
func (c *CambioMayusculas) Parrafo() {
	c.inicioDeFrase, c.trasFin, c.enPalabra = true, false, false
}

// Previo es lo que hay en el párrafo ANTES de la selección: no se cambia, pero decide
// cómo empieza («la» dentro de «hola» no es principio de palabra).
func (c *CambioMayusculas) Previo(texto string) {
	for _, r := range texto {
		c.runa(r)
	}
}

// Tramo transforma el siguiente trozo del párrafo en curso.
func (c *CambioMayusculas) Tramo(texto string) string {
	return strings.Map(c.runa, texto)
}

func (c *CambioMayusculas) runa(r rune) rune {
	switch c.modo {
	case Mayusculas:
		return unicode.ToUpper(r)
	case Minusculas:
		return unicode.ToLower(r)
	case CadaPalabra:
		out := r
		if unicode.IsLetter(r) {
			out = mayusculaSi(r, !c.enPalabra)
		}
		c.enPalabra = dePalabra(r)
		return out
	case TipoOracion:
	}
	// Tipo oración: todo en minúscula salvo la primera letra de cada frase.
	switch {
	case unicode.IsLetter(r):
		out := mayusculaSi(r, c.inicioDeFrase)
		c.inicioDeFrase, c.trasFin = false, false
		return out
	case strings.ContainsRune(".!?…", r):
		c.trasFin = true
	case unicode.IsSpace(r):
		c.inicioDeFrase = c.inicioDeFrase || c.trasFin
	case unicode.IsNumber(r):
		c.trasFin = false // «3.5» no es un final de frase
	}
	return r
}

func mayusculaSi(r rune, mayuscula bool) rune {
	if mayuscula {
		return unicode.ToUpper(r)
	}
	return unicode.ToLower(r)
}

// dePalabra es lo que sigue una palabra: letras, cifras, marcas y el apóstrofo («l’été»).
func dePalabra(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '\'' || r == '’'
}

// CambiarMayusculas transforma las letras de [desde, hasta) con `c`, sin tocar su formato.
// El bloque es un párrafo: empieza frase y palabra, y lo que tiene antes de `desde` decide
// cómo sigue. Los objetos (imágenes, citas…) no se tocan: no son letras.
func (e *Edicion) CambiarMayusculas(desde, hasta int, c *CambioMayusculas) {
	desde, hasta = max(desde, 0), min(hasta, len(e.runas))
	if desde >= hasta {
		return
	}
	c.Parrafo()
	c.Previo(string(e.runas[:desde]))
	cambio := false
	for i := desde; i < hasta; i++ {
		if r := c.runa(e.runas[i]); r != e.runas[i] && e.objeto(i) == nil {
			e.runas[i], cambio = r, true
		}
	}
	if cambio {
		e.reescribir()
	}
}
