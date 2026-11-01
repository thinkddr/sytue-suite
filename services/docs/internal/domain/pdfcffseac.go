// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// ACENTUADOS COMPUESTOS de un CFF (el «seac» de Type 2).
//
// Un charstring Type 2 puede acabar en `endchar` con cuatro operandos más
// —adx ady base acento—: el glifo es la base con el acento desplazado encima
// (Adobe TN 5177, apéndice C). `x/image/font/sfnt` no lo compone y deja el glifo
// VACÍO: una «ü» o una «ä» desaparecían del PNG en cuanto el CFF se pintaba con
// su contorno en vez de con la letra de reserva.
//
// Cuando un CFF simple trae alguno, se interpretan todos sus glifos hasta dejar
// rectas y curvas absolutas y se reescribe con armarCFFSimple, como una Type 1:
// los acentuados salen ya compuestos. Si algo no se entiende, se deja el CFF
// como estaba.
package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"golang.org/x/image/font/sfnt"
)

const maxPilaType2 = 48

type dictPrivCFF struct {
	locales                    [][]byte
	anchoDefecto, anchoNominal float64
	escala                     [2]float64
}

// programasCFF son los charstrings de un CFF (simple o CID) con sus subrutinas,
// diccionarios privados por FD y escalas.
type programasCFF struct {
	nombre    string
	glifos    [][]byte
	globales  [][]byte
	fds       []dictPrivCFF
	fdDeGlifo []int
	top       map[int][]float64
}

func (p *programasCFF) fd(gid int) dictPrivCFF {
	if len(p.fds) == 0 {
		return dictPrivCFF{escala: [2]float64{1, 1}}
	}
	if gid >= 0 && gid < len(p.fdDeGlifo) {
		if idx := p.fdDeGlifo[gid]; idx >= 0 && idx < len(p.fds) {
			return p.fds[idx]
		}
	}
	return p.fds[0]
}

func leerProgramasCFF(b []byte) (*programasCFF, error) {
	if len(b) < 4 || b[0] != 1 {
		return nil, errCFF
	}
	nombres, pos, err := indiceCFF(b, int(b[2]))
	if err != nil || len(nombres) == 0 {
		return nil, errCFF
	}
	tops, pos, err := indiceCFF(b, pos)
	if err != nil || len(tops) == 0 {
		return nil, errCFF
	}
	_, pos, err = indiceCFF(b, pos) // cadenas
	if err != nil {
		return nil, err
	}
	p := &programasCFF{nombre: string(nombres[0])}
	if p.globales, _, err = indiceCFF(b, pos); err != nil {
		return nil, err
	}
	top := dictCFF(tops[0])
	p.top = top
	if cs := top[17]; len(cs) == 1 {
		p.glifos, _, err = indiceCFF(b, int(cs[0]))
	}
	if err != nil || len(p.glifos) == 0 {
		return nil, errCFF
	}
	if err := cargarDiccionariosCFF(b, p, top); err != nil {
		return nil, err
	}
	return p, nil
}

func cargarDiccionariosCFF(b []byte, p *programasCFF, top map[int][]float64) error {
	escTop := [2]float64{1, 1}
	tieneTopMat := false
	if m := top[1207]; len(m) >= 4 && m[0] > 0 && m[3] != 0 {
		escTop = [2]float64{m[0] * unidadesCFFDeType1, math.Abs(m[3]) * unidadesCFFDeType1}
		tieneTopMat = true
	}
	if fda := top[1236]; len(fda) == 1 {
		rawFDs, _, errFD := indiceCFF(b, int(fda[0]))
		if errFD != nil || len(rawFDs) == 0 {
			return errCFF
		}
		p.fds = make([]dictPrivCFF, len(rawFDs))
		for i, fdb := range rawFDs {
			fd, errP := leerPrivadoCFF(b, dictCFF(fdb), escTop, tieneTopMat, true)
			if errP != nil {
				return errP
			}
			p.fds[i] = fd
		}
		if fdsel := top[1237]; len(fdsel) == 1 {
			p.fdDeGlifo = leerFDSelectCFF(b, int(fdsel[0]), len(p.glifos), len(p.fds))
		}
		return nil
	}
	fd0, err := leerPrivadoCFF(b, top, escTop, tieneTopMat, false)
	if err != nil {
		return err
	}
	p.fds = []dictPrivCFF{fd0}
	return nil
}

