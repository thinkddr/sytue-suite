// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"strings"
)

// Ingeniería (functions/engineering.ts): conversión de base con complemento a dos del
// ancho canónico de Excel (binario 10 bits, octal 30, hexadecimal 40), operaciones de
// bits, CONVERT, números complejos y funciones de Bessel.

func intArg(args []*Arg, i int) (float64, Error) {
	n, err := numArg(args, i)
	return math.Trunc(n), err
}

// baseOrigen es [bits, base, máximo de dígitos, validador]; baseDestino, [bits, base, mín, máx].
type baseOrigen struct {
	bits, base float64
	maxLen     int
	valido     *regexp.Regexp
}

type baseDestino struct{ bits, base, min, max float64 }

var (
	binSrc = baseOrigen{10, 2, 10, regexp.MustCompile(`^[01]+$`)}
	octSrc = baseOrigen{30, 8, 10, regexp.MustCompile(`^[0-7]+$`)}
	hexSrc = baseOrigen{40, 16, 10, regexp.MustCompile(`^[0-9A-F]+$`)}
	binDst = baseDestino{10, 2, -512, 511}
	octDst = baseDestino{30, 8, -(1 << 29), 1<<29 - 1}
	hexDst = baseDestino{40, 16, -(1 << 39), 1<<39 - 1}
)

// formatBase es el núcleo DEC2X: entero → texto, en complemento a dos si es negativo,
// con relleno opcional `places` (solo para no negativos).
func formatBase(n float64, d baseDestino, places float64, hayPlaces bool) Value {
	if n < d.min || n > d.max || math.IsNaN(n) {
		return ErrNum
	}
	if n < 0 {
		return enBase(n+math.Pow(2, d.bits), int(d.base)) // ancho completo: `places` se ignora
	}
	s := enBase(n, int(d.base))
	if hayPlaces {
		if places < 0 || places > d.bits || places < float64(len(s)) {
			return ErrNum // no cabe en `places` dígitos
		}
		s = padStart(s, int(places))
	}
	return s
}

// parseBase es el núcleo X2DEC: texto → entero con signo por complemento a dos.
func parseBase(t string, o baseOrigen) (float64, Error) {
	s := jsUpper(jsTrim(t))
	if s == "" || len(s) > o.maxLen || !o.valido.MatchString(s) {
		return 0, ErrNum
	}
	valor := 0.0
	for _, ch := range s {
		valor = valor*o.base + float64(strings.IndexRune(digitosBase, ch))
	}
	// El bit de signo solo se alcanza con el ancho completo: `>= mitad` basta.
	if valor >= math.Pow(2, o.bits-1) {
		valor -= math.Pow(2, o.bits)
	}
	return valor, ""
}

// placesArg lee el `places` opcional (segundo argumento).
func placesArg(args []*Arg) (float64, bool, Error) {
	if len(args) < 2 {
		return 0, false, ""
	}
	p, err := intArg(args, 1)
	return p, true, err
}

func decToBase(d baseDestino) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		n, err := intArg(args, 0)
		if err != "" {
			return err
		}
		places, hay, err := placesArg(args)
		if err != "" {
			return err
		}
		return formatBase(n, d, places, hay)
	}
}

func baseToDec(o baseOrigen) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		return valorOError(parseBase(t, o))
	}
}

// convBase es X2Y: compone parseBase (origen) y formatBase (destino).
func convBase(o baseOrigen, d baseDestino) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		dec, err := parseBase(t, o)
		if err != "" {
			return err
		}
		places, hay, err := placesArg(args)
		if err != "" {
			return err
		}
		return formatBase(dec, d, places, hay)
	}
}

const max48 = 1<<48 - 1

// bitArg valida un operando de bits: entero en [0, 2^48-1].
func bitArg(args []*Arg, i int) (uint64, Error) {
	n, err := intArg(args, i)
	if err != "" {
		return 0, err
	}
	if n < 0 || n > max48 || math.IsNaN(n) {
		return 0, ErrNum
	}
	return uint64(n), ""
}

