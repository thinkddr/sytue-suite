// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Extracción de texto de un PDF ya abierto por `pdfread.go`.
//
// El texto sale del MISMO intérprete que pinta la página (`pdfrender.go`), en
// modo recolección: las fuentes compuestas, /ToUnicode, /Differences, el texto
// dentro de un XObject de formulario, la `cm` y el /ActualText se resuelven
// igual que al dibujar. Hasta 2026-10 había aquí un segundo intérprete que leía
// los bytes de cada cadena como si fueran WinAnsi: en un PDF con fuente CID —el
// BOE, los impresos de la AEAT, casi todo lo que sale de Word con una fuente no
// latina— devolvía los códigos de glifo en crudo, y buscar no encontraba nada
// (medido con scripts/certipdf-compat: un 4 % de las palabras de un BOE).
package domain

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// PDFFragmento es un trozo de texto con el punto donde empieza, en el sistema de
// coordenadas del PDF: origen abajo-izquierda y unidades de 1/72 de pulgada.
type PDFFragmento struct {
	Texto  string
	X, Y   float64
	Tamano float64
	// izquierda: sus glifos avanzan hacia la izquierda (matriz volteada o un
	// espaciado negativo que los hace retroceder), así que el orden de la
	// cadena ya es el de lectura de una escritura RTL.
	izquierda bool
}

// TextoDePagina extrae los fragmentos de texto de la página i (0-based): lo que
// se ve en ella, también lo que pintan las apariencias de sus anotaciones —el
// valor de un campo de formulario, un sello, un texto libre—, que es lo que
// saca cualquier lector (Poppler incluido) y lo que alguien espera encontrar
// al buscar. Un formulario relleno daba antes una página sin texto.
func (d *PDFDoc) TextoDePagina(i int) ([]PDFFragmento, error) {
	return d.textoDePagina(i, true)
}

// TextoDelContenido es el texto del flujo de la página, sin las anotaciones:
// lo que no cambia al rellenar o firmar (comprobar que un firmado es el mismo
// documento) y lo que dice si una página escaneada tiene ya su capa de texto.
func (d *PDFDoc) TextoDelContenido(i int) (string, error) {
	frags, err := d.textoDePagina(i, false)
	if err != nil {
		return "", err
	}
	return juntarFragmentos(frags), nil
}

func (d *PDFDoc) textoDePagina(i int, conAnotaciones bool) ([]PDFFragmento, error) {
	p, err := d.Pagina(i)
	if err != nil {
		return nil, err
	}
	contenido, err := d.contenidoDePagina(p)
	if err != nil {
		return nil, err
	}
	// Sin SVG (soloRaster sin rasterizador) y sin decodificar imágenes
	// (recolectando): solo se quiere el texto.
	l := &lienzo{
		d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{},
		soloRaster: true, recolectando: true, texto: &recolectorTexto{},
	}
	recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
	// Matriz base identidad: coordenadas del PDF, no las del SVG.
	l.ejecutar(contenido, recursos, estadoInicial(identidad))
	if conAnotaciones {
		l.anotacionesDePagina(p, identidad)
	}
	for k := range l.texto.frags {
		if !l.texto.frags[k].izquierda {
			l.texto.frags[k].Texto = ordenLogico(l.texto.frags[k].Texto)
		}
	}
	return l.texto.frags, nil
}