func leerPrivadoCFF(b []byte, dic map[int][]float64, escTop [2]float64, tieneTopMat, esFD bool) (dictPrivCFF, error) {
	out := dictPrivCFF{escala: escTop}
	if esFD {
		if m := dic[1207]; len(m) >= 4 && m[0] > 0 && m[3] != 0 {
			switch {
			case tieneTopMat:
				out.escala = [2]float64{escTop[0] * m[0], escTop[1] * math.Abs(m[3])}
			case math.Round(1/m[0]) >= 16 && math.Round(1/m[0]) <= 16384:
				out.escala = [2]float64{m[0] * unidadesCFFDeType1, math.Abs(m[3]) * unidadesCFFDeType1}
			default:
				out.escala = [2]float64{m[0], math.Abs(m[3])}
			}
		}
	}
	if pr := dic[18]; len(pr) == 2 {
		tam, off := int(pr[0]), int(pr[1])
		if off < 0 || tam < 0 || off+tam > len(b) {
			return out, errCFF
		}
		priv := dictCFF(b[off : off+tam])
		if v := priv[20]; len(v) == 1 {
			out.anchoDefecto = v[0]
		}
		if v := priv[21]; len(v) == 1 {
			out.anchoNominal = v[0]
		}
		if v := priv[19]; len(v) == 1 {
			subrs, _, err := indiceCFF(b, off+int(v[0]))
			if err != nil {
				return out, err
			}
			out.locales = subrs
		}
	}
	return out, nil
}

func leerFDSelectCFF(b []byte, off, numGlifos, numFDs int) []int {
	if off < 0 || off >= len(b) || numGlifos <= 0 {
		return nil
	}
	out := make([]int, numGlifos)
	switch b[off] {
	case 0:
		for g := 0; g < numGlifos && off+1+g < len(b); g++ {
			if fd := int(b[off+1+g]); fd < numFDs {
				out[g] = fd
			}
		}
	case 3:
		if off+3 > len(b) {
			return nil
		}
		be := binary.BigEndian
		nRangos := int(be.Uint16(b[off+1:]))
		if off+3+3*nRangos+2 > len(b) {
			return nil
		}
		for r := 0; r < nRangos; r++ {
			pos := off + 3 + 3*r
			ini := int(be.Uint16(b[pos:]))
			fd := int(b[pos+2])
			fin := int(be.Uint16(b[pos+3:]))
			if fd >= numFDs {
				fd = 0
			}
			for g := max(ini, 0); g < min(fin, numGlifos); g++ {
				out[g] = fd
			}
		}
	}
	return out
}

// sesgoSubrs es lo que se suma al número de una subrutina Type 2 (§4.7).
func sesgoSubrs(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	}
	return 32768
}

// interpreteType2 ejecuta charstrings Type 2 y deja el trazado en el mismo
// formato que interpreteType1, para reescribirlo con charstringT2.
type interpreteType2 struct {
	interpreteType1
	prog        *programasCFF
	fd          dictPrivCFF
	gidDeNombre map[string]int
	stems       int
	anchoVisto  bool
	hayAcento   bool
	hayRaro     bool
}

// anchoSi es el primer operador que vacía la pila: si trae un operando de más,
// ese es el ancho (§4.1).
func (in *interpreteType2) anchoSi(sobra bool) {
	if in.anchoVisto {
		return
	}
	in.anchoVisto = true
	in.ancho = in.fd.anchoDefecto
	if sobra && len(in.pila) > 0 {
		in.ancho = in.fd.anchoNominal + in.pila[0]
		in.pila = in.pila[1:]
	}
}

