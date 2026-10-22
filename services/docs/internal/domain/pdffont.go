// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Fuentes para el render de PDF a SVG: qué anchura tiene cada código, qué texto
// representa y con qué tipografía se pinta.
//
// Hay dos cosas distintas que resolver y conviene no confundirlas:
//
//  1. QUÉ SE LEE. El PDF guarda códigos, no caracteres. /ToUnicode dice qué
//     carácter es cada código; sin él se cae a WinAnsi. De esto dependen la
//     selección, el copiar y la búsqueda.
//  2. QUÉ ANCHO OCUPA. Sale de /Widths (fuentes simples) o /W (compuestas). Es
//     lo que fija `textLength` en el SVG, y por eso el texto cae donde el PDF
//     dice aunque el navegador acabe usando otra tipografía.
//
// LA FUENTE INCRUSTADA. Un PDF trae la tipografía dentro (/FontFile2 TrueType,
// /FontFile3 OpenType) y se reemite como `@font-face` con un `data:`. Ahí surge
// el problema de siempre: en una fuente compuesta con codificación Identity-H
// los códigos son ÍNDICES DE GLIFO, no caracteres, y su tabla `cmap` suele venir
// vacía porque el PDF nunca la necesita. Si se le pasa texto Unicode al
// navegador, no encuentra nada y pinta cuadritos.
//
// La solución es reconstruir esa `cmap` invirtiendo /ToUnicode: se sabe qué
// carácter corresponde a cada glifo, así que se puede escribir la tabla que el
// navegador espera. Con eso, un PDF de InDesign o de LaTeX —donde casi todo es
// Identity-H— se ve con SU tipografía y además se puede copiar.
//
// El CFF suelto (/FontFile3 /Type1C o /CIDFontType0C) se envuelve en un
// contenedor OpenType (pdfcff.go), y el Type1 antiguo (/FontFile) se convierte
// antes en CFF (pdftype1.go). Lo que no se puede leer se sustituye por la familia
// genérica que más se le parezca, y `textLength` mantiene la línea cuadrada
// aunque los glifos no sean idénticos.
package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
)

// Topes de los identificadores que llegan del fichero. Un PDF manipulado puede
// declarar un CID de 2^40 o un punto de código imposible; acotarlos aquí evita
// que la conversión al tipo pequeño coloque el dato sobre OTRO glifo en silencio.
const (
	maxCID  = 0xFFFF
	maxRuna = 0x10FFFF
)

// glifo es un código ya resuelto: qué texto representa y cuánto avanza.
type glifo struct {
	texto string
	// codigo es el código tal y como venía en la cadena del PDF. En una fuente
	// compuesta ES el CID, que es por donde se llega al contorno sin pasar por
	// Unicode; el SVG no lo necesita porque el navegador busca por carácter.
	codigo uint32
	ancho  float64 // en milésimas de em, como los guarda el PDF
	// alto es el desplazamiento vertical (w1 de /W2 o /DW2, normalmente −1000)
	// en una fuente de escritura vertical; vx y vy colocan el origen vertical
	// respecto al horizontal (§9.2.4, §9.7.4.3).
	alto, vx, vy float64
	// esEspacioSimple marca el código 32 de UN byte, el único al que se le aplica
	// el espaciado de palabra (§9.3.3). En una fuente de dos bytes NO se aplica,
	// y aplicarlo separa las palabras el doble.
	esEspacioSimple bool
}

type fuentePDF struct {
	familia          string
	negrita, italica bool

	anchos   map[uint32]float64
	anchoDef float64
	// vertical es la escritura vertical (CMap «-V» o /WMode 1, §9.7.4.3): cada
	// glifo baja en vez de avanzar, lo que dicen /W2 y /DW2.
	vertical bool
	altos    map[uint32]float64
	vx, vy   map[uint32]float64
	altoDef  float64
	vyDef    float64
	aUnicode map[uint32]string
	dosBytes bool
	// subconjunto marca la fuente cuyo nombre lleva el prefijo «ABCDEF+», que
	// significa que el PDF solo incrustó los glifos QUE YA SE USABAN. Escribir en
	// ella una letra que el documento no tenía no dibuja nada, así que el editor
	// tiene que avisar antes en vez de dejar un hueco.
	subconjunto bool
	// cmap es la CMap de /Encoding de una compuesta cuando no es Identity
	// (pdfcmap.go); nil significa códigos de dos bytes que son el CID.
	cmap *cmapCodigos
	// unicodeIdentidad: la /ToUnicode dice «Identity» (por nombre, o con una
	// CMap de CIDs en vez de caracteres): el código es el carácter.
	unicodeIdentidad bool
	// gidARuna es el respaldo de una compuesta sin /ToUnicode útil: el
	// carácter de cada glifo según la `cmap` de su propia TrueType.
	gidARuna map[uint16]rune
	simple   map[byte]string
	// type3, si no es nil, son los DIBUJOS de una fuente de tipo 3: ahí cada
	// glifo es un flujo de contenido y no un contorno. Ver pdftype3.go.
	type3 *type3

	// Lo que sigue solo lo usa la salida PNG (`pdfraster.go`): el programa de la
	// tipografía tal y como el SVG lo incrusta, ya parseado, y la tabla que lleva
	// del CID al índice de glifo cuando no es la identidad.
	programa  []byte
	cara      *sfnt.Font
	caraLista bool
	reserva   bool // la cara es una de reserva, no la del documento
	cidAGid   []byte
	// cmapUnicode: la tipografía trae una tabla por carácter; si no, sus
	// tablas van por código (ver `indiceDeGlifo`).
	cmapUnicode bool
	cmapVisual  bool // SVG pinta los códigos simbólicos sin confundir glifos con texto
	simbolica   bool // una fuente simple simbólica se consulta por código, no por Unicode
	// cmapMacRoman permite consultar por carácter una fuente Macintosh normal.
	// Una simbólica de Word con códigos renumerados conserva la consulta por código.
	cmapMacRoman bool
}

// esSubconjunto reconoce el prefijo de seis mayúsculas y un «+» que la
// especificación reserva para los subconjuntos (§9.6.4).
func esSubconjunto(nombre string) bool {
	if len(nombre) < 8 || nombre[6] != '+' {
		return false
	}
	for i := 0; i < 6; i++ {
		if nombre[i] < 'A' || nombre[i] > 'Z' {
			return false
		}
	}
	return true
}

// fuenteDeReserva es lo que se usa cuando el flujo pinta texto sin haber
// seleccionado fuente. Es un PDF roto, pero pintar el texto con una métrica
// razonable es mejor que amontonarlo todo en el mismo punto.
func fuenteDeReserva() *fuentePDF {
	return &fuentePDF{familia: "sans-serif", anchoDef: 500, anchos: map[uint32]float64{}}
}

// decodificar parte la cadena en códigos y resuelve texto y anchura de cada uno.
func (f *fuentePDF) decodificar(s pdfString) []glifo {
	out := make([]glifo, 0, len(s))
	if f.cmap != nil {
		for i := 0; i < len(s); {
			c, n := f.cmap.siguiente(s[i:])
			// §9.3.3: el espaciado de palabra va al código 32 de UN byte también
			// en una compuesta cuya CMap lo define así.
			out = append(out, f.glifoCompuesto(c, f.cmap.cid(c, n), n, n == 1 && c == 32))
			i += n
		}
		return out
	}
	if f.dosBytes {
		for i := 0; i+1 < len(s); i += 2 {
			c := uint32(s[i])<<8 | uint32(s[i+1])
			out = append(out, f.glifoCompuesto(c, c, 2, false))
		}
		if len(s)%2 == 1 { // byte suelto al final: se toma como código corto
			c := uint32(s[len(s)-1])
			out = append(out, f.glifoCompuesto(c, c, 1, false))
		}
		return out
	}
	for _, b := range s {
		out = append(out, f.glifoDe(uint32(b), b == 32))
	}
	return out
}

