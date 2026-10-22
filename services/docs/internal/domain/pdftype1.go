// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// TYPE 1 INCRUSTADA (/FontFile) convertida a CFF.
//
// Es como incrustan sus fuentes LaTeX (dvips, pdfTeX), Ghostscript y muchos
// generadores viejos. Ni el navegador ni `x/image/font/sfnt` abren un Type 1, así
// que se pintaba con la letra de reserva: el 24 % de las páginas con estas
// fuentes salía mal contra Poppler en la medida de scripts/certipdf-compat.
//
// Se lee de la norma (Adobe, «Adobe Type 1 Font Format»): la parte en claro
// (/FontMatrix, /FontName y la codificación propia), la parte cifrada con eexec
// (/lenIV, /Subrs y /CharStrings) y los contornos de cada glifo, interpretando
// sus charstrings —subrutinas, flex, `seac` y `div` incluidos— hasta dejar solo
// rectas y curvas absolutas. Esas se reescriben como charstrings Type 2 dentro de
// un CFF (Adobe TN 5176 y 5177) que envolverCFF mete en un OpenType, como con el
// CFF suelto. Las indicaciones (hints) se descartan: el rasterizador no las usa.
package domain

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"regexp"
	"strconv"
)

var errType1 = errors.New("pdf: Type 1 ilegible")

const (
	maxGlifosType1     = 8192
	maxSubrsType1      = 1 << 16
	maxOpsGlifoType1   = 1 << 16
	maxProfSubrType1   = 10
	maxPilaType1       = 64
	unidadesCFFDeType1 = 1000
)

// type1ACFF convierte el programa de una fuente Type 1 (/FontFile, con su
// /Length1 = tamaño de la parte en claro) en un CFF con los mismos glifos.
func type1ACFF(b []byte, len1 int) ([]byte, error) {
	b = quitarSegmentosPFB(b)
	claro, cifrado := partirType1(b, len1)
	if len(cifrado) < 4 {
		return nil, errType1
	}
	priv := descifrarType1(cifrado, 55665, 4)
	lenIV := 4
	if m := reLenIV.FindSubmatch(priv); m != nil {
		lenIV, _ = strconv.Atoi(string(m[1]))
	}
	subrs := leerSubrsType1(priv, lenIV)
	nombres, programas := leerCharStringsType1(priv, lenIV)
	if len(nombres) == 0 || len(nombres) > maxGlifosType1 {
		return nil, errType1
	}
	esc := escalaType1(claro)
	porNombre := make(map[string][]byte, len(nombres))
	for i, n := range nombres {
		porNombre[n] = programas[i]
	}
	orden, codigos := ordenGlifosType1(nombres, codificacionType1(claro))
	cs := make([][]byte, len(orden))
	for g, n := range orden {
		in := &interpreteType1{subrs: subrs, glifos: porNombre}
		if p, ok := porNombre[n]; ok && in.ejecutar(p) == nil {
			cs[g] = in.charstringT2(esc)
		} else {
			cs[g] = []byte{139, 14} // ancho 0, endchar: un glifo vacío antes que ninguno
		}
	}
	return armarCFFSimple(nombreType1(claro), orden, cs, codigos), nil
}

var (
	reLenIV      = regexp.MustCompile(`/lenIV\s+(-?\d+)`)
	reFontMatrix = regexp.MustCompile(`/FontMatrix\s*\[([^\]]*)\]`)
	reFontName   = regexp.MustCompile(`/FontName\s*/([^\s/\[\]{}()<>%]+)`)
	reCodigo     = regexp.MustCompile(`dup\s+(\d+)\s*/([^\s/\[\]{}()<>%]+)\s+put`)
)

