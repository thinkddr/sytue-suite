// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pdffuente lee fuentes TrueType y escribe su SUBCONJUNTO incrustado en un PDF
// (CIDFontType2, Identity-H, /W y /ToUnicode). Es lo que PDF/UA y PDF/A piden y que
// pdfsimple, con las catorce fuentes estándar, no puede dar.
//
// Solo stdlib más golang.org/x/image/font/sfnt (BSD, ya en go.mod) para la tabla cmap. El
// resto —directorio de tablas, hmtx, glyf, loca, subconjunto— es código propio sobre la
// especificación pública de OpenType.
//
// Alcance declarado (ponytail): contornos TrueType (glyf) en fuentes sueltas. Una fuente
// CFF («OTTO»), una colección (.ttc) o una variable sin instanciar se RECHAZAN con su motivo.
// No hay kerning ni sustituciones GSUB/GPOS: el avance de cada glifo es el de hmtx.
package pdffuente

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"

	"golang.org/x/image/font/sfnt"
)

// Errores de lectura. Quien llama decide si la fuente se sustituye o la exportación falla.
var (
	ErrFormato      = errors.New("pdffuente: fuente TrueType no válida o no admitida")
	ErrIncrustacion = errors.New("pdffuente: la licencia de la fuente (OS/2 fsType) no permite incrustarla")
)

// Fuente es una cara TrueType leída. Es inmutable y segura para uso concurrente.
type Fuente struct {
	// Nombre PostScript sin la etiqueta de subconjunto (p. ej. «Figtree-Regular»).
	Nombre string

	datos     []byte
	tablas    map[string][]byte
	upem      int
	numGlyphs int
	numHM     int
	sf        *sfnt.Font
	glifos    sync.Map // rune -> uint16: caché de cmap

	ascent, descent, capHeight int
	bbox                       [4]int
	italicAngle                float64
	peso                       int
	negrita, cursiva           bool
}

func u16(b []byte, o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
func i16(b []byte, o int) int {
	return int(int16(binary.BigEndian.Uint16(b[o:]))) //nolint:gosec // dos bytes: la conversión con signo es la lectura
}
func u32(b []byte, o int) int { return int(binary.BigEndian.Uint32(b[o:])) }

// ponerU16 y ponerU32 escriben un entero acotado por construcción (conteos de tablas,
// longitudes de un fichero que ya cabe en memoria) en big endian.
func ponerU16(b []byte, o, v int) {
	binary.BigEndian.PutUint16(b[o:], uint16(v)) //nolint:gosec // v < 65536 por construcción
}

func ponerU32(b []byte, o, v int) {
	binary.BigEndian.PutUint32(b[o:], uint32(v)) //nolint:gosec // v < 4 GiB: el fichero cabe en memoria
}

// Abrir lee una fuente TrueType. Comprueba que las tablas obligatorias existen y que sus
// longitudes cuadran antes de que nada las indexe, y respeta fsType: una fuente con la
// incrustación restringida o sin permiso de subconjunto no se usa.
func Abrir(datos []byte) (*Fuente, error) {
	if len(datos) < 12 {
		return nil, fmt.Errorf("%w: cabecera truncada", ErrFormato)
	}
	switch string(datos[:4]) {
	case "\x00\x01\x00\x00", "true":
	case "OTTO":
		return nil, fmt.Errorf("%w: contornos CFF (OpenType PostScript)", ErrFormato)
	case "ttcf":
		return nil, fmt.Errorf("%w: colección .ttc", ErrFormato)
	default:
		return nil, fmt.Errorf("%w: firma desconocida", ErrFormato)
	}
	n := u16(datos, 4)
	if n == 0 || n > 64 || 12+16*n > len(datos) {
		return nil, fmt.Errorf("%w: directorio de tablas", ErrFormato)
	}
	f := &Fuente{datos: datos, tablas: make(map[string][]byte, n)}
	for i := 0; i < n; i++ {
		r := 12 + 16*i
		off, ln := u32(datos, r+8), u32(datos, r+12)
		if off < 0 || ln < 0 || off > len(datos) || ln > len(datos)-off {
			return nil, fmt.Errorf("%w: tabla %q fuera del fichero", ErrFormato, datos[r:r+4])
		}
		f.tablas[string(datos[r:r+4])] = datos[off : off+ln]
	}
	for _, t := range []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf"} {
		if f.tablas[t] == nil {
			return nil, fmt.Errorf("%w: falta la tabla %q", ErrFormato, t)
		}
	}
	if err := f.leerCabeceras(); err != nil {
		return nil, err
	}
	sf, err := sfnt.Parse(datos)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFormato, err)
	}
	f.sf = sf
	var b sfnt.Buffer
	if f.Nombre, err = sf.Name(&b, sfnt.NameIDPostScript); err != nil || f.Nombre == "" {
		f.Nombre = "Fuente"
	}
	return f, nil
}

