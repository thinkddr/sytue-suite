// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Funciones de TEXTO (functions/text.ts). Las cadenas se miden en unidades UTF-16, como
// hacen LEN, LEFT y MID en una hoja real (ver js.go).

func textArg(args []*Arg, i int) (string, Error) { return CoerceText(args[i].Value()) }

// textos coacciona a texto los n primeros argumentos, en orden.
func textos(args []*Arg, n int) ([]string, Error) {
	out := make([]string, n)
	for i := range out {
		t, err := textArg(args, i)
		if err != "" {
			return nil, err
		}
		out[i] = t
	}
	return out, ""
}

// intOpc es un argumento entero (truncado) opcional.
func intOpc(args []*Arg, i int, def float64) (float64, Error) {
	n, err := numOpc(args, i, def)
	return math.Trunc(n), err
}

/* ---------- Expresiones regulares (REGEXTEST, REGEXEXTRACT, REGEXREPLACE) ----------
 * ponytail: el original usa el RegExp de JavaScript y aquí es RE2 (regexp de Go). Lo que
 * RE2 no tiene —referencias hacia atrás y lookaround— da #VALUE! en vez de evaluarse, y
 * en un reemplazo global RE2 no cuenta la coincidencia VACÍA pegada a otra («X*» sobre
 * «aXb» da «-a-b-» y en JavaScript «-a--b-»). A cambio no hay backtracking catastrófico.
 */

const regexMaxPattern = 1000

var reEscapeU = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

func buildRegex(patron string, sinCaja bool) (*regexp.Regexp, Error) {
	if jsLen(patron) > regexMaxPattern {
		return nil, ErrValue
	}
	patron = reEscapeU.ReplaceAllString(patron, `\x{$1}`)
	if sinCaja {
		patron = "(?i)" + patron
	}
	re, err := regexp.Compile(patron)
	if err != nil {
		return nil, ErrValue
	}
	return re, ""
}

// caseInsensitive lee `case_sensitivity` (1 = sin distinguir mayúsculas); FALSE si falta.
func caseInsensitive(args []*Arg, i int) (bool, Error) {
	n, err := numOpc(args, i, 0)
	return n == 1, err
}

var reMarcaRepl = regexp.MustCompile(`\$(\d{1,2}|[$&])`)

// grupo es `m[n] ?? ”` sobre los índices de una coincidencia de Go.
func grupo(t string, m []int, n int) string {
	if 2*n+1 >= len(m) || m[2*n] < 0 {
		return ""
	}
	return t[m[2*n]:m[2*n+1]]
}

// expandRepl expande $&, $$ y $1…$99 con la coincidencia (la versión PROPIA del
// original, la que usa con «ocurrencia» > 0: un grupo que no existe vale "").
func expandRepl(tpl, t string, m []int) string {
	return reMarcaRepl.ReplaceAllStringFunc(tpl, func(marca string) string {
		switch g := marca[1:]; g {
		case "$":
			return "$"
		case "&":
			return grupo(t, m, 0)
		default:
			n, _ := strconv.Atoi(g)
			return grupo(t, m, n)
		}
	})
}

// sustitucionJS es GetSubstitution de ECMAScript, la de `String.prototype.replace`: la
// que el original usa al sustituir TODAS. Aquí un `$3` sin tercer grupo se queda literal.
func sustitucionJS(tpl, t string, m []int) string {
	grupos := len(m)/2 - 1
	var out strings.Builder
	for i := 0; i < len(tpl); i++ {
		if tpl[i] != '$' || i+1 >= len(tpl) {
			out.WriteByte(tpl[i])
			continue
		}
		switch c := tpl[i+1]; {
		case c == '$':
			out.WriteByte('$')
			i++
		case c == '&':
			out.WriteString(grupo(t, m, 0))
			i++
		case c == '`':
			out.WriteString(t[:m[0]])
			i++
		case c == '\'':
			out.WriteString(t[m[1]:])
			i++
		case esDigito(c):
			n, largo := int(c-'0'), 1
			if i+2 < len(tpl) && esDigito(tpl[i+2]) {
				if nn := n*10 + int(tpl[i+2]-'0'); nn >= 1 && nn <= grupos {
					n, largo = nn, 2
				}
			}
			if n < 1 || n > grupos {
				out.WriteByte('$')
				continue
			}
			out.WriteString(grupo(t, m, n))
			i += largo
		default:
			out.WriteByte('$')
		}
	}
	return out.String()
}

// concatImpl concatena todos los argumentos (rangos incluidos, en orden fila-columna).
func concatImpl(_ *FnCtx, args []*Arg) any {
	var out strings.Builder
	for _, a := range args {
		for _, fila := range a.Matrix() {
			for _, v := range fila {
				t, err := CoerceText(v)
				if err != "" {
					return err
				}
				out.WriteString(t)
			}
		}
	}
	return out.String()
}