// codificar es lo contrario de decodificar: los códigos de ESTA fuente que se
// leen como `texto`. Sirve para escribir en un flujo de contenido con la fuente
// que ya usa la página.
//
// Falla con la primera letra que la fuente no puede pintar: en una compuesta,
// la que no esté en su /ToUnicode; en un subconjunto, la que no tenga anchura
// (el generador no incrustó su glifo). Mejor decirlo que escribir un hueco.
func (f *fuentePDF) codificar(texto string) (pdfString, error) {
	inversa := f.tablaInversa()
	out := make(pdfString, 0, len(texto)*2)
	for _, r := range texto {
		c, ok := f.codigoDeRuna(r, inversa)
		if !ok && (r == '\u200b' || len(inversa) == 0) {
			return nil, ErrLetrasDesconocidas
		}
		if !ok {
			return nil, fmt.Errorf("pdf: la fuente de ese texto no tiene la letra «%c»", r)
		}
		switch {
		case f.cmap != nil:
			for i := f.cmap.largo(c) - 1; i >= 0; i-- {
				out = append(out, byte((c>>(8*i))&0xFF))
			}
		case f.dosBytes:
			out = append(out, byte((c>>8)&0xFF), byte(c&0xFF))
		default:
			out = append(out, byte(c&0xFF))
		}
	}
	return out, nil
}

// tablaInversa construye el mapa carácter → código para codificar con esta fuente.
func (f *fuentePDF) tablaInversa() map[rune]uint32 {
	inversa := map[rune]uint32{}
	apuntar := func(c uint32, t string) {
		r, n := utf8.DecodeRuneInString(t)
		if n != len(t) || r == utf8.RuneError {
			return // ligaduras y vacíos: no son una letra
		}
		if _, ya := inversa[r]; !ya {
			inversa[r] = c
		}
	}
	if !f.dosBytes {
		for c := uint32(0); c < 256; c++ {
			if f.subconjunto && c != ' ' && f.anchos[c] <= 0 {
				continue
			}
			if f.type3 != nil && c != ' ' && f.type3.procs[f.type3.codificacion[byte(c)]] == nil {
				continue
			}
			apuntar(c, f.glifoDe(c, false).texto)
		}
		return inversa
	}
	codigos := make([]uint32, 0, len(f.aUnicode))
	for c := range f.aUnicode {
		codigos = append(codigos, c)
	}
	slices.Sort(codigos) // el menor código gana, siempre el mismo
	for _, c := range codigos {
		if c <= 0xFFFF || f.cmap != nil {
			apuntar(c, f.aUnicode[c])
		}
	}
	f.apuntarInversaCID(apuntar)
	return inversa
}

// apuntarInversaCID completa el mapa carácter → código con la `cmap` de la
// TrueType incrustada (runaDeCID) y la CMap de /Encoding cuando no había /ToUnicode.
func (f *fuentePDF) apuntarInversaCID(apuntar func(uint32, string)) {
	if len(f.gidARuna) == 0 {
		return
	}
	if f.cmap != nil && (len(f.cmap.cids) > 0 || len(f.cmap.rangos) > 0) {
		cods := make([]uint32, 0, len(f.cmap.cids))
		for c := range f.cmap.cids {
			cods = append(cods, c)
		}
		slices.Sort(cods)
		for _, c := range cods {
			if r, ok := f.runaDeCID(f.cmap.cids[c]); ok {
				apuntar(c, string(r))
			}
		}
		for _, rg := range f.cmap.rangos {
			for c := rg.desde; c <= rg.hasta && c-rg.desde < 70000; c++ {
				if r, ok := f.runaDeCID(rg.base + c - rg.desde); ok {
					apuntar(c, string(r))
				}
			}
		}
		return
	}
	if len(f.cidAGid) >= 2 {
		for cid := uint32(0); int(2*cid+1) < len(f.cidAGid); cid++ {
			if r, ok := f.runaDeCID(cid); ok {
				apuntar(cid, string(r))
			}
		}
		return
	}
	gids := make([]int, 0, len(f.gidARuna))
	for g := range f.gidARuna {
		gids = append(gids, int(g))
	}
	slices.Sort(gids)
	for _, g := range gids {
		apuntar(uint32(g), string(f.gidARuna[uint16(g)])) //nolint:gosec // G115: g proviene de uint16
	}
}

func (f *fuentePDF) codigoDeRuna(r rune, inversa map[rune]uint32) (uint32, bool) {
	if c, ok := inversa[r]; ok {
		return c, true
	}
	if !f.dosBytes {
		return 0, false
	}
	if f.cmap != nil && f.cmap.unicode != "" {
		return f.cmap.codigoUnicode(r)
	}
	if f.unicodeIdentidad && r > 0 && (r <= 0xFFFF || f.cmap != nil) && (r < 0xD800 || r > 0xDFFF) {
		return uint32(r), true
	}
	return 0, false
}

// glifoCompuesto resuelve un código de una fuente compuesta: la anchura va por
// CID y el texto por código (/ToUnicode se indexa por código, no por CID).
func (f *fuentePDF) glifoCompuesto(codigo, cid uint32, n int, espacio bool) glifo {
	g := glifo{ancho: f.anchoDef, esEspacioSimple: espacio, codigo: cid}
	if w, ok := f.anchos[cid]; ok {
		g.ancho = w
	}
	if f.vertical {
		g.alto, g.vx, g.vy = f.altoDef, g.ancho/2, f.vyDef
		if h, ok := f.altos[cid]; ok {
			g.alto = h
		}
		if x, ok := f.vx[cid]; ok {
			g.vx = x
		}
		if y, ok := f.vy[cid]; ok {
			g.vy = y
		}
	}
	if t, ok := f.aUnicode[codigo]; ok {
		g.texto = t
		return g
	}
	if f.cmap != nil && f.cmap.unicode != "" {
		if t := f.cmap.textoUnicode(codigo, n); t != "" {
			g.texto = t
			return g
		}
	}
	if f.unicodeIdentidad && codigo > 0 && codigo <= maxRuna && (codigo < 0xD800 || codigo > 0xDFFF) {
		g.texto = string(rune(codigo))
		return g
	}
	if r, ok := f.runaDeCID(cid); ok {
		g.texto = string(r)
		return g
	}
	if len(f.aUnicode) > 0 && g.ancho == 0 {
		// Un CID de anchura 0 ausente del /ToUnicode de una fuente que sí lo trae
		// es una marca combinante posicionada sobre la base; emitir U+200B dentro
		// de la palabra partiría la búsqueda del texto base.
		return g
	}
	// Sin /ToUnicode ni tipografía que lo diga no hay forma honesta de saber qué
	// letra es. Se emite un espacio de anchura cero para no inventar texto en una
	// selección, pero el hueco sí avanza y la línea siguiente no se descoloca.
	g.texto = "\u200b"
	return g
}

// runaDeCID busca el carácter de un CID en la `cmap` de la propia TrueType:
// CID → glifo (por /CIDToGIDMap) → carácter.
func (f *fuentePDF) runaDeCID(cid uint32) (rune, bool) {
	if len(f.gidARuna) == 0 {
		return 0, false
	}
	gid := cid
	if len(f.cidAGid) > 0 {
		if int(2*cid+1) >= len(f.cidAGid) {
			return 0, false
		}
		gid = uint32(f.cidAGid[2*cid])<<8 | uint32(f.cidAGid[2*cid+1])
	}
	if gid > 0xFFFF {
		return 0, false
	}
	r, ok := f.gidARuna[uint16(gid)]
	return r, ok
}

