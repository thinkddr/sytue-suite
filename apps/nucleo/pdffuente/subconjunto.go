// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdffuente

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Escritor es lo que el subconjunto necesita del documento que lo incrusta: reservar un
// objeto y devolver su número. Flujo añade /Length y /Filter /FlateDecode al diccionario.
type Escritor interface {
	Objeto(cuerpo string) int
	Flujo(dict string, datos []byte) int
}

// Subconjunto acumula los glifos usados de UNA cara y la escribe, al final, como fuente
// PDF. Los glifos conservan su número original (CIDToGIDMap /Identity): lo que se recorta
// es el contorno de los que no se usan, no la numeración, así que no hay tabla de
// traducción que pueda desalinearse.
type Subconjunto struct {
	f     *Fuente
	runas map[uint16]rune // glifo -> primer carácter que lo pidió (para /ToUnicode)
}

func (f *Fuente) NuevoSubconjunto() *Subconjunto {
	return &Subconjunto{f: f, runas: map[uint16]rune{}}
}

// Usar registra que el glifo g, que representa r, aparece en el documento.
func (s *Subconjunto) Usar(g uint16, r rune) {
	if _, ok := s.runas[g]; !ok {
		s.runas[g] = r
	}
}

// Vacio dice si ningún glifo se ha usado.
func (s *Subconjunto) Vacio() bool { return len(s.runas) == 0 }

func (s *Subconjunto) glifos() []uint16 {
	gs := make([]uint16, 0, len(s.runas))
	for g := range s.runas {
		gs = append(gs, g)
	}
	slices.Sort(gs)
	return gs
}

// etiqueta es el prefijo de seis letras de BaseFont de un subconjunto (ISO 32000-1,
// 9.6.4): determinista —mismos glifos, misma etiqueta— para que el PDF sea reproducible.
func (s *Subconjunto) etiqueta() string {
	h := fnv.New32a()
	h.Write([]byte(s.f.Nombre))
	for _, g := range s.glifos() {
		h.Write([]byte{byte(g >> 8), byte(g & 0xff)})
	}
	v, out := h.Sum32(), make([]byte, 6)
	for i := range out {
		out[i] = 'A' + byte(v%26)
		v /= 26
	}
	return string(out)
}

// Escribir emite FontFile2, FontDescriptor, ToUnicode, CIDFont y la fuente Type0, y
// devuelve el número de esta última, la que el contenido nombra con /Fn.
func (s *Subconjunto) Escribir(w Escritor) (int, error) {
	prog, err := s.f.subconjunto(s.glifos())
	if err != nil {
		return 0, err
	}
	f := s.f
	base := s.etiqueta() + "+" + f.Nombre
	fichero := w.Flujo("/Length1 "+strconv.Itoa(len(prog)), prog)

	flags := 32 // no simbólica: se pinta con Unicode, no con la codificación propia de la fuente
	if f.cursiva {
		flags |= 64
	}
	stemV := 80 + (f.peso-400)/3
	desc := w.Objeto(fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%d %d %d %d] "+
		"/ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV %d /FontFile2 %d 0 R >>",
		base, flags, f.mil(f.bbox[0]), f.mil(f.bbox[1]), f.mil(f.bbox[2]), f.mil(f.bbox[3]),
		strconv.FormatFloat(f.italicAngle, 'f', -1, 64), f.mil(f.ascent), f.mil(f.descent),
		f.mil(f.capHeight), stemV, fichero))
	tou := w.Flujo("", s.toUnicode())
	cid := w.Objeto("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + base +
		" /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor " +
		strconv.Itoa(desc) + " 0 R /CIDToGIDMap /Identity /DW 1000 /W " + s.anchos() + " >>")
	return w.Objeto("<< /Type /Font /Subtype /Type0 /BaseFont /" + base + " /Encoding /Identity-H " +
		"/DescendantFonts [" + strconv.Itoa(cid) + " 0 R] /ToUnicode " + strconv.Itoa(tou) + " 0 R >>"), nil
}

// anchos es el array /W: «g [w1 w2 …]» por cada racha de glifos consecutivos.
func (s *Subconjunto) anchos() string {
	var b strings.Builder
	b.WriteByte('[')
	gs := s.glifos()
	for i := 0; i < len(gs); {
		j := i
		for j+1 < len(gs) && gs[j+1] == gs[j]+1 {
			j++
		}
		b.WriteString(strconv.Itoa(int(gs[i])) + " [")
		for k := i; k <= j; k++ {
			if k > i {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(s.f.Avance(gs[k])))
		}
		b.WriteString("] ")
		i = j + 1
	}
	return strings.TrimRight(b.String(), " ") + "]"
}

