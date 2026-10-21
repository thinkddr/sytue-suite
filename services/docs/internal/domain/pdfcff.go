// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CFF SUELTO (/FontFile3 /Type1C y /CIDFontType0C) envuelto en OpenType.
//
// Es como incrustan sus fuentes Acrobat Distiller, InDesign, Chrome y casi todo
// lo que pasa por la Adobe PDF Library. Un CFF suelto no lo abre ni un navegador
// ni `x/image/font/sfnt`: hay que meterlo en un contenedor OpenType (`OTTO`) con
// las tablas que piden (head, hhea, hmtx, maxp, name, OS/2, post y cmap). Sin
// esto se pintaba con la letra de reserva —otra forma, y en una fuente de
// símbolos otros glifos—: el 28 % de las páginas con estas fuentes salía mal
// contra Poppler en la medida de scripts/certipdf-compat.
//
// Se lee de la norma lo justo (Adobe TN 5176, «The Compact Font Format»): las
// INDEX de cabecera, el Top DICT, el charset (qué nombre o qué CID tiene cada
// glifo) y la codificación propia. Los contornos no se tocan: los interpreta
// quien abra el OpenType.
package domain

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

var errCFF = errors.New("pdf: CFF ilegible")

// infoCFF es lo que hace falta del CFF para envolverlo.
type infoCFF struct {
	numGlifos int
	cid       bool
	nombres   []string // fuente simple: nombre de cada glifo, por índice
	cids      []int    // fuente CID: CID de cada glifo, por índice
	propia    map[byte]int
	upem      int
}

// indiceCFF lee una INDEX (§5) y devuelve sus elementos y dónde acaba.
func indiceCFF(b []byte, pos int) ([][]byte, int, error) {
	if pos+2 > len(b) {
		return nil, 0, errCFF
	}
	n := int(binary.BigEndian.Uint16(b[pos:]))
	if n == 0 {
		return nil, pos + 2, nil
	}
	if pos+3 > len(b) {
		return nil, 0, errCFF
	}
	tam := int(b[pos+2])
	if tam < 1 || tam > 4 {
		return nil, 0, errCFF
	}
	offs := pos + 3
	datos := offs + (n+1)*tam - 1 // los desplazamientos cuentan desde 1
	if datos >= len(b) {
		return nil, 0, errCFF
	}
	leer := func(i int) int {
		v := 0
		for j := 0; j < tam; j++ {
			v = v<<8 | int(b[offs+i*tam+j])
		}
		return v
	}
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		a, z := datos+leer(i), datos+leer(i+1)
		if a < datos || z < a || z > len(b) {
			return nil, 0, errCFF
		}
		out[i] = b[a:z]
	}
	return out, datos + leer(n), nil
}

// dictCFF lee un DICT (§4): operadores de uno o dos bytes (12 x → 1200+x) con
// sus operandos delante.
func dictCFF(b []byte) map[int][]float64 {
	d := map[int][]float64{}
	pila := make([]float64, 0, 6)
	for i := 0; i < len(b); {
		c := int(b[i])
		switch {
		case c <= 21:
			op := c
			i++
			if c == 12 && i < len(b) {
				op = 1200 + int(b[i])
				i++
			}
			d[op] = pila
			pila = make([]float64, 0, 6)
		case c == 28 && i+2 < len(b):
			pila = append(pila, float64(int16(binary.BigEndian.Uint16(b[i+1:])))) //nolint:gosec // G115: el DICT guarda un entero con signo de 16 bits
			i += 3
		case c == 29 && i+4 < len(b):
			pila = append(pila, float64(int32(binary.BigEndian.Uint32(b[i+1:])))) //nolint:gosec // G115: idem, de 32 bits
			i += 5
		case c == 30:
			v, n := realCFF(b[i+1:])
			pila = append(pila, v)
			i += 1 + n
		case c >= 32 && c <= 246:
			pila = append(pila, float64(c-139))
			i++
		case c >= 247 && c <= 250 && i+1 < len(b):
			pila = append(pila, float64((c-247)*256+int(b[i+1])+108))
			i += 2
		case c >= 251 && c <= 254 && i+1 < len(b):
			pila = append(pila, float64(-(c-251)*256-int(b[i+1])-108))
			i += 2
		default:
			return d
		}
	}
	return d
}

