// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Color, funciones y sombreados para el render de PDF a SVG.
//
// Un PDF no dice «rojo»: dice «0.9 0.1 0.1 en el espacio que declaré hace tres
// operadores», y ese espacio puede ser una tabla indexada, una tinta plana con
// una función de conversión detrás, o un perfil ICC incrustado. Aquí se traduce
// todo eso a los seis dígitos hexadecimales que entiende CSS.
//
// Las funciones (§7.10) hacen falta para dos cosas: convertir tintas planas a un
// color que se pueda pintar, y sacar las paradas de un degradado. Se implementan
// los tipos 0 (muestreada), 2 (exponencial) y 3 (cosida); el tipo 4 —una
// calculadora PostScript— se aproxima por sus extremos, que para un degradado da
// la dirección correcta aunque no el punto medio exacto.
package domain

import (
	"math"
	"strconv"
	"strings"
)

// --- Espacios de color -------------------------------------------------------

type espacioColor struct {
	clase  string // Gray | RGB | CMYK | Indexed | Sep | Lab | Pattern
	n      int    // número de componentes de entrada
	base   *espacioColor
	tabla  []byte // paleta de Indexed, ya en componentes del espacio base
	maxIdx int
	fn     *funcionPDF // transformación de tinta de Separation/DeviceN
}

var (
	espacioGris    = &espacioColor{clase: "Gray", n: 1}
	espacioRGB     = &espacioColor{clase: "RGB", n: 3}
	espacioCMYK    = &espacioColor{clase: "CMYK", n: 4}
	espacioPatron  = &espacioColor{clase: "Pattern", n: 1}
	espacioDefecto = espacioGris
)

// inicial es el color que toma el espacio al seleccionarlo (§8.6.8): negro en
// todos los de dispositivo, que es lo que espera quien dibuja sin fijar color.
func (ec *espacioColor) inicial() string {
	if ec == nil {
		return "#000000"
	}
	if ec.clase == "Pattern" {
		return "none"
	}
	return "#000000"
}

func (ec *espacioColor) componentes() int {
	if ec == nil {
		return 1
	}
	return ec.n
}

// css convierte los componentes al color CSS del espacio.
func (ec *espacioColor) css(c []float64) string {
	if ec == nil {
		return cssGris(valorEn(c, 0))
	}
	switch ec.clase {
	case "RGB":
		return cssRGB(valorEn(c, 0), valorEn(c, 1), valorEn(c, 2))
	case "CMYK":
		return cssCMYK(valorEn(c, 0), valorEn(c, 1), valorEn(c, 2), valorEn(c, 3))
	case "Lab":
		return cssLab(valorEn(c, 0), valorEn(c, 1), valorEn(c, 2))
	case "Indexed":
		return ec.cssIndexado(int(valorEn(c, 0) + 0.5))
	case "Sep":
		if ec.fn != nil {
			return ec.base.css(ec.fn.evaluarVarias(c))
		}
		// Sin función que la convierta, una tinta al 100 % es negro y al 0 %
		// blanco: es la interpretación que menos sorprende.
		return cssGris(1 - valorEn(c, 0))
	case "Pattern":
		return "#808080"
	default:
		return cssGris(valorEn(c, 0))
	}
}

func (ec *espacioColor) cssIndexado(i int) string {
	base := ec.base
	if base == nil {
		base = espacioRGB
	}
	nb := base.componentes()
	if i < 0 {
		i = 0
	}
	if i > ec.maxIdx {
		i = ec.maxIdx
	}
	off := i * nb
	if off+nb > len(ec.tabla) {
		return "#000000"
	}
	comp := make([]float64, nb)
	for j := 0; j < nb; j++ {
		comp[j] = float64(ec.tabla[off+j]) / 255
	}
	if base.clase == "Lab" {
		// En Lab la tabla NO es 0-1: L va de 0 a 100 y a/b de -128 a 127.
		comp[0] *= 100
		comp[1] = comp[1]*255 - 128
		comp[2] = comp[2]*255 - 128
	}
	return base.css(comp)
}