func (f *fuentePDF) glifoDe(c uint32, espacio bool) glifo {
	if f.dosBytes {
		return f.glifoCompuesto(c, c, 2, espacio)
	}
	g := glifo{ancho: f.anchoDef, esEspacioSimple: espacio, codigo: c}
	if w, ok := f.anchos[c]; ok {
		g.ancho = w
	}
	if t, ok := f.aUnicode[c]; ok {
		g.texto = t
		return g
	}
	codigo := byte(c & 0xFF)
	if f.type3 != nil && f.type3.codificacion[codigo] == "" && g.ancho == 0 {
		return g
	}
	if t, ok := f.simple[codigo]; ok {
		g.texto = t
		return g
	}
	g.texto = decodificarTextoPDF(pdfString{codigo})
	return g
}

// --- Construcción ------------------------------------------------------------

func (l *lienzo) fuente(nombre pdfName, recursos pdfDict) *fuentePDF {
	if recursos == nil {
		return fuenteDeReserva()
	}
	cat, _ := l.d.resolver(recursos["Font"]).(pdfDict)
	if cat == nil {
		return fuenteDeReserva()
	}
	ref, ok := cat[nombre]
	if !ok {
		return fuenteDeReserva()
	}
	fd, ok := l.d.resolver(ref).(pdfDict)
	if !ok {
		return fuenteDeReserva()
	}
	return l.fuenteDeDict(fd, refClave(ref))
}

func (l *lienzo) fuenteDeDict(fd pdfDict, clave string) *fuentePDF {
	if clave != "" {
		if f, ok := l.fuentes[clave]; ok {
			return f
		}
	}
	f := l.construir(fd)
	if clave != "" {
		l.fuentes[clave] = f
	}
	return f
}

func (l *lienzo) construir(fd pdfDict) *fuentePDF {
	f := &fuentePDF{anchos: map[uint32]float64{}, aUnicode: map[uint32]string{}, anchoDef: 500}
	sub, _ := l.d.resolver(fd["Subtype"]).(pdfName)
	base, _ := l.d.resolver(fd["BaseFont"]).(pdfName)
	nombreBase := string(base)

	descendiente := fd
	if sub == "Type0" {
		f.dosBytes = true
		if arr, ok := l.d.resolver(fd["DescendantFonts"]).(pdfArray); ok && len(arr) > 0 {
			if dd, ok := l.d.resolver(arr[0]).(pdfDict); ok {
				descendiente = dd
			}
		}
		// Una CMap que no sea Identity parte la cadena en códigos de uno a cuatro
		// bytes y les da su CID (pdfcmap.go).
		switch v := l.d.resolver(fd["Encoding"]).(type) {
		case pdfName:
			f.cmap = cmapPredefinida(string(v))
			f.vertical = strings.HasSuffix(string(v), "-V")
		case *pdfStream:
			f.cmap = l.d.leerCMapCodigos(v)
			wm, _ := comoEntero(l.d.resolver(v.Dict["WMode"]))
			f.vertical = wm == 1
		}
		l.anchosCID(descendiente, f)
		if f.vertical {
			l.altosCID(descendiente, f)
		}
		if st, ok := l.d.resolver(descendiente["CIDToGIDMap"]).(*pdfStream); ok {
			if b, err := l.d.datosDeFlujo(st); err == nil {
				f.cidAGid = b
			}
		}
	} else {
		f.simple = l.codificacionSimple(fd, nombreBase, sub)
		l.anchosSimples(fd, f)
		// Una fuente de tipo 3 no tiene contornos: sus glifos son flujos de
		// contenido y hay que EJECUTARLOS. Se lee aquí para que `mostrar` pueda
		// bifurcar sin volver a resolver el diccionario.
		f.type3 = l.leerType3(fd)
	}

	l.leerToUnicode(fd, f)

	desc, _ := l.d.resolver(descendiente["FontDescriptor"]).(pdfDict)
	if f.dosBytes && len(f.aUnicode) == 0 && !f.unicodeIdentidad && (f.cmap == nil || f.cmap.unicode == "") {
		f.gidARuna = l.runasDeLaTrueType(desc)
	}
	f.subconjunto = esSubconjunto(nombreBase)
	f.negrita, f.italica = rasgos(nombreBase, l, desc)
	if desc != nil {
		flags, _ := comoEntero(l.d.resolver(desc["Flags"]))
		f.simbolica = flags&4 != 0 && !f.dosBytes
	}
	// Para sacar el texto no hace falta el programa de la fuente —el carácter
	// sale de /ToUnicode, /Encoding y /Widths—, y prepararlo (completar sus
	// tablas, rehacer su cmap) era casi todo el coste: un sumario del BOE pasaba
	// de 30 ms a varios segundos por página al extraer con el intérprete.
	if l.texto == nil {
		f.familia = l.incrustar(desc, f, sub)
	}
	if f.familia == "" {
		f.familia = familiaSustituta(nombreBase, l, desc)
	}
	return f
}

// runasDeLaTrueType invierte la `cmap` Unicode de la TrueType incrustada: el
// carácter de cada glifo (el menor, si dos comparten glifo). Es el último
// recurso de una compuesta sin /ToUnicode: medido en el corpus, sin él salían
// páginas enteras vacías de texto que la tipografía sí sabe leer.
func (l *lienzo) runasDeLaTrueType(desc pdfDict) map[uint16]rune {
	if desc == nil {
		return nil
	}
	st, ok := l.d.resolver(desc["FontFile2"]).(*pdfStream)
	if !ok {
		return nil
	}
	b, err := l.d.datosDeFlujo(st)
	if err != nil || len(b) > maxFuenteIncrustada || !tieneCmapUnicode(b) {
		return nil
	}
	cara, err := sfnt.Parse(b)
	if err != nil {
		return nil
	}
	var buf sfnt.Buffer
	out := map[uint16]rune{}
	for r := rune(0x20); r <= 0xFFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF || r >= 0xE000 && r <= 0xF8FF {
			continue // sustitutos y uso privado: no son texto
		}
		gid, err := cara.GlyphIndex(&buf, r)
		if err != nil || gid == 0 {
			continue
		}
		if _, ya := out[uint16(gid)]; !ya {
			out[uint16(gid)] = r
		}
	}
	return out
}

// anchosSimples lee /FirstChar + /Widths, que es un array de anchuras
// consecutivas desde el primer código.
func (l *lienzo) anchosSimples(fd pdfDict, f *fuentePDF) {
	primero := 0
	if v, ok := comoEntero(l.d.resolver(fd["FirstChar"])); ok && v >= 0 && v <= 255 {
		primero = int(v)
	}
	arr, _ := l.d.resolver(fd["Widths"]).(pdfArray)

	// Type3 mide sus glifos en el espacio de /FontMatrix, no en milésimas; y un
	// código fuera de /FirstChar..LastChar sin /MissingWidth tiene anchura 0.
	escala := 1.0
	if s, _ := l.d.resolver(fd["Subtype"]).(pdfName); s == "Type3" {
		f.anchoDef = 0
		if fm, ok := l.d.resolver(fd["FontMatrix"]).(pdfArray); ok && len(fm) >= 1 {
			escala = comoReal(l.d.resolver(fm[0])) * 1000
		}
	}
	for i, w := range arr {
		if primero+i > 255 {
			break
		}
		f.anchos[uint32(primero+i)] = comoReal(l.d.resolver(w)) * escala
	}
	if desc, ok := l.d.resolver(fd["FontDescriptor"]).(pdfDict); ok {
		if mw, ok := desc["MissingWidth"]; ok {
			f.anchoDef = comoReal(l.d.resolver(mw))
		}
	}
	if len(arr) == 0 {
		// Sin /Widths solo quedan las métricas de las 14 fuentes estándar, que la
		// especificación permite omitir precisamente porque el visor las conoce.
		aplicarMetricasEstandar(l, fd, f)
	}
}