//nolint:gocyclo,gocognit // un caso por operador de la norma (TN 5177, §4)
func (in *interpreteType2) correr(cs []byte, prof int) error {
	if prof > maxProfSubrType1 {
		return errCFF
	}
	for i := 0; i < len(cs) && !in.terminado; {
		in.ops++
		if in.ops > maxOpsGlifoType1 || len(in.pila) > maxPilaType2 {
			return errCFF
		}
		v := int(cs[i])
		switch {
		case v >= 32 && v <= 246:
			in.pila = append(in.pila, float64(v-139))
			i++
			continue
		case v >= 247 && v <= 250 && i+1 < len(cs):
			in.pila = append(in.pila, float64((v-247)*256+int(cs[i+1])+108))
			i += 2
			continue
		case v >= 251 && v <= 254 && i+1 < len(cs):
			in.pila = append(in.pila, float64(-(v-251)*256-int(cs[i+1])-108))
			i += 2
			continue
		case v == 28 && i+2 < len(cs):
			in.pila = append(in.pila, float64(int16(uint16(cs[i+1])<<8|uint16(cs[i+2])))) //nolint:gosec // G115: entero con signo de 16 bits
			i += 3
			continue
		case v == 255 && i+4 < len(cs):
			f := int32(uint32(cs[i+1])<<24 | uint32(cs[i+2])<<16 | uint32(cs[i+3])<<8 | uint32(cs[i+4])) //nolint:gosec // G115: 16.16 con signo
			in.pila = append(in.pila, float64(f)/65536)
			i += 5
			continue
		case v == 28 || v >= 247: // un número cortado al final del charstring
			return errCFF
		}
		op := v
		i++
		if v == 12 {
			if i >= len(cs) {
				return errCFF
			}
			op = 1200 + int(cs[i])
			i++
		}
		limpiar := true
		switch op {
		case 1, 3, 18, 23: // hstem, vstem, hstemhm, vstemhm
			in.anchoSi(len(in.pila)%2 == 1)
			in.stems += len(in.pila) / 2
		case 19, 20: // hintmask, cntrmask: los vstem de delante van implícitos
			in.anchoSi(len(in.pila)%2 == 1)
			in.stems += len(in.pila) / 2
			i += (in.stems + 7) / 8
		case 21: // rmoveto
			in.anchoSi(len(in.pila) > 2)
			a, ok := in.n(2)
			if !ok {
				return errCFF
			}
			in.mover(in.x+a[0], in.y+a[1])
		case 22, 4: // hmoveto, vmoveto
			in.anchoSi(len(in.pila) > 1)
			a, ok := in.n(1)
			if !ok {
				return errCFF
			}
			if op == 22 {
				in.mover(in.x+a[0], in.y)
			} else {
				in.mover(in.x, in.y+a[0])
			}
		case 5: // rlineto
			for a := in.pila; len(a) >= 2; a = a[2:] {
				in.recta(in.x+a[0], in.y+a[1])
			}
		case 6, 7: // hlineto, vlineto: alternan
			h := op == 6
			for _, d := range in.pila {
				if h {
					in.recta(in.x+d, in.y)
				} else {
					in.recta(in.x, in.y+d)
				}
				h = !h
			}
		case 8: // rrcurveto
			for a := in.pila; len(a) >= 6; a = a[6:] {
				in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			}
		case 24: // rcurveline
			a := in.pila
			for ; len(a) >= 8; a = a[6:] {
				in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			}
			if len(a) == 2 {
				in.recta(in.x+a[0], in.y+a[1])
			}
		case 25: // rlinecurve
			a := in.pila
			for ; len(a) >= 8; a = a[2:] {
				in.recta(in.x+a[0], in.y+a[1])
			}
			if len(a) == 6 {
				in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			}
		case 26: // vvcurveto
			a, dx1 := in.pila, 0.0
			if len(a)%2 == 1 {
				dx1, a = a[0], a[1:]
			}
			for ; len(a) >= 4; a = a[4:] {
				in.curvaRel(dx1, a[0], a[1], a[2], 0, a[3])
				dx1 = 0
			}
		case 27: // hhcurveto
			a, dy1 := in.pila, 0.0
			if len(a)%2 == 1 {
				dy1, a = a[0], a[1:]
			}
			for ; len(a) >= 4; a = a[4:] {
				in.curvaRel(a[0], dy1, a[1], a[2], a[3], 0)
				dy1 = 0
			}
		case 30, 31: // vhcurveto, hvcurveto: alternan, con un último desvío opcional
			h := op == 31
			for a := in.pila; len(a) >= 4; a = a[4:] {
				fin := 0.0
				if len(a) == 5 {
					fin = a[4]
				}
				if h {
					in.curvaRel(a[0], 0, a[1], a[2], fin, a[3])
				} else {
					in.curvaRel(0, a[0], a[1], a[2], a[3], fin)
				}
				h = !h
			}
		case 10, 29: // callsubr, callgsubr
			subrs := in.fd.locales
			if op == 29 {
				subrs = in.prog.globales
			}
			n := int(in.sacar()) + sesgoSubrs(len(subrs))
			if n < 0 || n >= len(subrs) {
				return errCFF
			}
			if err := in.correr(subrs[n], prof+1); err != nil {
				return err
			}
			limpiar = false
		case 11: // return
			return nil
		case 14: // endchar, y con cuatro operandos más, un acentuado
			in.anchoSi(len(in.pila) == 1 || len(in.pila) == 5)
			in.terminado = true
			if len(in.pila) == 4 {
				a := in.pila
				return in.componer(a[0], a[1], int(a[2]), int(a[3]))
			}
			return nil
		case 1200: // dotsection (TN 5177 §4.2): obsoleto sin efecto que x/image/font/sfnt rechaza
			in.hayRaro = true
		case 1235: // flex
			in.hayRaro = true
			a, ok := in.n(13)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			in.curvaRel(a[6], a[7], a[8], a[9], a[10], a[11])
		case 1234: // hflex
			in.hayRaro = true
			a, ok := in.n(7)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], 0, a[1], a[2], a[3], 0)
			in.curvaRel(a[4], 0, a[5], -a[2], a[6], 0)
		case 1236: // hflex1
			in.hayRaro = true
			a, ok := in.n(9)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], a[1], a[2], a[3], a[4], 0)
			in.curvaRel(a[5], 0, a[6], a[7], a[8], -(a[1] + a[3] + a[7]))
		case 1237: // flex1: el último punto vuelve a la altura (o a la x) del primero
			in.hayRaro = true
			a, ok := in.n(11)
			if !ok {
				return errCFF
			}
			dx := a[0] + a[2] + a[4] + a[6] + a[8]
			dy := a[1] + a[3] + a[5] + a[7] + a[9]
			x6, y6 := a[10], -dy
			if dx*dx <= dy*dy {
				x6, y6 = -dx, a[10]
			}
			in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			in.curvaRel(a[6], a[7], a[8], a[9], x6, y6)
		default:
			if !in.aritmetica(op) {
				return errCFF
			}
			limpiar = false
		}
		if limpiar {
			in.pila = in.pila[:0]
		}
	}
	return nil
}

