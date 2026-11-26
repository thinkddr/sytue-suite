// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sieve

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Respuesta es la respuesta automática (RFC 5230, `vacation`) del script propio.
type Respuesta struct {
	Asunto  string
	Mensaje string
	// Dias entre dos respuestas a la misma persona (`:days`), de [DiasMinimo] a
	// [DiasMaximo].
	Dias int
	// De es la dirección desde la que sale la respuesta (`:from`).
	De string
	// Direcciones son las de la persona (`:addresses`): el servidor solo responde a lo
	// que va dirigido a ella.
	Direcciones []string
	// Desde y Hasta son el primer y el último día en que responde, incluidos (con la hora
	// a cero en la zona de la persona); cero, sin límite. Piden las extensiones `date` y
	// `relational` (RFC 5260, RFC 5231).
	Desde, Hasta time.Time
}

// Días entre respuestas a la misma persona.
const (
	DiasMinimo  = 1
	DiasMaximo  = 60
	DiasDeSerie = 7
)

// ErrSinFechas: se pidieron fechas a un servidor sin `date` y `relational`.
var ErrSinFechas = errors.New("sieve: el servidor no admite fechas en la respuesta automática")

// cabecera es la primera línea del script propio: dice quién lo escribe y es la marca de
// que un script con ese nombre es de la app y no de la persona.
const cabecera = "# Sytue Office: respuesta automática (vacation)."

// marcaAnterior empieza el comentario que guarda el script de la persona que estaba
// activo (y que el propio incluye): al apagar la respuesta, vuelve a quedar activo.
const marcaAnterior = "# sytue-office: anterior "

// Componer escribe el script propio: la respuesta automática y, si anterior no está
// vacío, `include :personal :optional` del script de la persona DESPUÉS de ella, con un
// comentario que lo recuerda para restaurarlo. conFechas es si el servidor admite
// `date` y `relational`; sin ellas, unas fechas son [ErrSinFechas]. Fin de línea CRLF.
func Componer(r Respuesta, anterior string, conFechas bool) (string, error) {
	r = Normalizar(r)
	if (!r.Desde.IsZero() || !r.Hasta.IsZero()) && !conFechas {
		return "", ErrSinFechas
	}
	if anterior != "" {
		if err := NombreValido(anterior); err != nil {
			return "", err
		}
	}
	requiere := []string{"vacation"}
	var condiciones []string
	if !r.Desde.IsZero() {
		condiciones = append(condiciones, fecha("ge", r.Desde))
	}
	if !r.Hasta.IsZero() {
		condiciones = append(condiciones, fecha("le", r.Hasta))
	}
	if len(condiciones) > 0 {
		requiere = append(requiere, "date", "relational")
	}
	if anterior != "" {
		requiere = append(requiere, "include")
	}

	var lineas []string
	l := func(s ...string) { lineas = append(lineas, strings.Join(s, "")) }
	l(cabecera)
	l("# Lo escribe Sytue Office: lo que se cambie aquí a mano se pierde al guardar desde la app.")
	if anterior != "" {
		l(marcaAnterior, citar(anterior), " (el script que estaba activo: al desactivar la respuesta automática vuelve a quedar activo)")
	}
	l("require ", listaCadenas(requiere), ";")
	l("")
	sangria := ""
	switch len(condiciones) {
	case 1:
		l("if ", condiciones[0], " {")
		sangria = "    "
	case 2:
		l("if allof (", condiciones[0], ",")
		l("          ", condiciones[1], ") {")
		sangria = "    "
	}
	l(ordenVacation(r, sangria)...)
	if r.Mensaje != "" {
		for _, linea := range strings.Split(r.Mensaje, "\n") {
			if strings.HasPrefix(linea, ".") {
				linea = "." + linea // RFC 5228 §2.4.2: el punto de delante se dobla
			}
			l(linea)
		}
	}
	l(".")
	l(sangria, ";")
	if len(condiciones) > 0 {
		l("}")
	}
	if anterior != "" {
		l("")
		l("# Las reglas de la persona siguen funcionando detrás de la respuesta.")
		l("include :personal :optional ", citar(anterior), ";")
	}
	return strings.Join(lineas, "\r\n") + "\r\n", nil
}