func bitwise(op func(a, b uint64) uint64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		a, err := bitArg(args, 0)
		if err != "" {
			return err
		}
		b, err := bitArg(args, 1)
		if err != "" {
			return err
		}
		return float64(op(a, b))
	}
}

// desplazar es BITLSHIFT (signo +1) y BITRSHIFT (signo -1): el desplazamiento negativo va
// en el sentido contrario, como Excel.
func desplazar(sentido float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		n, err := intArg(args, 0)
		if err != "" {
			return err
		}
		if n < 0 || n > max48 || math.IsNaN(n) {
			return ErrNum
		}
		sh, err := intArg(args, 1)
		if err != "" {
			return err
		}
		if math.Abs(sh) > 53 || math.IsNaN(sh) {
			return ErrNum
		}
		if sh *= sentido; sh >= 0 {
			return n * math.Pow(2, sh) // BigInt(n) << sh, convertido a Number: exacto hasta 2^101
		}
		return math.Floor(n / math.Pow(2, -sh))
	}
}

// escalon es DELTA (1 si n1 = n2) y GESTEP (1 si n ≥ escalón), con el segundo a 0.
func escalon(cumple func(a, b float64) bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		a, err := numArg(args, 0)
		if err != "" {
			return err
		}
		b, err := numOpc(args, 1, 0)
		if err != "" {
			return err
		}
		return b2f(cumple(a, b))
	}
}

// unError es ERF.PRECISE, ERFC y ERFC.PRECISE: aridad exacta 1.
func unError(f func(x float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := arg1(args)
		if err != "" {
			return err
		}
		return f(x)
	}
}

/* ---------- CONVERT ---------- */
// ponytail: subconjunto útil (masa, longitud, tiempo y temperatura) con prefijos SI.
// Amplía las tablas si se piden volumen, energía, presión o velocidad.

type magnitud struct {
	cat    string
	factor float64
}

// unidades: unidad → categoría y factor a la base de su categoría (g, m, s).
var unidades = map[string]magnitud{
	"g": {"mass", 1}, "u": {"mass", 1.66053886e-24}, "grain": {"mass", 0.06479891}, "ozm": {"mass", 28.349523125},
	"lbm": {"mass", 453.59237}, "stone": {"mass", 6350.29318}, "sg": {"mass", 14593.9029372064},
	"ton": {"mass", 907184.74}, // tonelada corta (EE. UU.)
	"m":   {"dist", 1}, "mi": {"dist", 1609.344}, "Nmi": {"dist", 1852}, "in": {"dist", 0.0254}, "ft": {"dist", 0.3048},
	"yd": {"dist", 0.9144}, "ang": {"dist", 1e-10}, "ly": {"dist", 9.4607304725808e15}, "pica": {"dist", 0.0254 / 6},
	"sec": {"time", 1}, "s": {"time", 1}, "min": {"time", 60}, "mn": {"time", 60}, "hr": {"time", 3600},
	"day": {"time", 86400}, "d": {"time", 86400}, "yr": {"time", 31557600},
}

var prefijable = map[string]bool{"g": true, "m": true, "sec": true, "s": true}

// prefijos SI, en el ORDEN del original: gana el primero que encaja.
var prefijos = []struct {
	p string
	f float64
}{
	{"Y", 1e24},
	{"Z", 1e21},
	{"E", 1e18},
	{"P", 1e15},
	{"T", 1e12},
	{"G", 1e9},
	{"M", 1e6},
	{"k", 1e3},
	{"h", 1e2},
	{"da", 1e1},
	{"d", 1e-1},
	{"c", 1e-2},
	{"m", 1e-3},
	{"u", 1e-6},
	{"µ", 1e-6},
	{"n", 1e-9},
	{"p", 1e-12},
	{"f", 1e-15},
	{"a", 1e-18},
	{"z", 1e-21},
	{"y", 1e-24},
}

func resolveUnit(u string) (magnitud, bool) {
	if d, ok := unidades[u]; ok {
		return d, true
	}
	for _, p := range prefijos {
		if resto, ok := strings.CutPrefix(u, p.p); ok && resto != "" && prefijable[resto] {
			return magnitud{unidades[resto].cat, unidades[resto].factor * p.f}, true
		}
	}
	return magnitud{}, false
}

