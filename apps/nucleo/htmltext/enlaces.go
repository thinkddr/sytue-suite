// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package htmltext

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tramo es un trozo del texto de un correo: texto llano, o un enlace con su Destino.
// Juntando los Texto de todos los tramos sale el texto de partida, letra a letra.
type Tramo struct {
	Texto string `json:"texto"`
	// Destino es lo que se abre al pulsarlo (`https://…` o `mailto:…`); vacío, texto.
	Destino string `json:"destino,omitempty"`
}

// Las reglas son las de `enlazarTexto` en apps/frontend/apps/mail/src/lib/enlaces.ts,
// y los vectores de testdata/enlaces.json los recorren las dos: el mismo correo tiene
// que enlazar lo mismo en la web y en Fyne.
//
// Lo que NO puede ir dentro de un enlace es todo blanco de Unicode (los de
// `unicode.IsSpace`, que son por donde parte las líneas `ui.Repartir`, y los de `\s` de
// JavaScript), las comillas dobles, los ángulos —`<https://…>` es como se cita una
// dirección en texto plano— y los caracteres de control.
//
// Sin `(?i)`: en Go pliega también la «ſ» y el signo de kelvin sobre la «s» y la «k», y
// «httpſ://» salía como destino aunque no es http(s), cosa que `/i` de JavaScript no
// hace. Las mayúsculas se escriben a mano, solo las ASCII.
//
// direccion es una dirección de correo: algo, arroba, y un dominio con su punto.
const direccion = `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)*\.[a-zA-Z]{2,}`

const fueraDeEnlace = `\x00-\x20\x7f-\x{a0}\x{1680}\x{2000}-\x{200b}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}<>"`

var (
	reURL    = regexp.MustCompile(`(?:[hH][tT][tT][pP][sS]?://|[wW][wW][wW]\.)[^` + fueraDeEnlace + `]+`)
	reCorreo = regexp.MustCompile(`(?:[mM][aA][iI][lL][tT][oO]:)?` + direccion)
	// reDireccion es una dirección de correo ENTERA, sin nada más.
	reDireccion = regexp.MustCompile(`^` + direccion + `$`)
)

// Enlaces parte un texto plano en tramos, con las direcciones web (`http://`,
// `https://`, `www.`) y de correo convertidas en enlaces. Solo salen destinos http(s) y
// mailto: nada que el sistema pudiera ejecutar.
//
// La puntuación con la que acaba una frase no es de la dirección («visita
// https://sytue.eu.»), y un paréntesis de cierre solo lo es si abre otro dentro
// («(https://es.wikipedia.org/wiki/Gato_(desambiguación))»).
func Enlaces(texto string) []Tramo {
	var out []Tramo
	llano := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Destino == "" {
			out[n-1].Texto += s
			return
		}
		out = append(out, Tramo{Texto: s})
	}
	pos := 0
	for pos < len(texto) {
		ini, fin, destino := siguienteEnlace(texto, pos)
		if ini < 0 {
			break
		}
		llano(texto[pos:ini])
		out = append(out, Tramo{Texto: texto[ini:fin], Destino: destino})
		pos = fin
	}
	llano(texto[pos:])
	return out
}

// siguienteEnlace busca desde `pos` el primer enlace válido: el que empieza antes, y
// entre una web y un correo que empiezan igual, la web.
func siguienteEnlace(s string, pos int) (ini, fin int, destino string) {
	iu, fu, du := buscarURL(s, pos)
	ic, fc, dc := buscarCorreo(s, pos)
	switch {
	case iu < 0 && ic < 0:
		return -1, -1, ""
	case ic < 0 || (iu >= 0 && iu <= ic):
		return iu, fu, du
	}
	return ic, fc, dc
}

func buscarURL(s string, pos int) (int, int, string) {
	for pos < len(s) {
		m := reURL.FindStringIndex(s[pos:])
		if m == nil {
			return -1, -1, ""
		}
		ini, fin := pos+m[0], pos+m[1]
		if antes, _ := utf8.DecodeLastRuneInString(s[:ini]); ini == 0 || !pegado(antes) {
			fin = ini + recortarCola(s[ini:fin])
			cand := s[ini:fin]
			if d := destinoURL(cand); d != "" {
				return ini, fin, d
			}
		}
		// «xhttps://» o «correo@www.algo»: no empieza aquí; se sigue una letra después.
		_, ancho := utf8.DecodeRuneInString(s[ini:])
		pos = ini + ancho
	}
	return -1, -1, ""
}

func buscarCorreo(s string, pos int) (int, int, string) {
	for pos < len(s) {
		m := reCorreo.FindStringIndex(s[pos:])
		if m == nil {
			return -1, -1, ""
		}
		ini, fin := pos+m[0], pos+m[1]
		despues, _ := utf8.DecodeRuneInString(s[fin:])
		// «ana@sytue.eu2» o «ana@sytue.eu_x» no son una dirección que acabe en «.eu».
		if fin == len(s) || (!esAlfanumerico(despues) && despues != '_' && despues != '-') {
			cand := s[ini:fin]
			dir := cand
			if len(dir) > 7 && strings.EqualFold(dir[:7], "mailto:") {
				dir = dir[7:]
			}
			return ini, fin, "mailto:" + dir
		}
		pos = fin
	}
	return -1, -1, ""
}