func valorEn(c []float64, i int) float64 {
	if i < len(c) {
		return c[i]
	}
	return 0
}

func cssGris(g float64) string {
	v := byteDe(g)
	return "#" + hex2(v) + hex2(v) + hex2(v)
}

func cssRGB(r, g, b float64) string {
	return "#" + hex2(byteDe(r)) + hex2(byteDe(g)) + hex2(byteDe(b))
}

// cssCMYK usa la conversión naíf (§10.4.2.3). No es colorimétrica —para eso
// haría falta el perfil de salida— pero es la que aplican todos los visores y
// la que el diseñador vio al exportar.
func cssCMYK(c, m, y, k float64) string {
	return cssRGB((1-acotar01(c))*(1-acotar01(k)),
		(1-acotar01(m))*(1-acotar01(k)),
		(1-acotar01(y))*(1-acotar01(k)))
}

// cssLab pasa de CIE L*a*b* (D50) a sRGB.
func cssLab(l, a, b float64) string {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	g := func(t float64) float64 {
		if t > 6.0/29 {
			return t * t * t
		}
		return 3 * (6.0 / 29) * (6.0 / 29) * (t - 4.0/29)
	}
	// Blanco de referencia D50, que es el que fija la especificación para PDF.
	x, y, z := 0.9642*g(fx), 1.0*g(fy), 0.8249*g(fz)
	lr := 3.1338561*x - 1.6168667*y - 0.4906146*z
	lg := -0.9787684*x + 1.9161415*y + 0.0334540*z
	lb := 0.0719453*x - 0.2289914*y + 1.4052427*z
	gamma := func(v float64) float64 {
		if v <= 0.0031308 {
			return 12.92 * v
		}
		return 1.055*math.Pow(math.Max(v, 0), 1/2.4) - 0.055
	}
	return cssRGB(gamma(lr), gamma(lg), gamma(lb))
}

func byteDe(v float64) byte {
	x := int(math.Round(acotar01(v) * 255))
	return byte(x & 0xFF)
}

func hex2(v byte) string {
	const d = "0123456789abcdef"
	return string([]byte{d[v>>4], d[v&0x0f]})
}

// espacioPorNombre resuelve el operando de `cs`/`CS`: o es uno de los nombres de
// dispositivo, o es la clave de un recurso /ColorSpace.
func (l *lienzo) espacioPorNombre(nombre pdfName, recursos pdfDict) *espacioColor {
	switch nombre {
	case "DeviceGray", "G", "CalGray":
		return espacioGris
	case "DeviceRGB", "RGB", "CalRGB":
		return espacioRGB
	case "DeviceCMYK", "CMYK":
		return espacioCMYK
	case "Pattern":
		return espacioPatron
	}
	if recursos != nil {
		if cat, ok := l.d.resolver(recursos["ColorSpace"]).(pdfDict); ok {
			if v, ok := cat[nombre]; ok {
				return l.espacio(v, 0)
			}
		}
	}
	return espacioDefecto
}