/* ---------- Formato numérico y de fecha para TEXT ----------
 * ponytail (del original): patrones «0 # ? , . %», fecha y hora, y secciones «;». NO
 * soporta notación científica («0.00E+00»), fracciones («# ?/?») ni color ([Red]).
 */

var (
	mesesLargos = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	mesesCortos = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	diasLargos  = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	diasCortos  = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

func pad2(n float64) string { return padStart(jsNumero(n), 2) }

// roundDec redondea a `dec` decimales, que pueden ser negativos (decenas, centenas).
func roundDec(n, dec float64) float64 {
	p := jsPow(10, dec)
	return (jsSign(n) * jsRound(math.Abs(n)*p)) / p
}

type fechaHora struct {
	ymd
	h, min, seg, dow float64
}

// serialToDate es serial de hoja (base 1899-12-30) → componentes en UTC, redondeando a
// segundos para limpiar el ruido de float64. Como el original, NO aplica el día fantasma.
func serialToDate(serial float64) fechaHora {
	segundos := jsRound((serial - 25569) * 86400)
	dias := math.Floor(segundos / 86400)
	resto := segundos - dias*86400
	return fechaHora{
		civilDe(dias), math.Floor(resto / 3600), math.Floor(math.Mod(resto, 3600) / 60), math.Mod(resto, 60),
		math.Mod(math.Mod(dias+4, 7)+7, 7), // el 1970-01-01 fue jueves
	}
}

// saltaLiteral consume una comilla o un escape en la posición i de un formato: devuelve
// el literal y cuántos bytes ocupa, o 0 si en i no hay ninguno de los dos.
func saltaLiteral(f string, i int) (string, int) {
	switch f[i] {
	case '"':
		fin := strings.IndexByte(f[i+1:], '"')
		if fin < 0 {
			return f[i+1:], len(f) - i
		}
		return f[i+1 : i+1+fin], fin + 2
	case '\\':
		if i+1 < len(f) {
			_, n := utf8.DecodeRuneInString(f[i+1:])
			return f[i+1 : i+1+n], 1 + n
		}
		return "", 1
	}
	return "", 0
}

// splitSections divide un formato en secciones «;» respetando comillas y escapes.
func splitSections(f string) []string {
	var out []string
	ini := 0
	for i := 0; i < len(f); i++ {
		if _, n := saltaLiteral(f, i); n > 0 {
			i += n - 1
		} else if f[i] == ';' {
			out = append(out, f[ini:i])
			ini = i + 1
		}
	}
	return append(out, f[ini:])
}

// isDateFormat: ¿hay letras de fecha fuera de comillas?
func isDateFormat(seccion string) bool {
	for i := 0; i < len(seccion); i++ {
		if _, n := saltaLiteral(seccion, i); n > 0 {
			i += n - 1
		} else if strings.IndexByte("yYdDhHsSmM", seccion[i]) >= 0 {
			return true
		}
	}
	return false
}

// agrupar mete la coma de millares en una parte entera de solo dígitos.
func agrupar(ip string) string {
	if len(ip) <= 3 || strings.Trim(ip, "0123456789") != "" {
		return ip
	}
	var out strings.Builder
	for i, c := range ip {
		if i > 0 && (len(ip)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(c)
	}
	return out.String()
}

// applyPattern aplica un patrón numérico (solo «0#?», «.» y «,») a un número ≥ 0.
func applyPattern(n float64, patron string) string {
	intPat, fracPat, _ := strings.Cut(patron, ".")
	minInt := strings.Count(intPat, "0")
	var huecos []byte
	for i := 0; i < len(fracPat); i++ {
		if strings.IndexByte("0#?", fracPat[i]) >= 0 {
			huecos = append(huecos, fracPat[i])
		}
	}
	ip, fp, _ := strings.Cut(jsToFixed(n, min(len(huecos), 100)), ".")
	if len(ip) < minInt {
		ip = strings.Repeat("0", minInt-len(ip)) + ip
	}
	if minInt == 0 && ip == "0" {
		ip = "" // «#» sin parte entera: se omite el 0
	}
	if strings.Contains(intPat, ",") {
		ip = agrupar(ip)
	}
	// Recorta los ceros finales que caen sobre un «#» (opcional) de la fracción.
	keep := len(huecos)
	for k := len(huecos) - 1; k >= 0 && huecos[k] == '#' && k < len(fp) && fp[k] == '0'; k-- {
		keep = k
	}
	if keep > 0 {
		return ip + "." + fp[:min(keep, len(fp))]
	}
	return ip
}

// formatNumericSection formatea una sección numérica: literales + patrón + «%».
func formatNumericSection(valor float64, seccion string) string {
	var prefijo, sufijo, patron strings.Builder
	porcientos := 0.0
	for i := 0; i < len(seccion); i++ {
		lit, n := saltaLiteral(seccion, i)
		c := seccion[i]
		switch {
		case n > 0:
			i += n - 1
		case strings.IndexByte("0#?.,", c) >= 0:
			patron.WriteByte(c)
			continue
		default:
			lit = seccion[i : i+1]
			if c == '%' {
				porcientos++
			}
		}
		if patron.Len() == 0 {
			prefijo.WriteString(lit)
		} else {
			sufijo.WriteString(lit) // los literales tras el patrón (con su «%») son sufijo
		}
	}
	if patron.Len() == 0 {
		return prefijo.String() + sufijo.String()
	}
	return prefijo.String() + applyPattern(valor*jsPow(100, porcientos), patron.String()) + sufijo.String()
}

type tokFecha struct {
	k     string
	largo int
	texto string
}

// marcasFecha, en el orden en que el original las prueba (las largas primero).
var marcasFecha = []tokFecha{
	{"ap", 5, "am/pm"},
	{"ap", 3, "a/p"},
	{"y", 4, "yyyy"},
	{"y", 2, "yy"},
	{"m", 5, "mmmmm"},
	{"m", 4, "mmmm"},
	{"m", 3, "mmm"},
	{"m", 2, "mm"},
	{"m", 1, "m"},
	{"wd", 4, "dddd"},
	{"wd", 3, "ddd"},
	{"d", 2, "dd"},
	{"d", 1, "d"},
	{"h", 2, "hh"},
	{"h", 1, "h"},
	{"s", 2, "ss"},
	{"s", 1, "s"},
}

func tokensFecha(f string) (toks []tokFecha, ampm bool) {
	bajo := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return r
	}, f)
	for i := 0; i < len(f); {
		if lit, n := saltaLiteral(f, i); n > 0 {
			toks = append(toks, tokFecha{k: "lit", texto: lit})
			i += n
			continue
		}
		marca := tokFecha{k: "lit", largo: 1}
		for _, m := range marcasFecha {
			if strings.HasPrefix(bajo[i:], m.texto) {
				marca = m
				break
			}
		}
		marca.texto = f[i : i+max(marca.largo, 1)]
		if marca.k == "lit" {
			marca.largo = 0
		}
		ampm = ampm || marca.k == "ap"
		toks = append(toks, marca)
		i += len(marca.texto)
	}
	// «m» y «mm» son minutos si van tras «h» o antes de «s»; si no, mes.
	vecino := func(k, paso int) string {
		for p := k + paso; p >= 0 && p < len(toks); p += paso {
			if toks[p].k != "lit" {
				return toks[p].k
			}
		}
		return ""
	}
	for k := range toks {
		if toks[k].k == "m" && (vecino(k, -1) == "h" || vecino(k, 1) == "s") {
			toks[k].k = "mi"
		}
	}
	return toks, ampm
}