// ordenVacation son los trozos de la línea de `vacation` con sus argumentos, hasta el
// `text:` que abre el mensaje.
func ordenVacation(r Respuesta, sangria string) []string {
	orden := []string{sangria, "vacation :days ", strconv.Itoa(r.Dias)}
	if r.Asunto != "" {
		orden = append(orden, " :subject ", citar(r.Asunto))
	}
	if r.De != "" {
		orden = append(orden, " :from ", citar(r.De))
	}
	if len(r.Direcciones) > 0 {
		orden = append(orden, " :addresses ", listaCadenas(r.Direcciones))
	}
	return append(orden, " text:")
}

// Normalizar deja la respuesta como la escribe [Componer]: UTF-8 válido, sin caracteres
// de control (el asunto y las direcciones en una línea; el mensaje con saltos LF), los
// días dentro de su margen y las fechas como días a las 00:00.
func Normalizar(r Respuesta) Respuesta {
	linea := func(s string) string {
		s = strings.ToValidUTF8(s, "\uFFFD")
		s = strings.Map(func(c rune) rune {
			if c == '\t' || c == '\r' || c == '\n' {
				return ' '
			}
			if unicode.IsControl(c) || c == 0x2028 || c == 0x2029 {
				return -1
			}
			return c
		}, s)
		return strings.TrimSpace(s)
	}
	r.Asunto, r.De = linea(r.Asunto), linea(r.De)
	var dirs []string
	for _, d := range r.Direcciones {
		if d = linea(d); d != "" && !contieneSinMayusculas(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	r.Direcciones = dirs
	m := strings.ToValidUTF8(r.Mensaje, "\uFFFD")
	m = strings.ReplaceAll(m, "\r\n", "\n")
	m = strings.ReplaceAll(m, "\r", "\n")
	m = strings.Map(func(c rune) rune {
		if c == '\n' || c == '\t' {
			return c
		}
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, m)
	r.Mensaje = m
	switch {
	case r.Dias <= 0:
		r.Dias = DiasDeSerie
	case r.Dias > DiasMaximo:
		r.Dias = DiasMaximo
	}
	dia := func(t time.Time) time.Time {
		if t.IsZero() {
			return t
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	r.Desde, r.Hasta = dia(r.Desde), dia(r.Hasta)
	return r
}

func contieneSinMayusculas(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(x, y) {
			return true
		}
	}
	return false
}

// fecha es la prueba de un día: `currentdate :zone "+0100" :value "ge" "date" "AAAA-MM-DD"`,
// con la zona de la persona ese día.
func fecha(relacion string, dia time.Time) string {
	return "currentdate :zone " + citar(dia.Format("-0700")) + " :value " + citar(relacion) + ` "date" ` + citar(dia.Format(time.DateOnly))
}

// citar es s como cadena entre comillas de Sieve: la barra y las comillas, escapadas.
func citar(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func listaCadenas(xs []string) string {
	partes := make([]string, len(xs))
	for i, x := range xs {
		partes[i] = citar(x)
	}
	return "[" + strings.Join(partes, ", ") + "]"
}

// EsPropio dice si un script lo escribió la app ([Componer]).
func EsPropio(script string) bool {
	return strings.HasPrefix(strings.TrimLeft(strings.TrimPrefix(script, "\uFEFF"), " \t\r\n"), cabecera)
}

// LeerRespuesta vuelve a leer la respuesta de un script escrito por [Componer], y el
// script de la persona que guarda para restaurarlo (`anterior`). ok es que tiene una
// orden `vacation`.
func LeerRespuesta(script string) (r Respuesta, anterior string, ok bool) {
	a, err := analizar(script)
	if err != nil || !a.respuesta {
		return Respuesta{}, a.anterior, false
	}
	return a.r, a.anterior, true
}

// ---- Leer Sieve (RFC 5228 §8.1) ----

// pieza es un elemento léxico de Sieve.
type pieza struct {
	// tipo: 'i' identificador (en minúsculas), 't' etiqueta (`:days`, en minúsculas), 'n'
	// número, 's' cadena; o el propio carácter de `[](){},;`.
	tipo  byte
	texto string
	num   int
}

var errLexico = errors.New("sieve: script no válido")

// trocear parte un script en piezas. Lo que no entiende es [errLexico].
func trocear(s string) ([]pieza, error) {
	var ps []pieza
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '#':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			fin := strings.Index(s[i+2:], "*/")
			if fin < 0 {
				return nil, errLexico
			}
			i += fin + 4
		case c == '"', c == ':', c >= '0' && c <= '9', esLetraID(c, false):
			p, sig, err := leerPieza(s, i)
			if err != nil {
				return nil, err
			}
			ps = append(ps, p)
			i = sig
		case strings.IndexByte("[](){},;", c) >= 0:
			ps = append(ps, pieza{tipo: c})
			i++
		default:
			return nil, errLexico
		}
	}
	return ps, nil
}

// leerPieza lee la cadena, la etiqueta, el número o el identificador (o la cadena
// `text:`) que empieza en s[i]. Devuelve dónde sigue el script.
func leerPieza(s string, i int) (pieza, int, error) {
	switch c := s[i]; {
	case c == '"':
		return leerEntrecomillada(s, i)
	case c == ':':
		return leerEtiqueta(s, i)
	case c >= '0' && c <= '9':
		return leerNumero(s, i)
	default:
		return leerIdentificador(s, i)
	}
}

// leerEntrecomillada lee la cadena entre comillas que abre s[i].
func leerEntrecomillada(s string, i int) (pieza, int, error) {
	var b strings.Builder
	i++
	for {
		if i >= len(s) {
			return pieza{}, 0, errLexico
		}
		if s[i] == '"' {
			return pieza{tipo: 's', texto: strings.ReplaceAll(b.String(), "\r\n", "\n")}, i + 1, nil
		}
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
		i++
	}
}

// leerEtiqueta lee la etiqueta (`:days`) que empieza en s[i], en minúsculas.
func leerEtiqueta(s string, i int) (pieza, int, error) {
	j := i + 1
	for j < len(s) && esLetraID(s[j], j > i+1) {
		j++
	}
	if j == i+1 {
		return pieza{}, 0, errLexico
	}
	return pieza{tipo: 't', texto: strings.ToLower(s[i:j])}, j, nil
}

// leerNumero lee el número que empieza en s[i]. Con cuantificador (K, M, G) ya no es de
// días, y queda en -1.
func leerNumero(s string, i int) (pieza, int, error) {
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	n, err := strconv.Atoi(s[i:j])
	if err != nil || j-i > 9 {
		return pieza{}, 0, errLexico
	}
	if j < len(s) && strings.IndexByte("KMGkmg", s[j]) >= 0 {
		j++ // un cuantificador: el número ya no es de días
		n = -1
	}
	return pieza{tipo: 'n', num: n}, j, nil
}

// leerIdentificador lee el identificador que empieza en s[i], en minúsculas; si es
// `text:`, la cadena multilínea que abre.
func leerIdentificador(s string, i int) (pieza, int, error) {
	j := i
	for j < len(s) && esLetraID(s[j], true) {
		j++
	}
	id := strings.ToLower(s[i:j])
	if id == "text" && j < len(s) && s[j] == ':' {
		texto, fin, err := multilinea(s, j+1)
		if err != nil {
			return pieza{}, 0, err
		}
		return pieza{tipo: 's', texto: texto}, fin, nil
	}
	return pieza{tipo: 'i', texto: id}, j, nil
}

func esLetraID(c byte, sigue bool) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (sigue && c >= '0' && c <= '9')
}

// multilinea lee una cadena `text:` desde detrás de los dos puntos: espacios, un
// comentario o el fin de línea, y las líneas hasta la que es solo un punto. El punto que
// se dobló al escribirla se quita. Devuelve el texto (con LF tras cada línea) y dónde
// sigue el script.
func multilinea(s string, i int) (string, int, error) {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i < len(s) && s[i] == '#' {
		for i < len(s) && s[i] != '\n' {
			i++
		}
	}
	if i < len(s) && s[i] == '\r' {
		i++
	}
	if i >= len(s) || s[i] != '\n' {
		return "", 0, errLexico
	}
	i++
	var b strings.Builder
	for {
		if i >= len(s) {
			return "", 0, errLexico
		}
		fin := strings.IndexByte(s[i:], '\n')
		var linea string
		sig := len(s)
		if fin >= 0 {
			linea, sig = s[i:i+fin], i+fin+1
		} else {
			linea = s[i:]
		}
		linea = strings.TrimSuffix(linea, "\r")
		if linea == "." {
			return b.String(), sig, nil
		}
		if strings.HasPrefix(linea, "..") {
			linea = linea[1:]
		}
		b.WriteString(linea)
		b.WriteByte('\n')
		if fin < 0 {
			return "", 0, errLexico
		}
		i = sig
	}
}

// analisis es lo que interesa de un script.
type analisis struct {
	// respuesta: tiene `vacation`; r es lo que dice (la primera que haya).
	respuesta bool
	r         Respuesta
	// anterior es el de la marca del script propio (o, sin marca, el primer include).
	anterior string
	// incluidos son los scripts que incluye; global, si alguno es de los globales del
	// servidor, que no se pueden leer.
	incluidos []string
	global    bool
	// choca: hace algo que no casa con una respuesta automática delante (otra
	// respuesta, rechazar correo).
	choca bool
}

func analizar(script string) (analisis, error) {
	var a analisis
	a.anterior = leerMarca(script)
	ps, err := trocear(script)
	if err != nil {
		return a, err
	}
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if p.tipo != 'i' {
			continue
		}
		switch p.texto {
		case "vacation":
			if !a.respuesta {
				desde, hasta := a.r.Desde, a.r.Hasta // las fechas van delante, en su `if`
				a.r, i = leerVacation(ps, i+1)
				a.r.Desde, a.r.Hasta = desde, hasta
				a.respuesta = true
			}
			a.choca = true
		case "reject", "ereject":
			a.choca = true
		case "currentdate":
			var desde, hasta time.Time
			desde, hasta, i = leerFecha(ps, i+1)
			if !desde.IsZero() && a.r.Desde.IsZero() {
				a.r.Desde = desde
			}
			if !hasta.IsZero() && a.r.Hasta.IsZero() {
				a.r.Hasta = hasta
			}
		case "include":
			i = a.leerInclude(ps, i)
		}
	}
	if a.anterior == "" && len(a.incluidos) > 0 && EsPropio(script) {
		a.anterior = a.incluidos[0]
	}
	return a, nil
}

// leerInclude lee los argumentos de `include` desde i: sus etiquetas (`:global` es de
// los scripts del servidor) y el nombre. Devuelve dónde se quedó.
func (a *analisis) leerInclude(ps []pieza, i int) int {
	for i+1 < len(ps) && ps[i+1].tipo == 't' {
		i++
		if ps[i].texto == ":global" {
			a.global = true
		}
	}
	if i+1 < len(ps) && ps[i+1].tipo == 's' {
		i++
		a.incluidos = append(a.incluidos, ps[i].texto)
	}
	return i
}

// recorrido son las piezas de un script con la que se está leyendo.
type recorrido struct {
	ps []pieza
	i  int
}

// siguiente pasa a la pieza que sigue si es de ese tipo, y la da.
func (c *recorrido) siguiente(tipo byte) (pieza, bool) {
	if c.i+1 < len(c.ps) && c.ps[c.i+1].tipo == tipo {
		c.i++
		return c.ps[c.i], true
	}
	return pieza{}, false
}

// leerLista lee las cadenas de una lista `[…]` cuyo `[` está en ps[i]. Devuelve dónde
// está su `]`.
func leerLista(ps []pieza, i int) ([]string, int) {
	var cadenas []string
	for i+1 < len(ps) && ps[i+1].tipo != ']' {
		i++
		if ps[i].tipo == 's' {
			cadenas = append(cadenas, ps[i].texto)
		}
	}
	return cadenas, i + 1
}

// leerVacation lee los argumentos de `vacation` desde i hasta el `;`.
func leerVacation(ps []pieza, i int) (Respuesta, int) {
	var r Respuesta
	c := &recorrido{ps: ps, i: i}
	for ; c.i < len(ps) && ps[c.i].tipo != ';'; c.i++ {
		p := ps[c.i]
		switch p.tipo {
		case 't':
			argumentoVacation(&r, p.texto, c)
		case 's':
			r.Mensaje = strings.TrimSuffix(p.texto, "\n")
		}
	}
	return r, c.i
}

// argumentoVacation lee en r el argumento de la etiqueta de `vacation` que sigue en c.
func argumentoVacation(r *Respuesta, etiqueta string, c *recorrido) {
	switch etiqueta {
	case ":days":
		if n, ok := c.siguiente('n'); ok && n.num > 0 {
			r.Dias = n.num
		}
	case ":seconds":
		if n, ok := c.siguiente('n'); ok && n.num > 0 {
			r.Dias = max(1, (n.num+86399)/86400)
		}
	case ":subject":
		if s, ok := c.siguiente('s'); ok {
			r.Asunto = s.texto
		}
	case ":from":
		if s, ok := c.siguiente('s'); ok {
			r.De = s.texto
		}
	case ":handle":
		c.siguiente('s')
	case ":addresses":
		if s, ok := c.siguiente('s'); ok {
			r.Direcciones = []string{s.texto}
		} else if _, ok := c.siguiente('['); ok {
			var lista []string
			lista, c.i = leerLista(c.ps, c.i)
			r.Direcciones = append(r.Direcciones, lista...)
		}
	}
}

// leerFecha lee una prueba `currentdate … :value "ge"|"le" "date" "AAAA-MM-DD"`.
func leerFecha(ps []pieza, i int) (desde, hasta time.Time, fin int) {
	relacion := ""
	var cadenas []string
	for ; i < len(ps); i++ {
		p := ps[i]
		if p.tipo == 't' {
			switch p.texto {
			case ":value":
				if i+1 < len(ps) && ps[i+1].tipo == 's' {
					i++
					relacion = strings.ToLower(ps[i].texto)
				}
			case ":zone", ":comparator":
				if i+1 < len(ps) && ps[i+1].tipo == 's' {
					i++
				}
			}
			continue
		}
		if p.tipo == '[' {
			var lista []string
			lista, i = leerLista(ps, i)
			cadenas = append(cadenas, lista...)
			break
		}
		if p.tipo != 's' {
			i-- // no es de la prueba: que lo vea quien sigue
			break
		}
		cadenas = append(cadenas, p.texto)
		if len(cadenas) == 2 {
			break
		}
	}
	if len(cadenas) < 2 || !strings.EqualFold(cadenas[0], "date") {
		return time.Time{}, time.Time{}, i
	}
	dia, err := time.ParseInLocation(time.DateOnly, cadenas[1], time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, i
	}
	switch relacion {
	case "ge":
		desde = dia
	case "gt":
		desde = dia.AddDate(0, 0, 1)
	case "le":
		hasta = dia
	case "lt":
		hasta = dia.AddDate(0, 0, -1)
	}
	return desde, hasta, i
}

// leerMarca es el nombre guardado en el comentario de [marcaAnterior], o "".
func leerMarca(script string) string {
	for _, linea := range strings.Split(script, "\n") {
		resto, ok := strings.CutPrefix(strings.TrimRight(linea, "\r"), marcaAnterior)
		if !ok || !strings.HasPrefix(resto, `"`) {
			continue
		}
		var b strings.Builder
		for i := 1; i < len(resto); i++ {
			switch resto[i] {
			case '\\':
				if i+1 < len(resto) {
					i++
					b.WriteByte(resto[i])
				}
			case '"':
				if n := b.String(); NombreValido(n) == nil {
					return n
				}
				return ""
			default:
				b.WriteByte(resto[i])
			}
		}
		return ""
	}
	return ""
}
