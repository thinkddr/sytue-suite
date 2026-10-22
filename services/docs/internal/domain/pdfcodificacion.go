// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Codificación de las fuentes simples sin /ToUnicode (§9.6.5 y anexo D de ISO
// 32000): qué carácter es cada código.
//
// Hasta 2026-10 todo lo que no fuera WinAnsi se leía como WinAnsi. Medido en el
// corpus: con /MacRomanEncoding los apóstrofos salían «o» y la «é» «z» (seis
// ficheros); una Type 1 incrustada sin /Encoding perdía las ligaduras de su
// codificación propia («field» salía «eld», ocho ficheros); y un nombre de
// glifo fuera de la lista corta («scedilla», «f_t», «a.sc») se quedaba en el
// código crudo.
package domain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// textoDeNombreGlifo traduce un nombre de glifo a texto con las reglas de los
// nombres de Adobe: lo que va tras el primer punto es una variante («a.sc» es
// «a»), «_» une los componentes de una ligadura («f_f_i» es «ffi») y cada
// componente es un nombre conocido, «uniXXXX[XXXX…]» o «uXXXX[XX]».
func textoDeNombreGlifo(n string) (string, bool) {
	if i := strings.IndexByte(n, '.'); i > 0 {
		n = n[:i]
	}
	if n == "" {
		return "", false
	}
	var b strings.Builder
	for _, c := range strings.Split(n, "_") {
		t, ok := componenteGlifo(c)
		if !ok {
			return "", false
		}
		b.WriteString(t)
	}
	return b.String(), true
}

func componenteGlifo(c string) (string, bool) {
	if r, ok := nombresDeGlifo[c]; ok {
		return string(r), true
	}
	if r, ok := nombresAcentuados[c]; ok {
		return string(r), true
	}
	if r, ok := nombresLatinos[c]; ok {
		return string(r), true
	}
	if strings.HasPrefix(c, "uni") && len(c) >= 7 && (len(c)-3)%4 == 0 {
		var b strings.Builder
		for i := 3; i < len(c); i += 4 {
			v, err := strconv.ParseUint(c[i:i+4], 16, 32)
			if err != nil || (v >= 0xD800 && v <= 0xDFFF) {
				return "", false
			}
			b.WriteRune(rune(v & 0xFFFF))
		}
		return b.String(), true
	}
	if strings.HasPrefix(c, "u") && len(c) >= 5 && len(c) <= 7 {
		if v, err := strconv.ParseUint(c[1:], 16, 32); err == nil && v <= maxRuna && (v < 0xD800 || v > 0xDFFF) {
			return string(rune(v)), true
		}
	}
	if r, tam := utf8.DecodeRuneInString(c); tam == len(c) && r != utf8.RuneError {
		return c, true
	}
	return "", false
}

// runaDeNombreGlifo es textoDeNombreGlifo cuando el resultado es un solo carácter.
func runaDeNombreGlifo(n string) (rune, bool) {
	t, ok := textoDeNombreGlifo(n)
	if !ok {
		return 0, false
	}
	r, tam := utf8.DecodeRuneInString(t)
	return r, tam == len(t) && r != utf8.RuneError
}

// nombresLatinos son las letras latinas con diacrítico por su nombre de Adobe,
// que es la letra base seguida del nombre del acento («scedilla», «Ccaron»,
// «Idotaccent»). Se generan descomponiendo el carácter en vez de copiar la
// lista: los nombres siguen la misma regla en todo el bloque.
var nombresLatinos = func() map[string]rune {
	acentos := map[rune][]string{
		0x300: {"grave"}, 0x301: {"acute"}, 0x302: {"circumflex"}, 0x303: {"tilde"},
		0x304: {"macron"}, 0x306: {"breve"}, 0x307: {"dotaccent"}, 0x308: {"dieresis"},
		0x30A: {"ring"}, 0x30B: {"hungarumlaut"}, 0x30C: {"caron"},
		0x326: {"commaaccent"}, 0x327: {"cedilla", "commaaccent"}, 0x328: {"ogonek"},
	}
	out := map[string]rune{
		"Dcroat": 'Đ', "dcroat": 'đ', "Hbar": 'Ħ', "hbar": 'ħ', "Tbar": 'Ŧ', "tbar": 'ŧ',
		"IJ": 'Ĳ', "ij": 'ĳ', "Ldot": 'Ŀ', "ldot": 'ŀ', "Eng": 'Ŋ', "eng": 'ŋ',
		"kgreenlandic": 'ĸ', "napostrophe": 'ŉ', "longs": 'ſ', "Lslash": 'Ł', "lslash": 'ł',
		"Oslash": 'Ø', "oslash": 'ø', "dotlessj": 'ȷ', "florin": 'ƒ',
	}
	for r := rune(0xC0); r <= 0x24F; r++ {
		d := []rune(norm.NFD.String(string(r)))
		if len(d) != 2 || d[0] > unicode.MaxASCII || !unicode.IsLetter(d[0]) {
			continue
		}
		for _, a := range acentos[d[1]] {
			if _, ya := out[string(d[0])+a]; !ya {
				out[string(d[0])+a] = r
			}
		}
	}
	return out
}()