func dosOUno(largo int, v float64) string {
	if largo >= 2 {
		return pad2(v)
	}
	return jsNumero(v)
}

// deTabla es `tabla[i]` con guarda: una fecha inválida no debe tirar la hoja.
func deTabla(tabla []string, i float64) string {
	if i >= 0 && int(i) < len(tabla) {
		return tabla[int(i)]
	}
	return ""
}

// mesFormateado es el mes según cuántas «m» seguidas lleve el patrón: «mmmmm» es la
// inicial, «mmmm» el nombre largo, «mmm» el corto, y menos, el número.
func mesFormateado(largo int, m float64) string {
	switch {
	case largo >= 5:
		return jsSlice(deTabla(mesesLargos[:], m-1), 0, 1)
	case largo == 4:
		return deTabla(mesesLargos[:], m-1)
	case largo == 3:
		return deTabla(mesesCortos[:], m-1)
	}
	return dosOUno(largo, m)
}

// horaFormateada pasa a reloj de 12 si el patrón lleva AM/PM (y las 0 h son las 12).
func horaFormateada(h float64, ampm bool) float64 {
	if !ampm {
		return h
	}
	if h = math.Mod(h, 12); h == 0 {
		return 12
	}
	return h
}

// marcaAMPM es «am»/«pm» con la caja y la longitud que pida el patrón («A/P» es de una).
func marcaAMPM(h float64, patron string) string {
	marca := "am"
	if h >= 12 {
		marca = "pm"
	}
	if strings.ToLower(patron) != patron { // alguna mayúscula
		marca = strings.ToUpper(marca)
	}
	if len(patron) == 3 {
		marca = marca[:1]
	}
	return marca
}

// formatDateSection formatea una sección de fecha y hora a partir de un serial.
func formatDateSection(serial float64, seccion string) string {
	t := serialToDate(serial)
	toks, ampm := tokensFecha(seccion)
	var out strings.Builder
	for _, tk := range toks {
		switch tk.k {
		case "lit":
			out.WriteString(tk.texto)
		case "y":
			if tk.largo <= 2 {
				out.WriteString(pad2(math.Mod(t.y, 100)))
			} else {
				out.WriteString(jsNumero(t.y))
			}
		case "m":
			out.WriteString(mesFormateado(tk.largo, t.m))
		case "mi":
			out.WriteString(dosOUno(tk.largo, t.min))
		case "d":
			out.WriteString(dosOUno(tk.largo, t.d))
		case "wd":
			if tk.largo >= 4 {
				out.WriteString(deTabla(diasLargos[:], t.dow))
			} else {
				out.WriteString(deTabla(diasCortos[:], t.dow))
			}
		case "h":
			out.WriteString(dosOUno(tk.largo, horaFormateada(t.h, ampm)))
		case "s":
			out.WriteString(dosOUno(tk.largo, t.seg))
		case "ap":
			out.WriteString(marcaAMPM(t.h, tk.texto))
		}
	}
	return out.String()
}