// altosCID lee el desplazamiento vertical w1 de /DW2 y /W2 (§9.7.4.3), con las
// mismas dos formas que /W pero en tríos «w1 v1x v1y». Sin ellos, −1000.
func (l *lienzo) altosCID(dd pdfDict, f *fuentePDF) {
	f.altoDef, f.vyDef = -1000, 880
	f.altos, f.vx, f.vy = map[uint32]float64{}, map[uint32]float64{}, map[uint32]float64{}
	if a, ok := l.d.resolver(dd["DW2"]).(pdfArray); ok && len(a) == 2 {
		f.vyDef = comoReal(l.d.resolver(a[0]))
		f.altoDef = comoReal(l.d.resolver(a[1]))
	}
	arr, _ := l.d.resolver(dd["W2"]).(pdfArray)
	for i := 0; i+1 < len(arr); {
		c, ok := comoEntero(l.d.resolver(arr[i]))
		if !ok || c < 0 || c > maxCID {
			i++
			continue
		}
		if v, ok := l.d.resolver(arr[i+1]).(pdfArray); ok {
			l.altosCIDLista(f, c, v)
			i += 2
			continue
		}
		if i+2 >= len(arr) {
			return
		}
		l.altosCIDRango(f, c, arr[i+1:])
		i += 5
	}
}

func (l *lienzo) altosCIDLista(f *fuentePDF, c int64, v pdfArray) {
	for j := 0; 3*j < len(v) && c+int64(j) <= maxCID; j++ {
		cid := uint32(c + int64(j)) //nolint:gosec // G115: 0 ≤ c+j ≤ maxCID
		f.altos[cid] = comoReal(l.d.resolver(v[3*j]))
		if 3*j+2 < len(v) {
			f.vx[cid] = comoReal(l.d.resolver(v[3*j+1]))
			f.vy[cid] = comoReal(l.d.resolver(v[3*j+2]))
		}
	}
}

func (l *lienzo) altosCIDRango(f *fuentePDF, c int64, resto pdfArray) {
	hasta, _ := comoEntero(l.d.resolver(resto[0]))
	if hasta < c || hasta-c >= 70000 || hasta > maxCID {
		return
	}
	w1 := comoReal(l.d.resolver(resto[1]))
	vx, vy, tieneV := 0.0, 0.0, len(resto) >= 4
	if tieneV {
		vx = comoReal(l.d.resolver(resto[2]))
		vy = comoReal(l.d.resolver(resto[3]))
	}
	for x := c; x <= hasta; x++ {
		cid := uint32(x) //nolint:gosec // G115: 0 ≤ x ≤ maxCID
		f.altos[cid] = w1
		if tieneV {
			f.vx[cid], f.vy[cid] = vx, vy
		}
	}
}

// anchosCID lee /W de una fuente compuesta, que admite dos formas mezcladas en
// el mismo array: «c [w1 w2 …]» y «cPrimero cUltimo w».
func (l *lienzo) anchosCID(dd pdfDict, f *fuentePDF) {
	f.anchoDef = 1000
	if v, ok := dd["DW"]; ok {
		f.anchoDef = comoReal(l.d.resolver(v))
	}
	arr, _ := l.d.resolver(dd["W"]).(pdfArray)
	for i := 0; i < len(arr); {
		c, ok := comoEntero(l.d.resolver(arr[i]))
		if !ok {
			i++
			continue
		}
		if i+1 >= len(arr) {
			break
		}
		if c < 0 || c > maxCID {
			// Un CID fuera de rango viene de un fichero manipulado: se salta en vez
			// de truncarlo, que colocaría la anchura sobre OTRO glifo.
			i += 2
			continue
		}
		switch v := l.d.resolver(arr[i+1]).(type) {
		case pdfArray:
			for j, w := range v {
				if cid := c + int64(j); cid <= maxCID {
					f.anchos[uint32(cid)] = comoReal(l.d.resolver(w))
				}
			}
			i += 2
		default:
			hasta, _ := comoEntero(v)
			if i+2 >= len(arr) {
				return
			}
			w := comoReal(l.d.resolver(arr[i+2]))
			// Un rango absurdo llenaría el mapa hasta agotar la memoria.
			if hasta >= c && hasta-c < 70000 && hasta <= maxCID {
				for x := c; x <= hasta; x++ {
					f.anchos[uint32(x)] = w
				}
			}
			i += 3
		}
	}
}

// codificacionSimple resuelve /Encoding: la base y las /Differences, que es como
// un PDF mete un símbolo suelto en un hueco de la tabla. Sin base explícita, una
// Type 1 usa la codificación de su programa, ZapfDingbats la suya y el resto de
// Type 1 la estándar (§9.6.5.2); en una TrueType lo que falte se sigue leyendo
// como WinAnsi.
func (l *lienzo) codificacionSimple(fd pdfDict, base string, sub pdfName) map[byte]string {
	tabla := map[byte]string{}
	var baseEnc pdfName
	var difs pdfArray
	switch v := l.d.resolver(fd["Encoding"]).(type) {
	case pdfName:
		baseEnc = v
	case pdfDict:
		baseEnc, _ = l.d.resolver(v["BaseEncoding"]).(pdfName)
		difs, _ = l.d.resolver(v["Differences"]).(pdfArray)
	}
	l.aplicarBase(tabla, baseEnc, base, sub, fd)
	codigo := 0
	for _, e := range difs {
		switch x := l.d.resolver(e).(type) {
		case int64:
			if x < 0 || x > 255 {
				codigo = -1
			} else {
				codigo = int(x)
			}
		case float64:
			if x < 0 || x > 255 {
				codigo = -1
			} else {
				codigo = int(x)
			}
		case pdfName:
			if codigo >= 0 && codigo < 256 {
				if t, ok := textoDeNombreGlifo(string(x)); ok {
					// En Type 3 sin /ToUnicode los nombres de /Differences identifican el
					// dibujo en /CharProcs («/square», «/triangle» sobre 'a', 'b'); no se
					// sustituye un ASCII imprimible por un símbolo geométrico (U+2000..).
					r, _ := utf8.DecodeRuneInString(t)
					if sub != "Type3" || codigo < 32 || codigo >= 127 || r < 0x2000 {
						tabla[byte(codigo)] = t
					}
				}
			}
			codigo++
		}
	}
	return tabla
}

// aplicarBase pone la codificación base de una fuente simple (ver codificacionSimple).
func (l *lienzo) aplicarBase(tabla map[byte]string, baseEnc pdfName, base string, sub pdfName, fd pdfDict) {
	switch baseEnc {
	case "WinAnsiEncoding":
		for b, r := range winAnsiEspecial {
			tabla[b] = string(r)
		}
	case "MacRomanEncoding":
		for c := 128; c < 256; c++ {
			tabla[byte(c)] = textoMacRoman(byte(c))
		}
	case "StandardEncoding":
		aplicarEstandar(tabla)
	case "":
		switch sub {
		case "Type1", "MMType1":
			desc, _ := l.d.resolver(fd["FontDescriptor"]).(pdfDict)
			n := strings.ToLower(base)
			switch {
			case l.codificacionPropia(desc, tabla):
			case strings.Contains(n, "dingbats"):
				aplicarZapfDingbats(tabla)
			case !strings.Contains(n, "symbol"):
				aplicarEstandar(tabla)
			}
		case "TrueType":
			desc, _ := l.d.resolver(fd["FontDescriptor"]).(pdfDict)
			if desc["FontFile2"] == nil && desc["FontFile3"] == nil {
				aplicarEstandar(tabla)
			}
		}
	}
}