// toUnicode es el CMap que devuelve el texto de cada glifo: sin él un lector de pantalla,
// la búsqueda y el copiar/pegar ven códigos de glifo y no letras.
func (s *Subconjunto) toUnicode() []byte {
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	gs := s.glifos()
	for i := 0; i < len(gs); i += 100 {
		fin := min(i+100, len(gs))
		b.WriteString(strconv.Itoa(fin-i) + " beginbfchar\n")
		for _, g := range gs[i:fin] {
			fmt.Fprintf(&b, "<%04X> <", g)
			for _, u := range utf16.Encode([]rune{s.runas[g]}) {
				fmt.Fprintf(&b, "%04X", u)
			}
			b.WriteString(">\n")
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n") //nolint:dupword // cierra dos bloques anidados
	return []byte(b.String())
}

// subconjunto construye el programa TrueType reducido: las tablas que la especificación PDF
// pide para CIDFontType2 (head, hhea, maxp, hmtx, loca, glyf y, si existen, cvt, fpgm,
// prep) con el contorno solo de los glifos usados y de sus componentes.
func (f *Fuente) subconjunto(usados []uint16) ([]byte, error) {
	guarda := map[int]bool{0: true}
	var marca func(g, prof int) error
	marca = func(g, prof int) error {
		if g >= f.numGlyphs || prof > 8 {
			return fmt.Errorf("%w: glifo compuesto inválido", ErrFormato)
		}
		if guarda[g] && g != 0 && prof > 0 {
			return nil
		}
		guarda[g] = true
		d, err := f.glifo(g)
		if err != nil {
			return err
		}
		cs, err := componentes(d)
		if err != nil {
			return err
		}
		for _, c := range cs {
			if err := marca(c, prof+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, g := range usados {
		if err := marca(int(g), 0); err != nil {
			return nil, err
		}
	}

	var glyf []byte
	loca := make([]byte, 4*(f.numGlyphs+1))
	for g := 0; g < f.numGlyphs; g++ {
		ponerU32(loca, 4*g, len(glyf))
		if !guarda[g] {
			continue
		}
		d, err := f.glifo(g)
		if err != nil {
			return nil, err
		}
		glyf = append(glyf, d...)
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
	}
	ponerU32(loca, 4*f.numGlyphs, len(glyf))

	head := slices.Clone(f.tablas["head"])
	binary.BigEndian.PutUint32(head[8:], 0) // checkSumAdjustment, se calcula al final
	ponerU16(head, 50, 1)                   // loca en formato largo
	tablas := map[string][]byte{
		"head": head, "hhea": f.tablas["hhea"], "maxp": f.tablas["maxp"], "hmtx": f.tablas["hmtx"],
		"loca": loca, "glyf": glyf,
	}
	for _, t := range []string{"cvt ", "fpgm", "prep"} {
		if d := f.tablas[t]; d != nil {
			tablas[t] = d
		}
	}
	return ensamblar(tablas), nil
}

func suma(d []byte) uint32 {
	var s uint32
	for i := 0; i < len(d); i += 4 {
		var w [4]byte
		copy(w[:], d[i:min(i+4, len(d))])
		s += binary.BigEndian.Uint32(w[:])
	}
	return s
}

// ensamblar escribe el directorio y las tablas (ordenadas por etiqueta, alineadas a 4) y
// fija head.checkSumAdjustment.
func ensamblar(tablas map[string][]byte) []byte {
	tags := make([]string, 0, len(tablas))
	for t := range tablas {
		tags = append(tags, t)
	}
	slices.Sort(tags)
	n := len(tags)
	sr, es := 16, 0
	for sr*2 <= n*16 {
		sr, es = sr*2, es+1
	}
	dir := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(dir, 0x00010000)
	ponerU16(dir, 4, n)
	ponerU16(dir, 6, sr)
	ponerU16(dir, 8, es)
	ponerU16(dir, 10, n*16-sr)
	var cuerpo []byte
	headOff := 0
	for i, t := range tags {
		d := tablas[t]
		r := 12 + 16*i
		copy(dir[r:], t)
		ponerU32(dir, r+4, int(suma(d)))
		ponerU32(dir, r+8, len(dir)+len(cuerpo))
		ponerU32(dir, r+12, len(d))
		if t == "head" {
			headOff = len(dir) + len(cuerpo)
		}
		cuerpo = append(cuerpo, d...)
		for len(cuerpo)%4 != 0 {
			cuerpo = append(cuerpo, 0)
		}
	}
	out := slices.Concat(dir, cuerpo)
	binary.BigEndian.PutUint32(out[headOff+8:], 0xB1B0AFBA-suma(out))
	return out
}