// n devuelve los n operandos de la pila, o falso si no están.
func (in *interpreteType2) n(n int) ([]float64, bool) {
	if len(in.pila) < n {
		return nil, false
	}
	return in.pila[len(in.pila)-n:], true
}

func (in *interpreteType2) curvaRel(dxa, dya, dxb, dyb, dxc, dyc float64) {
	p1 := puntoT1{in.x + dxa, in.y + dya}
	p2 := puntoT1{p1.x + dxb, p1.y + dyb}
	in.curva(p1, p2, puntoT1{p2.x + dxc, p2.y + dyc})
}

// aritmetica son los operadores de cálculo que se ven en fuentes reales (§4.4);
// el resto (put, get, ifelse, random...) deja el CFF como estaba.
func (in *interpreteType2) aritmetica(op int) bool {
	switch op {
	case 1209: // abs
		v := in.sacar()
		in.pila = append(in.pila, max(v, -v))
	case 1210: // add
		b, a := in.sacar(), in.sacar()
		in.pila = append(in.pila, a+b)
	case 1211: // sub
		b, a := in.sacar(), in.sacar()
		in.pila = append(in.pila, a-b)
	case 1212: // div
		b, a := in.sacar(), in.sacar()
		if b == 0 {
			return false
		}
		in.pila = append(in.pila, a/b)
	case 1214: // neg
		in.pila = append(in.pila, -in.sacar())
	case 1224: // mul
		b, a := in.sacar(), in.sacar()
		in.pila = append(in.pila, a*b)
	case 1218: // drop
		in.sacar()
	case 1227: // dup
		v := in.sacar()
		in.pila = append(in.pila, v, v)
	case 1228: // exch
		b, a := in.sacar(), in.sacar()
		in.pila = append(in.pila, b, a)
	default:
		return false
	}
	return true
}