// nombresDeGlifo es el trozo de la lista de Adobe que hace falta de verdad: la
// puntuación y los signos que un generador mete por /Differences porque no caben
// en la codificación base. Las letras acentuadas no están porque llegan por
// «uniXXXX» o por /ToUnicode.
var nombresDeGlifo = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#', "dollar": '$',
	"percent": '%', "ampersand": '&', "quotesingle": '\'', "quoteright": '’',
	"quoteleft": '‘', "parenleft": '(', "parenright": ')', "asterisk": '*',
	"plus": '+', "comma": ',', "hyphen": '-', "period": '.', "slash": '/',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4', "five": '5',
	"six": '6', "seven": '7', "eight": '8', "nine": '9', "colon": ':',
	"semicolon": ';', "less": '<', "equal": '=', "greater": '>', "question": '?',
	"at": '@', "bracketleft": '[', "backslash": '\\', "bracketright": ']',
	"asciicircum": '^', "underscore": '_', "grave": '`', "braceleft": '{',
	"bar": '|', "braceright": '}', "asciitilde": '~', "quotedblleft": '“',
	"quotedblright": '”', "quotedblbase": '„', "quotesinglbase": '‚',
	"endash": '–', "emdash": '—', "bullet": '•', "dagger": '†',
	"daggerdbl": '‡', "ellipsis": '…', "perthousand": '‰',
	"guilsinglleft": '‹', "guilsinglright": '›', "guillemotleft": '«',
	"guillemotright": '»', "fraction": '⁄', "florin": 'ƒ',
	"trademark": '™', "Euro": '€', "euro": '€', "currency": '¤',
	"sterling": '£', "yen": '¥', "cent": '¢', "section": '§',
	"paragraph": '¶', "copyright": '©', "registered": '®',
	"degree": '°', "plusminus": '±', "multiply": '×',
	"divide": '÷', "minus": '−', "periodcentered": '·',
	"exclamdown": '¡', "questiondown": '¿', "germandbls": 'ß',
	"ordfeminine": 'ª', "ordmasculine": 'º', "logicalnot": '¬',
	"onequarter": '¼', "onehalf": '½', "threequarters": '¾',
	"fi": 'ﬁ', "fl": 'ﬂ', "ff": 'ﬀ', "ffi": 'ﬃ', "ffl": 'ﬄ',
	"nbspace": ' ', "hyphensoft": '­', "arrowright": '→',
	"arrowleft": '←', "arrowup": '↑', "arrowdown": '↓',
	"checkmark": '✓', "circle": '○', "square": '■',
}

// --- /ToUnicode --------------------------------------------------------------

// leerToUnicode interpreta la CMap de /ToUnicode. No es un flujo de contenido
// normal: son bloques `beginbfchar`/`beginbfrange` con pares de cadenas hex.
//
// Hay generadores que ponen ahí la CMap Identity-H —por nombre o copiando el
// fichero de Adobe, que es de CIDs y no trae ni un `bfchar`—: declaran que el
// código es el carácter, y así se lee.
func (l *lienzo) leerToUnicode(fd pdfDict, f *fuentePDF) {
	var st *pdfStream
	switch v := l.d.resolver(fd["ToUnicode"]).(type) {
	case pdfName:
		f.unicodeIdentidad = strings.HasPrefix(string(v), "Identity")
		return
	case *pdfStream:
		st = v
	default:
		return
	}
	datos, err := l.d.datosDeFlujo(st)
	if err != nil {
		return
	}
	cidIdentidad := l.interpretarCMapToUnicode(datos, f)
	if len(f.aUnicode) == 0 && !cidIdentidad && len(l.d.listaDeNombres(st.Dict["Filter"])) == 0 {
		if inflado, err := inflar(datos); err == nil {
			cidIdentidad = l.interpretarCMapToUnicode(inflado, f)
		}
	}
	f.unicodeIdentidad = len(f.aUnicode) == 0 && cidIdentidad
}

func (l *lienzo) interpretarCMapToUnicode(flujo []byte, f *fuentePDF) bool {
	cidIdentidad := false
	l.d.recorrerCMap(flujo, func(op string, pila []pdfObj) {
		switch op {
		case "endbfchar":
			for i := 0; i+1 < len(pila); i += 2 {
				o, ok1 := pila[i].(pdfString)
				dst, ok2 := pila[i+1].(pdfString)
				if ok1 && ok2 {
					if t := strings.Trim(textoUTF16(dst), "\x00"); t != "" {
						f.aUnicode[codigoDeHex(o)] = t
					}
				}
			}
		case "endbfrange":
			l.rangoBF(pila, f)
		case "endcidrange":
			for i := 0; i+2 < len(pila); i += 3 {
				a, ok := pila[i].(pdfString)
				cid, ok2 := comoEntero(pila[i+2])
				cidIdentidad = cidIdentidad || (ok && ok2 && int64(codigoDeHex(a)) == cid)
			}
		}
	})
	return cidIdentidad
}

func (l *lienzo) rangoBF(pila []pdfObj, f *fuentePDF) {
	for i := 0; i+2 < len(pila); i += 3 {
		desde, ok1 := pila[i].(pdfString)
		hasta, ok2 := pila[i+1].(pdfString)
		if !ok1 || !ok2 {
			continue
		}
		a, b := codigoDeHex(desde), codigoDeHex(hasta)
		if b < a || b-a > 70000 {
			continue
		}
		switch dst := pila[i+2].(type) {
		case pdfString:
			// Rango consecutivo: el último punto de código se incrementa.
			us := utf16Puntos(dst)
			if len(us) == 0 {
				continue
			}
			for c := a; c <= b; c++ {
				copia := append([]uint16(nil), us...)
				copia[len(copia)-1] += uint16((c - a) & 0xFFFF)
				f.aUnicode[c] = string(utf16.Decode(copia))
			}
		case pdfArray:
			for j, e := range dst {
				if s, ok := e.(pdfString); ok && a+uint32(j) <= b {
					f.aUnicode[a+uint32(j)] = textoUTF16(s)
				}
			}
		}
	}
}

// codigoDeHex convierte la cadena de una CMap a número: sus bytes son el código,
// en orden de mayor a menor peso.
func codigoDeHex(s pdfString) uint32 {
	var v uint32
	for i, b := range s {
		if i >= 4 {
			break
		}
		v = v<<8 | uint32(b)
	}
	return v
}

func utf16Puntos(s pdfString) []uint16 {
	out := make([]uint16, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		out = append(out, uint16(s[i])<<8|uint16(s[i+1]))
	}
	if len(s) == 1 {
		out = append(out, uint16(s[0]))
	}
	return out
}

func textoUTF16(s pdfString) string {
	t := string(utf16.Decode(utf16Puntos(s)))
	// Un destino de un solo carácter nulo aparece en subconjuntos mal generados;
	// dejarlo metería NUL en el SVG, que es un carácter XML inválido.
	return strings.ReplaceAll(t, "\x00", "")
}

// --- Sustitución e incrustación ---------------------------------------------

func rasgos(nombre string, l *lienzo, desc pdfDict) (negrita, italica bool) {
	n := strings.ToLower(nombre)
	negrita = strings.Contains(n, "bold") || strings.Contains(n, "black") ||
		strings.Contains(n, "heavy") || strings.Contains(n, "semibold")
	italica = strings.Contains(n, "italic") || strings.Contains(n, "oblique")
	if desc == nil {
		return negrita, italica
	}
	if v, ok := comoEntero(l.d.resolver(desc["Flags"])); ok {
		const italicaBit = 1 << 6
		italica = italica || v&italicaBit != 0
	}
	if v, ok := desc["ItalicAngle"]; ok && math.Abs(comoReal(l.d.resolver(v))) > 4 {
		italica = true
	}
	if v, ok := desc["FontWeight"]; ok && comoReal(l.d.resolver(v)) >= 600 {
		negrita = true
	}
	if v, ok := desc["StemV"]; ok && comoReal(l.d.resolver(v)) >= 120 {
		negrita = true
	}
	return negrita, italica
}