// toCelsius lleva una temperatura a grados Celsius; falso si la magnitud no es de temperatura.
func toCelsius(n float64, u string) (float64, bool) {
	switch u {
	case "C", "cel":
		return n, true
	case "F", "fah":
		return ((n - 32) * 5) / 9, true
	case "K", "kel":
		return n - 273.15, true
	}
	return 0, false
}

func fromCelsius(c float64, u string) (float64, bool) {
	switch u {
	case "C", "cel":
		return c, true
	case "F", "fah":
		return (c*9)/5 + 32, true
	case "K", "kel":
		return c + 273.15, true
	}
	return 0, false
}

// convert es CONVERT(número, de, a): #N/A si una magnitud es desconocida o las categorías
// no coinciden.
func convert(_ *FnCtx, args []*Arg) any {
	if len(args) != 3 {
		return ErrValue
	}
	n, err := numArg(args, 0)
	if err != "" {
		return err
	}
	de, err := CoerceText(args[1].Value())
	if err != "" {
		return err
	}
	a, err := CoerceText(args[2].Value())
	if err != "" {
		return err
	}
	if c, ok := toCelsius(n, de); ok {
		if r, ok := fromCelsius(c, a); ok {
			return r
		}
		return ErrNA
	}
	if _, ok := toCelsius(0, a); ok {
		return ErrNA // destino de temperatura y origen no
	}
	uf, okF := resolveUnit(de)
	ut, okT := resolveUnit(a)
	if !okF || !okT || uf.cat != ut.cat {
		return ErrNA
	}
	return (n * uf.factor) / ut.factor
}

/* ---------- Números complejos ----------
 * En una hoja un complejo NO es un tipo: es TEXTO «a+bi». Las IM* reciben y devuelven
 * ese texto, así que el parser y el formateador son la función de verdad.
 *
 * Las sumas de productos van con float64(...) explícito: impide que el compilador las
 * funda en una FMA (lo hace en arm64), que cambia el último bit, y aquí el último bit
 * se VE: el resultado es un texto con todas sus cifras.
 */

type complejo struct {
	re, im float64
	suf    string // «i» o «j»: se conserva el que traía
}

// cortePartes localiza el signo que separa la parte real de la imaginaria: de DERECHA a
// IZQUIERDA, saltando el signo de un exponente («1.5E+3+2i») y la posición 0.
func cortePartes(t string) int {
	for i := len(t) - 1; i > 0; i-- {
		if (t[i] == '+' || t[i] == '-') && t[i-1] != 'e' && t[i-1] != 'E' {
			return i
		}
	}
	return -1
}

// coefImaginario: «» y «+» valen 1, «-» vale −1.
func coefImaginario(t string) (float64, bool) {
	switch t {
	case "", "+":
		return 1, true
	case "-":
		return -1, true
	}
	n := jsNumberDe(t)
	return n, esFinito(n)
}

// aComplejo convierte un valor de celda: texto «a+bi» o un número. Un #error se PROPAGA
// tal cual, no se traduce a otro.
func aComplejo(v Value) (complejo, Error) {
	switch x := v.(type) {
	case Error:
		return complejo{}, x
	case float64:
		return complejo{x, 0, "i"}, ""
	case bool:
		return complejo{}, ErrValue
	}
	s, _ := v.(string)
	t := jsTrim(s)
	if t == "" {
		return complejo{0, 0, "i"}, ""
	}
	suf := t[len(t)-1:]
	if suf != "i" && suf != "j" {
		if n := jsNumberDe(t); esFinito(n) {
			return complejo{n, 0, "i"}, ""
		}
		return complejo{}, ErrNum
	}
	cuerpo := t[:len(t)-1]
	corte := cortePartes(cuerpo)
	if corte < 0 { // solo parte imaginaria: «4i», «i», «-i»
		im, ok := coefImaginario(cuerpo)
		if !ok {
			return complejo{}, ErrNum
		}
		return complejo{0, im, suf}, ""
	}
	re := jsNumberDe(cuerpo[:corte])
	im, ok := coefImaginario(cuerpo[corte:])
	if !esFinito(re) || !ok {
		return complejo{}, ErrNum
	}
	return complejo{re, im, suf}, ""
}

