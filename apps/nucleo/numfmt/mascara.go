// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package numfmt

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # Las máscaras de formato personalizado
//
// Es el puerto de `sheets/numfmt-mask.ts`, con el mismo alcance y los mismos atajos:
// secciones «positivo;negativo;cero», marcadores 0 # ?, millares y escalado por comas,
// porcentaje, literales, moneda, [Color], fechas y horas, tiempo transcurrido [h] y
// condiciones [>=100]. La máscara se ESCRIBE con los tokens de Excel («.» decimal, «,»
// millares) y la SALIDA sale en es-ES, como el resto de presets.
//
// Lo que la web deja fuera también queda fuera aquí, por la misma razón —que el mismo
// documento se vea igual en los dos sitios—: notación científica, relleno «*x», colores
// indexados y el ancho real de «_» y «?». Los vectores de `numfmt_test.go` son los de
// `packages/office/test/sheets-numfmt-mask.test.ts`.

var mesesES = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

var mesesCortosES = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

var diasES = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

var diasCortosES = [...]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}

// coloresMascara es MASK_COLORS.
var coloresMascara = map[string]string{
	"black": "#000000", "negro": "#000000", "blue": "#2563EB", "azul": "#2563EB",
	"cyan": "#0891B2", "green": "#16A34A", "verde": "#16A34A", "magenta": "#DB2777",
	"red": "#DC2626", "rojo": "#DC2626", "white": "#FFFFFF", "blanco": "#FFFFFF",
	"yellow": "#CA8A04", "amarillo": "#CA8A04",
}

var (
	reTranscurrido     = regexp.MustCompile(`(?i)^(h{1,2}|m{1,2}|s{1,2})$`)
	reTranscurridoEn   = regexp.MustCompile(`(?i)\[(h{1,2}|m{1,2}|s{1,2})\]`)
	reCondicion        = regexp.MustCompile(`^\s*(<=|>=|<>|<|>|=)\s*(-?\d+(?:\.\d+)?)\s*$`)
	reCorchete         = regexp.MustCompile(`\[([^\]]*)\]`)
	reCorcheteInicial  = regexp.MustCompile(`^\s*\[([^\]]*)\]`)
	reLiteralComillas  = regexp.MustCompile(`"[^"]*"`)
	reLiteralEscape    = regexp.MustCompile(`\\.`)
	reFechaHora        = regexp.MustCompile(`[ydhs]`)
	reMarcadorDigito   = regexp.MustCompile(`[0#?]`)
	tokensFecha        = []string{"yyyy", "yy", "mmmmm", "mmmm", "mmm", "mm", "m", "dddd", "ddd", "dd", "d", "hh", "h", "ss", "s", "am/pm", "a/p"}
	tokensTranscurrido = []string{"hh", "h", "mm", "m", "ss", "s"}
)

// partesFecha es `DateParts`.
type partesFecha struct{ a, mes, d, sem, h, mi, s int }

// RedondearJS es `Math.round`: la mitad hacia +∞, sobre el valor exacto.
func RedondearJS(x float64) float64 {
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	return r
}

// partesDeSerial es `serialToParts`: el serial del sistema 1900 en sus partes, o false
// si se sale del calendario.
func partesDeSerial(n float64) (partesFecha, bool) {
	dia := math.Floor(n)
	seg := int(RedondearJS((n - dia) * 86400))
	if seg >= 86400 {
		seg -= 86400
		dia++
	}
	if dia < 0 || dia > maxSerialFecha {
		return partesFecha{}, false
	}
	p := partesFecha{h: seg / 3600, mi: seg % 3600 / 60, s: seg % 60}
	if dia == 60 {
		p.a, p.mes, p.d, p.sem = 1900, 2, 29, 3 // el 29-2-1900 fantasma cae en miércoles
		return p, true
	}
	dias := int(dia)
	if dias > 60 {
		dias--
	}
	t := epocaFechas.AddDate(0, 0, dias)
	p.a, p.mes, p.d, p.sem = t.Year(), int(t.Month()), t.Day(), int(t.Weekday())
	return p, true
}