// espacio interpreta la definición de un espacio de color, que puede ser un
// nombre o un array con su familia en la primera posición.
// las once que existen no se simplifican, se enumeran.
//
//nolint:gocognit,gocyclo // Un espacio de color de PDF es una familia por rama (§8.6):
func (l *lienzo) espacio(o pdfObj, prof int) *espacioColor {
	if prof > 8 {
		return espacioDefecto
	}
	switch v := l.d.resolver(o).(type) {
	case pdfName:
		return l.espacioPorNombre(v, nil)
	case pdfArray:
		if len(v) == 0 {
			return espacioDefecto
		}
		familia, _ := l.d.resolver(v[0]).(pdfName)
		switch familia {
		case "ICCBased":
			// El perfil ICC no se interpreta: se usa el espacio alternativo, y si
			// no lo declara, el que corresponda al número de componentes. Es lo que
			// hace cualquier visor sin gestión de color, y acierta salvo en tintas
			// muy separadas del sRGB.
			if len(v) >= 2 {
				if st, ok := l.d.resolver(v[1]).(*pdfStream); ok {
					if alt, ok := st.Dict["Alternate"]; ok {
						return l.espacio(alt, prof+1)
					}
					if n, ok := comoEntero(l.d.resolver(st.Dict["N"])); ok {
						switch n {
						case 1:
							return espacioGris
						case 4:
							return espacioCMYK
						}
					}
				}
			}
			return espacioRGB
		case "CalRGB":
			return espacioRGB
		case "CalGray":
			return espacioGris
		case "Lab":
			return &espacioColor{clase: "Lab", n: 3}
		case "Indexed", "I":
			return l.espacioIndexado(v, prof)
		case "Separation":
			ec := &espacioColor{clase: "Sep", n: 1, base: espacioGris}
			if len(v) >= 3 {
				ec.base = l.espacio(v[2], prof+1)
			}
			if len(v) >= 4 {
				ec.fn = l.funcion(v[3], 0)
			}
			return ec
		case "DeviceN":
			ec := &espacioColor{clase: "Sep", n: 1, base: espacioGris}
			if nombres, ok := l.d.resolver(v[1]).(pdfArray); ok {
				ec.n = len(nombres)
			}
			if len(v) >= 3 {
				ec.base = l.espacio(v[2], prof+1)
			}
			if len(v) >= 4 {
				ec.fn = l.funcion(v[3], 0)
			}
			return ec
		case "Pattern":
			return espacioPatron
		case "DeviceGray":
			return espacioGris
		case "DeviceRGB":
			return espacioRGB
		case "DeviceCMYK":
			return espacioCMYK
		}
	}
	return espacioDefecto
}

func (l *lienzo) espacioIndexado(v pdfArray, prof int) *espacioColor {
	ec := &espacioColor{clase: "Indexed", n: 1, base: espacioRGB}
	if len(v) >= 2 {
		ec.base = l.espacio(v[1], prof+1)
	}
	if len(v) >= 3 {
		if hv, ok := comoEntero(l.d.resolver(v[2])); ok {
			ec.maxIdx = int(hv)
		}
	}
	if len(v) >= 4 {
		switch t := l.d.resolver(v[3]).(type) {
		case pdfString:
			ec.tabla = []byte(t)
		case *pdfStream:
			if b, err := l.d.datosDeFlujo(t); err == nil {
				ec.tabla = b
			}
		}
	}
	return ec
}

// color aplica `sc`/`scn`/`SC`/`SCN`. Con un espacio de patrón, el último
// operando es el NOMBRE del patrón y no un número.
func (e *ejecucion) color(ops []pdfObj, trazo bool) {
	ec := e.estado.csRelleno
	if trazo {
		ec = e.estado.csTrazo
	}
	if ec != nil && ec.clase == "Pattern" {
		if len(ops) > 0 {
			if nom, ok := e.l.d.resolver(ops[len(ops)-1]).(pdfName); ok {
				e.aplicarPatron(nom, trazo)
				return
			}
		}
		return
	}
	comps := make([]float64, 0, len(ops))
	for _, o := range ops {
		comps = append(comps, comoReal(e.l.d.resolver(o)))
	}
	css := ec.css(comps)
	if trazo {
		e.estado.trazo, e.estado.patronTrazo = css, ""
	} else {
		e.estado.relleno, e.estado.patronRelleno = css, ""
	}
}

// --- Funciones ---------------------------------------------------------------

type funcionPDF struct {
	tipo     int
	dominio  []float64
	rango    []float64
	c0, c1   []float64
	exp      float64
	trozos   []*funcionPDF
	limites  []float64
	codifica []float64
	muestras []byte
	tamano   []int
	bps      int
	decodif  []float64
	nSalida  int
}