// deComplejo es el texto canónico, con las omisiones de una hoja real: la parte nula
// desaparece y el coeficiente 1 no se escribe («3+i», no «3+1i»).
func deComplejo(c complejo) string {
	if c.im == 0 {
		return jsNumero(c.re)
	}
	coef := jsNumero(c.im)
	switch c.im {
	case 1:
		coef = ""
	case -1:
		coef = "-"
	}
	if c.re == 0 {
		return coef + c.suf
	}
	signo := "+"
	if c.im < 0 {
		signo = "" // el negativo ya trae su signo en `coef`
	}
	return jsNumero(c.re) + signo + coef + c.suf
}

func cplx(args []*Arg, i int) (complejo, Error) {
	if i >= len(args) {
		return complejo{}, ErrValue
	}
	return aComplejo(args[i].Value())
}

func textoComplejo(c complejo, err Error) Value {
	switch {
	case err != "":
		return err
	case !esFinito(c.re) || !esFinito(c.im):
		return ErrNum
	}
	return deComplejo(c)
}

// map1c aplica a un complejo una función que devuelve otro. Lee el argumento ANTES de
// mirar la aridad, como el original: el error del argumento gana al de aridad.
func map1c(f func(c complejo) (complejo, Error)) fn {
	return func(_ *FnCtx, args []*Arg) any {
		c, err := cplx(args, 0)
		if err != "" {
			return err
		}
		if len(args) != 1 {
			return ErrValue
		}
		return textoComplejo(f(c))
	}
}

func sinError(f func(c complejo) complejo) func(complejo) (complejo, Error) {
	return func(c complejo) (complejo, Error) { return f(c), "" }
}

// parteDe es IMREAL, IMAGINARY, IMABS e IMARGUMENT: un complejo → un número.
func parteDe(f func(c complejo) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		c, err := cplx(args, 0)
		if err != "" {
			return err
		}
		return f(c)
	}
}

// escalaLog es el logaritmo en otra base: el natural dividido por ln(base), parte a parte.
func escalaLog(ln float64) func(complejo) (complejo, Error) {
	return func(c complejo) (complejo, Error) {
		if c.re == 0 && c.im == 0 {
			return c, ErrNum
		}
		return complejo{math.Log(math.Hypot(c.re, c.im)) / ln, math.Atan2(c.im, c.re) / ln, c.suf}, ""
	}
}

func mulc(a, b complejo) complejo {
	return complejo{float64(a.re*b.re) - float64(a.im*b.im), float64(a.re*b.im) + float64(a.im*b.re), a.suf}
}

// potenciaEntera va por cuadrados sucesivos: exacta y en log₂(n) productos. El negativo
// se resuelve invirtiendo al final.
func potenciaEntera(c complejo, n float64) complejo {
	base, acc := c, complejo{1, 0, c.suf}
	for k := math.Abs(n); k > 0; k = math.Floor(k / 2) {
		if math.Mod(k, 2) == 1 {
			acc = mulc(acc, base)
		}
		base = mulc(base, base)
	}
	if n >= 0 {
		return acc
	}
	d := float64(acc.re*acc.re) + float64(acc.im*acc.im)
	return complejo{acc.re / d, -acc.im / d, c.suf}
}

// potenciaPolar es r^n · e^(inθ), rama principal: el único camino con exponente fraccionario.
func potenciaPolar(c complejo, n float64) complejo {
	r := jsPow(math.Hypot(c.re, c.im), n)
	t := math.Atan2(c.im, c.re) * n
	return complejo{r * math.Cos(t), r * math.Sin(t), c.suf}
}

// invc es 1/z. El origen no tiene inverso → #NUM!.
func invc(c complejo) (complejo, Error) {
	d := float64(c.re*c.re) + float64(c.im*c.im)
	if d == 0 {
		return c, ErrNum
	}
	return complejo{c.re / d, -c.im / d, c.suf}, ""
}