var reGeneral = regexp.MustCompile(`(?i)^` + jsS + `*general` + jsS + `*$`)

// formatWithPattern aplica un formato «TEXT» completo (secciones, signo, número o fecha).
func formatWithPattern(valor float64, fmt string) string {
	if fmt == "" || reGeneral.MatchString(fmt) {
		return FormatNumber(valor)
	}
	secciones := splitSections(fmt)
	seccion, signo := secciones[0], ""
	switch {
	case valor < 0:
		valor = -valor
		if len(secciones) > 1 && jsTrim(secciones[1]) != "" {
			seccion = secciones[1]
		} else {
			signo = "-"
		}
	case valor == 0 && len(secciones) >= 3:
		seccion = secciones[2]
	}
	if isDateFormat(seccion) {
		return signo + formatDateSection(valor, seccion)
	}
	cuerpo := formatNumericSection(valor, seccion)
	// Excel no imprime «-0»: sin ningún dígito significativo se omite el signo.
	if signo != "" && strings.ContainsAny(cuerpo, "123456789") {
		return signo + cuerpo
	}
	return cuerpo
}

/* ---------- Búsqueda con comodines para SEARCH ---------- */

// offsetsU16 da, para cada inicio de runa de s, su posición en unidades UTF-16.
func offsetsU16(s string) (bytes, unidades []int) {
	u := 0
	for i, r := range s {
		bytes, unidades = append(bytes, i), append(unidades, u)
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
	}
	return append(bytes, len(s)), append(unidades, u)
}

// wildcardIndex es el índice 0-based (en unidades UTF-16) de la primera coincidencia de
// `patron` (con «?», «*» y «~») en `texto` desde `desde`, sin distinguir mayúsculas.
func wildcardIndex(patron, texto string, desde float64) int {
	if !strings.ContainsAny(patron, "*?~") {
		return jsIndexOf(jsLower(texto), jsLower(patron), desde)
	}
	var re strings.Builder
	re.WriteString(`(?is)^(?:`)
	runas := []rune(patron)
	for i := 0; i < len(runas); i++ {
		switch c := runas[i]; {
		case c == '~' && i+1 < len(runas) && strings.ContainsRune("*?~", runas[i+1]):
			i++
			re.WriteString(regexp.QuoteMeta(string(runas[i])))
		case c == '*':
			re.WriteString(`.*`)
		case c == '?':
			re.WriteString(`.`)
		default:
			re.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	re.WriteString(`)`)
	rx, err := regexp.Compile(re.String())
	if err != nil {
		return -1
	}
	bytes, unidades := offsetsU16(texto)
	for k, u := range unidades {
		if float64(u) >= desde && rx.MatchString(texto[bytes[k]:]) {
			return u
		}
	}
	return -1
}

/* ---------- Ayudas de TEXTSPLIT, TEXTBEFORE, TEXTAFTER y ARRAYTOTEXT ---------- */

// delimiters recoge los delimitadores de un argumento, escalar o matriz, sin vacíos.
func delimiters(a *Arg) ([]string, Error) {
	var out []string
	for _, fila := range a.Matrix() {
		for _, v := range fila {
			t, err := CoerceText(v)
			if err != "" {
				return nil, err
			}
			if t != "" {
				out = append(out, t)
			}
		}
	}
	return out, ""
}

// splitByAny divide por CUALQUIERA de los delimitadores (el primero de la lista gana).
func splitByAny(texto string, delims []string, sinCaja bool) []string {
	if len(delims) == 0 {
		return []string{texto}
	}
	escapados := make([]string, len(delims))
	for i, d := range delims {
		escapados[i] = regexp.QuoteMeta(d)
	}
	patron := strings.Join(escapados, "|")
	if sinCaja {
		patron = "(?i)" + patron
	}
	return regexp.MustCompile(patron).Split(texto, -1)
}

// allOccurrences son los índices (UTF-16) de todas las apariciones de `delim`.
func allOccurrences(texto, delim string, sinCaja bool) []int {
	if sinCaja {
		texto, delim = jsLower(texto), jsLower(delim)
	}
	var out []int
	for desde := 0; ; {
		i := jsIndexOf(texto, delim, float64(desde))
		if i < 0 {
			return out
		}
		out = append(out, i)
		desde = i + jsLen(delim)
	}
}

// textBeforeAfter es el núcleo de TEXTBEFORE y TEXTAFTER.
func textBeforeAfter(antes bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 6 {
			return ErrValue
		}
		t, err := textos(args, 2)
		if err != "" {
			return err
		}
		texto, delim := t[0], t[1]
		if delim == "" {
			return ErrValue
		}
		inst, err := intOpc(args, 2, 1)
		if err != "" {
			return err
		}
		if inst == 0 {
			return ErrValue
		}
		sinCaja, err := caseInsensitive(args, 3)
		if err != "" {
			return err
		}
		var siNoHay Value = ErrNA
		if len(args) >= 6 {
			siNoHay = args[5].Value()
		}
		occ := allOccurrences(texto, delim, sinCaja)
		elegida := inst - 1
		if inst < 0 {
			elegida = float64(len(occ)) + inst // negativa: desde el final
		}
		if elegida < 0 || elegida >= float64(len(occ)) {
			return siNoHay
		}
		en := float64(occ[int(elegida)])
		if antes {
			return jsSlice(texto, 0, en)
		}
		return jsDesde(texto, en+float64(jsLen(delim)))
	}
}

// conUnTexto envuelve las de un solo texto (LEN, LOWER, UPPER, CLEAN…).
func conUnTexto(f func(t string) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := textArg(args, 0)
		if err != "" {
			return err
		}
		return f(t)
	}
}