func (f *Fuente) leerCabeceras() error {
	head, hhea, maxp := f.tablas["head"], f.tablas["hhea"], f.tablas["maxp"]
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 {
		return fmt.Errorf("%w: tablas básicas truncadas", ErrFormato)
	}
	f.upem = u16(head, 18)
	if f.upem < 16 || f.upem > 16384 {
		return fmt.Errorf("%w: unitsPerEm %d", ErrFormato, f.upem)
	}
	f.bbox = [4]int{i16(head, 36), i16(head, 38), i16(head, 40), i16(head, 42)}
	f.negrita, f.cursiva = head[45]&1 != 0, head[45]&2 != 0
	f.numGlyphs = u16(maxp, 4)
	f.ascent, f.descent, f.numHM = i16(hhea, 4), i16(hhea, 6), u16(hhea, 34)
	if f.numGlyphs == 0 || f.numHM == 0 || f.numHM > f.numGlyphs {
		return fmt.Errorf("%w: número de glifos", ErrFormato)
	}
	if len(f.tablas["hmtx"]) < 4*f.numHM+2*(f.numGlyphs-f.numHM) {
		return fmt.Errorf("%w: hmtx truncada", ErrFormato)
	}
	loc := 2
	if i16(head, 50) == 1 {
		loc = 4
	}
	if len(f.tablas["loca"]) < loc*(f.numGlyphs+1) {
		return fmt.Errorf("%w: loca truncada", ErrFormato)
	}
	f.capHeight, f.peso = f.ascent, 400
	if os2 := f.tablas["OS/2"]; len(os2) >= 10 {
		f.peso = u16(os2, 4)
		if fs := u16(os2, 8); fs&0x0002 != 0 || fs&0x0100 != 0 {
			return fmt.Errorf("%w (fsType=0x%04x)", ErrIncrustacion, fs)
		}
		if u16(os2, 0) >= 2 && len(os2) >= 90 {
			if c := i16(os2, 88); c > 0 {
				f.capHeight = c
			}
		}
	}
	if post := f.tablas["post"]; len(post) >= 8 {
		f.italicAngle = float64(int32(binary.BigEndian.Uint32(post[4:]))) / 65536 //nolint:gosec // Fixed 16.16 con signo
	}
	return nil
}

// NumGlifos es el número de glifos de la fuente.
func (f *Fuente) NumGlifos() int { return f.numGlyphs }

// Glifo devuelve el glifo de r, o 0 si la fuente no lo trae (0 es .notdef y NO debe
// pintarse: PDF/UA lo prohíbe).
func (f *Fuente) Glifo(r rune) uint16 {
	if g, ok := f.glifos.Load(r); ok {
		return g.(uint16) //nolint:errcheck,forcetypeassert // solo se guardan uint16
	}
	var b sfnt.Buffer
	g, err := f.sf.GlyphIndex(&b, r)
	if err != nil || int(g) >= f.numGlyphs {
		g = 0
	}
	// Medido (2026-10-04): sin esta caché cada carácter volvía a recorrer cmap con un búfer
	// nuevo, y un documento de 75 páginas reservaba 655 MiB. Un documento usa pocos cientos
	// de caracteres distintos, así que el mapa no crece.
	f.glifos.Store(r, uint16(g))
	return uint16(g)
}

// Avance es el ancho del glifo en milésimas de em, redondeado como irá en /W: el visor
// avanza con ese número, así que la maquetación tiene que medir con él y no con hmtx en
// bruto.
func (f *Fuente) Avance(g uint16) int {
	i := int(g)
	if i >= f.numHM {
		i = f.numHM - 1
	}
	adv := u16(f.tablas["hmtx"], 4*i)
	return int(math.Round(float64(adv) * 1000 / float64(f.upem)))
}

func (f *Fuente) mil(v int) int { return int(math.Round(float64(v) * 1000 / float64(f.upem))) }

// glifo devuelve los bytes de un glifo en glyf (vacío si no tiene contorno).
func (f *Fuente) glifo(g int) ([]byte, error) {
	head, loca, glyf := f.tablas["head"], f.tablas["loca"], f.tablas["glyf"]
	var a, b int
	if i16(head, 50) == 1 {
		a, b = u32(loca, 4*g), u32(loca, 4*g+4)
	} else {
		a, b = 2*u16(loca, 2*g), 2*u16(loca, 2*g+2)
	}
	if a < 0 || b < a || b > len(glyf) {
		return nil, fmt.Errorf("%w: glifo %d fuera de glyf", ErrFormato, g)
	}
	return glyf[a:b], nil
}

// componentes lista los glifos que un glifo compuesto referencia (vacío si es simple).
func componentes(d []byte) ([]int, error) {
	if len(d) < 10 || i16(d, 0) >= 0 {
		return nil, nil
	}
	var out []int
	for p := 10; ; {
		if p+4 > len(d) {
			return nil, fmt.Errorf("%w: glifo compuesto truncado", ErrFormato)
		}
		flags := u16(d, p)
		out = append(out, u16(d, p+2))
		p += 4
		if flags&0x0001 != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&0x0008 != 0:
			p += 2
		case flags&0x0040 != 0:
			p += 4
		case flags&0x0080 != 0:
			p += 8
		}
		if flags&0x0020 == 0 {
			return out, nil
		}
	}
}