// funcion interpreta un diccionario o flujo de función. Un array de funciones
// (una por componente de salida) se envuelve en una función compuesta.
// uno; el anidamiento es el del formato.
//
//nolint:gocognit,gocyclo // Cuatro tipos de función (§7.10) con campos distintos cada
func (l *lienzo) funcion(o pdfObj, prof int) *funcionPDF {
	if prof > 8 {
		return nil
	}
	res := l.d.resolver(o)
	if arr, ok := res.(pdfArray); ok {
		// Array de N funciones de 1 salida cada una: se representa como tipo 3
		// degenerado con `limites` vacío, que `evaluar` trata como concatenación.
		f := &funcionPDF{tipo: -1, dominio: []float64{0, 1}}
		for _, e := range arr {
			if sub := l.funcion(e, prof+1); sub != nil {
				f.trozos = append(f.trozos, sub)
			}
		}
		f.nSalida = len(f.trozos)
		return f
	}

	var dict pdfDict
	var st *pdfStream
	switch v := res.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict, st = v.Dict, v
	default:
		return nil
	}

	f := &funcionPDF{exp: 1}
	if v, ok := comoEntero(l.d.resolver(dict["FunctionType"])); ok {
		f.tipo = int(v)
	}
	f.dominio = l.reales(dict["Domain"])
	f.rango = l.reales(dict["Range"])
	if len(f.dominio) < 2 {
		f.dominio = []float64{0, 1}
	}

	switch f.tipo {
	case 2:
		f.c0 = l.reales(dict["C0"])
		f.c1 = l.reales(dict["C1"])
		if len(f.c0) == 0 {
			f.c0 = []float64{0}
		}
		if len(f.c1) == 0 {
			f.c1 = []float64{1}
		}
		if v, ok := dict["N"]; ok {
			f.exp = comoReal(l.d.resolver(v))
		}
		f.nSalida = len(f.c0)
	case 3:
		if arr, ok := l.d.resolver(dict["Functions"]).(pdfArray); ok {
			for _, e := range arr {
				f.trozos = append(f.trozos, l.funcion(e, prof+1))
			}
		}
		f.limites = l.reales(dict["Bounds"])
		f.codifica = l.reales(dict["Encode"])
		if len(f.trozos) > 0 && f.trozos[0] != nil {
			f.nSalida = f.trozos[0].nSalida
		}
	case 0:
		if st == nil {
			return nil
		}
		datos, err := l.d.datosDeFlujo(st)
		if err != nil {
			return nil
		}
		f.muestras = datos
		for _, v := range l.reales(dict["Size"]) {
			f.tamano = append(f.tamano, int(v))
		}
		f.bps = 8
		if v, ok := comoEntero(l.d.resolver(dict["BitsPerSample"])); ok {
			f.bps = int(v)
		}
		f.codifica = l.reales(dict["Encode"])
		f.decodif = l.reales(dict["Decode"])
		f.nSalida = len(f.rango) / 2
	case 4:
		// Calculadora PostScript. No se interpreta: se aproxima por el rango
		// declarado, que da un degradado con la dirección y los extremos
		// correctos aunque el centro no coincida.
		f.nSalida = len(f.rango) / 2
	}
	if f.nSalida == 0 {
		f.nSalida = 1
	}
	return f
}

func (l *lienzo) reales(o pdfObj) []float64 {
	arr, ok := l.d.resolver(o).(pdfArray)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		out = append(out, comoReal(l.d.resolver(e)))
	}
	return out
}

// evaluar aplica la función a una entrada escalar, que es el caso de los
// degradados y de las tintas planas de un solo componente.
func (f *funcionPDF) evaluar(x float64) []float64 {
	if f == nil {
		return []float64{0}
	}
	d0, d1 := f.dominio[0], f.dominio[1]
	if x < math.Min(d0, d1) {
		x = math.Min(d0, d1)
	}
	if x > math.Max(d0, d1) {
		x = math.Max(d0, d1)
	}

	switch f.tipo {
	case -1: // array de funciones: una salida por cada una
		out := make([]float64, 0, len(f.trozos))
		for _, sub := range f.trozos {
			v := sub.evaluar(x)
			if len(v) > 0 {
				out = append(out, v[0])
			} else {
				out = append(out, 0)
			}
		}
		return out
	case 2:
		t := math.Pow(normalizar(x, d0, d1), f.exp)
		n := len(f.c0)
		if len(f.c1) < n {
			n = len(f.c1)
		}
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			out[i] = f.c0[i] + t*(f.c1[i]-f.c0[i])
		}
		return out
	case 3:
		return f.evaluarCosida(x, d0, d1)
	case 0:
		return f.evaluarMuestreada(x, d0, d1)
	case 4:
		// Interpolación lineal entre los extremos del rango: no es la función,
		// pero es monótona y va del color inicial al final.
		out := make([]float64, f.nSalida)
		t := normalizar(x, d0, d1)
		for i := 0; i < f.nSalida && 2*i+1 < len(f.rango); i++ {
			out[i] = f.rango[2*i] + t*(f.rango[2*i+1]-f.rango[2*i])
		}
		return out
	}
	return []float64{0}
}