// izquierdaDerecha es LEFT y RIGHT.
func izquierdaDerecha(izquierda bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		t, err := textArg(args, 0)
		if err != "" {
			return err
		}
		n, err := intOpc(args, 1, 1)
		if err != "" {
			return err
		}
		if n < 0 {
			return ErrValue
		}
		if izquierda {
			return jsSlice(t, 0, n)
		}
		return jsDesde(t, math.Max(0, float64(jsLen(t))-n))
	}
}

// buscar es FIND (exacta) y SEARCH (comodines, sin distinguir mayúsculas).
func buscar(indice func(aguja, pajar string, desde float64) int) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		t, err := textos(args, 2)
		if err != "" {
			return err
		}
		inicio, err := intOpc(args, 2, 1)
		if err != "" {
			return err
		}
		if inicio < 1 || inicio > float64(jsLen(t[1])+1) {
			return ErrValue
		}
		if idx := indice(t[0], t[1], inicio-1); idx >= 0 {
			return float64(idx + 1)
		}
		return ErrValue
	}
}

var reNumeroLimpio = regexp.MustCompile(`^[-+]?(\d+(\.\d*)?|\.\d+)$`)

// estricto lee el `formato` opcional de ARRAYTOTEXT y VALUETOTEXT (1 = estricto).
func estricto(args []*Arg) (bool, Error) {
	f, err := numOpc(args, 1, 0)
	return f == 1, err
}

// conDecimales es el tramo común de DOLLAR y FIXED: número y decimales (2 por defecto);
// los decimales negativos redondean a la izquierda de la coma.
func conDecimales(args []*Arg) (valor float64, cuerpo string, err Error) {
	n, err := numArg(args, 0)
	if err != "" {
		return 0, "", err
	}
	dec, err := intOpc(args, 1, 2)
	if err != "" {
		return 0, "", err
	}
	if dec < 0 {
		return roundDec(n, dec), "", ""
	}
	if dec > 0 {
		cuerpo = "." + strings.Repeat("0", int(math.Min(dec, 100)))
	}
	return n, cuerpo, ""
}