// pegado dice si una letra delante de «https://» o «www.» hace que no sea el principio
// de una dirección: «xwww.algo», «correo@www.algo», «/www.algo».
func pegado(r rune) bool {
	return esAlfanumerico(r) || strings.ContainsRune("_@./-+%", r)
}

func esAlfanumerico(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// recortarCola devuelve cuánto de `u` es dirección: sin la puntuación final y sin los
// cierres que no abren dentro.
func recortarCola(u string) int {
	fin := len(u)
	for fin > 0 {
		c := u[fin-1]
		switch c {
		case '.', ',', ';', ':', '!', '?', '\'', '*':
			fin--
			continue
		case ')', ']', '}':
			abre := map[byte]byte{')': '(', ']': '[', '}': '{'}[c]
			if strings.Count(u[:fin], string(abre)) < strings.Count(u[:fin], string(c)) {
				fin--
				continue
			}
		}
		break
	}
	return fin
}

// destinoURL es lo que se abre: la dirección tal cual, o `https://` delante de un
// «www.». Vacío si no queda un nombre de máquina (un «https://» suelto) o si un «www.»
// no lleva dominio detrás («www.algo» es una palabra con un punto).
//
// La máquina se saca a mano y no con url.Parse, que rechaza una dirección entera por un
// `%` mal escrito: la web no la rechaza, y las dos tienen que enlazar lo mismo.
func destinoURL(u string) string {
	www := len(u) >= 4 && strings.EqualFold(u[:4], "www.")
	d, resto := u, u
	if www {
		d = "https://" + u
	} else if i := strings.Index(resto, "://"); i >= 0 {
		resto = resto[i+3:]
	}
	maquina := resto
	if i := strings.IndexAny(maquina, "/?#"); i >= 0 {
		maquina = maquina[:i]
	}
	if i := strings.LastIndexByte(maquina, '@'); i >= 0 {
		maquina = maquina[i+1:]
	}
	if i := strings.LastIndexByte(maquina, ':'); i >= 0 {
		maquina = maquina[:i]
	}
	if !strings.ContainsFunc(maquina, esAlfanumerico) {
		return ""
	}
	if www && !strings.Contains(strings.Trim(maquina[4:], "."), ".") {
		return ""
	}
	return d
}

// Los marcadores de TextoConEnlaces: Text quita todo lo que parece una etiqueta, así
// que el `<…>` del destino se escribe con dos caracteres de uso privado y se pone al
// final. Un correo no los trae: si llegaran, se borran antes.
const (
	abreDestino   = ""
	cierraDestino = ""
)

var (
	reAncla = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a\s*>`)
	reHref  = regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// TextoConEnlaces es Text sin perder adónde llevan los enlaces: cada `<a href>` http(s)
// o mailto queda como «texto <destino>», que es como se escribe un enlace en un correo
// de texto plano. Así el destino real se ve SIEMPRE junto al texto que puso el
// remitente —un «Restablece tu contraseña» que lleva a otro dominio lo enseña— y el
// lector lo enlaza con Enlaces como cualquier otra dirección.
//
// Si el texto ya es el destino («https://sytue.eu», «ana@sytue.eu») no se repite, y un
// enlace sin texto (una imagen) no deja nada: en un boletín serían decenas de
// direcciones de seguimiento sin nada que las explique. Text no cambia: lo usan las
// notas, las diapositivas y los widgets, que no enseñan destinos.
func TextoConEnlaces(s string) string {
	s = strings.NewReplacer(abreDestino, "", cierraDestino, "").Replace(s)
	s = reOculto.ReplaceAllString(s, "")
	s = reAncla.ReplaceAllStringFunc(s, func(a string) string {
		m := reAncla.FindStringSubmatch(a)
		h := reHref.FindStringSubmatch(m[1])
		if h == nil {
			return m[2]
		}
		destino := strings.TrimSpace(html.UnescapeString(h[1] + h[2] + h[3]))
		visible := Text(m[2])
		muestra := destinoVisible(destino)
		if muestra == "" || strings.TrimSpace(visible) == "" || mismoDestino(visible, muestra) {
			return m[2]
		}
		return m[2] + " " + abreDestino + html.EscapeString(muestra) + cierraDestino
	})
	return strings.NewReplacer(abreDestino, "<", cierraDestino, ">").Replace(Text(s))
}

// destinoVisible es cómo se escribe el destino en el texto: la dirección web entera, o
// la de correo sin «mailto:» ni lo que vaya tras «?». Vacío si no es http(s) ni mailto,
// o si no se podría leer como un único enlace (lleva blancos, comillas o ángulos).
func destinoVisible(destino string) string {
	if destino == "" || strings.ContainsAny(destino, "<>\" \t\r\n") {
		return ""
	}
	u, err := url.Parse(destino)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
		return destino
	case "mailto":
		dir, _, _ := strings.Cut(destino[len("mailto:"):], "?")
		if dir, err = url.PathUnescape(dir); err != nil || !reDireccion.MatchString(dir) {
			return ""
		}
		return dir
	}
	return ""
}

// mismoDestino dice si el texto del enlace ya es su destino, sin contar el esquema, el
// «www.» ni la barra final.
func mismoDestino(visible, destino string) bool {
	limpio := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		for _, p := range []string{"https://", "http://", "mailto:", "www."} {
			s = strings.TrimPrefix(s, p)
		}
		return strings.TrimSuffix(s, "/")
	}
	return limpio(visible) == limpio(destino)
}