// evaluarVarias aplica la función a una entrada de varios componentes. Solo la
// necesitan DeviceN; con una sola entrada delega en `evaluar`.
func (f *funcionPDF) evaluarVarias(c []float64) []float64 {
	if len(c) == 0 {
		return []float64{0}
	}
	return f.evaluar(c[0])
}

func (f *funcionPDF) evaluarCosida(x, d0, d1 float64) []float64 {
	if len(f.trozos) == 0 {
		return []float64{0}
	}
	k := 0
	for k < len(f.limites) && x >= f.limites[k] {
		k++
	}
	if k >= len(f.trozos) {
		k = len(f.trozos) - 1
	}
	bajo := d0
	if k > 0 {
		bajo = f.limites[k-1]
	}
	alto := d1
	if k < len(f.limites) {
		alto = f.limites[k]
	}
	e0, e1 := 0.0, 1.0
	if 2*k+1 < len(f.codifica) {
		e0, e1 = f.codifica[2*k], f.codifica[2*k+1]
	}
	t := e0 + normalizar(x, bajo, alto)*(e1-e0)
	if f.trozos[k] == nil {
		return []float64{0}
	}
	return f.trozos[k].evaluar(t)
}

// evaluarMuestreada lee la tabla de una función de tipo 0 interpolando
// linealmente entre las dos muestras vecinas. Solo se implementa una entrada:
// una función muestreada de dos entradas es un sombreado de malla, y esos no se
// pintan aquí.
func (f *funcionPDF) evaluarMuestreada(x, d0, d1 float64) []float64 {
	if len(f.tamano) == 0 || f.tamano[0] < 1 || f.nSalida == 0 {
		return []float64{0}
	}
	tam := f.tamano[0]
	e0, e1 := 0.0, float64(tam-1)
	if len(f.codifica) >= 2 {
		e0, e1 = f.codifica[0], f.codifica[1]
	}
	pos := e0 + normalizar(x, d0, d1)*(e1-e0)
	pos = math.Max(0, math.Min(float64(tam-1), pos))
	i0 := int(math.Floor(pos))
	i1 := i0 + 1
	if i1 > tam-1 {
		i1 = tam - 1
	}
	frac := pos - float64(i0)

	maxVal := float64(uint64(1)<<uint(f.bps) - 1)
	out := make([]float64, f.nSalida)
	for j := 0; j < f.nSalida; j++ {
		v0 := float64(f.muestra(i0*f.nSalida+j)) / maxVal
		v1 := float64(f.muestra(i1*f.nSalida+j)) / maxVal
		v := v0 + frac*(v1-v0)
		// /Decode, si está, reescala; si no, se usa /Range.
		lo, hi := 0.0, 1.0
		switch {
		case 2*j+1 < len(f.decodif):
			lo, hi = f.decodif[2*j], f.decodif[2*j+1]
		case 2*j+1 < len(f.rango):
			lo, hi = f.rango[2*j], f.rango[2*j+1]
		}
		out[j] = lo + v*(hi-lo)
	}
	return out
}