// quitarSegmentosPFB quita las cabeceras de segmento de un .pfb (0x80 0x01/0x02
// + longitud), que algún generador incrusta tal cual en vez del programa limpio.
func quitarSegmentosPFB(b []byte) []byte {
	if len(b) < 6 || b[0] != 0x80 {
		return b
	}
	var out []byte
	for p := 0; p+6 <= len(b) && b[p] == 0x80 && (b[p+1] == 1 || b[p+1] == 2); {
		n := int(binary.LittleEndian.Uint32(b[p+2:]))
		p += 6
		if n < 0 || p+n > len(b) {
			n = len(b) - p
		}
		out = append(out, b[p:p+n]...)
		p += n
	}
	return out
}

// partirType1 separa la parte en claro de la cifrada. /Length1 dice dónde
// empieza la cifrada; si no cuadra con el `eexec`, se busca a mano. La cifrada
// puede venir en hexadecimal (lo dice la norma: si los cuatro primeros bytes son
// dígitos hexadecimales).
func partirType1(b []byte, len1 int) (claro, cifrado []byte) {
	i := bytes.Index(b, []byte("eexec"))
	if i < 0 {
		return b, nil
	}
	inicio := len1
	if inicio < i+5 || inicio > len(b) {
		inicio = i + 5
		for inicio < len(b) && (b[inicio] == '\r' || b[inicio] == '\n' || b[inicio] == ' ' || b[inicio] == '\t') {
			inicio++
		}
	}
	claro, cifrado = b[:i], b[inicio:]
	if len(cifrado) >= 4 && esHex(cifrado[:4]) {
		limpio := make([]byte, 0, len(cifrado))
		for _, c := range cifrado {
			if esHex([]byte{c}) {
				limpio = append(limpio, c)
			} else if c > ' ' {
				break
			}
		}
		dec := make([]byte, len(limpio)/2)
		if n, err := hex.Decode(dec, limpio[:2*len(dec)]); err == nil {
			cifrado = dec[:n]
		}
	}
	return claro, cifrado
}

func esHex(b []byte) bool {
	for _, c := range b {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// descifrarType1 es el descifrado de la norma (§7): eexec con r = 55665 y los
// charstrings con r = 4330, quitando los primeros bytes de relleno.
func descifrarType1(b []byte, r uint16, saltar int) []byte {
	if saltar < 0 { // lenIV = -1: el charstring no va cifrado
		return b
	}
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = c ^ byte(r>>8)
		r = (uint16(c)+r)*52845 + 22719
	}
	if len(out) < saltar {
		return nil
	}
	return out[saltar:]
}

// escanerType1 lee los símbolos de la parte privada, que es PostScript con
// trozos binarios (los charstrings) metidos detrás de `RD` o `-|`.
type escanerType1 struct {
	b []byte
	p int
}

func esBlancoPS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}

func (s *escanerType1) simbolo() string {
	for s.p < len(s.b) && esBlancoPS(s.b[s.p]) {
		s.p++
	}
	ini := s.p
	if s.p < len(s.b) && s.b[s.p] == '/' {
		s.p++
	}
	for s.p < len(s.b) && !esBlancoPS(s.b[s.p]) && (s.p == ini || bytes.IndexByte([]byte("/[]{}()<>%"), s.b[s.p]) < 0) {
		s.p++
	}
	return string(s.b[ini:s.p])
}

// binario lee `<n> RD <n bytes>` y devuelve los bytes, ya descifrados.
func (s *escanerType1) binario(lenIV int) ([]byte, bool) {
	n, err := strconv.Atoi(s.simbolo())
	if err != nil || n < 0 {
		return nil, false
	}
	s.simbolo() // RD o -|
	s.p++       // un solo blanco antes de los bytes
	if s.p+n > len(s.b) {
		return nil, false
	}
	datos := s.b[s.p : s.p+n]
	s.p += n
	return descifrarType1(datos, 4330, lenIV), true
}

// esCierreType1 son los símbolos que cierran una entrada de /Subrs o de
// /CharStrings: NP, ND, |, |-, o su forma larga.
func esCierreType1(t string) bool {
	switch t {
	case "NP", "ND", "|", "|-", "noaccess", "put", "def", "readonly", "executeonly":
		return true
	}
	return false
}