func divc(a, b complejo) (complejo, Error) {
	inv, err := invc(b)
	if err != "" {
		return a, err
	}
	return mulc(a, inv), ""
}

// sen(a+bi) = sen(a)·cosh(b) + i·cos(a)·senh(b).
func senoc(c complejo) complejo {
	return complejo{math.Sin(c.re) * math.Cosh(c.im), math.Cos(c.re) * math.Sinh(c.im), c.suf}
}

// cos(a+bi) = cos(a)·cosh(b) − i·sen(a)·senh(b): el signo NO es simétrico al del seno.
func cosenoc(c complejo) complejo {
	return complejo{math.Cos(c.re) * math.Cosh(c.im), -math.Sin(c.re) * math.Sinh(c.im), c.suf}
}

// senh(a+bi) = senh(a)·cos(b) + i·cosh(a)·sen(b): aquí lo REAL va en las hiperbólicas.
func senohc(c complejo) complejo {
	return complejo{math.Sinh(c.re) * math.Cos(c.im), math.Cosh(c.re) * math.Sin(c.im), c.suf}
}

// cosh(a+bi) = cosh(a)·cos(b) + i·senh(a)·sen(b).
func cosenohc(c complejo) complejo {
	return complejo{math.Cosh(c.re) * math.Cos(c.im), math.Sinh(c.re) * math.Sin(c.im), c.suf}
}

// acumular es IMSUM e IMPRODUCT: N complejos con el MISMO sufijo (mezclar «i» y «j» es
// #VALUE!: el resultado no puede llevar los dos).
func acumular(neutro float64, op func(acc, c complejo) complejo) fn {
	return func(_ *FnCtx, args []*Arg) any {
		acc := complejo{neutro, 0, "i"}
		for i := range args {
			c, err := cplx(args, i)
			if err != "" {
				return err
			}
			if i == 0 {
				acc.suf = c.suf
			} else if acc.suf != c.suf {
				return ErrValue
			}
			suf := acc.suf
			acc = op(acc, c)
			acc.suf = suf
		}
		return deComplejo(acc)
	}
}

// dosComplejos lee los dos operandos de IMSUB e IMDIV y exige sufijo común.
func dosComplejos(f func(a, b complejo) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		a, err := cplx(args, 0)
		if err != "" {
			return err
		}
		b, err := cplx(args, 1)
		if err != "" {
			return err
		}
		if a.suf != b.suf {
			return ErrValue
		}
		return f(a, b)
	}
}

/* ---------- Funciones de Bessel ----------
 * Ninguna serie sola cubre todo el dominio: cada una usa el método exacto en su tramo.
 * El original las contrasta con los wronskianos JₙYₙ₊₁ − Jₙ₊₁Yₙ = −2/(πx) e
 * IₙKₙ₊₁ + Iₙ₊₁Kₙ = 1/x, identidades exactas que no dependen de ninguna tabla.
 */

// besselTermino0 es (x/2)ⁿ/n! como producto acumulado: el cociente de las dos cantidades
// desbordaría aunque el resultado quepa.
func besselTermino0(n, x float64) float64 {
	t := 1.0
	for i := 1.0; i <= n && t != 0 && !math.IsInf(t, 0); i++ { // en 0 o en ∞ ya no se mueve
		t *= x / 2 / i
	}
	return t
}

// besselSerie es Σ (∓1)ᵏ (x/2)^(n+2k) / (k!·(n+k)!), alterna para J y positiva para I.
// En J, a partir de |x| ≈ 15 el término mayor supera al resultado en más de 10⁵ y se
// cambia a cuadratura.
func besselSerie(n, x float64, alterna bool) float64 {
	q := (x * x) / 4
	term := besselTermino0(n, x)
	suma := term
	for k := 1.0; k <= 4000; k++ {
		if alterna {
			term = -term
		}
		term = float64(term*q) / (k * (n + k))
		suma += term
		if math.Abs(term) <= 1e-18*math.Abs(suma) {
			break
		}
	}
	return suma
}