// funcionesTexto es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesTexto = map[string]fn{
	// CONCAT y CONCATENATE se comportan igual en las hojas modernas (unen rangos).
	"CONCAT":      concatImpl,
	"CONCATENATE": concatImpl,
	"LEFT":        izquierdaDerecha(true),
	"RIGHT":       izquierdaDerecha(false),
	"MID": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 3 {
			return ErrValue
		}
		t, err := textArg(args, 0)
		if err != "" {
			return err
		}
		inicio, err := intOpc(args, 1, 0)
		if err != "" {
			return err
		}
		num, err := intOpc(args, 2, 0)
		if err != "" {
			return err
		}
		if inicio < 1 || num < 0 {
			return ErrValue
		}
		return jsSlice(t, inicio-1, inicio-1+num)
	},
	"LEN":   conUnTexto(func(t string) Value { return float64(jsLen(t)) }),
	"LOWER": conUnTexto(func(t string) Value { return jsLower(t) }),
	"UPPER": conUnTexto(func(t string) Value { return jsUpper(t) }),
	"PROPER": conUnTexto(func(t string) Value {
		var out strings.Builder
		previa := false
		for _, ch := range t {
			letra := unicode.IsLetter(ch)
			switch {
			case letra && previa:
				out.WriteString(jsLower(string(ch)))
			case letra:
				out.WriteString(jsUpper(string(ch)))
			default:
				out.WriteRune(ch)
			}
			previa = letra
		}
		return out.String()
	}),
	// Solo el espacio ASCII: colapsa las rachas y recorta los extremos, como Excel.
	"TRIM": conUnTexto(func(t string) Value {
		return strings.Join(strings.FieldsFunc(t, func(r rune) bool { return r == ' ' }), " ")
	}),
	"REPLACE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 4 {
			return ErrValue
		}
		viejo, err := textArg(args, 0)
		if err != "" {
			return err
		}
		inicio, err := intOpc(args, 1, 0)
		if err != "" {
			return err
		}
		num, err := intOpc(args, 2, 0)
		if err != "" {
			return err
		}
		rep, err := textArg(args, 3)
		if err != "" {
			return err
		}
		if inicio < 1 || num < 0 {
			return ErrValue
		}
		return jsSlice(viejo, 0, inicio-1) + rep + jsDesde(viejo, inicio-1+num)
	},
	"SUBSTITUTE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 4 {
			return ErrValue
		}
		t, err := textos(args, 3)
		if err != "" {
			return err
		}
		src, viejo, nuevo := t[0], t[1], t[2]
		if viejo == "" {
			return src // buscar "" no cambia nada
		}
		if len(args) == 3 {
			return strings.ReplaceAll(src, viejo, nuevo)
		}
		inst, err := intOpc(args, 3, 0)
		if err != "" {
			return err
		}
		if inst < 1 {
			return ErrValue
		}
		desde := 0
		for cuenta := 1.0; ; cuenta++ {
			idx := strings.Index(src[desde:], viejo)
			if idx < 0 {
				return src // no hay tantas apariciones: sin cambios
			}
			if idx += desde; cuenta == inst {
				return src[:idx] + nuevo + src[idx+len(viejo):]
			}
			desde = idx + len(viejo)
		}
	},
	"FIND":   buscar(func(aguja, pajar string, desde float64) int { return jsIndexOf(pajar, aguja, desde) }),
	"SEARCH": buscar(wildcardIndex),
	"TEXT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		crudo := args[0].Value()
		if IsError(crudo) {
			return crudo
		}
		formato, err := textArg(args, 1)
		if err != "" {
			return err
		}
		n, err := CoerceNumber(crudo)
		if err != "" {
			return crudo // un valor de texto se devuelve tal cual
		}
		return formatWithPattern(n, formato)
	},
	"VALUE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		var s string
		switch x := args[0].Value().(type) {
		case Error:
			return x
		case float64:
			return x
		case bool:
			return ErrValue // VALUE no acepta booleanos
		case string:
			s = jsTrim(x)
		}
		if s == "" {
			return ErrValue // ponytail: VALUE("") es #VALUE!; la celda en blanco es otro caso
		}
		pct := 0.0
		for ; strings.HasSuffix(s, "%"); pct++ {
			s = jsTrim(s[:len(s)-1])
		}
		s = strings.TrimLeftFunc(s, func(r rune) bool { return jsEspacio(r) || strings.ContainsRune("$€£¥", r) })
		s = strings.ReplaceAll(s, ",", "") // separadores de millares
		if !reNumero.MatchString(s) {
			return ErrValue
		}
		return jsNumber(s) / jsPow(100, pct)
	},
	"NUMBERVALUE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 3 {
			return ErrValue
		}
		t, err := textos(args, len(args))
		if err != "" {
			return err
		}
		seps := [2]string{".", ","} // decimal y de grupo
		for i, s := range t[1:] {
			if s != "" {
				_, n := utf8.DecodeRuneInString(s)
				seps[i] = s[:n]
			}
		}
		if seps[0] == seps[1] {
			return ErrValue
		}
		s := strings.Map(sinEspacios, t[0]) // NUMBERVALUE ignora los espacios
		if s == "" {
			return 0.0
		}
		pct := 0.0
		for ; strings.HasSuffix(s, "%"); pct++ {
			s = s[:len(s)-1]
		}
		s = strings.ReplaceAll(strings.ReplaceAll(s, seps[1], ""), seps[0], ".")
		if !reNumeroLimpio.MatchString(s) {
			return ErrValue
		}
		return jsNumber(s) / jsPow(100, pct)
	},
	"REPT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		t, err := textArg(args, 0)
		if err != "" {
			return err
		}
		veces, err := intOpc(args, 1, 0)
		if err != "" {
			return err
		}
		if veces < 0 || float64(jsLen(t))*veces > 32767 || math.IsNaN(veces) {
			return ErrValue // el límite de una celda
		}
		if t == "" {
			return "" // "" repetido 1e300 veces cabe en el límite, pero no en un int
		}
		return strings.Repeat(t, int(veces))
	},
	"TEXTJOIN": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 {
			return ErrValue
		}
		delim, err := textArg(args, 0)
		if err != "" {
			return err
		}
		sinVacios, err := CoerceBoolean(args[1].Value())
		if err != "" {
			return err
		}
		var partes []string
		for _, a := range args[2:] {
			for _, fila := range a.Matrix() {
				for _, v := range fila {
					t, err := CoerceText(v)
					if err != "" {
						return err
					}
					if !sinVacios || t != "" {
						partes = append(partes, t)
					}
				}
			}
		}
		return strings.Join(partes, delim)
	},
	"CHAR": func(_ *FnCtx, args []*Arg) any {
		n, err := arg1(args)
		if err != "" {
			return err
		}
		if n = math.Trunc(n); n < 1 || n > 255 || math.IsNaN(n) {
			return ErrValue
		}
		return string(rune(n))
	},
	"CODE": conUnTexto(func(t string) Value {
		if t == "" {
			return ErrValue
		}
		r, _ := utf8.DecodeRuneInString(t)
		return float64(unidad(r)) // charCodeAt(0): la primera unidad UTF-16
	}),
	"UNICHAR": func(_ *FnCtx, args []*Arg) any {
		n, err := arg1(args)
		if err != "" {
			return err
		}
		if n = math.Trunc(n); n < 1 || n > 0x10ffff || math.IsNaN(n) {
			return ErrValue
		}
		return string(rune(n))
	},
	"UNICODE": conUnTexto(func(t string) Value {
		if t == "" {
			return ErrValue
		}
		r, _ := utf8.DecodeRuneInString(t)
		return float64(r)
	}),
	"EXACT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		t, err := textos(args, 2)
		if err != "" {
			return err
		}
		return t[0] == t[1]
	},
	// Quita los caracteres de control ASCII (0–31), como el CLEAN clásico.
	"CLEAN": conUnTexto(func(t string) Value {
		return strings.Map(func(r rune) rune {
			if r < 0x20 {
				return -1
			}
			return r
		}, t)
	}),
	"T": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		switch x := args[0].Value().(type) {
		case Error:
			return x
		case string:
			return x
		}
		return ""
	},
	"TEXTSPLIT":  textsplit,
	"TEXTBEFORE": textBeforeAfter(true),
	"TEXTAFTER":  textBeforeAfter(false),
	// ARRAYTOTEXT(array, [formato]): 0 conciso («a, b, c»); 1 estricto («{"a";"b"}»).
	"ARRAYTOTEXT": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		m := args[0].Matrix()
		est, err := estricto(args)
		if err != "" {
			return err
		}
		filas := make([]string, 0, len(m))
		var partes []string
		for _, fila := range m {
			celdas := make([]string, len(fila))
			for c, v := range fila {
				t, err := CoerceText(v)
				switch s, esTexto := v.(string); {
				case est && esTexto:
					t = `"` + s + `"`
				case est && err != "":
					t = "#ERR"
				case err != "":
					return err
				}
				celdas[c] = t
			}
			partes = append(partes, celdas...)
			filas = append(filas, strings.Join(celdas, ","))
		}
		if est {
			return "{" + strings.Join(filas, ";") + "}"
		}
		return strings.Join(partes, ", ")
	},
	// VALUETOTEXT(valor, [formato]): un error se convierte en su código, no se propaga.
	"VALUETOTEXT": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		v := args[0].Value()
		est, err := estricto(args)
		if err != "" {
			return err
		}
		if s, ok := v.(string); ok && est {
			return `"` + s + `"`
		}
		return Display(v)
	},
	// DOLLAR(número, [decimales=2]): millares y «$»; los negativos entre paréntesis.
	"DOLLAR": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		valor, cuerpo, err := conDecimales(args)
		if err != "" {
			return err
		}
		return formatWithPattern(valor, `"$"#,##0`+cuerpo+`;("$"#,##0`+cuerpo+`)`)
	},
	// FIXED(número, [decimales=2], [sin_comas=FALSE]).
	"FIXED": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 3 {
			return ErrValue
		}
		valor, cuerpo, err := conDecimales(args)
		if err != "" {
			return err
		}
		patron := "#,##0"
		if len(args) >= 3 {
			sinComas, err := CoerceBoolean(args[2].Value())
			if err != "" {
				return err
			}
			if sinComas {
				patron = "0"
			}
		}
		return formatWithPattern(valor, patron+cuerpo)
	},
	"REGEXTEST": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		t, err := textos(args, 2)
		if err != "" {
			return err
		}
		sinCaja, err := caseInsensitive(args, 2)
		if err != "" {
			return err
		}
		re, err := buildRegex(t[1], sinCaja)
		if err != "" {
			return err
		}
		return re.MatchString(t[0])
	},
	"REGEXEXTRACT": regexExtract,
	"REGEXREPLACE": regexReplace,
}