func leerSubrsType1(priv []byte, lenIV int) [][]byte {
	i := bytes.Index(priv, []byte("/Subrs"))
	if i < 0 {
		return nil
	}
	s := &escanerType1{b: priv, p: i + len("/Subrs")}
	n, err := strconv.Atoi(s.simbolo())
	if err != nil || n <= 0 || n > maxSubrsType1 {
		return nil
	}
	subrs := make([][]byte, n)
	for {
		t := s.simbolo()
		if esCierreType1(t) || t == "array" {
			continue
		}
		if t != "dup" {
			return subrs
		}
		idx, err := strconv.Atoi(s.simbolo())
		datos, ok := s.binario(lenIV)
		if err != nil || !ok {
			return subrs
		}
		if idx >= 0 && idx < n {
			subrs[idx] = datos
		}
	}
}

func leerCharStringsType1(priv []byte, lenIV int) ([]string, [][]byte) {
	i := bytes.Index(priv, []byte("/CharStrings"))
	if i < 0 {
		return nil, nil
	}
	s := &escanerType1{b: priv, p: i + len("/CharStrings")}
	var nombres []string
	var programas [][]byte
	for len(nombres) <= maxGlifosType1 {
		t := s.simbolo()
		switch {
		case t == "" || t == "end":
			return nombres, programas
		case len(t) > 1 && t[0] == '/':
			datos, ok := s.binario(lenIV)
			if !ok {
				return nombres, programas
			}
			nombres = append(nombres, t[1:])
			programas = append(programas, datos)
		}
	}
	return nombres, programas
}

