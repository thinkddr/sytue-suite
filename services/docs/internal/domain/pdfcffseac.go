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

const maxPilaType2 = 48

// programasCFF son los charstrings de un CFF con sus subrutinas y anchos.
type programasCFF struct {
	nombre                     string
	glifos, globales, locales  [][]byte
	anchoDefecto, anchoNominal float64
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
	if cs := top[17]; len(cs) == 1 {
		p.glifos, _, err = indiceCFF(b, int(cs[0]))
	}
	if err != nil || len(p.glifos) == 0 {
		return nil, errCFF
	}
	if pr := top[18]; len(pr) == 2 {
		tam, off := int(pr[0]), int(pr[1])
		if off < 0 || tam < 0 || off+tam > len(b) {
			return nil, errCFF
		}
		priv := dictCFF(b[off : off+tam])
		if v := priv[20]; len(v) == 1 {
			p.anchoDefecto = v[0]
		}
		if v := priv[21]; len(v) == 1 {
			p.anchoNominal = v[0]
		}
		if v := priv[19]; len(v) == 1 {
			if p.locales, _, err = indiceCFF(b, off+int(v[0])); err != nil {
				return nil, err
			}
		}
	}
	return p, nil
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
	gidDeNombre map[string]int
	stems       int
	anchoVisto  bool
	hayAcento   bool
}

// anchoSi es el primer operador que vacía la pila: si trae un operando de más,
// ese es el ancho (§4.1).
func (in *interpreteType2) anchoSi(sobra bool) {
	if in.anchoVisto {
		return
	}
	in.anchoVisto = true
	in.ancho = in.prog.anchoDefecto
	if sobra && len(in.pila) > 0 {
		in.ancho = in.prog.anchoNominal + in.pila[0]
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
			subrs := in.prog.locales
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
		case 1235: // flex
			a, ok := in.n(13)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], a[1], a[2], a[3], a[4], a[5])
			in.curvaRel(a[6], a[7], a[8], a[9], a[10], a[11])
		case 1234: // hflex
			a, ok := in.n(7)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], 0, a[1], a[2], a[3], 0)
			in.curvaRel(a[4], 0, a[5], -a[2], a[6], 0)
		case 1236: // hflex1
			a, ok := in.n(9)
			if !ok {
				return errCFF
			}
			in.curvaRel(a[0], a[1], a[2], a[3], a[4], 0)
			in.curvaRel(a[5], 0, a[6], a[7], a[8], -(a[1] + a[3] + a[7]))
		case 1237: // flex1: el último punto vuelve a la altura (o a la x) del primero
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
		sub := &interpreteType2{prog: in.prog, gidDeNombre: in.gidDeNombre, hayAcento: true}
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

// aplanarSeacCFF devuelve el CFF con los acentuados ya compuestos si trae
// alguno, o el mismo CFF si no trae ninguno o no se puede.
func aplanarSeacCFF(cff []byte) []byte {
	info, err := leerCFF(cff)
	if err != nil || info.cid || info.upem != unidadesCFFDeType1 {
		return cff
	}
	prog, err := leerProgramasCFF(cff)
	if err != nil || len(prog.glifos) != len(info.nombres) {
		return cff
	}
	gidDeNombre := make(map[string]int, len(info.nombres))
	for g, n := range info.nombres {
		if _, repetido := gidDeNombre[n]; repetido || n == "" {
			return cff
		}
		gidDeNombre[n] = g
	}
	trazos := make([]*interpreteType2, len(prog.glifos))
	alguno := false
	for g, cs := range prog.glifos {
		in := &interpreteType2{prog: prog, gidDeNombre: gidDeNombre}
		if in.correr(cs, 0) != nil {
			return cff
		}
		alguno = alguno || in.hayAcento
		trazos[g] = in
	}
	if !alguno {
		return cff
	}
	cod := make(map[byte]string, len(info.propia))
	for c, g := range info.propia {
		cod[c] = info.nombres[g]
	}
	orden, codigos := ordenGlifosType1(info.nombres, cod)
	cs := make([][]byte, len(orden))
	for i, n := range orden {
		cs[i] = trazos[gidDeNombre[n]].charstringT2([2]float64{1, 1})
	}
	return armarCFFSimple(prog.nombre, orden, cs, codigos)
}