// muestra lee la muestra número i respetando /BitsPerSample, que puede no ser
// múltiplo de 8 (1, 2, 4, 12 y 24 son legales).
func (f *funcionPDF) muestra(i int) uint64 {
	bitIni := i * f.bps
	var v uint64
	for b := 0; b < f.bps; b++ {
		idx := (bitIni + b) / 8
		if idx >= len(f.muestras) {
			return v << uint(f.bps-b)
		}
		bit := (f.muestras[idx] >> uint(7-(bitIni+b)%8)) & 1
		v = v<<1 | uint64(bit)
	}
	return v
}

func normalizar(x, a, b float64) float64 {
	if math.Abs(b-a) < 1e-12 {
		return 0
	}
	return (x - a) / (b - a)
}

// --- Sombreados --------------------------------------------------------------

// sombrear ejecuta `sh`: pinta el sombreado sobre TODO el área de recorte
// vigente. Como el recorte ya es un <g clip-path>, basta con un rectángulo
// enorme dentro de él.
func (e *ejecucion) sombrear(nombre pdfName) {
	sh := e.l.d.resolver(e.recursoDe("Shading", nombre))

	if e.l.r != nil {
		// En PNG el degradado se resuelve con su color medio sobre el área de
		// recorte. Se ve el bloque de color donde va el sombreado, que es lo que
		// permite leer lo que tiene encima; el degradado fino es del SVG.
		if g := e.l.degradadoDe(sh, e.estado.ctm); g != nil {
			e.l.r.rellenarAreaCon(e.estado.recorte, g.conAlfa(e.estado.alfaRelleno))
		} else if medio := e.l.colorMedioDeSombreado(sh); medio != "" {
			e.l.r.rellenarArea(e.estado.recorte, colorRaster(medio, e.estado.alfaRelleno))
		}
	}

	// Una MALLA no es un servidor de pintura: son triángulos. Se dibujan aquí
	// mismo y el recorte vigente —que ya es un <g clip-path>— los acota, que es
	// exactamente lo que `sh` quiere decir.
	if tris, ok := e.l.mallaDeSombreado(sh); ok {
		e.cerrarTexto()
		e.l.dibujarMalla(tris, e.estado.ctm, &e.l.sb)
		return
	}

	id, ok := e.l.gradiente(sh, e.estado.ctm)
	if !ok {
		return
	}
	// El rectángulo va en coordenadas del SVG ya transformadas (identidad), no
	// en las del usuario: así cubre el recorte entero sea cual sea la CTM.
	e.l.escribir(`<rect x="-100000" y="-100000" width="200000" height="200000" fill="url(#` + id + `)"`)
	if e.estado.alfaRelleno < 1 {
		e.l.escribir(` fill-opacity="` + num(e.estado.alfaRelleno) + `"`)
	}
	e.l.escribir("/>\n")
}

// aplicarPatron resuelve `scn /P1` con espacio /Pattern: un sombreado (tipo 2) o
// un mosaico (tipo 1).
func (e *ejecucion) aplicarPatron(nombre pdfName, trazo bool) {
	obj := e.l.d.resolver(e.recursoDe("Pattern", nombre))
	var dict pdfDict
	var st *pdfStream
	switch v := obj.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict, st = v.Dict, v
	default:
		return
	}

	m := identidad
	if arr, ok := e.l.d.resolver(dict["Matrix"]).(pdfArray); ok && len(arr) >= 6 {
		for i := 0; i < 6; i++ {
			m[i] = comoReal(e.l.d.resolver(arr[i]))
		}
	}
	// La matriz del patrón se aplica sobre el espacio del PDF, NO sobre la CTM
	// del momento: un patrón no se mueve con lo que se pinta. Por eso se compone
	// con la matriz base de la página, que es la que hay en el fondo de la pila.
	base := e.pila[0].ctm
	tipo, _ := comoEntero(e.l.d.resolver(dict["PatternType"]))

	var id string
	var ok bool
	switch tipo {
	case 2:
		sh := e.l.d.resolver(dict["Shading"])
		// Con una malla dentro hace falta envolverla: un relleno necesita un
		// servidor de pintura y las figuras no lo son.
		if tris, hay := e.l.mallaDeSombreado(sh); hay {
			id, ok = e.l.mallaComoPatron(tris, m.por(base))
			break
		}
		id, ok = e.l.gradiente(sh, m.por(base))
	case 1:
		if st != nil {
			id, ok = e.l.mosaico(st, m.por(base))
		}
	}
	if !ok {
		return
	}
	// La salida PNG no tiene servidores de pintura: se queda con el color medio
	// del sombreado, que es visible y del tono correcto. De un mosaico no se
	// deduce color, así que se deja vacío y no se pinta.
	aprox := ""
	var deg *degradadoRaster
	if tipo == 2 && e.l.r != nil {
		aprox = e.l.colorMedioDeSombreado(e.l.d.resolver(dict["Shading"]))
		deg = e.l.degradadoDe(e.l.d.resolver(dict["Shading"]), m.por(base))
	}
	if trazo {
		e.estado.patronTrazo, e.estado.aproxTrazo = id, aprox
	} else {
		e.estado.patronRelleno, e.estado.aproxRelleno, e.estado.degRelleno = id, aprox, deg
	}
}