// realCFF lee un número real de medios bytes (§4, tabla 5).
func realCFF(b []byte) (float64, int) {
	var s strings.Builder
	for i, x := range b {
		for _, n := range [2]byte{x >> 4, x & 15} {
			switch {
			case n <= 9:
				s.WriteByte('0' + n)
			case n == 0xa:
				s.WriteByte('.')
			case n == 0xb:
				s.WriteByte('E')
			case n == 0xc:
				s.WriteString("E-")
			case n == 0xe:
				s.WriteByte('-')
			case n == 0xf:
				v, err := strconv.ParseFloat(s.String(), 64)
				if err != nil {
					return 0, i + 1
				}
				return v, i + 1
			}
		}
	}
	return 0, len(b)
}

func leerCFF(b []byte) (*infoCFF, error) {
	if len(b) < 4 || b[0] != 1 {
		return nil, errCFF
	}
	pos := int(b[2])
	_, pos, err := indiceCFF(b, pos) // nombres
	if err != nil {
		return nil, err
	}
	tops, pos, err := indiceCFF(b, pos)
	if err != nil || len(tops) == 0 {
		return nil, errCFF
	}
	cadenas, _, err := indiceCFF(b, pos)
	if err != nil {
		return nil, err
	}
	top := dictCFF(tops[0])
	cs := top[17]
	if len(cs) != 1 {
		return nil, errCFF
	}
	glifos, _, err := indiceCFF(b, int(cs[0]))
	if err != nil || len(glifos) == 0 {
		return nil, errCFF
	}
	info := &infoCFF{numGlifos: len(glifos), upem: 1000}
	_, info.cid = top[1230] // ROS
	if m := top[1207]; len(m) >= 1 && m[0] > 0 && math.Abs(m[0]-0.001) > 1e-9 {
		if u := int(math.Round(1 / m[0])); u >= 16 && u <= 16384 {
			info.upem = u
		}
	}
	sids, err := charsetCFF(b, top, info.numGlifos)
	if err != nil {
		return nil, err
	}
	if info.cid {
		info.cids = sids
		return info, nil
	}
	info.nombres = make([]string, len(sids))
	for gid, sid := range sids {
		switch {
		case sid < len(cadenasEstandarCFF):
			info.nombres[gid] = cadenasEstandarCFF[sid]
		case sid-len(cadenasEstandarCFF) < len(cadenas):
			info.nombres[gid] = string(cadenas[sid-len(cadenasEstandarCFF)])
		}
	}
	info.propia = codificacionCFF(b, top, sids)
	return info, nil
}

// charsetCFF devuelve el SID (o el CID) de cada glifo (§13). El glifo 0 es
// siempre .notdef.
func charsetCFF(b []byte, top map[int][]float64, n int) ([]int, error) {
	out := make([]int, n)
	off := 0
	if v := top[15]; len(v) == 1 {
		off = int(v[0])
	}
	if off == 0 { // ISOAdobe: el glifo i es el SID i
		for i := range out {
			out[i] = i
		}
		return out, nil
	}
	if off <= 2 || off >= len(b) {
		return nil, errCFF // Expert y ExpertSubset: no aparecen en un PDF de verdad
	}
	p := off + 1
	u16 := func() int {
		if p+2 > len(b) {
			p = len(b) + 1
			return 0
		}
		v := int(binary.BigEndian.Uint16(b[p:]))
		p += 2
		return v
	}
	switch b[off] {
	case 0:
		for g := 1; g < n; g++ {
			out[g] = u16()
		}
	case 1, 2:
		for g := 1; g < n && p < len(b); {
			primero := u16()
			var quedan int
			if b[off] == 1 {
				if p >= len(b) {
					break
				}
				quedan = int(b[p])
				p++
			} else {
				quedan = u16()
			}
			for k := 0; k <= quedan && g < n; k++ {
				out[g] = primero + k
				g++
			}
		}
	default:
		return nil, errCFF
	}
	if p > len(b) {
		return nil, errCFF
	}
	return out, nil
}