// componer pinta la base y, desplazado adx, ady, el acento: los dos por su
// código en la codificación estándar.
func (in *interpreteType2) componer(adx, ady float64, base, acento int) error {
	if in.hayAcento {
		return errCFF // un acentuado dentro de otro: la norma no lo permite
	}
	in.hayAcento = true
	for k, codigo := range [2]int{base, acento} {
		nombre, ok := nombreEstandarType1(codigo)
		g, okg := in.gidDeNombre[nombre]
		if !ok || !okg {
			return errCFF
		}
		sub := &interpreteType2{prog: in.prog, fd: in.prog.fd(g), gidDeNombre: in.gidDeNombre, hayAcento: true}
		if err := sub.correr(in.prog.glifos[g], 0); err != nil {
			return err
		}
		for _, s := range sub.segs {
			if k == 1 {
				for j := range s.p {
					s.p[j].x += adx
					s.p[j].y += ady
				}
			}
			in.segs = append(in.segs, s)
		}
	}
	return nil
}

// aplanarSeacCFF sanea y aplana un CFF suelto (simple o CID) cuando contiene
// acentuados `seac`, operadores Type 2 rechazados por `sfnt` (`dotsection`,
// `flex*`), `FontBBox` invertida o diccionarios no canónicos (`FDSelect` antes
// de `ROS`, `Private` en Top DICT de un CIDFont).
func aplanarSeacCFF(cff []byte) []byte {
	return aplanarCFFModo(cff, false)
}

func aplanarCFFEnSfnt(cff []byte) []byte {
	return aplanarCFFModo(cff, true)
}

func aplanarCFFModo(cff []byte, ordenFijo bool) []byte {
	info, err := leerCFF(cff)
	if err != nil || info.numGlifos == 0 || info.numGlifos > maxGlifosType1 {
		return cff
	}
	prog, err := leerProgramasCFF(cff)
	if err != nil || len(prog.glifos) != info.numGlifos {
		return cff
	}
	var gidDeNombre map[string]int
	var nombresUnicos []string
	if !info.cid {
		var ok bool
		gidDeNombre, nombresUnicos, ok = indexarNombresSimplesCFF(info.nombres, info.numGlifos)
		if !ok {
			return cff
		}
	}

	trazos := make([]*interpreteType2, len(prog.glifos))
	especial := false
	for g, cs := range prog.glifos {
		in := &interpreteType2{prog: prog, fd: prog.fd(g), gidDeNombre: gidDeNombre}
		if in.correr(cs, 0) != nil {
			return cff
		}
		especial = especial || in.hayAcento || in.hayRaro
		trazos[g] = in
	}
	if !necesitaAplanarCFF(cff, info, prog, especial) {
		return cff
	}
	return emitirCFFPlano(prog, info, trazos, gidDeNombre, nombresUnicos, ordenFijo)
}