// colorMedioDeSombreado evalúa la función del sombreado en el centro de su
// dominio. Es la misma degradación que `gradiente` aplica a los tipos que SVG no
// sabe expresar.
func (l *lienzo) colorMedioDeSombreado(sh pdfObj) string {
	var dict pdfDict
	switch v := sh.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict = v.Dict
	default:
		return ""
	}
	fn := l.funcion(dict["Function"], 0)
	if fn == nil {
		return ""
	}
	dom := l.reales(dict["Domain"])
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
	return l.espacio(dict["ColorSpace"], 0).css(fn.evaluar((dom[0] + dom[1]) / 2))
}

// mosaico traduce un patrón de tipo 1 a <pattern>: se renderiza la celda una vez
// y el navegador la repite. Es lo que dibuja los rayados de un plano, que es
// justo donde se mide.
func (l *lienzo) mosaico(st *pdfStream, m matriz) (string, bool) {
	if l.prof > maxProfundidadXObject {
		return "", false
	}
	contenido, err := l.d.datosDeFlujo(st)
	if err != nil {
		return "", false
	}
	paso := func(clave pdfName, alt float64) float64 {
		if v, ok := st.Dict[clave]; ok {
			if x := math.Abs(comoReal(l.d.resolver(v))); x > 1e-6 {
				return x
			}
		}
		return alt
	}
	caja, tiene := l.rectangulo(st.Dict["BBox"])
	if !tiene {
		return "", false
	}
	ancho := paso("XStep", caja[2]-caja[0])
	alto := paso("YStep", caja[3]-caja[1])
	if ancho <= 0 || alto <= 0 || ancho > 1e5 || alto > 1e5 {
		return "", false
	}

	// El contenido de la celda se renderiza a un lienzo aparte para no mezclarlo
	// con el flujo principal; sus defs sí se comparten, que es donde acaban las
	// fuentes y los recortes.
	sub := &lienzo{
		d:          l.d,
		opts:       l.opts,
		fuentes:    l.fuentes,
		caraCSS:    l.caraCSS,
		prof:       l.prof + 1,
		seq:        l.seq,
		prefijoIDs: l.prefijoIDs,
	}
	recursos, _ := l.d.resolver(st.Dict["Resources"]).(pdfDict)
	sub.ejecutar(contenido, recursos, estadoInicial(identidad))
	l.seq = sub.seq
	l.defsEscribir(sub.defs.String())

	id := l.id("mos")
	var b strings.Builder
	b.WriteString(`<pattern id="` + id + `" patternUnits="userSpaceOnUse" `)
	b.WriteString(`x="` + num(caja[0]) + `" y="` + num(caja[1]) + `" `)
	b.WriteString(`width="` + num(ancho) + `" height="` + num(alto) + `" `)
	b.WriteString(`patternTransform="` + m.svg() + `">`)
	b.WriteString(`<g transform="translate(` + num(-caja[0]) + " " + num(-caja[1]) + `)">`)
	b.WriteString(sub.sb.String())
	b.WriteString("</g></pattern>")
	l.defsEscribir(b.String())
	return id, true
}