// codificacionCFF es la codificación propia de una fuente simple (§12): código →
// glifo. La 0 es la estándar de Adobe; la 1 (experta) no se usa en un PDF real.
func codificacionCFF(b []byte, top map[int][]float64, sids []int) map[byte]int {
	gidDeSID := make(map[int]int, len(sids))
	for g, s := range sids {
		gidDeSID[s] = g
	}
	out := map[byte]int{}
	off := 0
	if v := top[16]; len(v) == 1 {
		off = int(v[0])
	}
	if off == 0 {
		for c, sid := range codificacionEstandarCFF {
			if g, ok := gidDeSID[sid]; ok {
				out[c] = g
			}
		}
		return out
	}
	if off == 1 || off >= len(b) {
		return out
	}
	formato, p := b[off], off+1
	if p >= len(b) {
		return out
	}
	switch formato & 0x7f {
	case 0:
		n := int(b[p])
		for g := 1; g <= n && p+g < len(b); g++ {
			out[b[p+g]] = g
		}
		p += 1 + n
	case 1:
		n, g := int(b[p]), 1
		p++
		for r := 0; r < n && p+1 < len(b); r++ {
			primero, quedan := int(b[p]), int(b[p+1])
			p += 2
			for k := 0; k <= quedan && primero+k < 256; k++ {
				out[byte((primero+k)&0xFF)] = g
				g++
			}
		}
	}
	if formato&0x80 != 0 {
		suplementosCFF(b, p, gidDeSID, out)
	}
	return out
}

// suplementosCFF añade los suplementos de la codificación: código → SID.
func suplementosCFF(b []byte, p int, gidDeSID map[int]int, out map[byte]int) {
	if p >= len(b) {
		return
	}
	n := int(b[p])
	p++
	for i := 0; i < n && p+2 < len(b); i++ {
		if g, ok := gidDeSID[int(binary.BigEndian.Uint16(b[p+1:]))]; ok {
			out[b[p]] = g
		}
		p += 3
	}
}

// envolverCFF devuelve el CFF dentro de un OpenType que abren el navegador y el
// rasterizador, con la cmap que cada uno necesita:
//
//   - Unicode → glifo, para el `<text>` del SVG y para buscar por carácter;
//   - U+F000 + código → glifo en una fuente simple, que es por donde el PNG
//     busca primero un código suelto (indiceDeGlifo);
//   - y en una compuesta, f.cidAGid: el CID no es el índice de glifo en un CFF.
func (f *fuentePDF) envolverCFF(cff []byte) ([]byte, bool) {
	info, err := leerCFF(cff)
	if err != nil || info.numGlifos > 0xFFFF {
		return nil, false
	}
	inverso := map[uint16]uint16{}
	anchos := make([]float64, info.numGlifos) // en milésimas de em
	if info.cid {
		f.cmapCID(info, inverso, anchos)
	} else {
		f.cmapSimpleCFF(info, inverso, anchos)
	}
	otf := sfntNuevo(0x4F54544F, "CFF ", cff)
	tablas := tablasMetricasCFF(info, anchos)
	for _, tag := range []string{"head", "hhea", "hmtx", "maxp", "name"} {
		var ok bool
		if otf, ok = reemplazarTablaSfnt(otf, tag, tablas[tag]); !ok {
			return nil, false
		}
	}
	otf = f.completarMetadatosTTF(otf)
	if len(inverso) == 0 {
		return otf, true
	}
	return reescribirCmapUnicodeTTF(otf, inverso)
}

// cmapCID resuelve una fuente compuesta: el código es el CID, y el charset dice
// qué glifo es cada CID.
func (f *fuentePDF) cmapCID(info *infoCFF, inverso map[uint16]uint16, anchos []float64) {
	gidDeCID := make(map[uint32]int, len(info.cids))
	mayor := 0
	for g, c := range info.cids {
		gidDeCID[uint32(c)] = g //nolint:gosec // G115: un CID cabe en 16 bits
		mayor = max(mayor, c)
	}
	// La tabla CID → glifo, en el formato de /CIDToGIDMap: dos bytes por CID.
	tabla := make([]byte, 2*(mayor+1))
	for c, g := range gidDeCID {
		binary.BigEndian.PutUint16(tabla[2*c:], uint16(g)) //nolint:gosec // G115: g < numGlifos ≤ 0xFFFF
	}
	f.cidAGid = tabla
	for c, t := range f.aUnicode {
		r := []rune(t)
		if g, ok := gidDeCID[c]; ok && len(r) == 1 && r[0] <= 0xFFFF {
			inverso[uint16(r[0])] = uint16(g) //nolint:gosec // G115: comprobado arriba
		}
	}
	for c, w := range f.anchos {
		if g, ok := gidDeCID[c]; ok {
			anchos[g] = w
		}
	}
}