// ordenLogico pasa a orden de lectura un fragmento con árabe o hebreo. Si sus
// glifos avanzan hacia la derecha (lo normal), su orden es el VISUAL: una
// palabra árabe sale al revés («ةيبرعلا» por «العربية») y buscarla no la encuentra.
// Medido en el corpus: nueve ficheros con escritura de derecha a izquierda.
//
// Se parte en tramos: los de derecha a izquierda (letras RTL y lo neutro entre
// ellas) se invierten, los de izquierda a derecha (latín y cifras) se quedan
// como están, y si manda la escritura RTL se invierte además el orden de los
// tramos. Los paréntesis de un tramo invertido se cambian por su pareja.
//
// ponytail: no es el algoritmo bidi completo (UAX #9: niveles anidados,
// marcas explícitas); basta para línea a línea, que es lo que trae un PDF.
func ordenLogico(s string) string {
	rs := []rune(s)
	rtl, ltr := 0, 0
	for _, r := range rs {
		switch {
		case esRTL(r):
			rtl++
		case unicode.IsLetter(r):
			ltr++
		}
	}
	if rtl == 0 {
		return s
	}
	tramos := tramosBidi(rs)
	if rtl >= ltr {
		slices.Reverse(tramos)
	}
	var b strings.Builder
	for _, t := range tramos {
		if !t.rtl {
			b.WriteString(string(t.rs))
			continue
		}
		for k := len(t.rs) - 1; k >= 0; k-- {
			r := t.rs[k]
			if par, ok := espejo[r]; ok {
				r = par
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

type tramoBidi struct {
	rs  []rune
	rtl bool
}

// fuerteLTR: letra o cifra que no es de escritura RTL.
func fuerteLTR(r rune) bool { return !esRTL(r) && (unicode.IsLetter(r) || unicode.IsDigit(r)) }

// tramosBidi parte en tramos: cada carácter fuerte abre o continúa el suyo, y lo
// neutro va con lo RTL salvo entre dos fuertes LTR.
func tramosBidi(rs []rune) []tramoBidi {
	var tramos []tramoBidi
	for i := 0; i < len(rs); {
		j := i
		if fuerteLTR(rs[i]) {
			// Hasta el último fuerte LTR antes del siguiente RTL.
			fin := i
			for ; j < len(rs) && !esRTL(rs[j]); j++ {
				if fuerteLTR(rs[j]) {
					fin = j
				}
			}
			j = fin + 1
			tramos = append(tramos, tramoBidi{rs[i:j], false})
		} else {
			for j < len(rs) && !fuerteLTR(rs[j]) {
				j++
			}
			tramos = append(tramos, tramoBidi{rs[i:j], true})
		}
		i = j
	}
	return tramos
}

var espejo = map[rune]rune{'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{', '<': '>', '>': '<', '«': '»', '»': '«'}

// esRTL: letra de escritura de derecha a izquierda. Las cifras arábigo-índicas
// son del bloque árabe pero se escriben de izquierda a derecha.
func esRTL(r rune) bool {
	return !unicode.IsDigit(r) && unicode.In(r, unicode.Arabic, unicode.Hebrew, unicode.Syriac, unicode.Thaana, unicode.Nko)
}

// TextoPlano devuelve el texto de la página como una cadena, juntando en la
// misma línea los fragmentos que comparten altura. Es lo que consume la
// búsqueda: sin esta agrupación, «hola mundo» partido en dos operadores Tj no
// casaría con la consulta.
func (d *PDFDoc) TextoPlano(i int) (string, error) {
	frags, err := d.TextoDePagina(i)
	if err != nil {
		return "", err
	}
	return juntarFragmentos(frags), nil
}

// juntarFragmentos pone en la misma línea los fragmentos que comparten altura.
func juntarFragmentos(frags []PDFFragmento) string {
	var sb strings.Builder
	yAnterior := 0.0
	primero := true
	for _, f := range frags {
		if f.Texto == "" {
			continue
		}
		if !primero {
			// Medio punto de tolerancia: la misma línea puede tener saltos
			// mínimos por ajustes de línea base (subíndices, cambios de fuente).
			if dif := f.Y - yAnterior; dif > 0.5 || dif < -0.5 {
				sb.WriteByte('\n')
			} else if !strings.HasSuffix(sb.String(), " ") {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(f.Texto)
		yAnterior = f.Y
		primero = false
	}
	return sb.String()
}

// recolectorTexto junta lo que el intérprete muestra en fragmentos.
//
// Un fragmento sigue mientras la cadena siguiente empieza en la MISMA línea y
// cerca de donde acabó la anterior. Hace falta porque los generadores trocean:
// un TJ con kerning parte cada palabra en varias cadenas, y Word o Chrome pintan
// a veces cada palabra —o cada letra— con su propio Tj. El buscador de la web
// busca DENTRO de cada fragmento, así que sin juntarlas «contrato» partido en
// «con» y «trato» no se encontraría.
type recolectorTexto struct {
	frags []PDFFragmento
	// Donde acabó la última cadena (espacio de usuario), la dirección de su
	// línea base y el tamaño de su letra.
	finX, finY, dirX, dirY, tam float64
	// Posición y longitud en bytes de la última marca de avance cero (para los
	// generadores como FOP que emiten el diacrítico antes de su consonante base).
	marcaX, marcaY float64
	marcaLen       int
}

// Umbrales en ems del tamaño de la letra. Un hueco de más de 0,15 em entre dos
// cadenas es un espacio entre palabras (el kerning no llega a 0,1); a partir de
// 3 em es otra columna, y a medio em de la línea base, otra línea.
const (
	emEspacio          = 0.15
	emEspacioFragmento = 0.10
	emOtraColumna      = 3.0
	emOtraLinea        = 0.5
)

// avX y avY son el desplazamiento de la cadena en el espacio de texto: avY solo
// en escritura vertical, donde la línea corre hacia abajo.
func (r *recolectorTexto) anadir(e *ejecucion, glifos []glifo, avX, avY float64) {
	texto := e.textoDeLaMarca(textoConHuecos(e, glifos))
	if texto == "" {
		return
	}
	escala := matriz{e.estado.escalaH, 0, 0, 1, 0, e.estado.subida}
	ini := escala.por(e.tm).por(e.estado.ctm)
	fin := escala.por(matriz{1, 0, 0, 1, avX, avY}.por(e.tm)).por(e.estado.ctm)
	tam := math.Abs(e.estado.tamano) * math.Hypot(ini[2], ini[3])
	if len(glifos) > 0 {
		if avMedio := math.Abs(avX) * math.Hypot(ini[0], ini[1]) / float64(len(glifos)); avMedio > tam {
			tam = avMedio
		}
	}
	dx, dy := ini[0], ini[1]
	vertical := avY != 0 || (e.estado.fuente != nil && e.estado.fuente.vertical)
	if vertical {
		dx, dy = -ini[2], -ini[3] // la línea baja por el eje y del texto
	} else if avX < 0 {
		dx, dy = -dx, -dy
	}
	if n := math.Hypot(dx, dy); n > 0 {
		dx, dy = dx/n, dy/n
	} else {
		dx, dy = 1, 0
	}

	if len(r.frags) > 0 && r.tam > 0 && r.unirAlUltimo(texto, ini, fin, avX, avY, math.Max(r.tam, tam)) {
		return
	}
	r.frags = append(r.frags, PDFFragmento{Texto: texto, X: ini[4], Y: ini[5], Tamano: tam, izquierda: !vertical && dx < 0})
	r.finX, r.finY, r.dirX, r.dirY, r.tam = fin[4], fin[5], dx, dy, tam
	if avX == 0 && avY == 0 {
		r.marcaX, r.marcaY, r.marcaLen = ini[4], ini[5], len(texto)
	} else {
		r.marcaLen = 0
	}
}

func (r *recolectorTexto) unirAlUltimo(texto string, ini, fin matriz, avX, avY, em float64) bool {
	hx, hy := ini[4]-r.finX, ini[5]-r.finY
	a := hx*r.dirX + hy*r.dirY  // a lo largo de la línea
	p := -hx*r.dirY + hy*r.dirX // de una línea a otra
	if math.Abs(p) >= emOtraLinea*em || a <= -em || a >= emOtraColumna*em {
		return false
	}
	ult := &r.frags[len(r.frags)-1]
	if avX == 0 && avY == 0 {
		ult.Texto += texto
		if r.marcaLen > 0 && math.Hypot(ini[4]-r.marcaX, ini[5]-r.marcaY) < 0.2*em {
			r.marcaLen += len(texto)
		} else {
			r.marcaX, r.marcaY, r.marcaLen = ini[4], ini[5], len(texto)
		}
		return true
	}
	if r.marcaLen > 0 && r.marcaLen <= len(ult.Texto) &&
		(ini[4]-r.marcaX)*r.dirX+(ini[5]-r.marcaY)*r.dirY < -0.1*em {
		_, nRuna := utf8.DecodeRuneInString(texto)
		corte := len(ult.Texto) - r.marcaLen
		ult.Texto = ult.Texto[:corte] + texto[:nRuna] + ult.Texto[corte:] + texto[nRuna:]
	} else {
		umbralEsp := emEspacioFragmento
		if avX < 0 {
			umbralEsp = emOtraLinea
		}
		if a > umbralEsp*em && !strings.HasSuffix(ult.Texto, " ") && !strings.HasPrefix(texto, " ") {
			ult.Texto += " "
		}
		ult.Texto += texto
	}
	r.marcaLen = 0
	r.finX, r.finY = fin[4], fin[5]
	return true
}

// textoConHuecos junta los glifos de una cadena y pone un espacio donde el
// espaciado entre letras (Tc) o el de palabra (Tw) abre un hueco de palabra
// DENTRO de la cadena: un rótulo que separa «PARTS» de «SOLUTION» con Tc sobre
// una sola «(SS) Tj» no tiene otro sitio donde decirlo.
func textoConHuecos(e *ejecucion, glifos []glifo) string {
	var sb strings.Builder
	tam := math.Abs(e.estado.tamano)
	for i, g := range glifos {
		sb.WriteString(g.texto)
		if i+1 == len(glifos) || tam == 0 {
			continue
		}
		hueco := e.estado.espCar
		if g.esEspacioSimple {
			hueco += e.estado.espPal
		}
		if g.alto != 0 {
			hueco = -hueco // en vertical, Tc negativo es el que separa
		}
		if hueco > emEspacio*tam && !strings.HasSuffix(g.texto, " ") && !strings.HasPrefix(glifos[i+1].texto, " ") &&
			!sinEspaciosEntrePalabras(g.texto) && !sinEspaciosEntrePalabras(glifos[i+1].texto) {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}

// sinEspaciosEntrePalabras dice si el texto es de una escritura que no separa
// palabras con espacios (chino, japonés, coreano): ahí el espaciado entre letras
// es solo espaciado, como el «中文姓名» de pdf.js issue19182.
func sinEspaciosEntrePalabras(t string) bool {
	r, _ := utf8.DecodeRuneInString(t)
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// operador lee una palabra de operador («Tj», «BT», «T*», «'», «"»).
func (lx *pdfLexer) operador() string {
	inicio := lx.pos
	for lx.pos < len(lx.buf) {
		c := lx.buf[lx.pos]
		if esEspacioPDF(c) || esDelimitadorPDF(c) {
			break
		}
		lx.pos++
	}
	if lx.pos == inicio {
		// ' y " son operadores de un solo carácter y además delimitadores para el
		// bucle de arriba; sin este caso se quedaría girando sobre ellos.
		if c := lx.byteActual(); c == '\'' || c == '"' {
			lx.pos++
			return string(c)
		}
		return ""
	}
	return string(lx.buf[inicio:lx.pos])
}

// decodificarTextoPDF pasa una cadena de PDF a UTF-8. Detecta el prefijo UTF-16BE
// (BOM FE FF) que usan los títulos y los formularios; el resto se trata como
// WinAnsi, cuyos 128 primeros códigos coinciden con ASCII.
func decodificarTextoPDF(s pdfString) string {
	if len(s) >= 2 && s[0] == 0xFE && s[1] == 0xFF {
		u := make([]uint16, 0, (len(s)-2)/2)
		for i := 2; i+1 < len(s); i += 2 {
			u = append(u, uint16(s[i])<<8|uint16(s[i+1]))
		}
		return string(utf16.Decode(u))
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for _, b := range s {
		if r, ok := winAnsiEspecial[b]; ok {
			sb.WriteRune(r)
			continue
		}
		sb.WriteRune(rune(b)) // el resto de WinAnsi coincide con Latin-1
	}
	return sb.String()
}

// winAnsiEspecial son los códigos 0x80-0x9F, donde WinAnsi (CP1252) se separa de
// Latin-1: ahí viven las comillas tipográficas y la raya, que salen en cualquier
// documento escrito en condiciones y sin esta tabla aparecerían como controles.
var winAnsiEspecial = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡',
	0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž',
	0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—',
	0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}