// codificacionType1 es la codificación propia de la parte en claro: nil si es
// la estándar (o no se entiende), y si no, código → nombre de glifo.
func codificacionType1(claro []byte) map[byte]string {
	i := bytes.Index(claro, []byte("/Encoding"))
	if i < 0 || bytes.HasPrefix(bytes.TrimLeft(claro[i+len("/Encoding"):], " \t\r\n"), []byte("StandardEncoding")) {
		return nil
	}
	out := map[byte]string{}
	for _, m := range reCodigo.FindAllSubmatch(claro[i:], -1) {
		if c, err := strconv.Atoi(string(m[1])); err == nil && c >= 0 && c < 256 {
			out[byte(c)] = string(m[2])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// escalaType1 lleva las coordenadas a milésimas de em desde la /FontMatrix (lo
// normal es 0.001, y entonces no cambia nada).
func escalaType1(claro []byte) [2]float64 {
	esc := [2]float64{1, 1}
	m := reFontMatrix.FindSubmatch(claro)
	if m == nil {
		return esc
	}
	var v []float64
	for _, t := range bytes.Fields(m[1]) {
		f, err := strconv.ParseFloat(string(t), 64)
		if err != nil {
			return esc
		}
		v = append(v, f)
	}
	if len(v) == 6 && v[0] > 0 && v[3] > 0 {
		esc = [2]float64{v[0] * unidadesCFFDeType1, v[3] * unidadesCFFDeType1}
	}
	return esc
}

func nombreType1(claro []byte) string {
	if m := reFontName.FindSubmatch(claro); m != nil {
		return string(m[1])
	}
	return "Type1"
}

// ordenGlifosType1 pone .notdef el primero y detrás los glifos de la
// codificación propia por orden de código: así cabe en una codificación CFF de
// formato 0 (el glifo 1 tiene el primer código, el 2 el segundo...). Devuelve el
// orden y esos códigos; sin codificación propia, la estándar no se escribe.
func ordenGlifosType1(nombres []string, cod map[byte]string) ([]string, []byte) {
	orden := []string{".notdef"}
	puesto := map[string]bool{".notdef": true}
	existe := make(map[string]bool, len(nombres))
	for _, n := range nombres {
		existe[n] = true
	}
	var codigos []byte
	for c := 0; c < 256 && len(codigos) < 255; c++ {
		n, ok := cod[byte(c)]
		if !ok || puesto[n] || !existe[n] {
			continue
		}
		orden = append(orden, n)
		puesto[n] = true
		codigos = append(codigos, byte(c))
	}
	for _, n := range nombres {
		if !puesto[n] {
			orden = append(orden, n)
			puesto[n] = true
		}
	}
	return orden, codigos
}

type puntoT1 struct{ x, y float64 }

// segmentoT1 es una orden ya absoluta: 'm' (mover), 'l' (recta) o 'c' (curva).
type segmentoT1 struct {
	op byte
	p  [3]puntoT1
}

// interpreteType1 ejecuta los charstrings de un glifo (§6 de la norma).
type interpreteType1 struct {
	subrs  [][]byte
	glifos map[string][]byte

	pila, ps []float64
	x, y     float64
	ancho    float64
	sbx      float64
	segs     []segmentoT1
	abierto  bool // hay un subtrazado al que añadir
	ops      int

	flex       bool
	puntosFlex []puntoT1

	// En un `seac`: el ancho es el del compuesto y el acento va desplazado.
	enSeac      bool
	origenX     float64
	origenY     float64
	terminado   bool
	anchoFijado bool
}

func (in *interpreteType1) ejecutar(cs []byte) error {
	return in.correr(cs, 0)
}

func (in *interpreteType1) mover(x, y float64) {
	in.x, in.y = x, y
	if in.flex {
		in.puntosFlex = append(in.puntosFlex, puntoT1{x, y})
		return
	}
	if n := len(in.segs); n > 0 && in.segs[n-1].op == 'm' {
		in.segs[n-1].p[0] = puntoT1{x, y}
	} else {
		in.segs = append(in.segs, segmentoT1{op: 'm', p: [3]puntoT1{{x, y}}})
	}
	in.abierto = true
}

func (in *interpreteType1) asegurarInicio() {
	if !in.abierto {
		in.segs = append(in.segs, segmentoT1{op: 'm', p: [3]puntoT1{{in.x, in.y}}})
		in.abierto = true
	}
}

func (in *interpreteType1) recta(x, y float64) {
	in.asegurarInicio()
	in.x, in.y = x, y
	in.segs = append(in.segs, segmentoT1{op: 'l', p: [3]puntoT1{{x, y}}})
}

func (in *interpreteType1) curva(a, b, c puntoT1) {
	in.asegurarInicio()
	in.x, in.y = c.x, c.y
	in.segs = append(in.segs, segmentoT1{op: 'c', p: [3]puntoT1{a, b, c}})
}

func (in *interpreteType1) sacar() float64 {
	if len(in.pila) == 0 {
		return 0
	}
	v := in.pila[len(in.pila)-1]
	in.pila = in.pila[:len(in.pila)-1]
	return v
}

// args devuelve los n operandos de abajo arriba (rellenando con 0 si faltan).
func (in *interpreteType1) args(n int) []float64 {
	out := make([]float64, n)
	k := len(in.pila)
	for i := 0; i < n; i++ {
		if j := k - n + i; j >= 0 {
			out[i] = in.pila[j]
		}
	}
	return out
}

//nolint:gocyclo,gocognit // un caso por operador de la norma (§6)
func (in *interpreteType1) correr(cs []byte, prof int) error {
	if prof > maxProfSubrType1 {
		return errType1
	}
	for i := 0; i < len(cs) && !in.terminado; {
		in.ops++
		if in.ops > maxOpsGlifoType1 || len(in.pila) > maxPilaType1 {
			return errType1
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
		case v == 255 && i+4 < len(cs):
			in.pila = append(in.pila, float64(int32(binary.BigEndian.Uint32(cs[i+1:])))) //nolint:gosec // G115: la norma lo define con signo
			i += 5
			continue
		case v >= 32:
			return errType1
		}
		op := v
		i++
		if v == 12 {
			if i >= len(cs) {
				return errType1
			}
			op = 1200 + int(cs[i])
			i++
		}
		limpiar := true
		switch op {
		case 13: // hsbw
			a := in.args(2)
			in.fijarAncho(a[0], 0, a[1])
		case 1207: // sbw
			a := in.args(4)
			in.fijarAncho(a[0], a[1], a[2])
		case 21: // rmoveto
			a := in.args(2)
			in.mover(in.x+a[0], in.y+a[1])
		case 22: // hmoveto
			in.mover(in.x+in.args(1)[0], in.y)
		case 4: // vmoveto
			in.mover(in.x, in.y+in.args(1)[0])
		case 5: // rlineto
			a := in.args(2)
			in.recta(in.x+a[0], in.y+a[1])
		case 6: // hlineto
			in.recta(in.x+in.args(1)[0], in.y)
		case 7: // vlineto
			in.recta(in.x, in.y+in.args(1)[0])
		case 8: // rrcurveto
			a := in.args(6)
			p1 := puntoT1{in.x + a[0], in.y + a[1]}
			p2 := puntoT1{p1.x + a[2], p1.y + a[3]}
			in.curva(p1, p2, puntoT1{p2.x + a[4], p2.y + a[5]})
		case 30: // vhcurveto
			a := in.args(4)
			p1 := puntoT1{in.x, in.y + a[0]}
			p2 := puntoT1{p1.x + a[1], p1.y + a[2]}
			in.curva(p1, p2, puntoT1{p2.x + a[3], p2.y})
		case 31: // hvcurveto
			a := in.args(4)
			p1 := puntoT1{in.x + a[0], in.y}
			p2 := puntoT1{p1.x + a[1], p1.y + a[2]}
			in.curva(p1, p2, puntoT1{p2.x, p2.y + a[3]})
		case 9: // closepath: el subtrazado siguiente empieza donde está el punto
			in.abierto = false
		case 10: // callsubr
			n := int(in.sacar())
			if n < 0 || n >= len(in.subrs) || in.subrs[n] == nil {
				return errType1
			}
			if err := in.correr(in.subrs[n], prof+1); err != nil {
				return err
			}
			limpiar = false
		case 11: // return
			return nil
		case 14: // endchar
			in.terminado = true
			return nil
		case 1216: // callothersubr
			in.otraSubr()
			limpiar = false
		case 1217: // pop
			if len(in.ps) > 0 {
				in.pila = append(in.pila, in.ps[len(in.ps)-1])
				in.ps = in.ps[:len(in.ps)-1]
			} else {
				in.pila = append(in.pila, 0)
			}
			limpiar = false
		case 1212: // div
			b, a := in.sacar(), in.sacar()
			if b == 0 {
				in.pila = append(in.pila, 0)
			} else {
				in.pila = append(in.pila, a/b)
			}
			limpiar = false
		case 1233: // setcurrentpoint
			a := in.args(2)
			in.x, in.y = a[0], a[1]
		case 1206: // seac
			a := in.args(5)
			return in.seac(a[0], a[1], a[2], int(a[3]), int(a[4]))
		case 1, 3, 1200, 1201, 1202: // hstem, vstem, dotsection, vstem3, hstem3: indicaciones
		default:
			return errType1
		}
		if limpiar {
			in.pila = in.pila[:0]
		}
	}
	return nil
}

// fijarAncho es hsbw/sbw: pone el punto en el lado izquierdo. Dentro de un
// `seac`, el ancho es el del compuesto y el punto, el del componente desplazado.
func (in *interpreteType1) fijarAncho(sbx, sby, ancho float64) {
	in.x, in.y = in.origenX+sbx, in.origenY+sby
	if in.anchoFijado {
		return
	}
	in.sbx, in.ancho, in.anchoFijado = sbx, ancho, true
}

// otraSubr hace lo que harían las OtherSubrs de la norma (§8): flex (0, 1 y 2)
// y el cambio de indicaciones (3), que aquí solo devuelve su argumento.
func (in *interpreteType1) otraSubr() {
	num := int(in.sacar())
	n := int(in.sacar())
	if n < 0 || n > len(in.pila) {
		n = len(in.pila)
	}
	args := append([]float64(nil), in.pila[len(in.pila)-n:]...)
	in.pila = in.pila[:len(in.pila)-n]
	in.ps = in.ps[:0]
	switch num {
	case 1: // empieza un flex: los rmoveto que siguen son sus puntos
		in.flex = true
		in.puntosFlex = in.puntosFlex[:0]
		return
	case 2:
		return
	case 0: // acaba: dos curvas con los puntos 1 a 6 (el 0 es el de referencia)
		in.flex = false
		if p := in.puntosFlex; len(p) >= 7 {
			in.curva(p[1], p[2], p[3])
			in.curva(p[4], p[5], p[6])
		}
		// `pop pop setcurrentpoint` recoge x y luego y.
		in.ps = append(in.ps, in.y, in.x)
		return
	}
	for i := len(args) - 1; i >= 0; i-- {
		in.ps = append(in.ps, args[i])
	}
}

// seac compone un acentuado con dos glifos de la codificación estándar: la base
// en su sitio y el acento con su lado izquierdo desplazado adx, ady del lado
// izquierdo del compuesto.
func (in *interpreteType1) seac(asb, adx, ady float64, base, acento int) error {
	nb, okb := nombreEstandarType1(base)
	na, oka := nombreEstandarType1(acento)
	if !okb || !oka || in.enSeac {
		return errType1
	}
	cb, cA := in.glifos[nb], in.glifos[na]
	if cb == nil || cA == nil {
		return errType1
	}
	in.enSeac = true
	in.pila, in.ps = in.pila[:0], in.ps[:0]
	in.terminado = false
	if err := in.correr(cb, 1); err != nil {
		return err
	}
	in.terminado, in.abierto = false, false
	in.origenX, in.origenY = in.sbx+adx-asb, ady
	in.pila, in.ps = in.pila[:0], in.ps[:0]
	if err := in.correr(cA, 1); err != nil {
		return err
	}
	in.terminado = true
	return nil
}

func nombreEstandarType1(c int) (string, bool) {
	if c < 0 || c > 255 {
		return "", false
	}
	sid, ok := codificacionEstandarCFF[byte(c)]
	if !ok || sid >= len(cadenasEstandarCFF) {
		return "", false
	}
	return cadenasEstandarCFF[sid], true
}

// charstringT2 escribe los segmentos como charstring Type 2: ancho delante,
// rmoveto/rlineto/rrcurveto relativos y endchar. Sin indicaciones.
func (in *interpreteType1) charstringT2(esc [2]float64) []byte {
	out := numeroT2(nil, in.ancho*esc[0])
	var x, y float64
	for _, s := range in.segs {
		n := 1
		if s.op == 'c' {
			n = 3
		}
		for k := 0; k < n; k++ {
			px, py := s.p[k].x*esc[0], s.p[k].y*esc[1]
			out = numeroT2(out, px-x)
			out = numeroT2(out, py-y)
			x, y = px, py
		}
		switch s.op {
		case 'm':
			out = append(out, 21)
		case 'l':
			out = append(out, 5)
		case 'c':
			out = append(out, 8)
		}
	}
	return append(out, 14)
}

// numeroT2 codifica un operando de charstring Type 2: entero en la forma más
// corta, o 16.16 con signo si tiene decimales.
func numeroT2(b []byte, v float64) []byte {
	r := math.Round(v)
	switch {
	case math.Abs(v-r) > 1e-6 || r < -32768 || r > 32767:
		f := int32(math.Round(math.Max(-32768, math.Min(32767.99, v)) * 65536))
		return binary.BigEndian.AppendUint32(append(b, 255), uint32(f)) //nolint:gosec // G115: 16.16 con signo
	case r >= -107 && r <= 107:
		return append(b, byte(int(r)+139)) //nolint:gosec // G115: 32..246
	case r >= 108 && r <= 1131:
		n := int(r) - 108
		return append(b, byte(n/256+247), byte(n%256)) //nolint:gosec // G115: n ≤ 1023
	case r >= -1131 && r <= -108:
		n := -int(r) - 108
		return append(b, byte(n/256+251), byte(n%256)) //nolint:gosec // G115: idem
	default:
		return binary.BigEndian.AppendUint16(append(b, 28), uint16(int16(r))) //nolint:gosec // G115: comprobado el rango
	}
}

// indiceCFF4 escribe una INDEX con desplazamientos de 4 bytes.
func indiceCFF4(elems [][]byte) []byte {
	if len(elems) == 0 {
		return []byte{0, 0}
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(len(elems))) //nolint:gosec // G115: ≤ maxGlifosType1
	out = append(out, 4)
	off := uint32(1)
	out = binary.BigEndian.AppendUint32(out, off)
	for _, e := range elems {
		off += uint32(len(e)) //nolint:gosec // G115: unos megas como mucho
		out = binary.BigEndian.AppendUint32(out, off)
	}
	for _, e := range elems {
		out = append(out, e...)
	}
	return out
}

func enteroDictCFF(b []byte, v, op int) []byte {
	b = binary.BigEndian.AppendUint32(append(b, 29), uint32(v)) //nolint:gosec // G115: desplazamientos positivos
	return append(b, byte(op))                                  //nolint:gosec // G115: operadores de un byte
}

// armarCFFSimple arma el CFF (TN 5176): cabecera, nombre, Top DICT, cadenas,
// subrutinas globales (ninguna), charset, codificación, glifos y Private DICT.
func armarCFFSimple(nombre string, orden []string, cs [][]byte, codigos []byte) []byte {
	sidEstandar := make(map[string]int, len(cadenasEstandarCFF))
	for i, s := range cadenasEstandarCFF {
		sidEstandar[s] = i
	}
	var cadenas [][]byte
	charset := []byte{0}
	for _, n := range orden[1:] {
		sid, ok := sidEstandar[n]
		if !ok {
			sid = len(cadenasEstandarCFF) + len(cadenas)
			cadenas = append(cadenas, []byte(n))
		}
		charset = binary.BigEndian.AppendUint16(charset, uint16(sid)) //nolint:gosec // G115: < 391 + maxGlifosType1
	}
	var codif []byte
	if len(codigos) > 0 {
		codif = append([]byte{0, byte(len(codigos))}, codigos...) //nolint:gosec // G115: ≤ 255
	}
	privado := []byte{139, 20} // defaultWidthX 0
	cabecera := []byte{1, 0, 4, 4}
	nombres := indiceCFF4([][]byte{[]byte(nombre)})
	cadIdx := indiceCFF4(cadenas)
	glifos := indiceCFF4(cs)
	tamTop := 6 + 6 + 11 // charset y CharStrings (número de 5 bytes + operador) y Private (dos números)
	if codif != nil {
		tamTop += 6
	}
	tamTopIdx := len(indiceCFF4([][]byte{make([]byte, tamTop)}))
	offCharset := len(cabecera) + len(nombres) + tamTopIdx + len(cadIdx) + 2
	offCodif := offCharset + len(charset)
	offGlifos := offCodif + len(codif)
	offPrivado := offGlifos + len(glifos)

	top := enteroDictCFF(nil, offCharset, 15)
	if codif != nil {
		top = enteroDictCFF(top, offCodif, 16)
	}
	top = enteroDictCFF(top, offGlifos, 17)
	top = binary.BigEndian.AppendUint32(append(top, 29), uint32(len(privado))) //nolint:gosec // G115: dos bytes
	top = enteroDictCFF(top, offPrivado, 18)

	var b bytes.Buffer
	for _, parte := range [][]byte{cabecera, nombres, indiceCFF4([][]byte{top}), cadIdx, {0, 0}, charset, codif, glifos, privado} {
		b.Write(parte)
	}
	return b.Bytes()
}