// cmapSimpleCFF resuelve una fuente simple: cada código se busca por la letra que
// el PDF dice que es, y si no casa, por la codificación propia del CFF.
func (f *fuentePDF) cmapSimpleCFF(info *infoCFF, inverso map[uint16]uint16, anchos []float64) {
	gidDeRuna := map[rune]int{}
	for g, n := range info.nombres {
		r, ok := runaDeNombreGlifo(n)
		if !ok {
			r, ok = nombresAcentuados[n]
		}
		if _, ya := gidDeRuna[r]; ok && !ya {
			gidDeRuna[r] = g
		}
	}
	for c := 0; c < 256; c++ {
		gl := f.glifoDe(uint32(c), false)
		r := primeraRuna(gl.texto)
		g, ok := gidDeRuna[r]
		if !ok || f.simbolica {
			// Sin nombre que case —o una fuente de símbolos, cuyos códigos solo
			// entiende su propia codificación—: la del CFF.
			if gp, okp := info.propia[byte(c)]; okp {
				g, ok = gp, true
			}
		}
		if !ok {
			continue
		}
		inverso[uint16(0xF000+c)] = uint16(g)                                        //nolint:gosec // G115: g < numGlifos ≤ 0xFFFF
		if _, ya := inverso[uint16(r)]; !ya && r > 0 && r <= 0xFFFF && r != 0xFFFD { //nolint:gosec // G115: r ≤ 0xFFFF
			inverso[uint16(r)] = uint16(g) //nolint:gosec // G115: idem
		}
		anchos[g] = gl.ancho
	}
}

// sfntNuevo arma un sfnt de una sola tabla, para ir añadiendo las demás con
// reemplazarTablaSfnt.
func sfntNuevo(version uint32, tag string, datos []byte) []byte {
	be := binary.BigEndian
	out := make([]byte, 28, 28+len(datos)+4)
	be.PutUint32(out[0:], version)
	be.PutUint16(out[4:], 1)  // una tabla
	be.PutUint16(out[6:], 16) // searchRange
	copy(out[12:16], tag)
	be.PutUint32(out[16:], sumaSfnt(datos))
	be.PutUint32(out[20:], 28)
	be.PutUint32(out[24:], uint32(len(datos))) //nolint:gosec // G115: acotado por maxFuenteIncrustada
	return append(out, datos...)
}

// tablasMetricasCFF son las tablas mínimas de un OpenType con contornos CFF: los
// avances salen de las anchuras del PDF, que son las que mandan al pintar.
func tablasMetricasCFF(info *infoCFF, anchos []float64) map[string][]byte {
	be := binary.BigEndian
	u := float64(info.upem)
	n := info.numGlifos
	head := make([]byte, 54)
	be.PutUint32(head[0:], 0x00010000)
	be.PutUint32(head[12:], 0x5F0F3CF5)
	be.PutUint16(head[16:], 3)
	be.PutUint16(head[18:], uint16(info.upem)) //nolint:gosec // G115: 16..16384
	be.PutUint16(head[44:], 0)                 // macStyle
	be.PutUint16(head[46:], 8)
	be.PutUint16(head[48:], 2)
	hmtx := make([]byte, 4*n)
	maxAvance := 0
	for g, w := range anchos {
		a := int(math.Round(w / 1000 * u))
		if a < 0 || a > 0xFFFF {
			a = 0
		}
		maxAvance = max(maxAvance, a)
		be.PutUint16(hmtx[4*g:], uint16(a)) //nolint:gosec // G115: acotado arriba
	}
	sube, baja := int16(0.8*u), int16(-0.2*u)
	hhea := make([]byte, 36)
	be.PutUint32(hhea[0:], 0x00010000)
	be.PutUint16(hhea[4:], uint16(sube))       //nolint:gosec // G115: hhea guarda int16 en esos dos campos
	be.PutUint16(hhea[6:], uint16(baja))       //nolint:gosec // G115: idem
	be.PutUint16(hhea[10:], uint16(maxAvance)) //nolint:gosec // G115: ≤ 0xFFFF
	be.PutUint16(hhea[18:], 1)                 // caretSlopeRise
	be.PutUint16(hhea[34:], uint16(n))         //nolint:gosec // G115: n ≤ 0xFFFF, comprobado al leer
	maxp := make([]byte, 6)
	be.PutUint32(maxp[0:], 0x00005000) // versión 0.5: la de los contornos CFF
	be.PutUint16(maxp[4:], uint16(n))  //nolint:gosec // G115: n ≤ 0xFFFF
	return map[string][]byte{"head": head, "hhea": hhea, "hmtx": hmtx, "maxp": maxp, "name": tablaNombre("SytuePDF")}
}