func dos(n int) string {
	if n < 10 && n >= 0 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// secciones es `splitSections`: parte por «;» de primer nivel, respetando comillas,
// escapes y corchetes.
func secciones(m string) []string {
	var out []string
	var cur strings.Builder
	r := []rune(m)
	for i := 0; i < len(r); i++ {
		switch ch := r[i]; ch {
		case '"', '[':
			cierre := '"'
			if ch == '[' {
				cierre = ']'
			}
			cur.WriteRune(ch)
			i++
			for i < len(r) && r[i] != cierre {
				cur.WriteRune(r[i])
				i++
			}
			if i < len(r) {
				cur.WriteRune(r[i])
			}
		case '\\':
			cur.WriteRune(ch)
			if i+1 < len(r) {
				i++
				cur.WriteRune(r[i])
			}
		case ';':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(ch)
		}
	}
	return append(out, cur.String())
}

// quitarCorchetes es `stripBrackets`: saca el color de la sección y cambia cada [ … ]
// por su literal —la moneda de «[$€-C0A]»—, lo conserva si es tiempo transcurrido o lo
// quita.
func quitarCorchetes(s string) (color, cuerpo string) {
	cuerpo = reCorchete.ReplaceAllStringFunc(s, func(m string) string {
		dentro := m[1 : len(m)-1]
		clave := strings.ToLower(strings.TrimSpace(dentro))
		if c, ok := coloresMascara[clave]; ok {
			if color == "" {
				color = c
			}
			return ""
		}
		if strings.HasPrefix(dentro, "$") {
			lit, _, _ := strings.Cut(dentro[1:], "-")
			if lit == "" {
				return ""
			}
			return `"` + lit + `"`
		}
		if reTranscurrido.MatchString(clave) {
			return "[" + clave + "]"
		}
		return ""
	})
	return color, cuerpo
}

// condicion es `readCondition`: el predicado de la sección, o nil.
func condicion(s string) func(float64) bool {
	m := reCorcheteInicial.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	c := reCondicion.FindStringSubmatch(m[1])
	if c == nil {
		return nil
	}
	ref, err := strconv.ParseFloat(c[2], 64)
	if err != nil {
		return nil
	}
	return func(n float64) bool {
		switch c[1] {
		case "<":
			return n < ref
		case ">":
			return n > ref
		case "<=":
			return n <= ref
		case ">=":
			return n >= ref
		case "<>":
			return n != ref
		}
		return n == ref
	}
}

func sinLiterales(s string) string {
	return reLiteralEscape.ReplaceAllString(reLiteralComillas.ReplaceAllString(s, ""), "")
}

// esFechaHora es `isDateTime`: lleva y/d/h/s, o una «m» sin marcadores de dígito.
func esFechaHora(cuerpo string) bool {
	s := strings.ToLower(sinLiterales(cuerpo))
	if reFechaHora.MatchString(s) {
		return true
	}
	return strings.Contains(s, "m") && !reMarcadorDigito.MatchString(s)
}

// ---- la sección numérica ----

type tokNum struct {
	t byte // 'l' literal, 'd' dígito, '.' punto, ',' coma, '%'
	s string
	c rune
}

// tokensNumero es `scanNumberTokens`.
func tokensNumero(cuerpo string) []tokNum {
	var toks []tokNum
	r := []rune(cuerpo)
	for i := 0; i < len(r); i++ {
		switch ch := r[i]; ch {
		case '"':
			var s strings.Builder
			for i++; i < len(r) && r[i] != '"'; i++ {
				s.WriteRune(r[i])
			}
			toks = append(toks, tokNum{t: 'l', s: s.String()})
		case '\\':
			if i+1 < len(r) {
				i++
				toks = append(toks, tokNum{t: 'l', s: string(r[i])})
			}
		case '_':
			if i+1 < len(r) {
				i++ // «_x» es el ancho de x: un espacio (aproximado, como la web)
			}
			toks = append(toks, tokNum{t: 'l', s: " "})
		case '*':
			if i+1 < len(r) {
				i++ // «*x» rellena: se omite, como la web
			}
		case '0', '#', '?':
			toks = append(toks, tokNum{t: 'd', c: ch})
		case '.', ',', '%':
			toks = append(toks, tokNum{t: byte(ch)})
		default:
			toks = append(toks, tokNum{t: 'l', s: string(ch)})
		}
	}
	return toks
}

// seccionNumero es `formatNumberSection`: una MAGNITUD (≥ 0) con una sección numérica.
// Va en tres pasos, como en la web: medir la máscara, redondear y componer.
func seccionNumero(cuerpo string, magnitud float64) string {
	toks := tokensNumero(cuerpo)
	m := medirMascara(toks)
	escalado := magnitud * math.Pow(100, float64(m.pct)) / math.Pow(1000, float64(m.finales))
	// Una máscara con muchos «%» (o «,» finales) escala hasta salirse de float64:
	// +Inf, o NaN con 0. `fijo` no sabe qué hacer con eso y entraba en pánico al
	// pintar, cerrando Calcus (FYE-01). Se enseña el número sin máscara.
	if math.IsNaN(escalado) || math.IsInf(escalado, 0) {
		return formula.FormatNumber(magnitud)
	}
	nfrac := min(len(m.frac), 100)
	cifrasEnt, cifrasFrac := fijo(escalado, nfrac, 0)

	minimo := strings.Count(string(m.ent), "0")
	verEnt := cifrasEnt
	if verEnt == "0" && minimo == 0 {
		verEnt = ""
	} else if len(verEnt) < minimo {
		verEnt = strings.Repeat("0", minimo-len(verEnt)) + verEnt
	}
	if m.agrupa && verEnt != "" {
		verEnt = millares(verEnt, ".")
	}
	quedan := nfrac
	for quedan > 0 && m.frac[quedan-1] == '#' && cifrasFrac[quedan-1] == '0' {
		quedan--
	}
	return componerNumero(toks, m, verEnt, cifrasFrac[:quedan])
}

// mascaraNum es lo que se mide de una sección numérica antes de formatear.
type mascaraNum struct {
	ent, frac []rune // los marcadores de la parte entera y de la decimal
	pct       int
	agrupa    bool // una coma ENTRE marcadores enteros: millares
	finales   int  // comas al final de la parte entera: cada una divide entre mil
}

func medirMascara(toks []tokNum) mascaraNum {
	var m mascaraNum
	punto := false
	for _, t := range toks {
		switch {
		case t.t == '.' && !punto:
			punto = true
		case t.t == 'd' && punto:
			m.frac = append(m.frac, t.c)
		case t.t == 'd':
			m.ent = append(m.ent, t.c)
		case t.t == '%':
			m.pct++
		}
	}
	vistos := 0
	for _, t := range toks {
		if t.t == '.' {
			break
		}
		switch {
		case t.t == 'd':
			vistos++
		case t.t == ',' && vistos > 0 && vistos < len(m.ent):
			m.agrupa = true
		case t.t == ',' && vistos > 0:
			m.finales++
		}
	}
	return m
}

// componerNumero recorre la máscara y va poniendo cada cosa en su sitio.
//
// Sin agrupar, las cifras enteras se REPARTEN entre sus marcadores de derecha a
// izquierda (el primero se lleva lo que sobre): «00000-0000» + 123456789 →
// «12345-6789». Con millares, el entero ya lleva sus puntos y sale en el primero.
func componerNumero(toks []tokNum, m mascaraNum, verEnt, verFrac string) string {
	var partes []string
	if !m.agrupa && len(m.ent) > 0 {
		partes = make([]string, len(m.ent))
		resto := verEnt
		for k := len(m.ent) - 1; k >= 1 && resto != ""; k-- {
			partes[k], resto = resto[len(resto)-1:], resto[:len(resto)-1]
		}
		partes[0] = resto
	}
	var b strings.Builder
	iEnt, entPuesto, fracPuesto, punto := 0, false, false, false
	for _, t := range toks {
		switch t.t {
		case 'l':
			b.WriteString(t.s)
		case '%':
			b.WriteByte('%')
		case '.':
			if punto {
				b.WriteByte('.') // un punto de más es un literal
				continue
			}
			punto = true
			if !entPuesto && len(m.ent) == 0 {
				b.WriteString(verEnt) // «.00», sin marcador entero
				entPuesto = true
			}
			if verFrac != "" {
				b.WriteByte(',')
			}
		case 'd':
			switch {
			case punto && !fracPuesto:
				b.WriteString(verFrac)
				fracPuesto = true
			case punto:
			case partes != nil:
				if iEnt < len(partes) {
					b.WriteString(partes[iEnt])
				}
				iEnt++
			case !entPuesto:
				b.WriteString(verEnt)
				entPuesto = true
			}
		}
	}
	return b.String()
}

// ---- la sección de fecha y hora ----

type tokFecha struct {
	lit string
	tok string // vacío si es literal
}

// tokensFechaDe es `scanDateTokens`.
func tokensFechaDe(cuerpo string) []tokFecha {
	var toks []tokFecha
	r := []rune(cuerpo)
	bajo := []rune(strings.Map(unicode.ToLower, cuerpo))
	for i := 0; i < len(r); {
		switch r[i] {
		case '"':
			var s strings.Builder
			for i++; i < len(r) && r[i] != '"'; i++ {
				s.WriteRune(r[i])
			}
			i++
			toks = append(toks, tokFecha{lit: s.String()})
			continue
		case '\\':
			if i+1 < len(r) {
				toks = append(toks, tokFecha{lit: string(r[i+1])})
			}
			i += 2
			continue
		}
		if tk := prefijo(bajo, i, tokensFecha); tk != "" {
			toks = append(toks, tokFecha{tok: string(r[i : i+len(tk)])})
			i += len(tk)
			continue
		}
		toks = append(toks, tokFecha{lit: string(r[i])})
		i++
	}
	return toks
}

// prefijo es el primero de `tokens` con el que empieza `s` en la posición i.
func prefijo(s []rune, i int, tokens []string) string {
	for _, tk := range tokens {
		if i+len(tk) <= len(s) && string(s[i:i+len(tk)]) == tk {
			return tk
		}
	}
	return ""
}

// minutos es `minuteIndices`: una «m» o «mm» es MINUTO si tiene una hora delante o un
// segundo detrás; si no, es mes.
func minutos(toks []tokFecha) map[int]bool {
	clase := func(i int) string { return strings.ToLower(toks[i].tok) }
	cerca := func(i, paso int) string {
		for j := i + paso; j >= 0 && j < len(toks); j += paso {
			if toks[j].tok != "" {
				return clase(j)
			}
		}
		return ""
	}
	out := map[int]bool{}
	for i := range toks {
		if k := clase(i); k == "m" || k == "mm" {
			antes, despues := cerca(i, -1), cerca(i, 1)
			if antes == "h" || antes == "hh" || despues == "s" || despues == "ss" {
				out[i] = true
			}
		}
	}
	return out
}

// tokenFecha es `renderDateTok`: los de fecha aquí, los de hora en tokenHora.
func tokenFecha(tok string, p partesFecha, minuto, ampm bool) string {
	switch strings.ToLower(tok) {
	case "yyyy":
		if p.a < 1000 {
			return strings.Repeat("0", 4-len(strconv.Itoa(p.a))) + strconv.Itoa(p.a)
		}
		return strconv.Itoa(p.a)
	case "yy":
		return dos(p.a % 100)
	case "mmmmm":
		inicial, _ := utf8.DecodeRuneInString(mesesES[p.mes-1])
		return strings.ToUpper(string(inicial))
	case "mmmm":
		return mesesES[p.mes-1]
	case "mmm":
		return mesesCortosES[p.mes-1]
	case "mm":
		if minuto {
			return dos(p.mi)
		}
		return dos(p.mes)
	case "m":
		if minuto {
			return strconv.Itoa(p.mi)
		}
		return strconv.Itoa(p.mes)
	case "dddd":
		return diasES[p.sem]
	case "ddd":
		return diasCortosES[p.sem]
	case "dd":
		return dos(p.d)
	case "d":
		return strconv.Itoa(p.d)
	}
	return tokenHora(tok, p, ampm)
}

// tokenHora son los tokens de hora de `renderDateTok`.
func tokenHora(tok string, p partesFecha, ampm bool) string {
	hora := p.h
	if ampm {
		if hora = p.h % 12; hora == 0 {
			hora = 12
		}
	}
	switch strings.ToLower(tok) {
	case "hh":
		return dos(hora)
	case "h":
		return strconv.Itoa(hora)
	case "ss":
		return dos(p.s)
	case "s":
		return strconv.Itoa(p.s)
	case "am/pm":
		return mananaTarde(p.h, tok == "AM/PM", "AM", "PM")
	case "a/p":
		return mananaTarde(p.h, tok == "A/P", "A", "P")
	}
	return tok
}

func mananaTarde(h int, mayus bool, am, pm string) string {
	s := pm
	if h < 12 {
		s = am
	}
	if mayus {
		return s
	}
	return strings.ToLower(s)
}

// seccionFecha es `formatDateSection`.
func seccionFecha(cuerpo string, serial float64) string {
	p, ok := partesDeSerial(serial)
	if !ok {
		return formula.FormatNumber(serial)
	}
	toks := tokensFechaDe(cuerpo)
	mins := minutos(toks)
	ampm := false
	for _, t := range toks {
		if k := strings.ToLower(t.tok); k == "am/pm" || k == "a/p" {
			ampm = true
		}
	}
	var b strings.Builder
	for i, t := range toks {
		if t.tok == "" {
			b.WriteString(t.lit)
			continue
		}
		b.WriteString(tokenFecha(t.tok, p, mins[i], ampm))
	}
	return b.String()
}

// seccionTranscurrido es `formatElapsedSection`: la unidad entre corchetes ACUMULA
// (26:30 no da la vuelta) y lo que la sigue es su resto.
func seccionTranscurrido(cuerpo string, serial float64) string {
	t := transcurrido{total: int(RedondearJS(math.Abs(serial) * 86400))}
	var b strings.Builder
	if serial < 0 {
		b.WriteByte('-')
	}
	r := []rune(cuerpo)
	bajo := []rune(strings.Map(unicode.ToLower, cuerpo))
	for i := 0; i < len(r); {
		switch r[i] {
		case '"':
			for i++; i < len(r) && r[i] != '"'; i++ {
				b.WriteRune(r[i])
			}
			i++
			continue
		case '\\':
			if i+1 < len(r) {
				b.WriteRune(r[i+1])
			}
			i += 2
			continue
		case '[':
			i = t.corchete(&b, r, bajo, i)
			continue
		}
		if tk := prefijo(bajo, i, tokensTranscurrido); tk != "" {
			b.WriteString(t.cifras(rune(tk[0]), false, len(tk)))
			i += len(tk)
			continue
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

// transcurrido es una duración en segundos, con lo que hace falta para escribir sus
// unidades: la ENTERA (la del corchete, que acumula) o el resto.
type transcurrido struct{ total int }

func (t transcurrido) valor(unidad rune, entera bool) int {
	div, mod := 1, 60
	switch unidad {
	case 'h':
		div, mod = 3600, 24
	case 'm':
		div = 60
	}
	if entera {
		return t.total / div
	}
	return t.total / div % mod
}

// cifras escribe una unidad con `ancho` cifras como mínimo (1 o 2).
func (t transcurrido) cifras(unidad rune, entera bool, ancho int) string {
	v := strconv.Itoa(t.valor(unidad, entera))
	if ancho == 2 && len(v) < 2 {
		v = "0" + v
	}
	return v
}

// corchete escribe un «[h]», «[mm]»… (la unidad entera) o, si no lo es, el corchete
// tal cual; devuelve dónde sigue.
func (t transcurrido) corchete(b *strings.Builder, r, bajo []rune, i int) int {
	if cierre := indiceRuna(r, ']', i); cierre >= 0 {
		dentro := string(bajo[i+1 : cierre])
		if reTranscurrido.MatchString(dentro) {
			u, _ := utf8.DecodeRuneInString(dentro)
			b.WriteString(t.cifras(u, true, len(dentro)))
			return cierre + 1
		}
	}
	b.WriteRune(r[i])
	return i + 1
}

// elegirSeccion decide qué sección se aplica: con condiciones, la primera que casa (o
// la primera sin condición, de reserva); sin ellas, por el signo —positivo;negativo;
// cero—. `menos` dice si hay que poner el «-» delante. false si ninguna vale: General.
func elegirSeccion(secs []string, n float64) (sec string, menos, ok bool) {
	conds := make([]func(float64) bool, len(secs))
	hayCond := false
	for i, s := range secs {
		conds[i] = condicion(s)
		hayCond = hayCond || conds[i] != nil
	}
	switch {
	case hayCond:
		idx := slices.IndexFunc(conds, func(c func(float64) bool) bool { return c != nil && c(n) })
		if idx < 0 {
			idx = slices.IndexFunc(conds, func(c func(float64) bool) bool { return c == nil })
		}
		if idx < 0 {
			return "", false, false
		}
		// Aquí la posición no implica el signo: se conserva, salvo que la máscara lo
		// escriba (el guardia de conMascara evita el doble «-»).
		return secs[idx], n < 0, true
	case n < 0:
		if len(secs) > 1 {
			return secs[1], false, true
		}
		return secs[0], true, true // sin sección de negativos, el «-» automático
	case n == 0 && len(secs) > 2:
		return secs[2], false, true
	}
	return secs[0], false, true
}

func indiceRuna(r []rune, c rune, desde int) int {
	for j := desde; j < len(r); j++ {
		if r[j] == c {
			return j
		}
	}
	return -1
}

// ConMascara es `formatWithMask`: elige la sección por el signo (o por su condición) y
// la aplica. Con una sola sección, los negativos llevan «-» delante.
func ConMascara(mascara string, n float64) (texto, color string) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return formula.FormatNumber(n), ""
	}
	sec, menos, ok := elegirSeccion(secciones(mascara), n)
	if !ok {
		return formula.FormatNumber(n), ""
	}
	color, cuerpo := quitarCorchetes(sec)
	recortado := strings.TrimSpace(cuerpo)
	magnitud := math.Abs(n)
	switch {
	case recortado == "":
		texto, menos = "", false // «0.00;;» esconde el cero
	case strings.EqualFold(recortado, "general"):
		texto = formula.FormatNumber(magnitud)
	case reTranscurridoEn.MatchString(sinLiterales(cuerpo)):
		texto, menos = seccionTranscurrido(cuerpo, n), false
	case esFechaHora(cuerpo):
		texto, menos = seccionFecha(cuerpo, n), false
	default:
		texto = seccionNumero(cuerpo, magnitud)
	}
	if menos && !strings.HasPrefix(texto, "-") {
		texto = "-" + texto
	}
	return texto, color
}