// familiaSustituta elige la familia genérica CSS que más se parece. Es el plan B
// cuando la fuente no viene incrustada o no se puede reemitir.
func familiaSustituta(nombre string, l *lienzo, desc pdfDict) string {
	n := strings.ToLower(nombre)
	fijo := strings.Contains(n, "courier") || strings.Contains(n, "mono") ||
		strings.Contains(n, "consol")
	serif := strings.Contains(n, "times") || strings.Contains(n, "serif") ||
		strings.Contains(n, "georgia") || strings.Contains(n, "garamond") ||
		strings.Contains(n, "book") || strings.Contains(n, "roman") ||
		strings.Contains(n, "minion") || strings.Contains(n, "cambria")
	if desc != nil {
		if v, ok := comoEntero(l.d.resolver(desc["Flags"])); ok {
			const (
				pasoFijo  = 1 << 0
				conRemate = 1 << 1
			)
			fijo = fijo || v&pasoFijo != 0
			serif = serif || v&conRemate != 0
		}
	}
	switch {
	case fijo:
		return "ui-monospace,monospace"
	case serif:
		return "Georgia,'Times New Roman',serif"
	default:
		return "'Helvetica Neue',Helvetica,Arial,sans-serif"
	}
}

// incrustar reemite la tipografía del PDF como @font-face y devuelve el nombre
// de familia a usar. Cadena vacía = no se pudo, hay que sustituir.
func (l *lienzo) incrustar(desc pdfDict, f *fuentePDF, sub pdfName) string {
	if desc == nil {
		return ""
	}
	var datos []byte
	var mime string
	if st, ok := l.d.resolver(desc["FontFile2"]).(*pdfStream); ok {
		if b, err := l.d.datosDeFlujo(st); err == nil && len(b) > 12 {
			datos, mime = b, "font/ttf"
		}
	}
	if datos == nil {
		datos, mime = l.programaFontFile3(desc, f)
	}
	if datos == nil {
		datos, mime = l.programaType1(desc, f)
	}
	if datos == nil || len(datos) > maxFuenteIncrustada {
		return ""
	}

	var ok bool
	datos, ok = l.prepararFuenteIncrustada(datos, mime, sub, f)
	if !ok {
		return ""
	}

	// @font-face es global aunque el SVG esté inline en otra página. El orden
	// local (sypdf1, sypdf2...) mezcla programas/subconjuntos entre páginas o
	// documentos. La identidad incluye el programa y la cmap ya preparados.
	huella := sha256.Sum256(datos)
	id := "sypdf-" + hex.EncodeToString(huella[:])
	if f.negrita {
		id += "-b"
	}
	if f.italica {
		id += "-i"
	}
	familia := `"` + id + `",sans-serif`
	if l.caraCSS[id] {
		return familia
	}
	l.caraCSS[id] = true
	if l.soloRaster {
		// Nadie va a leer el `@font-face`: codificar megas de tipografía en base64
		// para tirarlos sería trabajo perdido en cada página.
		return familia
	}
	l.defsEscribir(f.reglaFontFace(id, mime, datos))
	// La familia genérica detrás cubre el carácter que el subconjunto no traiga.
	return familia
}

// programaFontFile3 lee /FontFile3. El contenedor OpenType se sirve tal cual;
// un CFF suelto se envuelve en uno (pdfcff.go), que es lo que abren el
// navegador y el rasterizador.
func (l *lienzo) programaFontFile3(desc pdfDict, f *fuentePDF) ([]byte, string) {
	st, ok := l.d.resolver(desc["FontFile3"]).(*pdfStream)
	if !ok {
		return nil, ""
	}
	s3, _ := l.d.resolver(st.Dict["Subtype"]).(pdfName)
	switch s3 {
	case "OpenType":
		if b, err := l.d.datosDeFlujo(st); err == nil && len(b) > 12 {
			return b, "font/otf"
		}
	case "Type1C", "CIDFontType0C":
		if b, err := l.d.datosDeFlujo(st); err == nil && len(b) > 4 && len(b) <= maxFuenteIncrustada {
			if otf, ok := f.envolverCFF(aplanarSeacCFF(b)); ok {
				return otf, "font/otf"
			}
		}
	}
	return nil, ""
}

// programaType1 convierte la Type 1 de /FontFile en CFF (pdftype1.go) y la
// envuelve en OpenType como un CFF suelto.
func (l *lienzo) programaType1(desc pdfDict, f *fuentePDF) ([]byte, string) {
	st, ok := l.d.resolver(desc["FontFile"]).(*pdfStream)
	if !ok {
		return nil, ""
	}
	b, err := l.d.datosDeFlujo(st)
	if err != nil || len(b) < 16 || len(b) > maxFuenteIncrustada {
		return nil, ""
	}
	len1, _ := numeroPDF(l.d.resolver(st.Dict["Length1"]))
	cff, err := type1ACFF(b, int(len1))
	if err != nil {
		return nil, ""
	}
	if otf, ok := f.envolverCFF(cff); ok {
		return otf, "font/otf"
	}
	return nil, ""
}

func (l *lienzo) prepararFuenteIncrustada(datos []byte, mime string, sub pdfName, f *fuentePDF) ([]byte, bool) {
	// El subconjunto PDF puede omitir post: el PDF no necesita nombres de glifo,
	// pero sfnt y los navegadores sí necesitan una tabla válida para abrirlo.
	if mime == "font/ttf" {
		datos = f.completarMetadatosTTF(datos)
	}
	// El rasterizador usa el programa por CÓDIGO; la cmap Unicode es para SVG.
	f.guardarPrograma(l, datos)
	if sub != "Type0" && mime == "font/ttf" && !l.soloRaster {
		if parcheada, ok := f.cmapSimpleParaTexto(datos); ok {
			datos = parcheada
		}
	}

	if sub == "Type0" && mime == "font/ttf" {
		parcheada, ok := reescribirCmapTTF(datos, f.aUnicode)
		if !ok {
			// El PNG sí puede con ella: dibuja por índice de glifo y no necesita
			// cmap ninguna. El SVG sustituye, que es lo que ya hacía.
			f.guardarPrograma(l, datos)
			return nil, false
		}
		datos = parcheada
		f.guardarPrograma(l, datos)
	}

	return datos, true
}

// reglaFontFace declara el estilo real de cada fuente incrustada: declararla
// siempre normal obliga al navegador a sintetizar negritas e inclinaciones.
func (f *fuentePDF) reglaFontFace(id, mime string, datos []byte) string {
	var b strings.Builder
	b.WriteString(`<style>@font-face{font-family:"` + id + `";src:url("data:` + mime + `;base64,`)
	b.WriteString(base64.StdEncoding.EncodeToString(datos))
	b.WriteString(`");font-weight:`)
	if f.negrita {
		b.WriteString("bold")
	} else {
		b.WriteString("normal")
	}
	b.WriteString(";font-style:")
	if f.italica {
		b.WriteString("italic")
	} else {
		b.WriteString("normal")
	}
	b.WriteString(";}</style>")
	return b.String()
}