// gradiente traduce un sombreado a <linearGradient> o <radialGradient>. Los
// tipos de malla (4-7) no tienen equivalente en SVG y se resuelven con su color
// medio, que es mejor que un hueco.
func (l *lienzo) gradiente(sh pdfObj, m matriz) (string, bool) {
	var dict pdfDict
	switch v := sh.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict = v.Dict
	default:
		return "", false
	}

	tipo, _ := comoEntero(l.d.resolver(dict["ShadingType"]))
	ec := l.espacio(dict["ColorSpace"], 0)
	fn := l.funcion(dict["Function"], 0)
	coords := l.reales(dict["Coords"])
	dom := l.reales(dict["Domain"])
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
	// /Extend no se lee: SVG solo tiene `spreadMethod`, que no distingue los dos
	// extremos, y `pad` es lo que hace un visor de PDF con y sin extensión —el
	// color de la última parada sigue hasta el borde del área de recorte—.

	paradas := l.paradas(fn, ec, dom)
	id := l.id("grad")
	var b strings.Builder

	switch tipo {
	case 2:
		if len(coords) < 4 {
			return "", false
		}
		b.WriteString(`<linearGradient id="` + id + `" gradientUnits="userSpaceOnUse" `)
		b.WriteString(`x1="` + num(coords[0]) + `" y1="` + num(coords[1]) + `" `)
		b.WriteString(`x2="` + num(coords[2]) + `" y2="` + num(coords[3]) + `" `)
		b.WriteString(`gradientTransform="` + m.svg() + `" `)
		b.WriteString(`spreadMethod="pad">`)
		b.WriteString(paradas)
		b.WriteString("</linearGradient>")
	case 3:
		if len(coords) < 6 {
			return "", false
		}
		// SVG solo tiene el círculo exterior y un foco dentro; el interior de PDF
		// se traduce a `fr`, que los navegadores actuales sí respetan.
		b.WriteString(`<radialGradient id="` + id + `" gradientUnits="userSpaceOnUse" `)
		b.WriteString(`fx="` + num(coords[0]) + `" fy="` + num(coords[1]) + `" fr="` + num(coords[2]) + `" `)
		b.WriteString(`cx="` + num(coords[3]) + `" cy="` + num(coords[4]) + `" r="` + num(coords[5]) + `" `)
		b.WriteString(`gradientTransform="` + m.svg() + `" `)
		b.WriteString(`spreadMethod="pad">`)
		b.WriteString(paradas)
		b.WriteString("</radialGradient>")
	default:
		// Un tipo de sombreado que no se reconoce. Las mallas (4-7) NO llegan
		// aquí: las atiende `mallaDeSombreado` antes, en pdfmesh.go.
		medio := ec.css(fn.evaluar((dom[0] + dom[1]) / 2))
		b.WriteString(`<linearGradient id="` + id + `"><stop offset="0" stop-color="` + medio +
			`"/><stop offset="1" stop-color="` + medio + `"/></linearGradient>`)
	}
	l.defsEscribir(b.String())
	return id, true
}

// paradasDeGradiente es cuántas veces se muestrea la función. 32 puntos bastan
// para que un degradado no se vea a bandas y no inflan el SVG.
const paradasDeGradiente = 32

func (l *lienzo) paradas(fn *funcionPDF, ec *espacioColor, dom []float64) string {
	var b strings.Builder
	anterior := ""
	for i := 0; i <= paradasDeGradiente; i++ {
		t := float64(i) / paradasDeGradiente
		css := ec.css(fn.evaluar(dom[0] + t*(dom[1]-dom[0])))
		// Paradas idénticas seguidas no aportan nada; se salta la del medio.
		if css == anterior && i != paradasDeGradiente {
			continue
		}
		anterior = css
		b.WriteString(`<stop offset="` + strconv.FormatFloat(t, 'f', 4, 64) +
			`" stop-color="` + css + `"/>`)
	}
	return b.String()
}