// besselJCuadratura es Jₙ(x) = (1/π)∫₀^π cos(nθ − x·senθ)dθ por trapecios: sobre un
// periodo completo converge EXPONENCIALMENTE.
// ponytail (del original): suelo de ruido ABSOLUTO ~1e−16; si el orden es tan alto que
// Jₙ(x) cae por debajo (n ≳ 1,5·x con |x| > 15) sale ruido. Ahí haría falta Miller.
//
// ponytail: el coste es O(|x|) y el original no lo acota: BESSELJ(1E+15; 1) cuelga la
// pestaña. Aquí, por encima de 1e6 es #NUM! (NaN); relevar por el asintótico de Hankel.
func besselJCuadratura(n, x float64) float64 {
	if math.Abs(x) > 1e6 {
		return math.NaN()
	}
	nodos := 64 + 4*math.Ceil(math.Abs(x))
	s := 0.0
	for j := 0.0; j < nodos; j++ {
		t := (2 * math.Pi * j) / nodos
		s += math.Cos(float64(n*t) - float64(x*math.Sin(t)))
	}
	return s / nodos
}

func besselJ(n, x float64) float64 {
	if math.Abs(x) <= 15 {
		return besselSerie(n, x, true)
	}
	return besselJCuadratura(n, x)
}

// gammaEuler es la constante de Euler-Mascheroni al double más cercano.
const gammaEuler = 0.5772156649015329

// digamma es ψ(m) para m entero ≥ 1: −γ + Σ 1/i.
func digamma(m float64) float64 {
	s := -gammaEuler
	for i := 1.0; i < m; i++ {
		s += 1 / i
	}
	return s
}

// besselYSerie es Y de orden 0 o 1 por su desarrollo en serie (A&S 9.1.11).
func besselYSerie(n, x float64) float64 {
	q := (x * x) / 4
	polo := 0.0
	if n != 0 {
		polo = -2 / (math.Pi * x)
	}
	logaritmico := (2 / math.Pi) * math.Log(x/2) * besselJ(n, x)
	term, suma := besselTermino0(n, x), 0.0
	for k := 0.0; k <= 4000; k++ {
		add := (digamma(k+1) + digamma(n+k+1)) * term
		suma += add
		term = float64(-term*q) / ((k + 1) * (n + k + 1))
		if k > 3 && math.Abs(add) <= 1e-18*math.Abs(suma) {
			break
		}
	}
	return polo + logaritmico - suma/math.Pi
}

// hankelPQ son los módulos P y Q del desarrollo asintótico de Hankel. La serie es
// DIVERGENTE: se corta en el término más pequeño, donde da su mejor precisión.
func hankelPQ(n, x float64) (float64, float64) {
	mu := 4 * n * n
	p, q, term, menor := 1.0, 0.0, 1.0, math.Inf(1)
	for k := 1.0; k <= 80; k++ {
		impar := 2*k - 1
		term = float64(term*(mu-impar*impar)) / (k * 8 * x)
		if math.Abs(term) > menor {
			break
		}
		menor = math.Abs(term)
		mitad, destino := k/2, &p
		if math.Mod(k, 2) != 0 {
			mitad, destino = (k-1)/2, &q
		}
		if math.Mod(mitad, 2) == 0 {
			*destino += term
		} else {
			*destino -= term
		}
	}
	return p, q
}

func besselYAsintotico(n, x float64) float64 {
	p, q := hankelPQ(n, x)
	chi := x - (n/2+0.25)*math.Pi
	return math.Sqrt(2/(math.Pi*x)) * (float64(p*math.Sin(chi)) + float64(q*math.Cos(chi)))
}

// besselY: Y₀ e Y₁ por serie cerca del origen y por Hankel lejos (corte en x = 13), y de
// ahí se sube con Yₙ₊₁ = (2n/x)Yₙ − Yₙ₋₁, ESTABLE hacia arriba porque Y crece con el orden.
func besselY(n, x float64) float64 {
	y := besselYAsintotico
	if x <= 13 {
		y = besselYSerie
	}
	anterior := y(0, x)
	if n == 0 {
		return anterior
	}
	actual := y(1, x)
	for k := 1.0; k < n && esFinito(actual); k++ { // desbordada ya no vuelve
		anterior, actual = actual, float64(((2*k)/x)*actual)-anterior
	}
	return actual
}