// completarMetadatosTTF repone tablas que un subconjunto PDF puede omitir.
// post formato 3 declara que no hay nombres; OS/2 conserva peso y métricas del
// head/hhea. No se alteran contornos ni avances. OTS exige ambas en el navegador.
func (f *fuentePDF) completarMetadatosTTF(datos []byte) []byte {
	if len(datosTablaSfnt(datos, "post")) == 0 {
		post := make([]byte, 32)
		binary.BigEndian.PutUint32(post, 0x00030000)
		if salida, ok := reemplazarTablaSfnt(datos, "post", post); ok {
			datos = salida
		}
	}
	if len(datosTablaSfnt(datos, "OS/2")) != 0 {
		return datos
	}
	head, hhea := datosTablaSfnt(datos, "head"), datosTablaSfnt(datos, "hhea")
	if len(head) < 54 || len(hhea) < 12 {
		return datos
	}
	os2 := make([]byte, 78) // versión 0
	be := binary.BigEndian
	be.PutUint16(os2[2:4], be.Uint16(hhea[10:12])) // anchura máxima disponible
	peso, seleccion := uint16(400), uint16(64)
	if f.negrita {
		peso, seleccion = 700, 32
	}
	if f.italica {
		seleccion &^= 64
		seleccion |= 1
	}
	be.PutUint16(os2[4:6], peso)
	be.PutUint16(os2[6:8], 5) // anchura normal
	copy(os2[58:62], "SYTU")
	be.PutUint16(os2[62:64], seleccion)
	be.PutUint16(os2[66:68], 0xFFFF)
	copy(os2[68:74], hhea[4:10]) // ascendente, descendente y leading
	copy(os2[74:76], hhea[4:6])
	descenso := be.Uint16(hhea[6:8])
	be.PutUint16(os2[76:78], ^descenso+1) // hhea guarda el descenso negativo
	if salida, ok := reemplazarTablaSfnt(datos, "OS/2", os2); ok {
		return salida
	}
	return datos
}

func datosTablaSfnt(datos []byte, etiqueta string) []byte {
	if len(datos) < 12 {
		return nil
	}
	be := binary.BigEndian
	for i := 0; i < int(be.Uint16(datos[4:6])); i++ {
		pos := 12 + 16*i
		if pos+16 > len(datos) {
			return nil
		}
		if string(datos[pos:pos+4]) != etiqueta {
			continue
		}
		off, longitud := uint64(be.Uint32(datos[pos+8:pos+12])), uint64(be.Uint32(datos[pos+12:pos+16]))
		if off+longitud > uint64(len(datos)) {
			return nil
		}
		return datos[off : off+longitud]
	}
	return nil
}

// cmapSimpleParaTexto resuelve cada código con las mismas reglas del PNG y
// reconstruye Unicode → glifo para SVG. Macintosh formato 6 puede llevar códigos
// renumerados de Word: su plataforma por sí sola no garantiza que sean caracteres.
func (f *fuentePDF) cmapSimpleParaTexto(datos []byte) ([]byte, bool) {
	cara, err := sfnt.Parse(datos)
	if err != nil {
		return nil, false
	}
	f.cmapUnicode = tieneCmapUnicode(datos)
	f.cmapMacRoman = tieneCmapMacRoman(datos)
	var buf sfnt.Buffer
	unicodeAGlifo := map[uint16]uint16{}
	for codigo := uint32(0); codigo < 256; codigo++ {
		g := f.glifoDe(codigo, codigo == 32)
		rs := []rune(g.texto)
		if len(rs) != 1 || rs[0] <= 0 || rs[0] > 0xFFFF {
			continue
		}
		if gid, ok := f.indiceDeGlifo(cara, &buf, g); ok {
			unicodeAGlifo[uint16(rs[0]&0xFFFF)] = uint16(gid)
			if f.simbolica {
				unicodeAGlifo[0xE000+uint16(codigo)] = uint16(gid)
			}
		}
	}
	salida, ok := reescribirCmapUnicodeTTF(datos, unicodeAGlifo)
	f.cmapVisual = ok && f.simbolica
	return salida, ok
}

// guardarPrograma se queda con la tipografía para dibujar sus contornos. Solo
// cuando hay rasterizador: en SVG son megas de memoria que nadie lee.
func (f *fuentePDF) guardarPrograma(l *lienzo, datos []byte) {
	if l.r != nil {
		f.programa = datos
	}
}

// maxFuenteIncrustada acota lo que se reemite en el SVG. Una CJK completa pasa
// de 15 MB y multiplicaría por diez el peso de la página; por encima del tope se
// sustituye, que se ve peor pero llega.
const maxFuenteIncrustada = 6 << 20

// --- Reconstrucción de la tabla cmap ----------------------------------------

// reescribirCmapTTF devuelve el TrueType con una tabla `cmap` nueva que mapea
// Unicode → índice de glifo, deducida de /ToUnicode. Sin ella, una fuente
// Identity-H incrustada no sabe qué dibujar para una letra.
//
// Se emite formato 4 en la plataforma (3,1), que es el que todos los navegadores
// entienden y el que su saneador de fuentes acepta sin discutir.
// de campos con sus validaciones; no hay bucle que extraer.
//
//nolint:gocyclo // Construir una subtabla cmap de formato 4 es una secuencia
func reescribirCmapTTF(ttf []byte, aUnicode map[uint32]string) ([]byte, bool) {
	if len(aUnicode) == 0 || len(ttf) < 12 {
		return nil, false
	}
	// Invertir: carácter → glifo. Si dos glifos dan el mismo carácter (ligaduras
	// descompuestas, variantes), gana el de índice menor, que es el que el
	// subconjunto colocó primero y casi siempre la forma normal.
	inverso := map[uint16]uint16{}
	for gid, texto := range aUnicode {
		rs := []rune(texto)
		if len(rs) != 1 || gid > 0xFFFF || rs[0] > 0xFFFF || rs[0] == 0 {
			continue
		}
		u := uint16(rs[0] & 0xFFFF)
		if v, ok := inverso[u]; !ok || gid&0xFFFF < uint32(v) {
			inverso[u] = uint16(gid & 0xFFFF)
		}
	}
	return reescribirCmapUnicodeTTF(ttf, inverso)
}

// reescribirCmapUnicodeTTF conserva también alias Unicode del mismo glifo.
//
//nolint:gocyclo // Construcción y validación de una subtabla cmap de formato 4.
func reescribirCmapUnicodeTTF(ttf []byte, inverso map[uint16]uint16) ([]byte, bool) {
	if len(inverso) == 0 {
		return nil, false
	}

	claves := make([]int, 0, len(inverso))
	for u := range inverso {
		claves = append(claves, int(u))
	}
	sort.Ints(claves)

	// Segmentos: un tramo consecutivo con delta constante cabe en uno solo.
	type seg struct{ ini, fin, delta uint16 }
	var segs []seg
	for _, k := range claves {
		u := uint16(k & 0xFFFF)
		g := inverso[u]
		delta := g - u // aritmética uint16, que es exactamente lo que pide idDelta
		if n := len(segs); n > 0 && segs[n-1].delta == delta && segs[n-1].fin+1 == u {
			segs[n-1].fin = u
			continue
		}
		segs = append(segs, seg{ini: u, fin: u, delta: delta})
	}
	// El último segmento tiene que ser el centinela 0xFFFF (§cmap formato 4).
	if segs[len(segs)-1].fin != 0xFFFF {
		segs = append(segs, seg{ini: 0xFFFF, fin: 0xFFFF, delta: 1})
	}

	n := len(segs)
	sub := make([]byte, 0, 16+n*8)
	be := binary.BigEndian
	u16 := func(v uint16) { sub = be.AppendUint16(sub, v) }
	longitud := 16 + n*8
	u16(4)
	u16(uint16(longitud & 0xFFFF))
	u16(0) // idioma
	u16(uint16(n * 2 & 0xFFFF))
	potencia := uint16(1)
	sel := uint16(0)
	for int(potencia)*2 <= n {
		potencia *= 2
		sel++
	}
	u16(potencia * 2)
	u16(sel)
	u16(uint16(n*2&0xFFFF) - potencia*2)
	for _, s := range segs {
		u16(s.fin)
	}
	u16(0) // relleno reservado
	for _, s := range segs {
		u16(s.ini)
	}
	for _, s := range segs {
		u16(s.delta)
	}
	for range segs {
		u16(0) // idRangeOffset: 0 = usar idDelta directamente
	}

	tabla := make([]byte, 0, 12+len(sub))
	tabla = be.AppendUint16(tabla, 0) // versión
	tabla = be.AppendUint16(tabla, 1) // una subtabla
	tabla = be.AppendUint16(tabla, 3) // plataforma Windows
	tabla = be.AppendUint16(tabla, 1) // BMP Unicode
	tabla = be.AppendUint32(tabla, 12)
	tabla = append(tabla, sub...)

	return reemplazarTablaSfnt(ttf, "cmap", tabla)
}