func init() { registrar(funcionesTexto) }

// textsplit es TEXTSPLIT(texto, delim_col, [delim_fila], [ignorar_vacíos], [modo],
// [relleno=#N/A]): parte el texto en una MATRIZ y rellena las filas irregulares.
// opcionesTextsplit lee los seis argumentos de TEXTSPLIT: el texto, los delimitadores de
// columna y de fila, si se saltan los vacíos, si se ignora la caja y con qué se rellena.
type opcionesTextsplit struct {
	texto       string
	colD, filaD []string
	sinVacios   bool
	sinCaja     bool
	relleno     Value
}

func leerTextsplit(args []*Arg) (opcionesTextsplit, Error) {
	o := opcionesTextsplit{relleno: ErrNA}
	if len(args) < 2 || len(args) > 6 {
		return o, ErrValue
	}
	var err Error
	if o.texto, err = textArg(args, 0); err != "" {
		return o, err
	}
	if o.colD, err = delimiters(args[1]); err != "" {
		return o, err
	}
	if len(args) >= 3 {
		if o.filaD, err = delimiters(args[2]); err != "" {
			return o, err
		}
	}
	if len(o.colD) == 0 && len(o.filaD) == 0 {
		return o, ErrValue
	}
	if len(args) >= 4 {
		if o.sinVacios, err = CoerceBoolean(args[3].Value()); err != "" {
			return o, err
		}
	}
	if o.sinCaja, err = caseInsensitive(args, 4); err != "" {
		return o, err
	}
	if len(args) >= 6 {
		if o.relleno = args[5].Value(); IsError(o.relleno) {
			return o, Error(o.relleno.(string)) //nolint:errcheck,forcetypeassert // IsError ya dice que es una cadena de error
		}
	}
	return o, ""
}