// tablaNombre es una tabla name con la familia (1) y el nombre PostScript (6), en
// la plataforma de Windows: lo mínimo que el saneador del navegador acepta.
func tablaNombre(nombre string) []byte {
	be := binary.BigEndian
	texto := utf16.Encode([]rune(nombre))
	cadena := make([]byte, 2*len(texto))
	for i, c := range texto {
		be.PutUint16(cadena[2*i:], c)
	}
	out := make([]byte, 6+2*12+len(cadena))
	copy(out[6+2*12:], cadena)
	be.PutUint16(out[2:], 2)
	be.PutUint16(out[4:], 6+2*12)
	for i, id := range []uint16{1, 6} {
		r := out[6+12*i:]
		be.PutUint16(r[0:], 3)
		be.PutUint16(r[2:], 1)
		be.PutUint16(r[4:], 0x409)
		be.PutUint16(r[6:], id)
		be.PutUint16(r[8:], uint16(len(cadena))) //nolint:gosec // G115: nombre corto
		be.PutUint16(r[10:], 0)                  // las dos comparten la cadena
	}
	return out
}

// cadenasEstandarCFF son los 391 nombres que todo CFF trae sin escribirlos
// (TN 5176, apéndice A): un SID menor que 391 es uno de estos.
var cadenasEstandarCFF = strings.Fields(`
.notdef space exclam quotedbl numbersign dollar percent ampersand quoteright parenleft parenright
asterisk plus comma hyphen period slash zero one two three four five six seven eight nine colon
semicolon less equal greater question at A B C D E F G H I J K L M N O P Q R S T U V W X Y Z
bracketleft backslash bracketright asciicircum underscore quoteleft a b c d e f g h i j k l m n o p
q r s t u v w x y z braceleft bar braceright asciitilde exclamdown cent sterling fraction yen florin
section currency quotesingle quotedblleft guillemotleft guilsinglleft guilsinglright fi fl endash
dagger daggerdbl periodcentered paragraph bullet quotesinglbase quotedblbase quotedblright
guillemotright ellipsis perthousand questiondown grave acute circumflex tilde macron breve dotaccent
dieresis ring cedilla hungarumlaut ogonek caron emdash AE ordfeminine Lslash Oslash OE ordmasculine
ae dotlessi lslash oslash oe germandbls onesuperior logicalnot mu trademark Eth onehalf plusminus
Thorn onequarter divide brokenbar degree thorn threequarters twosuperior registered minus eth
multiply threesuperior copyright Aacute Acircumflex Adieresis Agrave Aring Atilde Ccedilla Eacute
Ecircumflex Edieresis Egrave Iacute Icircumflex Idieresis Igrave Ntilde Oacute Ocircumflex Odieresis
Ograve Otilde Scaron Uacute Ucircumflex Udieresis Ugrave Yacute Ydieresis Zcaron aacute acircumflex
adieresis agrave aring atilde ccedilla eacute ecircumflex edieresis egrave iacute icircumflex
idieresis igrave ntilde oacute ocircumflex odieresis ograve otilde scaron uacute ucircumflex
udieresis ugrave yacute ydieresis zcaron exclamsmall Hungarumlautsmall dollaroldstyle dollarsuperior
ampersandsmall Acutesmall parenleftsuperior parenrightsuperior twodotenleader onedotenleader
zerooldstyle oneoldstyle twooldstyle threeoldstyle fouroldstyle fiveoldstyle sixoldstyle
sevenoldstyle eightoldstyle nineoldstyle commasuperior threequartersemdash periodsuperior
questionsmall asuperior bsuperior centsuperior dsuperior esuperior isuperior lsuperior msuperior
nsuperior osuperior rsuperior ssuperior tsuperior ff ffi ffl parenleftinferior parenrightinferior
Circumflexsmall hyphensuperior Gravesmall Asmall Bsmall Csmall Dsmall Esmall Fsmall Gsmall Hsmall
Ismall Jsmall Ksmall Lsmall Msmall Nsmall Osmall Psmall Qsmall Rsmall Ssmall Tsmall Usmall Vsmall
Wsmall Xsmall Ysmall Zsmall colonmonetary onefitted rupiah Tildesmall exclamdownsmall centoldstyle
Lslashsmall Scaronsmall Zcaronsmall Dieresissmall Brevesmall Caronsmall Dotaccentsmall Macronsmall
figuredash hypheninferior Ogoneksmall Ringsmall Cedillasmall questiondownsmall oneeighth
threeeighths fiveeighths seveneighths onethird twothirds zerosuperior foursuperior fivesuperior
sixsuperior sevensuperior eightsuperior ninesuperior zeroinferior oneinferior twoinferior
threeinferior fourinferior fiveinferior sixinferior seveninferior eightinferior nineinferior
centinferior dollarinferior periodinferior commainferior Agravesmall Aacutesmall Acircumflexsmall
Atildesmall Adieresissmall Aringsmall AEsmall Ccedillasmall Egravesmall Eacutesmall Ecircumflexsmall
Edieresissmall Igravesmall Iacutesmall Icircumflexsmall Idieresissmall Ethsmall Ntildesmall
Ogravesmall Oacutesmall Ocircumflexsmall Otildesmall Odieresissmall OEsmall Oslashsmall Ugravesmall
Uacutesmall Ucircumflexsmall Udieresissmall Yacutesmall Thornsmall Ydieresissmall 001.000 001.001
001.002 001.003 Black Bold Book Light Medium Regular Roman Semibold`)