// reemplazarTablaSfnt reconstruye un fichero sfnt sustituyendo (o añadiendo) una
// tabla. Hay que rehacer el directorio entero porque cambian los desplazamientos
// de todo lo que va detrás.
func reemplazarTablaSfnt(fuente []byte, etiqueta string, contenido []byte) ([]byte, bool) {
	be := binary.BigEndian
	if len(fuente) < 12 {
		return nil, false
	}
	version := be.Uint32(fuente[0:4])
	numTablas := int(be.Uint16(fuente[4:6]))
	if numTablas <= 0 || numTablas > 512 || 12+numTablas*16 > len(fuente) {
		return nil, false
	}

	type entrada struct {
		tag   string
		datos []byte
	}
	var tablas []entrada
	visto := false
	for i := 0; i < numTablas; i++ {
		rec := fuente[12+i*16 : 12+i*16+16]
		tag := string(rec[0:4])
		off := int(be.Uint32(rec[8:12]))
		lon := int(be.Uint32(rec[12:16]))
		if off < 0 || lon < 0 || off+lon > len(fuente) {
			return nil, false
		}
		if tag == etiqueta {
			tablas = append(tablas, entrada{tag, contenido})
			visto = true
			continue
		}
		tablas = append(tablas, entrada{tag, fuente[off : off+lon]})
	}
	if !visto {
		tablas = append(tablas, entrada{etiqueta, contenido})
	}
	// El directorio va ordenado por etiqueta; el saneador de fuentes del
	// navegador rechaza el fichero si no lo está.
	sort.Slice(tablas, func(i, j int) bool { return tablas[i].tag < tablas[j].tag })

	n := len(tablas)
	cabecera := 12 + n*16
	// Capacidad, no longitud: el directorio se rellena por índice más abajo y
	// las tablas se van anexando detrás.
	out := make([]byte, cabecera, cabecera+4096)
	be.PutUint32(out[0:4], version)
	be.PutUint16(out[4:6], uint16(n&0xFFFF))
	potencia := 1
	sel := 0
	for potencia*2 <= n {
		potencia *= 2
		sel++
	}
	be.PutUint16(out[6:8], uint16(potencia*16&0xFFFF))
	be.PutUint16(out[8:10], uint16(sel&0xFFFF))
	be.PutUint16(out[10:12], uint16((n*16-potencia*16)&0xFFFF))

	for i, t := range tablas {
		off := len(out)
		out = append(out, t.datos...)
		for len(out)%4 != 0 { // cada tabla empieza en múltiplo de 4
			out = append(out, 0)
		}
		rec := out[12+i*16 : 12+i*16+16]
		copy(rec[0:4], t.tag)
		be.PutUint32(rec[4:8], sumaSfnt(t.datos))
		be.PutUint32(rec[8:12], uint32(off&0xFFFFFFFF))
		be.PutUint32(rec[12:16], uint32(len(t.datos)&0xFFFFFFFF))
	}
	return out, true
}

// sumaSfnt es la suma de comprobación de una tabla: enteros de 32 bits sumados
// con desbordamiento, rellenando con ceros hasta múltiplo de cuatro.
func sumaSfnt(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i += 4 {
		var v uint32
		for j := 0; j < 4; j++ {
			v <<= 8
			if i+j < len(b) {
				v |= uint32(b[i+j])
			}
		}
		s += v
	}
	return s
}

// --- Métricas de las 14 fuentes estándar ------------------------------------

// aplicarMetricasEstandar rellena las anchuras cuando el PDF omite /Widths, algo
// que la especificación permite solo con las 14 fuentes estándar. Sin esto el
// avance sería cero y el texto saldría amontonado en el margen.
func aplicarMetricasEstandar(l *lienzo, fd pdfDict, f *fuentePDF) {
	base, _ := l.d.resolver(fd["BaseFont"]).(pdfName)
	n := strings.ToLower(string(base))
	var tabla *[95]uint16
	switch {
	case strings.Contains(n, "courier"):
		f.anchoDef = 600
		return // Courier es de paso fijo: 600 en todos sus glifos
	case strings.Contains(n, "times"):
		if strings.Contains(n, "bold") {
			tabla = &anchosTimesNegrita
		} else {
			tabla = &anchosTimes
		}
		f.anchoDef = 500
	case strings.Contains(n, "symbol"), strings.Contains(n, "zapf"), strings.Contains(n, "dingbat"):
		f.anchoDef = 600
		return
	default:
		if strings.Contains(n, "bold") {
			tabla = &anchosHelveticaNegrita
		} else {
			tabla = &anchosHelvetica
		}
		f.anchoDef = 556
	}
	for i, w := range tabla {
		f.anchos[uint32(32+i)] = float64(w)
	}
	for c := 128; c < 256; c++ {
		t := f.simple[byte(c)]
		if t == "" {
			t = decodificarTextoPDF(pdfString{byte(c)})
		}
		switch t {
		case "…":
			f.anchos[uint32(c)] = 1000
		case "º", "ª":
			f.anchos[uint32(c)] = 300
		default:
			if d := norm.NFD.String(t); d != "" && d[0] >= 32 && d[0] <= 126 {
				f.anchos[uint32(c)] = float64(tabla[d[0]-32])
			}
		}
	}
}

// Anchuras de los códigos 32 a 126 tomadas de las métricas AFM publicadas por
// Adobe para las fuentes estándar. Las variantes cursivas comparten tabla con su
// versión recta: la diferencia real cabe en unas pocas milésimas y `textLength`
// la absorbe.
var (
	anchosHelvetica = [95]uint16{
		278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
		1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
		667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
		333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
		556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
	}
	anchosHelveticaNegrita = [95]uint16{
		278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611,
		975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778,
		667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556,
		333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611,
		611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584,
	}
	anchosTimes = [95]uint16{
		250, 333, 408, 500, 500, 833, 778, 180, 333, 333, 500, 564, 250, 333, 250, 278,
		500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 278, 278, 564, 564, 564, 444,
		921, 722, 667, 667, 722, 611, 556, 722, 722, 333, 389, 722, 611, 889, 722, 722,
		556, 722, 667, 556, 611, 722, 722, 944, 722, 722, 611, 333, 278, 333, 469, 500,
		333, 444, 500, 444, 500, 444, 333, 500, 500, 278, 278, 500, 278, 778, 500, 500,
		500, 500, 333, 389, 278, 500, 500, 722, 500, 500, 444, 480, 200, 480, 541,
	}
	anchosTimesNegrita = [95]uint16{
		250, 333, 555, 500, 500, 1000, 833, 278, 333, 333, 500, 570, 250, 333, 250, 278,
		500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 333, 333, 570, 570, 570, 500,
		930, 722, 667, 722, 722, 667, 611, 778, 778, 389, 500, 778, 667, 944, 722, 778,
		611, 778, 722, 556, 667, 722, 722, 1000, 722, 722, 667, 333, 278, 333, 581, 500,
		333, 500, 556, 444, 556, 444, 333, 500, 556, 278, 333, 556, 278, 833, 556, 500,
		556, 556, 444, 389, 333, 556, 500, 722, 500, 500, 444, 394, 220, 394, 520,
	}
)