// besselK es Kₙ(x) = ∫₀^∞ e^(−x·cosh t)·cosh(nt)dt por trapecios (h = 0,1). Los cosenos
// hiperbólicos se separan en exponenciales: cosh(nt) desbordaría por sí solo.
func besselK(n, x float64) float64 {
	const h = 0.1
	s := math.Exp(-x) / 2
	for j := 1.0; j <= 20_000; j++ {
		t := j * h
		a := x * math.Cosh(t)
		if !(a <= 745) {
			break // e^(−745) ya es cero en un double
		}
		s += (math.Exp(float64(n*t)-a) + math.Exp(float64(-n*t)-a)) / 2
	}
	return s * h
}

// bessel envuelve una Bessel: orden truncado y no negativo (Excel da #NUM!, no la
// identidad J₋ₙ = (−1)ⁿJₙ), x > 0 si diverge en el origen, y desbordamiento → #NUM!.
func bessel(soloPositivos bool, f func(n, x float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		x, orden := v[0], math.Trunc(v[1])
		if orden < 0 || (soloPositivos && !(x > 0)) {
			return ErrNum
		}
		if orden > 1e6 {
			return ErrNum // ponytail: la recurrencia de Y cuesta O(orden) y el original no la acota
		}
		return finito(f(orden, x))
	}
}