func indexarNombresSimplesCFF(nombres []string, numGlifos int) (map[string]int, []string, bool) {
	if len(nombres) != numGlifos {
		return nil, nil, false
	}
	gidDeNombre := make(map[string]int, len(nombres))
	nombresUnicos := make([]string, len(nombres))
	usados := map[string]bool{".notdef": true}
	nombresUnicos[0] = ".notdef"
	gidDeNombre[".notdef"] = 0
	for g := 1; g < len(nombres); g++ {
		n := nombres[g]
		if n != "" && n != ".notdef" {
			if _, ya := gidDeNombre[n]; !ya {
				gidDeNombre[n] = g
			}
		}
		n = nombreUnicoGlifoCFF(n, g, usados)
		usados[n] = true
		nombresUnicos[g] = n
		if _, ya := gidDeNombre[n]; !ya {
			gidDeNombre[n] = g
		}
	}
	return gidDeNombre, nombresUnicos, true
}

func nombreUnicoGlifoCFF(n string, g int, usados map[string]bool) string {
	if n == "" || usados[n] {
		n = fmt.Sprintf("g%d", g)
		for usados[n] {
			n += "_"
		}
	}
	return n
}

func emitirCFFPlano(prog *programasCFF, info *infoCFF, trazos []*interpreteType2, gidDeNombre map[string]int, nombresUnicos []string, ordenFijo bool) []byte {
	if info.cid {
		cs := make([][]byte, len(trazos))
		for g, tr := range trazos {
			cs[g] = tr.charstringT2(prog.fd(g).escala)
		}
		return armarCFFCid(prog.nombre, info.cids, cs)
	}
	if ordenFijo {
		cs := make([][]byte, len(trazos))
		for g, tr := range trazos {
			cs[g] = tr.charstringT2(prog.fd(g).escala)
		}
		return armarCFFSimple(prog.nombre, nombresUnicos, cs, nil)
	}
	cod := make(map[byte]string, len(info.propia))
	for c, g := range info.propia {
		if g >= 0 && g < len(nombresUnicos) {
			cod[c] = nombresUnicos[g]
		}
	}
	orden, codigos := ordenGlifosType1(nombresUnicos, cod)
	cs := make([][]byte, len(orden))
	for i, n := range orden {
		g := gidDeNombre[n]
		cs[i] = trazos[g].charstringT2(prog.fd(g).escala)
	}
	return armarCFFSimple(prog.nombre, orden, cs, codigos)
}

func necesitaAplanarCFF(cff []byte, info *infoCFF, prog *programasCFF, especial bool) bool {
	if especial {
		return true
	}
	if bb := prog.top[5]; len(bb) == 4 && (bb[0] > bb[2] || bb[1] > bb[3]) {
		return true
	}
	if info.cid && len(prog.top[18]) > 0 {
		return true
	}
	for _, fd := range prog.fds {
		if math.Abs(fd.escala[0]-1) > 1e-4 || math.Abs(fd.escala[1]-1) > 1e-4 {
			return true
		}
	}
	return cffFallaEnSfnt(cff, info)
}

func cffFallaEnSfnt(cff []byte, info *infoCFF) bool {
	if info.numGlifos <= 0 || info.numGlifos > 0xFFFF {
		return true
	}
	otf := sfntNuevo(0x4F54544F, "CFF ", cff)
	tablas := tablasMetricasCFF(info, make([]float64, info.numGlifos))
	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post[0:], 0x00030000)
	tablas["post"] = post
	for _, tag := range []string{"head", "hhea", "hmtx", "maxp", "post"} {
		var ok bool
		if otf, ok = reemplazarTablaSfnt(otf, tag, tablas[tag]); !ok {
			return true
		}
	}
	var ok bool
	if otf, ok = reescribirCmapUnicodeTTF(otf, map[uint16]uint16{0xE000: 0}); !ok {
		return true
	}
	cara, err := sfnt.Parse(otf)
	if err != nil {
		return true
	}
	var buf sfnt.Buffer
	for g := 0; g < cara.NumGlyphs(); g++ {
		if _, err := cara.LoadGlyph(&buf, sfnt.GlyphIndex(g), 1000, nil); err != nil {
			return true
		}
	}
	return false
}

