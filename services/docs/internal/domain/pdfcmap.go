// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CMaps de una fuente compuesta (§9.7.5 y §9.7.6 de ISO 32000): cómo se parte
// una cadena en códigos cuando la /Encoding no es Identity-H/V, y qué CID es
// cada código.
//
// Identity-H lee de dos en dos bytes y el código ES el CID, que es el 99 % de lo
// que se ve. El resto rompía la extracción de texto entera: una CMap incrustada
// de un byte (OneByteIdentityH de Word, la Prince-*-H de Prince XML) leída de
// dos en dos junta letras que no van juntas, y una en UTF-8 (Uni-Utf8-H) no
// sacaba nada. Medido en el corpus: tres ficheros con 0 y 0,06 de recall.
//
// Las CMaps predefinidas por Unicode (Uni*-UCS2, -UTF16, -UTF8, -UTF32) se
// resuelven sin tablas: el código ya es el carácter. Las de codificaciones
// nacionales (90ms-RKSJ-H, GBK-EUC-H…) necesitan las tablas de Adobe y se
// siguen leyendo de dos en dos, que es lo que se hacía.
package domain

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// cmapCodigos es la CMap de /Encoding cuando no es Identity.
type cmapCodigos struct {
	espacios []espacioCodigos
	cids     map[uint32]uint32 // cidchar
	rangos   []rangoCID        // cidrange
	// unicode, si no está vacío, dice que el código es el carácter en esa
	// forma de codificación: "ucs2", "utf16", "utf8" o "utf32".
	unicode string
}

// espacioCodigos es un rango de `begincodespacerange`: n bytes, cada uno entre
// su mínimo y su máximo.
type espacioCodigos struct {
	n      int
	lo, hi [4]byte
}

type rangoCID struct {
	n                  int
	desde, hasta, base uint32
}

// cmapPredefinida resuelve las CMaps por nombre que no necesitan tablas.
// Devuelve nil para Identity (y para las nacionales, que no se interpretan).
func cmapPredefinida(nombre string) *cmapCodigos {
	if !strings.HasPrefix(nombre, "Uni") {
		return nil
	}
	espacio := func(n int, lo, hi byte) espacioCodigos {
		e := espacioCodigos{n: n}
		for i := 0; i < n; i++ {
			e.lo[i], e.hi[i] = lo, hi
		}
		return e
	}
	switch {
	case strings.Contains(nombre, "-UCS2-"):
		return &cmapCodigos{unicode: "ucs2", espacios: []espacioCodigos{espacio(2, 0, 0xFF)}}
	case strings.Contains(nombre, "-UTF16-"):
		return &cmapCodigos{unicode: "utf16", espacios: []espacioCodigos{
			{n: 2, lo: [4]byte{0, 0}, hi: [4]byte{0xD7, 0xFF}},
			{n: 4, lo: [4]byte{0xD8, 0, 0xDC, 0}, hi: [4]byte{0xDB, 0xFF, 0xDF, 0xFF}},
			{n: 2, lo: [4]byte{0xE0, 0}, hi: [4]byte{0xFF, 0xFF}},
		}}
	case strings.Contains(nombre, "-UTF8-"):
		return &cmapCodigos{unicode: "utf8", espacios: espaciosUTF8()}
	case strings.Contains(nombre, "-UTF32-"):
		return &cmapCodigos{unicode: "utf32", espacios: []espacioCodigos{espacio(4, 0, 0xFF)}}
	}
	return nil
}

func espaciosUTF8() []espacioCodigos {
	return []espacioCodigos{
		{n: 1, lo: [4]byte{0}, hi: [4]byte{0x7F}},
		{n: 2, lo: [4]byte{0xC0, 0x80}, hi: [4]byte{0xDF, 0xBF}},
		{n: 3, lo: [4]byte{0xE0, 0x80, 0x80}, hi: [4]byte{0xEF, 0xBF, 0xBF}},
		{n: 4, lo: [4]byte{0xF0, 0x80, 0x80, 0x80}, hi: [4]byte{0xF7, 0xBF, 0xBF, 0xBF}},
	}
}

// leerCMapCodigos interpreta una CMap incrustada (/Encoding como flujo).
// Sin espacio de códigos no sirve para partir nada y se devuelve nil.
func (d *PDFDoc) leerCMapCodigos(st *pdfStream) *cmapCodigos {
	datos, err := d.datosDeFlujo(st)
	if err != nil {
		return nil
	}
	c := &cmapCodigos{cids: map[uint32]uint32{}}
	d.recorrerCMap(datos, func(op string, pila []pdfObj) {
		switch op {
		case "endcodespacerange":
			c.anadirEspacios(pila)
		case "endcidchar":
			c.anadirCIDs(pila)
		case "endcidrange":
			c.anadirRangos(pila)
		}
	})
	if len(c.espacios) == 0 {
		return nil
	}
	return c
}

func (c *cmapCodigos) anadirEspacios(pila []pdfObj) {
	for i := 0; i+1 < len(pila); i += 2 {
		lo, ok1 := pila[i].(pdfString)
		hi, ok2 := pila[i+1].(pdfString)
		if ok1 && ok2 && len(lo) == len(hi) && len(lo) >= 1 && len(lo) <= 4 && len(c.espacios) < 64 {
			e := espacioCodigos{n: len(lo)}
			copy(e.lo[:], lo)
			copy(e.hi[:], hi)
			c.espacios = append(c.espacios, e)
		}
	}
}

