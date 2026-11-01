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
	if r, ok := runaAfii(c); ok {
		return string(r), true
	}
	if r, ok := runaZapfDingbats(c); ok {
		return string(r), true
	}
	if s, ok := componenteGlifoHex(c); ok {
		return s, true
	}
	if r, tam := utf8.DecodeRuneInString(c); tam == len(c) && r != utf8.RuneError {
		return c, true
	}
	return "", false
}

func componenteGlifoHex(c string) (string, bool) {
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
		0x304: {"macron"}, 0x306: {"breve"}, 0x307: {"dotaccent", "dot"}, 0x308: {"dieresis"},
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

// byteMacRoman codifica una runa en Macintosh Roman (0..255).
func byteMacRoman(r rune) (byte, bool) {
	if r > 0 && r < 0x80 {
		return byte(r), true
	}
	return charmap.Macintosh.EncodeRune(r)
}

var afiiEslavas = map[int]rune{
	10023: 0x0401, // Ё
	10050: 0x0490, 10051: 0x0402, 10052: 0x0403, 10053: 0x0404, 10054: 0x0405,
	10055: 0x0406, 10056: 0x0407, 10057: 0x0408, 10058: 0x0409, 10059: 0x040A,
	10060: 0x040B, 10061: 0x040C, 10062: 0x040E, 10145: 0x040F,
	10071: 0x0451, // ё
	10098: 0x0491, 10099: 0x0452, 10100: 0x0453, 10101: 0x0454, 10102: 0x0455,
	10103: 0x0456, 10104: 0x0457, 10105: 0x0458, 10106: 0x0459, 10107: 0x045A,
	10108: 0x045B, 10109: 0x045C, 10110: 0x045E, 10193: 0x045F,
	61352: 0x2116, // №
}

// runaAfii traduce los nombres `afiiNNNNN` de Adobe para cirílico y hebreo
// (usados por fuentes Type 1 y CFF de Europa del Este, TeX y generadores antiguos).
func runaAfii(c string) (rune, bool) {
	if !strings.HasPrefix(c, "afii") || len(c) != 9 {
		return 0, false
	}
	n, err := strconv.Atoi(c[4:])
	if err != nil {
		return 0, false
	}
	if r, ok := afiiEslavas[n]; ok {
		return r, true
	}
	switch {
	case n >= 10017 && n <= 10022:
		return rune(0x0410 + (n - 10017)), true
	case n >= 10024 && n <= 10049:
		return rune(0x0416 + (n - 10024)), true
	case n >= 10065 && n <= 10070:
		return rune(0x0430 + (n - 10065)), true
	case n >= 10072 && n <= 10097:
		return rune(0x0436 + (n - 10072)), true
	case n >= 57664 && n <= 57690:
		return rune(0x05D0 + (n - 57664)), true
	default:
		return 0, false
	}
}

// aplicarSymbol aplica la codificación intrínseca de la fuente estándar Symbol
// (ISO 32000-1, anexo D.5).
func aplicarSymbol(tabla map[byte]string) {
	for c := byte(0x20); c <= 0x7E; c++ {
		tabla[c] = string(rune(c))
	}
	especialesASCII := map[byte]rune{
		0x22: '∀', 0x24: '∃', 0x27: '∋', 0x2A: '∗', 0x2D: '−', 0x40: '≅',
		0x5C: '∴', 0x5E: '⊥', 0x60: '‾', 0x7E: '∼',
	}
	for b, r := range especialesASCII {
		tabla[b] = string(r)
	}
	griegoMay := [26]rune{
		'Α', 'Β', 'Χ', 'Δ', 'Ε', 'Φ', 'Γ', 'Η', 'Ι', 'ϑ', 'Κ', 'Λ', 'Μ',
		'Ν', 'Ο', 'Π', 'Θ', 'Ρ', 'Σ', 'Τ', 'Υ', 'ς', 'Ω', 'Ξ', 'Ψ', 'Ζ',
	}
	griegoMin := [26]rune{
		'α', 'β', 'χ', 'δ', 'ε', 'φ', 'γ', 'η', 'ι', 'ϕ', 'κ', 'λ', 'μ',
		'ν', 'ο', 'π', 'θ', 'ρ', 'σ', 'τ', 'υ', 'ϖ', 'ω', 'ξ', 'ψ', 'ζ',
	}
	for i := 0; i < 26; i++ {
		tabla[byte('A'+i)] = string(griegoMay[i])
		tabla[byte('a'+i)] = string(griegoMin[i])
	}
	altos := map[byte]rune{
		0xA0: '€', 0xA1: 'ϒ', 0xA2: '′', 0xA3: '≤', 0xA4: '⁄', 0xA5: '∞', 0xA6: 'ƒ',
		0xA7: '♣', 0xA8: '♦', 0xA9: '♥', 0xAA: '♠', 0xAB: '↔', 0xAC: '←', 0xAD: '↑', 0xAE: '→', 0xAF: '↓',
		0xB0: '°', 0xB1: '±', 0xB2: '″', 0xB3: '≥', 0xB4: '×', 0xB5: '∝', 0xB6: '∂', 0xB7: '•',
		0xB8: '÷', 0xB9: '≠', 0xBA: '≡', 0xBB: '≈', 0xBC: '…', 0xBD: '∣', 0xBE: '⎯', 0xBF: '↵',
		0xC0: 'ℵ', 0xC1: 'ℑ', 0xC2: 'ℜ', 0xC3: '℘', 0xC4: '⊗', 0xC5: '⊕', 0xC6: '∅', 0xC7: '∩',
		0xC8: '∪', 0xC9: '⊃', 0xCA: '⊇', 0xCB: '⊄', 0xCC: '⊂', 0xCD: '⊆', 0xCE: '∈', 0xCF: '∉',
		0xD0: '∠', 0xD1: '∇', 0xD2: '®', 0xD3: '©', 0xD4: '™', 0xD5: '∏', 0xD6: '√', 0xD7: '⋅',
		0xD8: '¬', 0xD9: '∧', 0xDA: '∨', 0xDB: '⇔', 0xDC: '⇐', 0xDD: '⇑', 0xDE: '⇒', 0xDF: '⇓',
		0xE0: '◊', 0xE1: '⟨', 0xE2: '®', 0xE3: '©', 0xE4: '™', 0xE5: '∑',
		0xE6: '⎛', 0xE7: '⎜', 0xE8: '⎝', 0xE9: '⎡', 0xEA: '⎢', 0xEB: '⎣', 0xEC: '⎧', 0xED: '⎨', 0xEE: '⎩', 0xEF: '⎪',
		0xF1: '⟩', 0xF2: '∫', 0xF3: '⌠', 0xF4: '⎮', 0xF5: '⌡',
		0xF6: '⎞', 0xF7: '⎟', 0xF8: '⎠', 0xF9: '⎤', 0xFA: '⎥', 0xFB: '⎦', 0xFC: '⎫', 0xFD: '⎬', 0xFE: '⎭',
	}
	for b, r := range altos {
		tabla[b] = string(r)
	}
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

// runaZapfDingbats resuelve los nombres estándar `a1`..`a206` de ITC Zapf
// Dingbats (ISO 32000-1 anexo D.6) que aparecen en `/Differences`.
func runaZapfDingbats(c string) (rune, bool) {
	if len(c) < 2 || len(c) > 4 || c[0] != 'a' || c[1] < '1' || c[1] > '9' {
		return 0, false
	}
	n, err := strconv.Atoi(c[1:])
	if err != nil || n < 1 || n >= len(runasZapfA) {
		return 0, false
	}
	r := runasZapfA[n]
	return r, r != 0
}

var runasZapfA = [207]rune{
	0,
	0x2701, 0x2702, 0x2704, 0x260E, 0x2706, 0x271D, 0x271E, 0x271F, 0x2720, 0x2721,
	0x261B, 0x261E, 0x270C, 0x270D, 0x270E, 0x270F, 0x2711, 0x2712, 0x2713, 0x2714,
	0x2715, 0x2716, 0x2717, 0x2718, 0x2719, 0x271A, 0x271B, 0x271C, 0x2722, 0x2723,
	0x2724, 0x2725, 0x2726, 0x2727, 0x2605, 0x2729, 0x272A, 0x272B, 0x272C, 0x272D,
	0x272E, 0x272F, 0x2730, 0x2731, 0x2732, 0x2733, 0x2734, 0x2735, 0x2736, 0x2737,
	0x2738, 0x2739, 0x273A, 0x273B, 0x273C, 0x273D, 0x273E, 0x273F, 0x2740, 0x2741,
	0x2742, 0x2743, 0x2744, 0x2745, 0x2746, 0x2747, 0x2748, 0x2749, 0x274A, 0x274B,
	0x25CF, 0x274D, 0x25A0, 0x274F, 0x2751, 0x25B2, 0x25BC, 0x25C6, 0x2756, 0,
	0x25D7, 0x2758, 0x2759, 0x275A, 0x276F, 0x2771, 0x2772, 0x2773, 0x2768, 0x2769,
	0x276C, 0x276D, 0x276A, 0x276B, 0x2774, 0x2775, 0x275B, 0x275C, 0x275D, 0x275E,
	0x2761, 0x2762, 0x2763, 0x2764, 0x2710, 0x2765, 0x2766, 0x2767, 0x2660, 0x2665,
	0x2666, 0x2663, 0, 0, 0, 0, 0x2709, 0x2708, 0x2707, 0x2460,
	0x2461, 0x2462, 0x2463, 0x2464, 0x2465, 0x2466, 0x2467, 0x2468, 0x2469, 0x2776,
	0x2777, 0x2778, 0x2779, 0x277A, 0x277B, 0x277C, 0x277D, 0x277E, 0x277F, 0x2780,
	0x2781, 0x2782, 0x2783, 0x2784, 0x2785, 0x2786, 0x2787, 0x2788, 0x2789, 0x278A,
	0x278B, 0x278C, 0x278D, 0x278E, 0x278F, 0x2790, 0x2791, 0x2792, 0x2793, 0x2794,
	0x2192, 0x27A3, 0x2194, 0x2195, 0x2799, 0x279B, 0x279C, 0x279D, 0x279E, 0x279F,
	0x27A0, 0x27A1, 0x27A2, 0x27A4, 0x27A5, 0x27A6, 0x27A7, 0x27A8, 0x27A9, 0x27AB,
	0x27AD, 0x27AF, 0x27B2, 0x27B3, 0x27B5, 0x27B8, 0x27BA, 0x27BB, 0x27BC, 0x27BD,
	0x27BE, 0x279A, 0x27AA, 0x27B6, 0x27B9, 0x2798, 0x27B4, 0x27B7, 0x27AC, 0x27AE,
	0x27B1, 0x2703, 0x2750, 0x2752, 0x276E, 0x2770,
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