func enteroDictCFF2(b []byte, v, op1, op2 int) []byte {
	b = binary.BigEndian.AppendUint32(append(b, 29), uint32(v)) //nolint:gosec // desplazamientos positivos
	return append(b, byte(op1), byte(op2))                      //nolint:gosec // operadores CFF de dos bytes
}

func armarCFFCid(nombre string, cids []int, cs [][]byte) []byte {
	if nombre == "" {
		nombre = "CIDFont"
	}
	be := binary.BigEndian
	cabecera := []byte{1, 0, 4, 4}
	nombres := indiceCFF4([][]byte{[]byte(nombre)})
	cadenas := indiceCFF4([][]byte{[]byte("Adobe"), []byte("Identity"), []byte(nombre)})
	sinSubrs := []byte{0, 0}

	usados := make(map[int]bool, len(cs))
	usados[0] = true
	mayorCID := 0
	for g := 1; g < len(cs) && g < len(cids); g++ {
		if c := cids[g]; c > 0 && c <= 0xFFFE {
			mayorCID = max(mayorCID, c)
		}
	}
	charset := make([]byte, 1, 1+2*max(len(cs)-1, 0))
	for g := 1; g < len(cs); g++ {
		cid := 0
		if g < len(cids) {
			cid = cids[g]
		}
		if cid <= 0 || cid > 0xFFFE || usados[cid] {
			mayorCID = min(mayorCID+1, 0xFFFE)
			cid = mayorCID
			for usados[cid] && cid > 1 {
				cid--
			}
		}
		usados[cid] = true
		mayorCID = max(mayorCID, cid)
		charset = be.AppendUint16(charset, uint16(cid)) //nolint:gosec // 1..0xFFFE
	}
	cidCount := max(8720, mayorCID+1)

	nGlifos := uint16(len(cs)) //nolint:gosec // len(cs) <= maxGlifosType1
	fdSelect := []byte{3, 0, 1, 0, 0, 0, byte(nGlifos >> 8), byte(nGlifos & 0xFF)}
	glifos := indiceCFF4(cs)
	privado := []byte{139, 20}

	armarFD := func(offPriv int) []byte {
		fd := []byte{28, 1, 137, 12, 38, 29, 0, 0, 0, 2} // SID 393 FontName + tamaño Private (2)
		fd = enteroDictCFF(fd, offPriv, 18)
		return fd
	}
	armarTop := func(offCharset, offFDSelect, offFDArray, offCS int) []byte {
		top := []byte{28, 1, 135, 28, 1, 136, 139, 12, 30} // ROS primero
		top = enteroDictCFF2(top, cidCount, 12, 34)
		top = enteroDictCFF(top, offCharset, 15)
		top = enteroDictCFF2(top, offFDSelect, 12, 37)
		top = enteroDictCFF2(top, offFDArray, 12, 36)
		top = enteroDictCFF(top, offCS, 17)
		return top
	}

	topDummy := indiceCFF4([][]byte{armarTop(0, 0, 0, 0)})
	fdDummy := indiceCFF4([][]byte{armarFD(0)})
	offCharset := len(cabecera) + len(nombres) + len(topDummy) + len(cadenas) + len(sinSubrs)
	offFDSelect := offCharset + len(charset)
	offCS := offFDSelect + len(fdSelect)
	offFDArray := offCS + len(glifos)
	offPriv := offFDArray + len(fdDummy)
	topReal := indiceCFF4([][]byte{armarTop(offCharset, offFDSelect, offFDArray, offCS)})
	fdReal := indiceCFF4([][]byte{armarFD(offPriv)})

	var b bytes.Buffer
	for _, parte := range [][]byte{cabecera, nombres, topReal, cadenas, sinSubrs, charset, fdSelect, glifos, fdReal, privado} {
		b.Write(parte)
	}
	return b.Bytes()
}
