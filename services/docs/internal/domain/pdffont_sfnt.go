// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Saneamiento de contenedores SFNT/TTC y lectura directa de sus tablas crudas
// (`cmap`, `post`, `CFF `) antes de reescribirlas para SVG y el rasterizador.
package domain

import (
	"bytes"
	"encoding/binary"
	"slices"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/encoding/charmap"
)

// tablasCrudasSfnt conserva los índices de glifo originales de una TrueType u
// OpenType incrustada antes de que `saneamientoSfnt` y la reconstrucción de
// `cmap` reemplacen `post` y `cmap` para el navegador.
type tablasCrudasSfnt struct {
	numGlifos    int
	esOTTO       bool
	tieneUnicode bool
	tieneMac     bool
	tieneSimbolo bool

	unicodeAGid  map[rune]uint16
	unicodeCero  map[rune]bool
	gidARuna     map[uint16]rune
	simboloAGid  map[uint16]uint16
	simboloCero  map[uint16]bool
	macRomanAGid [256]uint16
	macArabeAGid [256]uint16
	propiaAGid   map[byte]uint16
	nombreAGid   map[string]uint16
	cidAGidCFF   []byte
}

type entradaTablaSfnt struct {
	tag   string
	datos []byte
}

func etiquetaASCIIImprimible(b []byte) bool {
	if len(b) != 4 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

// desempaquetarTTC extrae una cara individual de una TrueType Collection
// («ttcf»), eligiendo la cuyo nombre PostScript o de familia coincida con
// `nombreBase` (sin prefijo de subconjunto) o la primera si ninguna casa.
func desempaquetarTTC(datos []byte, nombreBase string) []byte {
	if len(datos) < 16 || string(datos[:4]) != "ttcf" {
		return datos
	}
	be := binary.BigEndian
	numCaras := int(be.Uint32(datos[8:12]))
	if numCaras <= 0 || numCaras > 256 || 12+4*numCaras > len(datos) {
		return datos
	}
	buscado := limpiarNombreFuente(nombreBase)
	var primera []byte
	for idx := 0; idx < numCaras; idx++ {
		offDir := int(be.Uint32(datos[12+4*idx : 16+4*idx]))
		cara, ok := extraerCaraDeTTC(datos, offDir)
		if !ok {
			continue
		}
		if primera == nil {
			primera = cara
		}
		if buscado != "" && coincideNombreSfnt(cara, buscado) {
			return cara
		}
	}
	if primera != nil {
		return primera
	}
	return datos
}

func limpiarNombreFuente(nombre string) string {
	if esSubconjunto(nombre) {
		nombre = nombre[7:]
	}
	return strings.ToLower(strings.TrimSpace(nombre))
}

func extraerCaraDeTTC(ttc []byte, offDir int) ([]byte, bool) {
	if offDir < 12 || offDir+12 > len(ttc) {
		return nil, false
	}
	be := binary.BigEndian
	ver, ok := versionSfntNormalizada(ttc[offDir : offDir+4])
	if !ok {
		return nil, false
	}
	n := int(be.Uint16(ttc[offDir+4 : offDir+6]))
	if n <= 0 || n > 512 || offDir+12+16*n > len(ttc) {
		return nil, false
	}
	var tablas []entradaTablaSfnt
	vistos := map[string]bool{}
	for i := 0; i < n; i++ {
		rec := ttc[offDir+12+i*16 : offDir+12+i*16+16]
		if !etiquetaASCIIImprimible(rec[0:4]) {
			continue
		}
		tag := string(rec[0:4])
		if vistos[tag] {
			continue
		}
		off, lon := uint64(be.Uint32(rec[8:12])), uint64(be.Uint32(rec[12:16]))
		if off+lon > uint64(len(ttc)) {
			return nil, false
		}
		vistos[tag] = true
		tablas = append(tablas, entradaTablaSfnt{tag: tag, datos: ttc[off : off+lon]})
	}
	if len(tablas) == 0 {
		return nil, false
	}
	return ensamblarSfnt(ver, tablas), true
}

func coincideNombreSfnt(cara []byte, buscado string) bool {
	tb := datosTablaSfnt(cara, "name")
	if len(tb) < 6 {
		return false
	}
	be := binary.BigEndian
	cnt := int(be.Uint16(tb[2:4]))
	offStr := int(be.Uint16(tb[4:6]))
	for i := 0; i < cnt && 6+12*(i+1) <= len(tb); i++ {
		r := tb[6+12*i : 6+12*(i+1)]
		pid := be.Uint16(r[0:2])
		nid := be.Uint16(r[6:8])
		if nid != 1 && nid != 4 && nid != 6 {
			continue
		}
		lon, off := int(be.Uint16(r[8:10])), int(be.Uint16(r[10:12]))
		if offStr+off+lon > len(tb) || lon <= 0 {
			continue
		}
		raw := tb[offStr+off : offStr+off+lon]
		var s string
		if pid == 3 || pid == 0 {
			u := make([]uint16, lon/2)
			for j := range u {
				u[j] = be.Uint16(raw[2*j:])
			}
			s = string(utf16.Decode(u))
		} else {
			s = string(raw)
		}
		s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
		bSinEsp := strings.ReplaceAll(buscado, " ", "")
		if s != "" && (s == bSinEsp || strings.Contains(s, bSinEsp) || strings.Contains(bSinEsp, s)) {
			return true
		}
	}
	return false
}

func versionSfntNormalizada(sig []byte) (uint32, bool) {
	switch string(sig) {
	case "\x00\x01\x00\x00", "true", "typ1":
		return 0x00010000, true
	case "OTTO":
		return 0x4F54544F, true
	default:
		return 0, false
	}
}

// normalizarSfnt desempaqueta colecciones TTC, traduce firmas de Apple («true»,
// «typ1») a 0x00010000, descarta entradas finales basura con etiquetas no ASCII
// (como en issue3405r.pdf), rechaza tablas ASCII truncadas fuera del fichero y
// reensambla todas las tablas ordenadas y alineadas a múltiplo de 4 bytes.
func normalizarSfnt(datos []byte, nombreBase string) ([]byte, bool) {
	datos = desempaquetarTTC(datos, nombreBase)
	if len(datos) < 12 {
		return datos, false
	}
	ver, ok := versionSfntNormalizada(datos[:4])
	if !ok {
		return datos, false
	}
	be := binary.BigEndian
	numTablas := int(be.Uint16(datos[4:6]))
	if numTablas <= 0 || numTablas > 512 || 12+16*numTablas > len(datos) {
		return datos, false
	}
	delta := detectarDesajusteHeadSfnt(datos, numTablas)
	tablas := make([]entradaTablaSfnt, 0, numTablas)
	vistos := make(map[string]bool, numTablas)
	for i := 0; i < numTablas; i++ {
		rec := datos[12+i*16 : 12+i*16+16]
		if !etiquetaASCIIImprimible(rec[0:4]) {
			continue
		}
		tag := string(rec[0:4])
		if vistos[tag] {
			continue
		}
		csum := be.Uint32(rec[4:8])
		off := uint64(be.Uint32(rec[8:12]))
		lon := uint64(be.Uint32(rec[12:16]))
		reb, ok := rebanadaTablaSfnt(datos, tag, csum, off, lon, delta)
		if !ok {
			return datos, false
		}
		vistos[tag] = true
		tablas = append(tablas, entradaTablaSfnt{tag: tag, datos: reb})
	}
	if len(tablas) == 0 {
		return datos, false
	}
	ensamblada := repararCabecerasBaseSfnt(ensamblarSfnt(ver, tablas))
	if delta == -1 {
		ensamblada = repararGlyfConBytePerdido(ensamblada)
	}
	return ensamblada, true
}

func detectarDesajusteHeadSfnt(datos []byte, numTablas int) int {
	be := binary.BigEndian
	for i := 0; i < numTablas; i++ {
		rec := datos[12+i*16 : 12+i*16+16]
		if string(rec[0:4]) != "head" {
			continue
		}
		off := int(be.Uint32(rec[8:12]))
		lon := int(be.Uint32(rec[12:16]))
		if lon < 16 || off < 12 || off+16 > len(datos) {
			return 0
		}
		if be.Uint32(datos[off+12:off+16]) == 0x5F0F3CF5 {
			return 0
		}
		for _, d := range [4]int{-1, 1, -2, 2} {
			p := off + d
			if p >= 12 && p+16 <= len(datos) && be.Uint32(datos[p+12:p+16]) == 0x5F0F3CF5 {
				return d
			}
		}
		return 0
	}
	return 0
}

func rebanadaTablaSfnt(datos []byte, tag string, csum uint32, off, lon uint64, delta int) ([]byte, bool) {
	lim := uint64(len(datos))
	if lon > lim || off > lim+64 {
		return nil, false
	}
	offI, lonI, n := int(off), int(lon), len(datos) //nolint:gosec // acotados por len(datos)
	cabeOrig := offI >= 0 && offI+lonI <= n
	if delta != 0 {
		p := offI + delta
		cabeDelta := p >= 12 && p+lonI <= n
		if tag == "head" && cabeDelta {
			return datos[p : p+lonI], true
		}
		if csum != 0 {
			if cabeOrig && sumaSfnt(datos[offI:offI+lonI]) == csum {
				return datos[offI : offI+lonI], true
			}
			if cabeDelta && sumaSfnt(datos[p:p+lonI]) == csum {
				return datos[p : p+lonI], true
			}
		}
		if !cabeOrig && cabeDelta {
			return datos[p : p+lonI], true
		}
	}
	if !cabeOrig {
		return nil, false
	}
	return datos[offI : offI+lonI], true
}

func leerOffsetsLoca(head, loca []byte, ng int) ([]int, bool, bool) {
	be := binary.BigEndian
	corta := int16(be.Uint16(head[50:52])) == 0 //nolint:gosec // int16 indexToLocFormat
	offs := make([]int, ng+1)
	if corta {
		if len(loca) < 2*(ng+1) {
			return nil, false, false
		}
		for i := 0; i <= ng; i++ {
			offs[i] = 2 * int(be.Uint16(loca[2*i:]))
		}
		return offs, true, true
	}
	if len(loca) < 4*(ng+1) {
		return nil, false, false
	}
	for i := 0; i <= ng; i++ {
		offs[i] = int(be.Uint32(loca[4*i:]))
	}
	return offs, false, true
}

func repararGlyfConBytePerdido(datos []byte) []byte {
	head := datosTablaSfnt(datos, "head")
	maxp := datosTablaSfnt(datos, "maxp")
	loca := datosTablaSfnt(datos, "loca")
	glyf := datosTablaSfnt(datos, "glyf")
	if len(head) < 54 || len(maxp) < 6 || len(glyf) < 10 {
		return datos
	}
	ng := int(binary.BigEndian.Uint16(maxp[4:6]))
	if ng <= 0 || ng > 0xFFFF {
		return datos
	}
	offs, _, ok := leerOffsetsLoca(head, loca, ng)
	if !ok || offs[ng] > len(glyf) {
		return datos
	}
	gFail, nFallos := probarGlifosTTF(head, maxp, loca, glyf, ng)
	if nFallos == 0 {
		return datos
	}
	if mejor, okB := buscarGlyfRestaurado(head, maxp, loca, glyf, offs, ng, gFail, nFallos); okB {
		if s, okR := reemplazarTablaSfnt(datos, "glyf", mejor); okR {
			return s
		}
	}
	return datos
}

func buscarGlyfRestaurado(head, maxp, loca, glyf []byte, offs []int, ng, gFail, nFallos int) ([]byte, bool) {
	mejorGlyf := glyf
	mejorFallos := nFallos
	vistos := 0
	for g := gFail; g >= 0 && vistos < 6; g-- {
		a, z := offs[g], offs[g+1]
		if z <= a || z > len(glyf) {
			continue
		}
		vistos++
		cand := make([]byte, len(glyf))
		copy(cand, glyf[:z-1])
		copy(cand[z:], glyf[z-1:len(glyf)-1])
		p1, f1 := probarGlifosTTF(head, maxp, loca, cand, ng)
		if f1 > 0 && p1 == g && z-a >= 10 {
			clear(cand[a:z])
			_, f1 = probarGlifosTTF(head, maxp, loca, cand, ng)
		}
		if f1 < mejorFallos {
			mejorGlyf, mejorFallos = cand, f1
			if f1 == 0 {
				break
			}
		}
	}
	return mejorGlyf, mejorFallos < nFallos
}

func probarGlifosTTF(head, maxp, loca, glyf []byte, ng int) (int, int) {
	be := binary.BigEndian
	hhea := make([]byte, 36)
	be.PutUint32(hhea[0:], 0x00010000)
	be.PutUint16(hhea[4:], 800)
	be.PutUint16(hhea[34:], 1)
	hmtx := make([]byte, 4+2*max(ng-1, 0))
	post := make([]byte, 32)
	be.PutUint32(post[0:], 0x00030000)
	otf := sfntNuevo(0x00010000, "head", head)
	for _, par := range []entradaTablaSfnt{
		{tag: "maxp", datos: maxp},
		{tag: "loca", datos: loca},
		{tag: "glyf", datos: glyf},
		{tag: "hhea", datos: hhea},
		{tag: "hmtx", datos: hmtx},
		{tag: "post", datos: post},
	} {
		var ok bool
		if otf, ok = reemplazarTablaSfnt(otf, par.tag, par.datos); !ok {
			return 0, ng
		}
	}
	var ok bool
	if otf, ok = reescribirCmapUnicodeTTF(otf, map[uint16]uint16{0xE000: 0}); !ok {
		return 0, ng
	}
	cara, err := sfnt.Parse(otf)
	if err != nil {
		return 0, ng
	}
	var buf sfnt.Buffer
	primer, fallos := -1, 0
	for g := 0; g < ng; g++ {
		if _, err := cara.LoadGlyph(&buf, sfnt.GlyphIndex(g), 1000, nil); err != nil {
			if primer < 0 {
				primer = g
			}
			fallos++
		}
	}
	return max(primer, 0), fallos
}

func ensamblarSfnt(version uint32, tablas []entradaTablaSfnt) []byte {
	sort.Slice(tablas, func(i, j int) bool { return tablas[i].tag < tablas[j].tag })
	be := binary.BigEndian
	n := len(tablas)
	cabecera := 12 + n*16
	total := cabecera
	for _, t := range tablas {
		total += (len(t.datos) + 3) &^ 3
	}
	out := make([]byte, cabecera, total)
	be.PutUint32(out[0:4], version)
	be.PutUint16(out[4:6], uint16(n&0xFFFF)) //nolint:gosec // n <= 512
	potencia, sel := 1, 0
	for potencia*2 <= n {
		potencia *= 2
		sel++
	}
	be.PutUint16(out[6:8], uint16(potencia*16&0xFFFF))          //nolint:gosec // n <= 512
	be.PutUint16(out[8:10], uint16(sel&0xFFFF))                 //nolint:gosec // n <= 512
	be.PutUint16(out[10:12], uint16((n*16-potencia*16)&0xFFFF)) //nolint:gosec // n <= 512
	for i, t := range tablas {
		off := len(out)
		out = append(out, t.datos...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		rec := out[12+i*16 : 12+i*16+16]
		copy(rec[0:4], t.tag)
		be.PutUint32(rec[4:8], sumaSfnt(t.datos))
		be.PutUint32(rec[8:12], uint32(off))           //nolint:gosec // cabe en memoria
		be.PutUint32(rec[12:16], uint32(len(t.datos))) //nolint:gosec // cabe en memoria
	}
	return out
}

// repararCabecerasBaseSfnt corrige tamaños y desplazamientos de 1 byte en
// `head`, `hhea`, `maxp` y `loca` antes de leer las tablas crudas o abrir `sfnt`.
func repararCabecerasBaseSfnt(datos []byte) []byte {
	esCFF := len(datosTablaSfnt(datos, "CFF ")) > 0
	datos = repararTablaHeadSfnt(datos, esCFF)

	if hhea := datosTablaSfnt(datos, "hhea"); len(hhea) > 36 {
		if s, ok := reemplazarTablaSfnt(datos, "hhea", hhea[:36]); ok {
			datos = s
		}
	}

	if !esCFF {
		if maxp := datosTablaSfnt(datos, "maxp"); len(maxp) >= 6 && maxp[0] == 1 && maxp[1] == 0 && maxp[2] == 0 {
			nueva := make([]byte, 32)
			copy(nueva[1:], maxp[:min(31, len(maxp))])
			if s, ok := reemplazarTablaSfnt(datos, "maxp", nueva); ok {
				datos = s
			}
		}
		datos = repararTablaLocaSfnt(datos)
	}
	return datos
}

func repararTablaHeadSfnt(datos []byte, esCFF bool) []byte {
	head := datosTablaSfnt(datos, "head")
	if len(head) < 53 {
		return datos
	}
	be := binary.BigEndian
	actual := normalizarLongitudHead(head)
	if len(actual) != 54 {
		return datos
	}
	cambio := !bytes.Equal(actual, head)
	copia := append([]byte(nil), actual...)
	if be.Uint32(copia[12:16]) != 0x5F0F3CF5 {
		be.PutUint32(copia[12:16], 0x5F0F3CF5)
		cambio = true
	}
	if upem := be.Uint16(copia[18:20]); upem < 16 || upem > 16384 {
		be.PutUint16(copia[18:20], 1000)
		cambio = true
	}
	if !esCFF && int16(be.Uint16(copia[50:52])) != 0 && int16(be.Uint16(copia[50:52])) != 1 { //nolint:gosec // campo int16 de head
		be.PutUint16(copia[50:52], 1)
		cambio = true
	}
	if cambio {
		if s, ok := reemplazarTablaSfnt(datos, "head", copia); ok {
			return s
		}
	}
	return datos
}

func normalizarLongitudHead(head []byte) []byte {
	be := binary.BigEndian
	if len(head) >= 15 && (len(head) < 16 || be.Uint32(head[12:16]) != 0x5F0F3CF5) && be.Uint32(head[11:15]) == 0x5F0F3CF5 {
		nueva := make([]byte, 54)
		copy(nueva[1:], head[:min(53, len(head))])
		return nueva
	}
	if len(head) > 54 {
		return head[:54]
	}
	return head
}

func repararTablaLocaSfnt(datos []byte) []byte {
	be := binary.BigEndian
	head := datosTablaSfnt(datos, "head")
	maxp := datosTablaSfnt(datos, "maxp")
	loca := datosTablaSfnt(datos, "loca")
	glyf := datosTablaSfnt(datos, "glyf")
	if len(head) < 54 || len(maxp) < 6 || len(loca) < 2 || glyf == nil {
		return datos
	}
	ng := int(be.Uint16(maxp[4:6]))
	if ng <= 0 {
		return datos
	}
	if int16(be.Uint16(head[50:52])) == 0 { //nolint:gosec // int16 indexToLocFormat
		return repararLocaCortaSfnt(datos, loca, glyf, ng)
	}
	return repararLocaLargaSfnt(datos, loca, glyf, ng)
}

func repararLocaCortaSfnt(datos, loca, glyf []byte, ng int) []byte {
	be := binary.BigEndian
	esp := 2 * (ng + 1)
	switch {
	case len(loca) > esp:
		if s, ok := reemplazarTablaSfnt(datos, "loca", loca[:esp]); ok {
			return s
		}
	case len(loca) < esp:
		nVal := len(loca) / 2
		ult := be.Uint16(loca[2*(nVal-1):])
		if 2*int(ult) > len(glyf) {
			ult = uint16(min(len(glyf)/2, 0xFFFF)) //nolint:gosec // acotado a 0xFFFF
		}
		nueva := make([]byte, esp)
		copy(nueva, loca[:2*nVal])
		for i := nVal; i <= ng; i++ {
			be.PutUint16(nueva[2*i:], ult)
		}
		if s, ok := reemplazarTablaSfnt(datos, "loca", nueva); ok {
			return s
		}
	}
	return datos
}

func repararLocaLargaSfnt(datos, loca, glyf []byte, ng int) []byte {
	be := binary.BigEndian
	esp := 4 * (ng + 1)
	switch {
	case len(loca) > esp:
		loca = loca[:esp]
	case len(loca) >= 4 && len(loca) < esp:
		nVal := len(loca) / 4
		ult := be.Uint32(loca[4*(nVal-1):])
		if uint64(ult) > uint64(len(glyf)) {
			ult = uint32(len(glyf)) //nolint:gosec // acotado por maxFuenteIncrustada
		}
		nueva := make([]byte, esp)
		copy(nueva, loca[:4*nVal])
		for i := nVal; i <= ng; i++ {
			be.PutUint32(nueva[4*i:], ult)
		}
		loca = nueva
	}
	if len(loca) == esp && !esLocaLargaValida(loca, ng, len(glyf)) {
		cand := make([]byte, esp)
		copy(cand[1:], loca[:esp-1])
		if esLocaLargaValida(cand, ng, len(glyf)) {
			loca = cand
		} else {
			loca = acotarLocaLarga(loca, ng, len(glyf))
		}
	}
	if !bytes.Equal(loca, datosTablaSfnt(datos, "loca")) {
		if s, ok := reemplazarTablaSfnt(datos, "loca", loca); ok {
			return s
		}
	}
	return datos
}

func esLocaLargaValida(loca []byte, ng, lenGlyf int) bool {
	if len(loca) < 4*(ng+1) {
		return false
	}
	be := binary.BigEndian
	prev := uint32(0)
	tope := uint32(lenGlyf) //nolint:gosec // acotado por maxFuenteIncrustada
	for i := 0; i <= ng; i++ {
		v := be.Uint32(loca[4*i:])
		if (i > 0 && v < prev) || v > tope {
			return false
		}
		prev = v
	}
	return true
}

func acotarLocaLarga(loca []byte, ng, lenGlyf int) []byte {
	be := binary.BigEndian
	out := make([]byte, 4*(ng+1))
	prev := uint32(0)
	tope := uint32(lenGlyf) //nolint:gosec // acotado por maxFuenteIncrustada
	for i := 0; i <= ng; i++ {
		v := min(max(be.Uint32(loca[4*i:]), prev), tope)
		be.PutUint32(out[4*i:], v)
		prev = v
	}
	return out
}

// repararTablasMetricasSfnt garantiza que `head`, `maxp`, `loca`, `CFF `, `hhea`,
// `hmtx`, `post`, `OS/2` y `name` cumplen las invariantes de `x/image/font/sfnt`
// y del saneador OTS del navegador.
func (f *fuentePDF) repararTablasMetricasSfnt(datos []byte) []byte {
	be := binary.BigEndian
	datos = repararCabecerasBaseSfnt(datos)
	if cff := datosTablaSfnt(datos, "CFF "); len(cff) > 0 {
		if sano := aplanarCFFEnSfnt(cff); !bytes.Equal(sano, cff) {
			if s, ok := reemplazarTablaSfnt(datos, "CFF ", sano); ok {
				datos = s
			}
		}
	}
	datos = clonarGlifoCeroSiHaceFalta(datos, f.crudo)

	maxp := datosTablaSfnt(datos, "maxp")
	numGlifos := 0
	if len(maxp) >= 6 {
		numGlifos = int(be.Uint16(maxp[4:6]))
		if f.crudo != nil && numGlifos > 0 {
			f.crudo.numGlifos = numGlifos
		}
	}
	datos = ajustarHmtxYHheaSfnt(datos, numGlifos)
	if !esTablaPostValida(datosTablaSfnt(datos, "post"), numGlifos) {
		post := make([]byte, 32)
		be.PutUint32(post, 0x00030000)
		if s, ok := reemplazarTablaSfnt(datos, "post", post); ok {
			datos = s
		}
	}
	if !esTablaOS2Valida(datosTablaSfnt(datos, "OS/2")) {
		datos = f.reponerOS2(datos)
	}
	if !esTablaNameValida(datosTablaSfnt(datos, "name")) {
		if s, ok := reemplazarTablaSfnt(datos, "name", tablaNombre("SytuePDF")); ok {
			datos = s
		}
	}
	return datos
}

func ajustarHmtxYHheaSfnt(datos []byte, numGlifos int) []byte {
	hhea := datosTablaSfnt(datos, "hhea")
	hmtx := datosTablaSfnt(datos, "hmtx")
	if numGlifos <= 0 || len(hhea) < 36 {
		return datos
	}
	be := binary.BigEndian
	numHM := int(be.Uint16(hhea[34:36]))
	if numHMNuevo := min(max(numHM, 1), numGlifos); numHMNuevo != numHM {
		hheaCopia := append([]byte(nil), hhea...)
		be.PutUint16(hheaCopia[34:36], uint16(numHMNuevo)) //nolint:gosec // <= numGlifos <= 0xFFFF
		if s, ok := reemplazarTablaSfnt(datos, "hhea", hheaCopia); ok {
			datos = s
		}
		numHM = numHMNuevo
	}
	esperado := 4*numHM + 2*(numGlifos-numHM)
	if len(hmtx) == esperado {
		return datos
	}
	nueva := make([]byte, esperado)
	copy(nueva, hmtx)
	if len(hmtx) >= 4 && len(hmtx) < 4*numHM {
		ultOff := (len(hmtx)/4 - 1) * 4
		ultAvance := be.Uint16(hmtx[ultOff : ultOff+2])
		for m := len(hmtx) / 4; m < numHM; m++ {
			be.PutUint16(nueva[4*m:4*m+2], ultAvance)
		}
	}
	if s, ok := reemplazarTablaSfnt(datos, "hmtx", nueva); ok {
		return s
	}
	return datos
}

func esTablaOS2Valida(os2 []byte) bool {
	if len(os2) < 78 {
		return false
	}
	be := binary.BigEndian
	ver := be.Uint16(os2[0:2])
	peso := be.Uint16(os2[4:6])
	ancho := be.Uint16(os2[6:8])
	return ver <= 5 && peso >= 1 && peso <= 1000 && ancho >= 1 && ancho <= 9
}

// clonarGlifoCeroSiHaceFalta duplica el contorno de GID 0 en un nuevo GID > 0
// cuando la `cmap` original de un subconjunto TrueType apunta explícitamente a
// GID 0 (issue8234.pdf): los motores web descartan siempre GID 0 como `.notdef`.
func clonarGlifoCeroSiHaceFalta(datos []byte, crudo *tablasCrudasSfnt) []byte {
	if crudo == nil || crudo.esOTTO || (len(crudo.simboloCero) == 0 && len(crudo.unicodeCero) == 0) {
		return datos
	}
	nuevos, gidClon, ok := clonarGlifoCeroTTF(datos)
	if !ok {
		return datos
	}
	crudo.numGlifos = int(gidClon) + 1
	for u := range crudo.simboloCero {
		if crudo.simboloAGid[u] == 0 {
			crudo.simboloAGid[u] = gidClon
		}
	}
	for r := range crudo.unicodeCero {
		if crudo.unicodeAGid[r] == 0 {
			crudo.unicodeAGid[r] = gidClon
		}
	}
	return nuevos
}

func clonarGlifoCeroTTF(datos []byte) ([]byte, uint16, bool) {
	be := binary.BigEndian
	head := datosTablaSfnt(datos, "head")
	maxp := datosTablaSfnt(datos, "maxp")
	loca := datosTablaSfnt(datos, "loca")
	glyf := datosTablaSfnt(datos, "glyf")
	hhea := datosTablaSfnt(datos, "hhea")
	hmtx := datosTablaSfnt(datos, "hmtx")
	if len(head) < 54 || len(maxp) < 6 || len(hhea) < 36 || len(hmtx) < 4 || glyf == nil {
		return nil, 0, false
	}
	ng := int(be.Uint16(maxp[4:6]))
	if ng <= 0 || ng >= 0xFFFF {
		return nil, 0, false
	}
	offs, corta, ok := leerOffsetsLoca(head, loca, ng)
	if !ok || offs[0] < 0 || offs[1] <= offs[0] || offs[1] > len(glyf) || offs[ng] < 0 || offs[ng] > len(glyf) {
		return nil, 0, false
	}
	glifo0 := append([]byte(nil), glyf[offs[0]:offs[1]]...)
	if len(glifo0)%2 == 1 {
		glifo0 = append(glifo0, 0)
	}
	baseGlyf := make([]byte, 0, offs[ng]+len(glifo0)+2)
	baseGlyf = append(baseGlyf, glyf[:offs[ng]]...)
	if len(baseGlyf)%2 == 1 {
		baseGlyf = append(baseGlyf, 0)
		offs[ng] = len(baseGlyf)
	}
	baseGlyf = append(baseGlyf, glifo0...)
	offsN := make([]int, len(offs)+1)
	copy(offsN, offs)
	offsN[len(offs)] = len(baseGlyf)

	headN, locaN := codificarLocaClonada(head, offsN, corta, len(baseGlyf))
	maxpN := append([]byte(nil), maxp...)
	be.PutUint16(maxpN[4:6], uint16(ng+1)) //nolint:gosec // ng+1 <= 0xFFFF
	hheaN, hmtxN := extenderHmtxClonada(hhea, hmtx, ng)

	out := datos
	for _, par := range []entradaTablaSfnt{
		{tag: "head", datos: headN},
		{tag: "maxp", datos: maxpN},
		{tag: "loca", datos: locaN},
		{tag: "glyf", datos: baseGlyf},
		{tag: "hhea", datos: hheaN},
		{tag: "hmtx", datos: hmtxN},
	} {
		if out, ok = reemplazarTablaSfnt(out, par.tag, par.datos); !ok {
			return nil, 0, false
		}
	}
	return out, uint16(ng), true //nolint:gosec // ng < 0xFFFF
}

func codificarLocaClonada(head []byte, offs []int, corta bool, lenGlyf int) ([]byte, []byte) {
	be := binary.BigEndian
	headN := append([]byte(nil), head...)
	if corta && lenGlyf/2 <= 0xFFFF {
		locaN := make([]byte, 2*len(offs))
		for i, o := range offs {
			be.PutUint16(locaN[2*i:], uint16(o/2)) //nolint:gosec // <= 0xFFFF comprobado
		}
		return headN, locaN
	}
	be.PutUint16(headN[50:52], 1)
	locaN := make([]byte, 4*len(offs))
	for i, o := range offs {
		be.PutUint32(locaN[4*i:], uint32(o)) //nolint:gosec // acotado por maxFuenteIncrustada
	}
	return headN, locaN
}

func extenderHmtxClonada(hhea, hmtx []byte, ng int) ([]byte, []byte) {
	be := binary.BigEndian
	nuevoNG := ng + 1
	numHM := min(max(int(be.Uint16(hhea[34:36])), 1), ng)
	hmtxN := make([]byte, 4*nuevoNG)
	for g := 0; g < ng; g++ {
		if g < numHM && 4*(g+1) <= len(hmtx) {
			copy(hmtxN[4*g:4*g+4], hmtx[4*g:4*g+4])
			continue
		}
		ultAv := be.Uint16(hmtx[4*(min(numHM, len(hmtx)/4)-1):])
		be.PutUint16(hmtxN[4*g:], ultAv)
		offLSB := 4*numHM + 2*(g-numHM)
		if offLSB+2 <= len(hmtx) {
			copy(hmtxN[4*g+2:4*g+4], hmtx[offLSB:offLSB+2])
		}
	}
	copy(hmtxN[4*ng:4*ng+4], hmtx[0:4])
	hheaN := append([]byte(nil), hhea...)
	be.PutUint16(hheaN[34:36], uint16(nuevoNG)) //nolint:gosec // nuevoNG <= 0xFFFF
	return hheaN, hmtxN
}

func (f *fuentePDF) reponerOS2(datos []byte) []byte {
	head, hhea := datosTablaSfnt(datos, "head"), datosTablaSfnt(datos, "hhea")
	if len(head) < 54 || len(hhea) < 12 {
		return datos
	}
	os2 := make([]byte, 78)
	be := binary.BigEndian
	be.PutUint16(os2[2:4], be.Uint16(hhea[10:12]))
	peso, seleccion := uint16(400), uint16(64)
	if f.negrita {
		peso, seleccion = 700, 32
	}
	if f.italica {
		seleccion &^= 64
		seleccion |= 1
	}
	be.PutUint16(os2[4:6], peso)
	be.PutUint16(os2[6:8], 5)
	copy(os2[58:62], "SYTU")
	be.PutUint16(os2[62:64], seleccion)
	be.PutUint16(os2[66:68], 0xFFFF)
	copy(os2[68:74], hhea[4:10])
	copy(os2[74:76], hhea[4:6])
	descenso := be.Uint16(hhea[6:8])
	be.PutUint16(os2[76:78], ^descenso+1)
	if salida, ok := reemplazarTablaSfnt(datos, "OS/2", os2); ok {
		return salida
	}
	return datos
}

func esTablaPostValida(post []byte, numGlifos int) bool {
	if len(post) < 32 {
		return false
	}
	ver := binary.BigEndian.Uint32(post[0:4])
	switch ver {
	case 0x00010000, 0x00030000:
		return true
	case 0x00020000:
		if len(post) < 34 {
			return false
		}
		n := int(binary.BigEndian.Uint16(post[32:34]))
		if (numGlifos > 0 && n != numGlifos) || 34+2*n > len(post) {
			return false
		}
		// Verificar que todas las cadenas Pascal caben dentro de la tabla.
		p := 34 + 2*n
		var extras int
		for p < len(post) {
			l := int(post[p])
			if p+1+l > len(post) {
				return false
			}
			p += 1 + l
			extras++
		}
		for i := 0; i < n; i++ {
			idx := int(binary.BigEndian.Uint16(post[34+2*i:]))
			if idx >= 258 && idx-258 >= extras {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func esTablaNameValida(tb []byte) bool {
	if len(tb) < 18 {
		return false
	}
	be := binary.BigEndian
	fmtVer := be.Uint16(tb[0:2])
	cnt := int(be.Uint16(tb[2:4]))
	offStr := int(be.Uint16(tb[4:6]))
	if (fmtVer != 0 && fmtVer != 1) || cnt <= 0 || 6+12*cnt > offStr || offStr > len(tb) {
		return false
	}
	tieneWin := false
	var claveAnt uint64
	for i := 0; i < cnt; i++ {
		r := tb[6+12*i : 6+12*(i+1)]
		pid, eid, lid, nid := be.Uint16(r[0:2]), be.Uint16(r[2:4]), be.Uint16(r[4:6]), be.Uint16(r[6:8])
		lon, off := int(be.Uint16(r[8:10])), int(be.Uint16(r[10:12]))
		if offStr+off+lon > len(tb) {
			return false
		}
		clave := uint64(pid)<<48 | uint64(eid)<<32 | uint64(lid)<<16 | uint64(nid)
		if i > 0 && clave < claveAnt {
			return false
		}
		claveAnt = clave
		if pid == 3 && (nid == 1 || nid == 6) && lon > 0 && lon%2 == 0 {
			tieneWin = true
		}
	}
	return tieneWin
}

// --- Lectura directa de cmap, post y CFF crudos ------------------------------

func leerTablasCrudasSfnt(datos []byte) *tablasCrudasSfnt {
	t := &tablasCrudasSfnt{
		unicodeAGid: map[rune]uint16{},
		unicodeCero: map[rune]bool{},
		gidARuna:    map[uint16]rune{},
		simboloAGid: map[uint16]uint16{},
		simboloCero: map[uint16]bool{},
		propiaAGid:  map[byte]uint16{},
		nombreAGid:  map[string]uint16{},
	}
	if len(datos) < 12 {
		return t
	}
	t.esOTTO = string(datos[:4]) == "OTTO" || len(datosTablaSfnt(datos, "CFF ")) > 0
	if maxp := datosTablaSfnt(datos, "maxp"); len(maxp) >= 6 {
		t.numGlifos = int(binary.BigEndian.Uint16(maxp[4:6]))
	}
	t.leerCmapCruda(datosTablaSfnt(datos, "cmap"))
	t.leerPostCruda(datosTablaSfnt(datos, "post"))
	if cff := datosTablaSfnt(datos, "CFF "); len(cff) > 0 {
		t.leerCFFCruda(cff)
	}
	t.completarGidARuna()
	return t
}

type subInfoCmap struct {
	off  int
	prio int
}

func (t *tablasCrudasSfnt) leerCmapCruda(cmap []byte) {
	if len(cmap) < 4 {
		return
	}
	be := binary.BigEndian
	numSub := int(be.Uint16(cmap[2:4]))
	var subsUnicode []subInfoCmap
	for i := 0; i < numSub && 4+8*(i+1) <= len(cmap); i++ {
		rec := cmap[4+8*i : 4+8*(i+1)]
		pid, psid := be.Uint16(rec[0:2]), be.Uint16(rec[2:4])
		off := int(be.Uint32(rec[4:8]))
		if off < 0 || off+4 > len(cmap) {
			continue
		}
		if prio := prioridadUnicodeCmap(pid, psid); prio > 0 {
			subsUnicode = append(subsUnicode, subInfoCmap{off: off, prio: prio})
			continue
		}
		t.leerSubtablaNoUnicodeCmap(pid, psid, cmap[off:])
	}
	sort.SliceStable(subsUnicode, func(i, j int) bool { return subsUnicode[i].prio < subsUnicode[j].prio })
	for _, s := range subsUnicode {
		t.tieneUnicode = true
		t.acumularUnicodeCmap(cmap[s.off:])
	}
}

func prioridadUnicodeCmap(pid, psid uint16) int {
	switch {
	case pid == 3 && psid == 10:
		return 1
	case pid == 0 && (psid == 4 || psid == 6):
		return 2
	case pid == 3 && psid == 1:
		return 3
	case pid == 0:
		return 4
	default:
		return 0
	}
}

func (t *tablasCrudasSfnt) leerSubtablaNoUnicodeCmap(pid, psid uint16, sub []byte) {
	glifoValido := func(gid uint16) bool {
		return gid != 0 && (t.numGlifos == 0 || int(gid) < t.numGlifos)
	}
	switch {
	case pid == 3 && psid == 0:
		t.tieneSimbolo = true
		recorrerSubtablaCmapConCero(sub, func(c uint32, gid uint16) {
			if c <= 0xFFFF && glifoValido(gid) {
				u := uint16(c & 0xFFFF)
				if _, ya := t.simboloAGid[u]; !ya {
					t.simboloAGid[u] = gid
				}
			}
		}, func(c uint32) {
			if c <= 0xFFFF {
				t.simboloCero[uint16(c&0xFFFF)] = true
			}
		})
	case pid == 1 && psid == 0:
		t.tieneMac = true
		recorrerSubtablaCmap(sub, func(c uint32, gid uint16) {
			if c < 256 && glifoValido(gid) && t.macRomanAGid[c] == 0 {
				t.macRomanAGid[c] = gid
			}
		})
	case pid == 1 && psid == 4:
		recorrerSubtablaCmap(sub, func(c uint32, gid uint16) {
			if c < 256 && glifoValido(gid) && t.macArabeAGid[c] == 0 {
				t.macArabeAGid[c] = gid
			}
		})
	}
}

func (t *tablasCrudasSfnt) acumularUnicodeCmap(sub []byte) {
	recorrerSubtablaCmapConCero(sub, func(c uint32, gid uint16) {
		if gid == 0 || c > maxRuna || (c >= 0xD800 && c <= 0xDFFF) {
			return
		}
		if t.numGlifos > 0 && int(gid) >= t.numGlifos {
			return
		}
		r := rune(c) //nolint:gosec // c <= maxRuna
		if _, ya := t.unicodeAGid[r]; !ya {
			t.unicodeAGid[r] = gid
		}
	}, func(c uint32) {
		if c <= maxRuna && (c < 0xD800 || c > 0xDFFF) {
			t.unicodeCero[rune(c)] = true //nolint:gosec // c <= maxRuna
		}
	})
}

func recorrerSubtablaCmap(sub []byte, fn func(codigo uint32, gid uint16)) {
	recorrerSubtablaCmapConCero(sub, fn, nil)
}

func recorrerSubtablaCmapConCero(sub []byte, fn func(codigo uint32, gid uint16), ceroFn func(codigo uint32)) {
	if len(sub) < 4 {
		return
	}
	be := binary.BigEndian
	formato := be.Uint16(sub[0:2])
	switch formato {
	case 0:
		lon := int(be.Uint16(sub[2:4]))
		if lon > len(sub) {
			lon = len(sub)
		}
		for c := 0; c < 256 && 6+c < lon; c++ {
			if g := uint16(sub[6+c]); g != 0 {
				fn(uint32(c), g)
			}
		}
	case 4:
		recorrerCmapFormato4(sub, fn, ceroFn)
	case 6:
		if len(sub) < 10 {
			return
		}
		lon := int(be.Uint16(sub[2:4]))
		if lon > len(sub) {
			lon = len(sub)
		}
		primero := uint32(be.Uint16(sub[6:8]))
		cuenta := int(be.Uint16(sub[8:10]))
		for i := 0; i < cuenta && 10+2*(i+1) <= lon; i++ {
			if g := be.Uint16(sub[10+2*i:]); g != 0 {
				fn(primero+uint32(i), g)
			}
		}
	case 12, 13:
		recorrerCmapFormato12O13(sub, formato == 13, fn, ceroFn)
	}
}

func recorrerCmapFormato4(sub []byte, fn func(codigo uint32, gid uint16), ceroFn func(codigo uint32)) {
	be := binary.BigEndian
	lon := int(be.Uint16(sub[2:4]))
	if lon > len(sub) {
		lon = len(sub)
	}
	if lon < 16 {
		return
	}
	segCount := int(be.Uint16(sub[6:8])) / 2
	offEnd := 14
	offStart := offEnd + 2*segCount + 2
	offDelta := offStart + 2*segCount
	offRange := offDelta + 2*segCount
	if offRange+2*segCount > lon {
		return
	}
	for i := 0; i < segCount; i++ {
		fin := uint32(be.Uint16(sub[offEnd+2*i:]))
		ini := uint32(be.Uint16(sub[offStart+2*i:]))
		delta := be.Uint16(sub[offDelta+2*i:])
		roPos := offRange + 2*i
		ro := int(be.Uint16(sub[roPos:]))
		if ini > fin || (ini == 0xFFFF && fin == 0xFFFF) {
			continue
		}
		for c := ini; c <= fin; c++ {
			var gid uint16
			if ro == 0 {
				gid = uint16(c&0xFFFF) + delta
				if gid == 0 && ceroFn != nil && c >= 0x20 && c != 0x7F && c < 0xFFFD {
					ceroFn(c)
				}
			} else {
				idx := roPos + ro + 2*int(c-ini)
				if idx < 0 || idx+2 > lon {
					continue
				}
				gid = be.Uint16(sub[idx:])
				if gid != 0 {
					gid += delta
				}
			}
			if gid != 0 {
				fn(c, gid)
			}
		}
	}
}

func recorrerCmapFormato12O13(sub []byte, constante bool, fn func(codigo uint32, gid uint16), ceroFn func(codigo uint32)) {
	if len(sub) < 16 {
		return
	}
	be := binary.BigEndian
	lon := uint64(be.Uint32(sub[4:8]))
	if lon > uint64(len(sub)) {
		lon = uint64(len(sub))
	}
	nGrupos := uint64(be.Uint32(sub[12:16]))
	for i := uint64(0); i < nGrupos && 16+12*(i+1) <= lon; i++ {
		g := sub[16+12*i : 16+12*(i+1)]
		ini, fin, gidBase := be.Uint32(g[0:4]), be.Uint32(g[4:8]), be.Uint32(g[8:12])
		if fin < ini || fin-ini > 0x20000 || ini > maxRuna {
			continue
		}
		if fin > maxRuna {
			fin = maxRuna
		}
		for c := ini; c <= fin; c++ {
			gid := gidBase
			if !constante {
				gid += c - ini
			}
			if gid == 0 && ceroFn != nil && c >= 0x20 && c != 0x7F && (c < 0xFFFD || c > 0xFFFF) {
				ceroFn(c)
			} else if gid > 0 && gid <= 0xFFFF {
				fn(c, uint16(gid))
			}
		}
	}
}

func (t *tablasCrudasSfnt) leerPostCruda(post []byte) {
	if len(post) < 32 {
		return
	}
	switch binary.BigEndian.Uint32(post[0:4]) {
	case 0x00010000:
		t.leerPostV1()
	case 0x00020000:
		t.leerPostV2(post)
	}
}

func (t *tablasCrudasSfnt) registrarNombreGlifo(nom string, g uint16) {
	if nom != "" && nom != ".notdef" {
		if _, ya := t.nombreAGid[nom]; !ya {
			t.nombreAGid[nom] = g
		}
	}
}

func (t *tablasCrudasSfnt) leerPostV1() {
	lim := 258
	if t.numGlifos > 0 && t.numGlifos < lim {
		lim = t.numGlifos
	}
	for g := 1; g < lim; g++ {
		t.registrarNombreGlifo(nombresMacPost[g], uint16(g)) //nolint:gosec // g < 258
	}
}

func (t *tablasCrudasSfnt) leerPostV2(post []byte) {
	if len(post) < 34 {
		return
	}
	be := binary.BigEndian
	n := int(be.Uint16(post[32:34]))
	if n <= 0 || 34+2*n > len(post) {
		return
	}
	extras := leerCadenasPascalPost(post[34+2*n:])
	for g := 1; g < n && g <= 0xFFFF; g++ {
		idx := int(be.Uint16(post[34+2*g:]))
		if idx < 258 {
			t.registrarNombreGlifo(nombresMacPost[idx], uint16(g)) //nolint:gosec // g <= 0xFFFF
		} else if idx-258 < len(extras) {
			t.registrarNombreGlifo(extras[idx-258], uint16(g)) //nolint:gosec // g <= 0xFFFF
		}
	}
}

func leerCadenasPascalPost(datos []byte) []string {
	var extras []string
	for p := 0; p < len(datos); {
		l := int(datos[p])
		if p+1+l > len(datos) {
			break
		}
		extras = append(extras, string(datos[p+1:p+1+l]))
		p += 1 + l
	}
	return extras
}

func (t *tablasCrudasSfnt) leerCFFCruda(cff []byte) {
	info, err := leerCFF(cff)
	if err != nil || info.numGlifos <= 0 || info.numGlifos > 0xFFFF {
		return
	}
	if t.numGlifos == 0 {
		t.numGlifos = info.numGlifos
	}
	if info.cid {
		t.cidAGidCFF = construirMapaCidCFF(info.cids)
		return
	}
	for c, g := range info.propia {
		if g > 0 && g <= 0xFFFF {
			t.propiaAGid[c] = uint16(g) //nolint:gosec // comprobado
		}
	}
	for g, nom := range info.nombres {
		if g > 0 && g <= 0xFFFF {
			t.registrarNombreGlifo(nom, uint16(g)) //nolint:gosec // comprobado
		}
	}
}

func construirMapaCidCFF(cids []int) []byte {
	mayor := 0
	for _, cid := range cids {
		mayor = max(mayor, cid)
	}
	if mayor < 0 || mayor > maxCID {
		return nil
	}
	tabla := make([]byte, 2*(mayor+1))
	for g, cid := range cids {
		if cid >= 0 && cid <= mayor {
			binary.BigEndian.PutUint16(tabla[2*cid:], uint16(g)) //nolint:gosec // g < numGlifos <= 0xFFFF
		}
	}
	return tabla
}

func esRunaTextoUtil(r rune) bool {
	if r < 0x20 || (r >= 0xD800 && r <= 0xDFFF) {
		return false
	}
	if (r >= 0xE000 && r <= 0xF8FF) || r >= 0xF0000 {
		return false
	}
	return r <= maxRuna
}

func (t *tablasCrudasSfnt) completarGidARuna() {
	runas := make([]rune, 0, len(t.unicodeAGid))
	for r := range t.unicodeAGid {
		if esRunaTextoUtil(r) {
			runas = append(runas, r)
		}
	}
	slices.Sort(runas)
	for _, r := range runas {
		gid := t.unicodeAGid[r]
		if _, ya := t.gidARuna[gid]; !ya {
			t.gidARuna[gid] = r
		}
	}
	if len(t.nombreAGid) > 0 {
		nombres := make([]string, 0, len(t.nombreAGid))
		for n := range t.nombreAGid {
			nombres = append(nombres, n)
		}
		slices.Sort(nombres)
		for _, n := range nombres {
			gid := t.nombreAGid[n]
			if _, ya := t.gidARuna[gid]; ya {
				continue
			}
			if r, ok := runaDeNombreGlifo(n); ok && esRunaTextoUtil(r) {
				t.gidARuna[gid] = r
			}
		}
	}
	for c := 0; c < 256; c++ {
		if gid := t.macRomanAGid[c]; gid != 0 {
			if _, ya := t.gidARuna[gid]; !ya {
				r := charmap.Macintosh.DecodeByte(byte(c))
				if esRunaTextoUtil(r) {
					t.gidARuna[gid] = r
				}
			}
		}
	}
}

// buscarSimbolo30 consulta una subtabla Microsoft Symbol (3,0) en las cuatro
// páginas permitidas por ISO 32000-1 §9.6.6.4 (F000, F100, F200 y 0000).
func (t *tablasCrudasSfnt) buscarSimbolo30(c byte) (uint16, bool) {
	if len(t.simboloAGid) == 0 {
		return 0, false
	}
	for _, base := range [4]uint16{0xF000, 0xF100, 0xF200, 0x0000} {
		if gid := t.simboloAGid[base|uint16(c)]; gid != 0 {
			return gid, true
		}
	}
	return 0, false
}

// resolverGlifoSimple aplica el orden de ISO 32000-1 §9.6.6.4 sobre las tablas
// crudas originales de la fuente simple (TrueType, OpenType o CFF envuelto).
func (f *fuentePDF) resolverGlifoSimple(crudo *tablasCrudasSfnt, g glifo) (uint16, bool) {
	if crudo == nil {
		return 0, false
	}
	c := byte(g.codigo & 0xFF)
	uni := primeraRuna(g.texto)
	if f.simbolica {
		return f.resolverGlifoSimbolico(crudo, c, uni)
	}
	if uniEnc := f.runaEncodingSimple(c); uniEnc != 0 {
		if gid, ok := f.resolverGlifoNoSimbolico(crudo, c, uniEnc); ok {
			return gid, true
		}
		if uni == uniEnc {
			return 0, false
		}
	}
	return f.resolverGlifoNoSimbolico(crudo, c, uni)
}

func (f *fuentePDF) resolverGlifoSimbolico(crudo *tablasCrudasSfnt, c byte, uni rune) (uint16, bool) {
	if gid, ok := crudo.buscarSimbolo30(c); ok {
		return gid, true
	}
	for _, gid := range [3]uint16{crudo.propiaAGid[c], crudo.macRomanAGid[c], crudo.macArabeAGid[c]} {
		if gid != 0 {
			return gid, true
		}
	}
	for _, r := range [4]rune{0xF000 + rune(c), 0xF100 + rune(c), 0xF200 + rune(c), rune(c)} {
		if gid := crudo.unicodeAGid[r]; gid != 0 {
			return gid, true
		}
	}
	if nom := f.nombresSimple[c]; nom != "" {
		if gid := crudo.nombreAGid[nom]; gid != 0 {
			return gid, true
		}
	}
	if uni != 0 {
		if gid := crudo.unicodeAGid[uni]; gid != 0 {
			return gid, true
		}
	}
	return 0, false
}

func (f *fuentePDF) resolverGlifoNoSimbolico(crudo *tablasCrudasSfnt, c byte, uni rune) (uint16, bool) {
	// Fuente no simbólica: primero nombre explícito de /Differences, luego
	// carácter Unicode (en tablas Unicode o MacRoman) y después nombres de post/CFF.
	if nom := f.nombresSimple[c]; nom != "" {
		if gid := crudo.nombreAGid[nom]; gid != 0 {
			return gid, true
		}
	}
	if gid := crudo.buscarPorUnicodeOMac(uni); gid != 0 {
		return gid, true
	}
	if len(crudo.nombreAGid) > 0 {
		if gid := buscarGlifoPorRunaEnNombres(crudo.nombreAGid, uni, c); gid != 0 {
			return gid, true
		}
	}
	if !crudo.tieneUnicode && !crudo.tieneMac {
		if gid, ok := crudo.buscarSimbolo30(c); ok {
			return gid, true
		}
		for _, gid := range [2]uint16{crudo.propiaAGid[c], crudo.macArabeAGid[c]} {
			if gid != 0 {
				return gid, true
			}
		}
	}
	return 0, false
}

func (t *tablasCrudasSfnt) buscarPorUnicodeOMac(uni rune) uint16 {
	if uni == 0 {
		return 0
	}
	if gid := t.unicodeAGid[uni]; gid != 0 {
		return gid
	}
	if mb, ok := byteMacRoman(uni); ok {
		return t.macRomanAGid[mb]
	}
	return 0
}

func buscarGlifoPorRunaEnNombres(nombreAGid map[string]uint16, uni rune, codigo byte) uint16 {
	if uni > 0 && uni < 0x80 {
		for n, r := range nombresDeGlifo {
			if r == uni {
				if gid := nombreAGid[n]; gid != 0 {
					return gid
				}
			}
		}
	}
	if sid, ok := codificacionEstandarCFF[codigo]; ok && sid < len(cadenasEstandarCFF) {
		if gid := nombreAGid[cadenasEstandarCFF[sid]]; gid != 0 {
			return gid
		}
	}
	for n, gid := range nombreAGid {
		if r, ok := runaDeNombreGlifo(n); ok && r == uni && uni != 0 {
			return gid
		}
	}
	return 0
}

// --- Construcción de cmap Unicode completa (BMP + plano suplementario) ------

// runaVisualGID asigna un punto de uso privado determinista y de delta
// constante a cada glifo: 0xE000+gid en el BMP hasta 6399 glifos (un solo
// segmento de 8 bytes en formato 4) y 0xF0000+gid en el plano 15 a partir de ahí.
func runaVisualGID(gid uint16) rune {
	if gid < 0x1900 {
		return 0xE000 + int32(gid)
	}
	return 0xF0000 + int32(gid)
}

// cidDeCodigo traduce el código del flujo de una fuente compuesta a su CID.
func (f *fuentePDF) cidDeCodigo(codigo uint32) uint32 {
	if f.cmap != nil {
		return f.cmap.cid(codigo, f.cmap.largo(codigo))
	}
	return codigo
}

// gidDeCID traduce un CID al índice de glifo real en la fuente incrustada.
func (f *fuentePDF) gidDeCID(cid uint32) (uint16, bool) {
	if cid > maxCID {
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
	return uint16(gid), true
}

// reescribirCmapCompuesta construye la `cmap` de una fuente Type0 resolviendo
// código -> CID -> GID, soportando caracteres fuera del BMP (formato 12) e
// incluyendo el tramo visual de uso privado para que ni `sfnt.Parse` ni OTS
// rechacen jamás una fuente compuesta sin `/ToUnicode`.
func (f *fuentePDF) reescribirCmapCompuesta(sfntDatos []byte, numGlifos int) ([]byte, map[rune]uint16, bool) {
	if numGlifos <= 0 {
		if maxp := datosTablaSfnt(sfntDatos, "maxp"); len(maxp) >= 6 {
			numGlifos = int(binary.BigEndian.Uint16(maxp[4:6]))
		}
	}
	inverso := f.inversoUnicodeCompuesta(numGlifos)
	tope := min(max(numGlifos, 1), 0xFFFE)
	for g := 1; g < tope; g++ {
		rv := runaVisualGID(uint16(g)) //nolint:gosec // g < 0xFFFE
		if _, ocupado := inverso[rv]; !ocupado {
			inverso[rv] = uint16(g) //nolint:gosec // g < 0xFFFE
		}
	}
	if len(inverso) == 0 {
		// Incluso una fuente de 1 solo glifo (.notdef) recibe una cmap válida.
		inverso[0xE000] = 0
	}
	out, ok := construirCmapCompleta(sfntDatos, inverso)
	return out, inverso, ok
}

func (f *fuentePDF) inversoUnicodeCompuesta(numGlifos int) map[rune]uint16 {
	inverso := map[rune]uint16{}
	anotar := func(r rune, gid uint16) {
		if gid == 0 || (numGlifos > 0 && int(gid) >= numGlifos) {
			return
		}
		if prev, ya := inverso[r]; !ya || gid < prev {
			inverso[r] = gid
		}
	}
	cods := make([]uint32, 0, len(f.aUnicode))
	for c := range f.aUnicode {
		cods = append(cods, c)
	}
	slices.Sort(cods)
	for _, codigo := range cods {
		rs := []rune(f.aUnicode[codigo])
		if len(rs) != 1 || rs[0] <= 0 || rs[0] > maxRuna || (rs[0] >= 0xD800 && rs[0] <= 0xDFFF) {
			continue
		}
		if gid, ok := f.gidDeCID(f.cidDeCodigo(codigo)); ok {
			anotar(rs[0], gid)
		}
	}
	if len(inverso) == 0 && f.unicodeIdentidad && f.cmap != nil {
		f.anotarInversoIdentidadCmap(anotar)
	}
	if len(inverso) == 0 {
		for gid, r := range f.gidARuna {
			if esRunaTextoUtil(r) {
				anotar(r, gid)
			}
		}
	}
	return inverso
}

func (f *fuentePDF) anotarInversoIdentidadCmap(anotar func(rune, uint16)) {
	cidsCods := make([]uint32, 0, len(f.cmap.cids))
	for c := range f.cmap.cids {
		cidsCods = append(cidsCods, c)
	}
	slices.Sort(cidsCods)
	for _, c := range cidsCods {
		f.anotarCodigoIdentidad(c, f.cmap.cids[c], anotar)
	}
	for _, rg := range f.cmap.rangos {
		for c := rg.desde; c <= rg.hasta && c-rg.desde < 70000; c++ {
			f.anotarCodigoIdentidad(c, rg.base+c-rg.desde, anotar)
		}
	}
}

func (f *fuentePDF) anotarCodigoIdentidad(c, cid uint32, anotar func(rune, uint16)) {
	if c == 0 || c > maxRuna || (c >= 0xD800 && c <= 0xDFFF) {
		return
	}
	if gid, ok := f.gidDeCID(cid); ok {
		anotar(rune(c), gid) //nolint:gosec // c <= maxRuna
	}
}

// construirCmapCompleta escribe `(3,1)` formato 4 para el BMP y, cuando hay
// puntos fuera del BMP (`> 0xFFFF`), añade `(3,10)` formato 12 con el conjunto
// completo (BMP + suplementarios) para que los motores que priorizan `(3,10)`
// encuentren todos los caracteres.
func construirCmapCompleta(ttf []byte, inverso map[rune]uint16) ([]byte, bool) {
	if len(inverso) == 0 || len(ttf) < 12 {
		return nil, false
	}
	bmp := make(map[uint16]uint16, len(inverso))
	haySuplementario := false
	for r, gid := range inverso {
		if r <= 0 || r > maxRuna || (r >= 0xD800 && r <= 0xDFFF) {
			continue
		}
		if r <= 0xFFFF {
			bmp[uint16(r)] = gid
		} else {
			haySuplementario = true
		}
	}
	if !haySuplementario {
		return reescribirCmapUnicodeTTF(ttf, bmp)
	}
	if len(bmp) == 0 {
		bmp[0xE000] = 0
	}
	sub4 := subtablaCmapFormato4(bmp)
	sub12 := subtablaCmapFormato12(inverso)

	be := binary.BigEndian
	cab := 4 + 2*8
	tabla := make([]byte, 0, cab+len(sub4)+len(sub12))
	tabla = be.AppendUint16(tabla, 0)
	tabla = be.AppendUint16(tabla, 2)
	tabla = be.AppendUint16(tabla, 3)
	tabla = be.AppendUint16(tabla, 1)
	tabla = be.AppendUint32(tabla, uint32(cab)) //nolint:gosec // fijo = 20
	tabla = be.AppendUint16(tabla, 3)
	tabla = be.AppendUint16(tabla, 10)
	tabla = be.AppendUint32(tabla, uint32(cab+len(sub4))) //nolint:gosec // cabe en memoria
	tabla = append(tabla, sub4...)
	tabla = append(tabla, sub12...)
	return reemplazarTablaSfnt(ttf, "cmap", tabla)
}

func subtablaCmapFormato4(inverso map[uint16]uint16) []byte {
	claves := make([]int, 0, len(inverso))
	for u := range inverso {
		claves = append(claves, int(u))
	}
	sort.Ints(claves)

	type seg struct{ ini, fin, delta uint16 }
	var segs []seg
	for _, k := range claves {
		u := uint16(k & 0xFFFF)
		g := inverso[u]
		delta := g - u
		if n := len(segs); n > 0 && segs[n-1].delta == delta && segs[n-1].fin+1 == u {
			segs[n-1].fin = u
			continue
		}
		segs = append(segs, seg{ini: u, fin: u, delta: delta})
	}
	if len(segs) == 0 || segs[len(segs)-1].fin != 0xFFFF {
		segs = append(segs, seg{ini: 0xFFFF, fin: 0xFFFF, delta: 1})
	}

	n := len(segs)
	sub := make([]byte, 0, 16+n*8)
	be := binary.BigEndian
	u16 := func(v uint16) { sub = be.AppendUint16(sub, v) }
	u16(4)
	u16(uint16((16 + n*8) & 0xFFFF)) //nolint:gosec // formato 4 BMP
	u16(0)
	u16(uint16(n * 2 & 0xFFFF)) //nolint:gosec // formato 4 BMP
	potencia, sel := uint16(1), uint16(0)
	for int(potencia)*2 <= n {
		potencia *= 2
		sel++
	}
	u16(potencia * 2)
	u16(sel)
	u16(uint16(n*2&0xFFFF) - potencia*2) //nolint:gosec // formato 4 BMP
	for _, s := range segs {
		u16(s.fin)
	}
	u16(0)
	for _, s := range segs {
		u16(s.ini)
	}
	for _, s := range segs {
		u16(s.delta)
	}
	for range segs {
		u16(0)
	}
	return sub
}

func subtablaCmapFormato12(inverso map[rune]uint16) []byte {
	claves := make([]rune, 0, len(inverso))
	for r := range inverso {
		if r > 0 && r <= maxRuna && (r < 0xD800 || r > 0xDFFF) {
			claves = append(claves, r)
		}
	}
	slices.Sort(claves)

	type grupo struct{ ini, fin, gidIni uint32 }
	var grupos []grupo
	for _, r := range claves {
		u := uint32(r) //nolint:gosec // r > 0 comprobado arriba
		g := uint32(inverso[r])
		if n := len(grupos); n > 0 && grupos[n-1].fin+1 == u && grupos[n-1].gidIni+(u-grupos[n-1].ini) == g {
			grupos[n-1].fin = u
			continue
		}
		grupos = append(grupos, grupo{ini: u, fin: u, gidIni: g})
	}

	be := binary.BigEndian
	lon := 16 + 12*len(grupos)
	sub := make([]byte, 0, lon)
	sub = be.AppendUint16(sub, 12)
	sub = be.AppendUint16(sub, 0)
	sub = be.AppendUint32(sub, uint32(lon))         //nolint:gosec // cabe en memoria
	sub = be.AppendUint32(sub, 0)                   // idioma
	sub = be.AppendUint32(sub, uint32(len(grupos))) //nolint:gosec // cabe en memoria
	for _, g := range grupos {
		sub = be.AppendUint32(sub, g.ini)
		sub = be.AppendUint32(sub, g.fin)
		sub = be.AppendUint32(sub, g.gidIni)
	}
	return sub
}

// nombresMacPost son los 258 nombres estándar de Macintosh en `post` v1.0 y v2.0
// (especificación OpenType, tabla `post`, orden 0..257).
var nombresMacPost = [258]string{
	".notdef", ".null", "nonmarkingreturn", "space", "exclam", "quotedbl", "numbersign", "dollar",
	"percent", "ampersand", "quotesingle", "parenleft", "parenright", "asterisk", "plus", "comma",
	"hyphen", "period", "slash", "zero", "one", "two", "three", "four",
	"five", "six", "seven", "eight", "nine", "colon", "semicolon", "less",
	"equal", "greater", "question", "at", "A", "B", "C", "D",
	"E", "F", "G", "H", "I", "J", "K", "L",
	"M", "N", "O", "P", "Q", "R", "S", "T",
	"U", "V", "W", "X", "Y", "Z", "bracketleft", "backslash",
	"bracketright", "asciicircum", "underscore", "grave", "a", "b", "c", "d",
	"e", "f", "g", "h", "i", "j", "k", "l",
	"m", "n", "o", "p", "q", "r", "s", "t",
	"u", "v", "w", "x", "y", "z", "braceleft", "bar",
	"braceright", "asciitilde", "Adieresis", "Aring", "Ccedilla", "Eacute", "Ntilde", "Odieresis",
	"Udieresis", "aacute", "agrave", "acircumflex", "adieresis", "atilde", "aring", "ccedilla",
	"eacute", "egrave", "ecircumflex", "edieresis", "iacute", "igrave", "icircumflex", "idieresis",
	"ntilde", "oacute", "ograve", "ocircumflex", "odieresis", "otilde", "uacute", "ugrave",
	"ucircumflex", "udieresis", "dagger", "degree", "cent", "sterling", "section", "bullet",
	"paragraph", "germandbls", "registered", "copyright", "trademark", "acute", "dieresis", "notequal",
	"AE", "Oslash", "infinity", "plusminus", "lessequal", "greaterequal", "yen", "mu",
	"partialdiff", "summation", "product", "pi", "integral", "ordfeminine", "ordmasculine", "Omega",
	"ae", "oslash", "questiondown", "exclamdown", "logicalnot", "radical", "florin", "approxequal",
	"Delta", "guillemotleft", "guillemotright", "ellipsis", "nonbreakingspace", "Agrave", "Atilde", "Otilde",
	"OE", "oe", "endash", "emdash", "quotedblleft", "quotedblright", "quoteleft", "quoteright",
	"divide", "lozenge", "ydieresis", "Ydieresis", "fraction", "currency", "guilsinglleft", "guilsinglright",
	"fi", "fl", "daggerdbl", "periodcentered", "quotesinglbase", "quotedblbase", "perthousand", "Acircumflex",
	"Ecircumflex", "Aacute", "Edieresis", "Egrave", "Iacute", "Icircumflex", "Idieresis", "Igrave",
	"Oacute", "Ocircumflex", "apple", "Ograve", "Uacute", "Ucircumflex", "Ugrave", "dotlessi",
	"circumflex", "tilde", "macron", "breve", "dotaccent", "ring", "cedilla", "hungarumlaut",
	"ogonek", "caron", "Lslash", "lslash", "Scaron", "scaron", "Zcaron", "zcaron",
	"brokenbar", "Eth", "eth", "Yacute", "yacute", "Thorn", "thorn", "minus",
	"multiply", "onesuperior", "twosuperior", "threesuperior", "onehalf", "onequarter", "threequarters", "franc",
	"Gbreve", "gbreve", "Idotaccent", "Scedilla", "scedilla", "Cacute", "cacute", "Ccaron",
	"ccaron", "dcroat",
}