// textoMacRoman es el carácter de un código ≥ 128 de /MacRomanEncoding.
func textoMacRoman(c byte) string {
	return string(charmap.Macintosh.DecodeByte(c))
}

// aplicarEstandar pone la StandardEncoding de Adobe: la base de una Type 1 sin
// /Encoding. Comparte tabla con la codificación estándar del CFF.
func aplicarEstandar(tabla map[byte]string) {
	for c, sid := range codificacionEstandarCFF {
		if sid < len(cadenasEstandarCFF) {
			if t, ok := textoDeNombreGlifo(cadenasEstandarCFF[sid]); ok {
				tabla[c] = t
			}
		}
	}
	tabla['\''], tabla['`'] = "’", "‘"
}

// aplicarZapfDingbats es la codificación propia de ZapfDingbats (sin incrustar
// casi siempre, porque es una de las 14 estándar). Los bloques corresponden, en
// orden, al de Dingbats de Unicode, que nació de esta fuente; los huecos que
// Unicode ya tenía en otra parte se cubren a mano.
func aplicarZapfDingbats(tabla map[byte]string) {
	tramo := func(desde, hasta byte, inicio rune) {
		for c, r := desde, inicio; ; c, r = c+1, r+1 {
			tabla[c] = string(r)
			if c == hasta {
				break
			}
		}
	}
	tabla[0x20] = " "
	tramo(0x21, 0x24, 0x2701)
	tabla[0x25] = "☎"
	tramo(0x26, 0x29, 0x2706)
	tabla[0x2A], tabla[0x2B] = "☛", "☞"
	tramo(0x2C, 0x47, 0x270C)
	tabla[0x48] = "★"
	tramo(0x49, 0x6B, 0x2729)
	tabla[0x6C], tabla[0x6D], tabla[0x6E] = "●", "❍", "■"
	tramo(0x6F, 0x72, 0x274F)
	tabla[0x73], tabla[0x74], tabla[0x75], tabla[0x76], tabla[0x77] = "▲", "▼", "◆", "❖", "◗"
	tramo(0x78, 0x7E, 0x2758)
	tramo(0xA1, 0xA7, 0x2761)
	tabla[0xA8], tabla[0xA9], tabla[0xAA], tabla[0xAB] = "♣", "♦", "♥", "♠"
	tramo(0xAC, 0xB5, 0x2460)
	tramo(0xB6, 0xD3, 0x2776)
	tabla[0xD4], tabla[0xD5], tabla[0xD6], tabla[0xD7] = "➔", "→", "↔", "↕"
	tramo(0xD8, 0xEF, 0x2798)
	tramo(0xF1, 0xFE, 0x27B1)
}

// codificacionPropia es la codificación del programa incrustado (Type 1 o CFF):
// la base de una Type 1 cuya /Encoding no la da. Falso si no hay programa o si
// su codificación es la estándar.
func (l *lienzo) codificacionPropia(desc pdfDict, tabla map[byte]string) bool {
	if desc == nil {
		return false
	}
	if st, ok := l.d.resolver(desc["FontFile"]).(*pdfStream); ok {
		b, err := l.d.datosDeFlujo(st)
		if err != nil || len(b) > maxFuenteIncrustada {
			return false
		}
		len1, _ := numeroPDF(l.d.resolver(st.Dict["Length1"]))
		claro, _ := partirType1(quitarSegmentosPFB(b), int(len1))
		propia := codificacionType1(claro)
		for c, n := range propia {
			if t, ok := textoDeNombreGlifo(n); ok {
				tabla[c] = t
			}
		}
		return propia != nil
	}
	if st, ok := l.d.resolver(desc["FontFile3"]).(*pdfStream); ok {
		if s, _ := l.d.resolver(st.Dict["Subtype"]).(pdfName); s != "Type1C" {
			return false
		}
		b, err := l.d.datosDeFlujo(st)
		if err != nil || len(b) > maxFuenteIncrustada {
			return false
		}
		info, err := leerCFF(b)
		if err != nil || info.cid || len(info.propia) == 0 {
			return false
		}
		for c, g := range info.propia {
			if g < len(info.nombres) {
				if t, ok := textoDeNombreGlifo(info.nombres[g]); ok {
					tabla[c] = t
				}
			}
		}
		return true
	}
	return false
}