func (c *cmapCodigos) anadirCIDs(pila []pdfObj) {
	for i := 0; i+1 < len(pila); i += 2 {
		o, ok := pila[i].(pdfString)
		cid, ok2 := comoEntero(pila[i+1])
		if ok && ok2 && cid >= 0 && cid <= maxCID && len(c.cids) < 70000 {
			c.cids[codigoDeHex(o)] = uint32(cid)
		}
	}
}

func (c *cmapCodigos) anadirRangos(pila []pdfObj) {
	for i := 0; i+2 < len(pila); i += 3 {
		a, ok1 := pila[i].(pdfString)
		b, ok2 := pila[i+1].(pdfString)
		cid, ok3 := comoEntero(pila[i+2])
		if ok1 && ok2 && ok3 && cid >= 0 && cid <= maxCID && len(c.rangos) < 70000 {
			c.rangos = append(c.rangos, rangoCID{n: len(a), desde: codigoDeHex(a), hasta: codigoDeHex(b), base: uint32(cid)})
		}
	}
}

// siguiente parte el próximo código de s: el espacio de códigos más corto que
// lo contiene entero. Si ninguno casa (§9.7.6.3), se consume lo que mide el
// más corto cuyo primer byte sí casa, o un byte.
func (c *cmapCodigos) siguiente(s pdfString) (codigo uint32, n int) {
	parcial := 0
	for largo := 1; largo <= 4 && largo <= len(s); largo++ {
		for _, e := range c.espacios {
			if e.n != largo {
				continue
			}
			dentro := true
			for i := 0; i < largo && dentro; i++ {
				dentro = s[i] >= e.lo[i] && s[i] <= e.hi[i]
			}
			if dentro {
				return codigoDeHex(s[:largo]), largo
			}
			if parcial == 0 && s[0] >= e.lo[0] && s[0] <= e.hi[0] {
				parcial = largo
			}
		}
	}
	n = max(1, min(parcial, len(s)))
	return codigoDeHex(s[:n]), n
}

// cid es el CID de un código de n bytes. Sin entrada en la CMap, la tabla del
// generador no lo cubre y se usa el propio código, como Identity.
func (c *cmapCodigos) cid(codigo uint32, n int) uint32 {
	if c.unicode != "" {
		return codigo
	}
	if v, ok := c.cids[codigo]; ok {
		return v
	}
	for _, r := range c.rangos {
		if r.n == n && codigo >= r.desde && codigo <= r.hasta {
			return r.base + codigo - r.desde
		}
	}
	return codigo
}

// largo dice cuántos bytes ocupa un código al escribirlo: el primer espacio de
// códigos que lo contiene.
func (c *cmapCodigos) largo(codigo uint32) int {
	for n := 1; n <= 4; n++ {
		if n < 4 && codigo>>(8*n) != 0 {
			continue
		}
		for _, e := range c.espacios {
			dentro := e.n == n
			for i := 0; i < n && dentro; i++ {
				b := byte((codigo >> (8 * (n - 1 - i))) & 0xFF)
				dentro = b >= e.lo[i] && b <= e.hi[i]
			}
			if dentro {
				return n
			}
		}
	}
	return 2
}

// textoUnicode es el carácter de un código cuando la CMap es Unicode.
func (c *cmapCodigos) textoUnicode(codigo uint32, n int) string {
	switch c.unicode {
	case "ucs2", "utf16":
		if n == 4 {
			return string(utf16.DecodeRune(rune(codigo>>16), rune(codigo&0xFFFF)))
		}
		return string(rune(codigo & 0xFFFF))
	case "utf8":
		var b [4]byte
		for i := 0; i < n; i++ {
			b[i] = byte((codigo >> (8 * (n - 1 - i))) & 0xFF)
		}
		if r, m := utf8.DecodeRune(b[:n]); r != utf8.RuneError && m == n {
			return string(r)
		}
	case "utf32":
		if codigo <= maxRuna {
			return string(rune(codigo))
		}
	}
	return ""
}

// recorrerCMap pasa por los operadores de una CMap con la pila de operandos que
// los precede. Sirve para /ToUnicode y para las CMaps de /Encoding.
func (d *PDFDoc) recorrerCMap(datos []byte, op func(string, []pdfObj)) {
	lx := &pdfLexer{buf: datos}
	var pila []pdfObj
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			return
		}
		c := lx.buf[lx.pos]
		if c == '/' || c == '(' || c == '<' || c == '[' || c == '+' || c == '-' || c == '.' ||
			(c >= '0' && c <= '9') {
			antes := lx.pos
			o, err := d.parsearObjeto(lx)
			if err != nil || lx.pos == antes {
				lx.pos = antes + 1
				continue
			}
			pila = append(pila, o)
			if len(pila) > 400 {
				pila = pila[len(pila)-400:]
			}
			continue
		}
		nombre := lx.operador()
		if nombre == "" {
			lx.pos++
			continue
		}
		op(nombre, pila)
		if strings.HasPrefix(nombre, "begin") || strings.HasPrefix(nombre, "end") {
			pila = pila[:0]
		}
	}
}