// codificacionEstandarCFF es la codificación estándar de Adobe (TN 5176,
// apéndice B), código → SID: la que vale cuando el CFF no trae la suya.
var codificacionEstandarCFF = map[byte]int{
	0x20: 1, 0x21: 2, 0x22: 3, 0x23: 4, 0x24: 5, 0x25: 6, 0x26: 7, 0x27: 8, 0x28: 9, 0x29: 10,
	0x2A: 11, 0x2B: 12, 0x2C: 13, 0x2D: 14, 0x2E: 15, 0x2F: 16, 0x30: 17, 0x31: 18, 0x32: 19,
	0x33: 20, 0x34: 21, 0x35: 22, 0x36: 23, 0x37: 24, 0x38: 25, 0x39: 26, 0x3A: 27, 0x3B: 28,
	0x3C: 29, 0x3D: 30, 0x3E: 31, 0x3F: 32, 0x40: 33, 0x41: 34, 0x42: 35, 0x43: 36, 0x44: 37,
	0x45: 38, 0x46: 39, 0x47: 40, 0x48: 41, 0x49: 42, 0x4A: 43, 0x4B: 44, 0x4C: 45, 0x4D: 46,
	0x4E: 47, 0x4F: 48, 0x50: 49, 0x51: 50, 0x52: 51, 0x53: 52, 0x54: 53, 0x55: 54, 0x56: 55,
	0x57: 56, 0x58: 57, 0x59: 58, 0x5A: 59, 0x5B: 60, 0x5C: 61, 0x5D: 62, 0x5E: 63, 0x5F: 64,
	0x60: 65, 0x61: 66, 0x62: 67, 0x63: 68, 0x64: 69, 0x65: 70, 0x66: 71, 0x67: 72, 0x68: 73,
	0x69: 74, 0x6A: 75, 0x6B: 76, 0x6C: 77, 0x6D: 78, 0x6E: 79, 0x6F: 80, 0x70: 81, 0x71: 82,
	0x72: 83, 0x73: 84, 0x74: 85, 0x75: 86, 0x76: 87, 0x77: 88, 0x78: 89, 0x79: 90, 0x7A: 91,
	0x7B: 92, 0x7C: 93, 0x7D: 94, 0x7E: 95, 0xA1: 96, 0xA2: 97, 0xA3: 98, 0xA4: 99, 0xA5: 100,
	0xA6: 101, 0xA7: 102, 0xA8: 103, 0xA9: 104, 0xAA: 105, 0xAB: 106, 0xAC: 107, 0xAD: 108, 0xAE: 109,
	0xAF: 110, 0xB1: 111, 0xB2: 112, 0xB3: 113, 0xB4: 114, 0xB6: 115, 0xB7: 116, 0xB8: 117, 0xB9: 118,
	0xBA: 119, 0xBB: 120, 0xBC: 121, 0xBD: 122, 0xBF: 123, 0xC1: 124, 0xC2: 125, 0xC3: 126, 0xC4: 127,
	0xC5: 128, 0xC6: 129, 0xC7: 130, 0xC8: 131, 0xCA: 132, 0xCB: 133, 0xCD: 134, 0xCE: 135, 0xCF: 136,
	0xD0: 137, 0xE1: 138, 0xE3: 139, 0xE8: 140, 0xE9: 141, 0xEA: 142, 0xEB: 143, 0xF1: 144, 0xF5: 145,
	0xF8: 146, 0xF9: 147, 0xFA: 148, 0xFB: 149,
}