// funcionesIngenieria es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesIngenieria = map[string]fn{
	"DEC2BIN": decToBase(binDst), "DEC2OCT": decToBase(octDst), "DEC2HEX": decToBase(hexDst),
	"BIN2DEC": baseToDec(binSrc), "OCT2DEC": baseToDec(octSrc), "HEX2DEC": baseToDec(hexSrc),
	"BIN2OCT": convBase(binSrc, octDst), "BIN2HEX": convBase(binSrc, hexDst),
	"OCT2BIN": convBase(octSrc, binDst), "OCT2HEX": convBase(octSrc, hexDst),
	"HEX2BIN": convBase(hexSrc, binDst), "HEX2OCT": convBase(hexSrc, octDst),
	"BITAND":    bitwise(func(a, b uint64) uint64 { return a & b }),
	"BITOR":     bitwise(func(a, b uint64) uint64 { return a | b }),
	"BITXOR":    bitwise(func(a, b uint64) uint64 { return a ^ b }),
	"BITLSHIFT": desplazar(1),
	"BITRSHIFT": desplazar(-1),
	"DELTA":     escalon(func(a, b float64) bool { return a == b }),
	"GESTEP":    escalon(func(a, b float64) bool { return a >= b }),
	// ERF(inf, [sup]): con dos argumentos, erf(sup) − erf(inf).
	"ERF": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		lo, err := numArg(args, 0)
		if err != "" {
			return err
		}
		if len(args) == 1 {
			return erf(lo)
		}
		hi, err := numArg(args, 1)
		if err != "" {
			return err
		}
		return erf(hi) - erf(lo)
	},
	"ERF.PRECISE":  unError(erf),
	"ERFC":         unError(erfc),
	"ERFC.PRECISE": unError(erfc),
	"CONVERT":      convert,

	// COMPLEX(re, im, [sufijo]): «I» mayúscula tampoco vale.
	"COMPLEX": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		suf := "i"
		if len(args) == 3 {
			if suf, err = CoerceText(args[2].Value()); err != "" {
				return err
			}
			if suf != "i" && suf != "j" {
				return ErrValue
			}
		}
		return deComplejo(complejo{v[0], v[1], suf})
	},
	"IMREAL":    parteDe(func(c complejo) Value { return c.re }),
	"IMAGINARY": parteDe(func(c complejo) Value { return c.im }),
	"IMABS":     parteDe(func(c complejo) Value { return math.Hypot(c.re, c.im) }),
	// Rango (−π, π]; el origen no tiene argumento → #DIV/0!.
	"IMARGUMENT": parteDe(func(c complejo) Value {
		if c.re == 0 && c.im == 0 {
			return ErrDiv0
		}
		return math.Atan2(c.im, c.re)
	}),
	"IMCONJUGATE": map1c(sinError(func(c complejo) complejo { return complejo{c.re, -c.im, c.suf} })),
	"IMSUM":       acumular(0, func(a, c complejo) complejo { return complejo{a.re + c.re, a.im + c.im, a.suf} }),
	"IMPRODUCT":   acumular(1, mulc),
	"IMSUB": dosComplejos(func(a, b complejo) Value {
		return deComplejo(complejo{a.re - b.re, a.im - b.im, a.suf})
	}),
	// Denominador cero → #NUM! (el criterio de Excel para las IM*).
	"IMDIV": dosComplejos(func(a, b complejo) Value {
		d := float64(b.re*b.re) + float64(b.im*b.im)
		if d == 0 {
			return ErrNum
		}
		return deComplejo(complejo{
			(float64(a.re*b.re) + float64(a.im*b.im)) / d,
			(float64(a.im*b.re) - float64(a.re*b.im)) / d, a.suf,
		})
	}),
	// Raíz principal por la forma polar: √r · e^(iθ/2).
	"IMSQRT": map1c(sinError(func(c complejo) complejo {
		r, t := math.Sqrt(math.Hypot(c.re, c.im)), math.Atan2(c.im, c.re)/2
		return complejo{r * math.Cos(t), r * math.Sin(t), c.suf}
	})),
	// Con exponente ENTERO se multiplica: (1+i)² vale 2i exacto, y por la polar sale
	// «1.2246e-16+2.0000000000000004i», basura que el usuario ve y arrastra.
	"IMPOWER": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		c, err := cplx(args, 0)
		if err != "" {
			return err
		}
		n, err := numArg(args, 1)
		switch {
		case err != "":
			return err
		case c.re == 0 && c.im == 0 && n == 0:
			return ErrNum
		case c.re == 0 && c.im == 0:
			return deComplejo(complejo{0, 0, c.suf})
		case esEntero(n):
			return textoComplejo(potenciaEntera(c, n), "")
		}
		return textoComplejo(potenciaPolar(c, n), "")
	},
	"IMEXP": map1c(sinError(func(c complejo) complejo {
		e := math.Exp(c.re)
		return complejo{e * math.Cos(c.im), e * math.Sin(c.im), c.suf}
	})),
	"IMLN":    map1c(escalaLog(1)),
	"IMLOG10": map1c(escalaLog(math.Ln10)),
	"IMLOG2":  map1c(escalaLog(math.Ln2)),
	"IMSIN":   map1c(sinError(senoc)),
	"IMCOS":   map1c(sinError(cosenoc)),
	// Los ceros del denominador son polos: #NUM!, no un número enorme.
	"IMTAN":  map1c(func(c complejo) (complejo, Error) { return divc(senoc(c), cosenoc(c)) }),
	"IMCOT":  map1c(func(c complejo) (complejo, Error) { return divc(cosenoc(c), senoc(c)) }),
	"IMSEC":  map1c(func(c complejo) (complejo, Error) { return invc(cosenoc(c)) }),
	"IMCSC":  map1c(func(c complejo) (complejo, Error) { return invc(senoc(c)) }),
	"IMSINH": map1c(sinError(senohc)),
	"IMCOSH": map1c(sinError(cosenohc)),
	"IMSECH": map1c(func(c complejo) (complejo, Error) { return invc(cosenohc(c)) }),
	"IMCSCH": map1c(func(c complejo) (complejo, Error) { return invc(senohc(c)) }),

	"BESSELJ": bessel(false, besselJ),
	"BESSELY": bessel(true, besselY), // diverge en el origen: x ≤ 0 es #NUM!
	"BESSELI": bessel(false, func(n, x float64) float64 { return besselSerie(n, x, false) }),
	"BESSELK": bessel(true, besselK),
}

func init() { registrar(funcionesIngenieria) }