func textsplit(_ *FnCtx, args []*Arg) any {
	o, err := leerTextsplit(args)
	if err != "" {
		return err
	}
	var rejilla [][]string
	ancho := 0
	for _, fila := range splitByAny(o.texto, o.filaD, o.sinCaja) {
		var celdas []string
		for _, c := range splitByAny(fila, o.colD, o.sinCaja) {
			if !o.sinVacios || c != "" {
				celdas = append(celdas, c)
			}
		}
		if !o.sinVacios || len(celdas) > 0 {
			rejilla = append(rejilla, celdas)
			ancho = max(ancho, len(celdas))
		}
	}
	if len(rejilla) == 0 {
		rejilla, ancho = [][]string{{""}}, 1
	}
	out := make(Matrix, len(rejilla))
	for r, fila := range rejilla {
		out[r] = make([]Value, ancho)
		for c := range out[r] {
			if out[r][c] = o.relleno; c < len(fila) {
				out[r][c] = fila[c]
			}
		}
	}
	return out
}

// regexExtract es REGEXEXTRACT(texto, patrón, [modo], [case]): modo 0, la primera
// coincidencia; 1, todas (en columna); 2, los grupos de la primera (en fila). Sin
// coincidencia → #N/A.
func regexExtract(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 || len(args) > 4 {
		return ErrValue
	}
	t, err := textos(args, 2)
	if err != "" {
		return err
	}
	modo, err := intOpc(args, 2, 0)
	if err != "" {
		return err
	}
	sinCaja, err := caseInsensitive(args, 3)
	if err != "" {
		return err
	}
	re, err := buildRegex(t[1], sinCaja)
	if err != "" {
		return err
	}
	if modo == 1 {
		var out Matrix
		for _, m := range re.FindAllString(t[0], -1) {
			out = append(out, []Value{m})
		}
		if len(out) == 0 {
			return ErrNA
		}
		return out
	}
	m := re.FindStringSubmatchIndex(t[0])
	switch {
	case m == nil:
		return ErrNA
	case modo == 2 && len(m) > 2:
		grupos := make([]Value, len(m)/2-1)
		for g := range grupos {
			grupos[g] = grupo(t[0], m, g+1)
		}
		return Matrix{grupos}
	case modo != 0 && modo != 2:
		return ErrValue
	}
	return grupo(t[0], m, 0)
}

// regexReplace es REGEXREPLACE(texto, patrón, reemplazo, [ocurrencia], [case]):
// ocurrencia 0 sustituye todas; n > 0, solo la n-ésima.
func regexReplace(_ *FnCtx, args []*Arg) any {
	if len(args) < 3 || len(args) > 5 {
		return ErrValue
	}
	t, err := textos(args, 3)
	if err != "" {
		return err
	}
	occ, err := intOpc(args, 3, 0)
	if err != "" {
		return err
	}
	if occ < 0 {
		return ErrValue
	}
	sinCaja, err := caseInsensitive(args, 4)
	if err != "" {
		return err
	}
	re, err := buildRegex(t[1], sinCaja)
	if err != "" {
		return err
	}
	texto, rep := t[0], t[2]
	todas := re.FindAllStringSubmatchIndex(texto, -1)
	if occ > 0 {
		if float64(len(todas)) < occ {
			return texto // menos coincidencias que `ocurrencia`: sin cambios
		}
		m := todas[int(occ)-1]
		return texto[:m[0]] + expandRepl(rep, texto, m) + texto[m[1]:]
	}
	var out strings.Builder
	fin := 0
	for _, m := range todas {
		out.WriteString(texto[fin:m[0]])
		out.WriteString(sustitucionJS(rep, texto, m))
		fin = m[1]
	}
	return out.String() + texto[fin:]
}