// nombresAcentuados completa runaDeNombreGlifo con las letras acentuadas y los
// signos de la lista estándar, que en un CFF llegan por su nombre.
var nombresAcentuados = map[string]rune{
	"AE": 'Æ', "Aacute": 'Á', "Acircumflex": 'Â', "Adieresis": 'Ä', "Agrave": 'À', "Aring": 'Å',
	"Atilde": 'Ã', "Ccedilla": 'Ç', "Eacute": 'É', "Ecircumflex": 'Ê', "Edieresis": 'Ë',
	"Egrave": 'È', "Eth": 'Ð', "Iacute": 'Í', "Icircumflex": 'Î', "Idieresis": 'Ï', "Igrave": 'Ì',
	"Lslash": 'Ł', "Ntilde": 'Ñ', "OE": 'Œ', "Oacute": 'Ó', "Ocircumflex": 'Ô', "Odieresis": 'Ö',
	"Ograve": 'Ò', "Oslash": 'Ø', "Otilde": 'Õ', "Scaron": 'Š', "Thorn": 'Þ', "Uacute": 'Ú',
	"Ucircumflex": 'Û', "Udieresis": 'Ü', "Ugrave": 'Ù', "Yacute": 'Ý', "Ydieresis": 'Ÿ',
	"Zcaron": 'Ž', "aacute": 'á', "acircumflex": 'â', "acute": '´', "adieresis": 'ä', "ae": 'æ',
	"agrave": 'à', "aring": 'å', "atilde": 'ã', "breve": '˘', "brokenbar": '¦', "caron": 'ˇ',
	"ccedilla": 'ç', "cedilla": '¸', "circumflex": 'ˆ', "dieresis": '¨', "dotaccent": '˙',
	"dotlessi": 'ı', "eacute": 'é', "ecircumflex": 'ê', "edieresis": 'ë', "egrave": 'è', "eth": 'ð',
	"hungarumlaut": '˝', "iacute": 'í', "icircumflex": 'î', "idieresis": 'ï', "igrave": 'ì',
	"lslash": 'ł', "macron": '¯', "mu": 'µ', "ntilde": 'ñ', "oacute": 'ó', "ocircumflex": 'ô',
	"odieresis": 'ö', "oe": 'œ', "ogonek": '˛', "ograve": 'ò', "onehalf": '½', "onequarter": '¼',
	"onesuperior": '¹', "oslash": 'ø', "otilde": 'õ', "ring": '˚', "scaron": 'š', "thorn": 'þ',
	"threequarters": '¾', "threesuperior": '³', "tilde": '˜', "twosuperior": '²', "uacute": 'ú',
	"ucircumflex": 'û', "udieresis": 'ü', "ugrave": 'ù', "yacute": 'ý', "ydieresis": 'ÿ',
	"zcaron": 'ž',
}
