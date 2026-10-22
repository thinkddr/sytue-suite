// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain contiene el núcleo sin estado de Sytue Analytics (ADR 0026):
// la identidad efímera del visitante, el saneado de todo lo que llega del
// navegador, la deducción de navegador/SO/dispositivo desde el user-agent, el
// filtrado de bots y la resolución de país.
//
// Regla que gobierna el paquete: la IP y el user-agent CRUDOS son variables
// locales que entran aquí y no salen. Event —lo único que se persiste— no tiene
// campo para ninguno de los dos, ni para la URL completa, ni para el Referer
// completo, ni para el idioma.
package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"
	"time"
	"unicode"
)

// Límites de longitud de cada campo persistido. Coinciden con los CHECK de la
// migración 0001: el saneado recorta antes y la base rechaza si algo se cuela.
const (
	MaxPath  = 255
	MaxHost  = 253
	MaxName  = 50
	MaxUTM   = 64
	MaxShort = 64
)

// UTMKeys es la ALLOWLIST EXACTA de claves de query string que sobreviven. El
// navegador ya descarta el resto antes de enviar nada (es donde se cuelan
// correos y tokens de sesión); esta lista es la segunda barrera en el servidor,
// que es público y recibe lo que cualquiera quiera mandarle.
var UTMKeys = [6]string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "ref"}

// Eventos automáticos que el script emite sin configuración alguna. No pasan por
// la allowlist del sitio: sus nombres los fija el producto, no el cliente.
const (
	EventPageview = "pageview"
	EventOutbound = "outbound"
	EventDownload = "download"
	EventMailto   = "mailto"
	EventTel      = "tel"
)

var autoEvents = map[string]bool{
	EventPageview: true, EventOutbound: true, EventDownload: true,
	EventMailto: true, EventTel: true,
}

// Event es una fila de analytics_events: TODO lo que Sytue Analytics guarda de
// una visita. Que la lista sea corta es el producto, no una omisión.
type Event struct {
	Tenant   string
	SiteID   string
	TS       time.Time
	Visitor  []byte // 16 bytes; deja de ser recalculable al destruirse la sal del día
	Name     string
	Path     string
	Referrer string
	Target   string
	Country  string
	Browser  string
	OS       string
	Device   string
	UTM      [6]string // en el orden de UTMKeys
}

// Visitor deriva la identidad efímera del visitante:
//
//	HMAC-SHA256(sal_del_día, site_id ‖ ip ‖ user_agent) truncado a 16 bytes
//
// HMAC y no hash(sal‖datos): es la primitiva correcta para hashear con clave y
// está en la stdlib. Los separadores NUL evitan que dos concatenaciones
// distintas produzcan la misma entrada.
//
// Las dos propiedades que compra, y que son imposibilidades, no promesas:
//   - con OTRA sal (otro día) el resultado es incorrelacionable con el de hoy, y
//     la sal de ayer ya no existe;
//   - con OTRO site_id también, así que la misma persona en los sitios de dos
//     clientes no se puede cruzar.
func Visitor(salt []byte, siteID, ip, userAgent string) []byte {
	m := hmac.New(sha256.New, salt)
	m.Write([]byte(siteID))
	m.Write([]byte{0})
	m.Write([]byte(ip))
	m.Write([]byte{0})
	m.Write([]byte(userAgent))
	return m.Sum(nil)[:16]
}

// CleanPath sanea la ruta que envía el navegador: corta en '?' y en '#' (el
// script ya descartó la query string, pero el colector es público y recibe lo
// que cualquiera le mande), fuerza la barra inicial y recorta a MaxPath.
func CleanPath(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSpace(printable(p))
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	// Una ruta larguísima es cardinalidad basura, no información.
	return truncate(p, MaxPath)
}

// CleanHost normaliza un hostname (referente, destino de un enlace saliente):
// minúsculas, sin puerto, sin credenciales y solo con caracteres de hostname.
// Devuelve "" si lo recibido no lo es — nunca una URL, nunca una ruta.
func CleanHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	// Tolerante con quien mande una URL entera: se queda SOLO con el host.
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.IndexByte(h, '@'); i >= 0 {
		h = h[i+1:]
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 && !strings.Contains(h, "]") {
		h = h[:i]
	}
	h = strings.Trim(h, ".")
	if h == "" || len(h) > MaxHost {
		return ""
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		ok := c == '.' || c == '-' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return ""
		}
	}
	if !strings.Contains(h, ".") {
		// "localhost" y demás nombres sin punto no son referentes útiles.
		return ""
	}
	return h
}

// CleanEventName acepta el nombre de evento: los automáticos siempre; los
// declarados con data-sytue-event SOLO si están en la allowlist del sitio. Sin
// esa verja, data-sytue-event="pepe@correo.com" convierte un almacén anónimo en
// un canal de exfiltración. Devuelve "" si hay que descartar el evento.
func CleanEventName(name string, allowlist []string) string {
	name = truncate(strings.TrimSpace(printable(name)), MaxName)
	if name == "" {
		return EventPageview
	}
	if autoEvents[name] {
		return name
	}
	for _, a := range allowlist {
		if a == name {
			return name
		}
	}
	return ""
}

// CleanShort sanea un valor corto declarado (utm_*, ref, detalle del evento):
// sin caracteres de control y recortado. No interpreta nada: es texto opaco.
func CleanShort(s string, maxLen int) string {
	return truncate(strings.TrimSpace(printable(s)), maxLen)
}

// printable elimina los caracteres de control y las runas inválidas: lo que
// llega es texto del navegador y acaba en un JSON de la API y en un panel.
func printable(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return r == unicode.ReplacementChar || unicode.IsControl(r) }) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// truncate recorta a n BYTES sin partir una runa por la mitad.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// Dims son las tres dimensiones que se deducen del user-agent y sustituyen a
// cualquier huella activa: el navegador ya las envía, no hay que preguntar al
// dispositivo por ellas (ni con screen.*, que las Directrices EDPB 2/2023
// tratan como acceso al equipo terminal).
type Dims struct {
	Browser string
	OS      string
	Device  string
}

// browserRules y osRules se recorren EN ORDEN: la primera coincidencia gana,
// porque los user-agents mienten por compatibilidad (Edge dice "Chrome", Chrome
// dice "Safari"). Lo específico va antes que lo genérico.
var browserRules = []struct{ needle, name string }{
	{"edg/", "Edge"},
	{"edga/", "Edge"},
	{"edgios/", "Edge"},
	{"opr/", "Opera"},
	{"opera", "Opera"},
	{"vivaldi", "Vivaldi"},
	{"brave", "Brave"},
	{"duckduckgo", "DuckDuckGo"},
	{"samsungbrowser", "Samsung Internet"},
	{"firefox/", "Firefox"},
	{"fxios/", "Firefox"},
	{"chrome/", "Chrome"},
	{"crios/", "Chrome"},
	{"chromium", "Chrome"},
	{"safari/", "Safari"},
}

var osRules = []struct{ needle, name string }{
	{"windows nt", "Windows"},
	{"windows", "Windows"},
	{"android", "Android"},
	{"iphone", "iOS"},
	{"ipad", "iOS"},
	{"ipod", "iOS"},
	{"cros ", "ChromeOS"},
	{"mac os x", "macOS"},
	{"macintosh", "macOS"},
	{"ubuntu", "Linux"},
	{"fedora", "Linux"},
	{"linux", "Linux"},
	{"freebsd", "BSD"},
	{"openbsd", "BSD"},
}

// ParseUA deduce navegador, sistema y tipo de dispositivo del user-agent.
// Deliberadamente tosco: son tres enums pequeños para un panel, no un motor de
// detección. Lo desconocido es "Other"/"other", no una cadena libre (que sería
// cardinalidad ilimitada en el agregado).
func ParseUA(ua string) Dims {
	lo := strings.ToLower(ua)
	d := Dims{Browser: "Other", OS: "Other", Device: "desktop"}
	for _, r := range browserRules {
		if strings.Contains(lo, r.needle) {
			d.Browser = r.name
			break
		}
	}
	for _, r := range osRules {
		if strings.Contains(lo, r.needle) {
			d.OS = r.name
			break
		}
	}
	switch {
	case strings.Contains(lo, "ipad") || strings.Contains(lo, "tablet") ||
		(strings.Contains(lo, "android") && !strings.Contains(lo, "mobile")):
		d.Device = "tablet"
	case strings.Contains(lo, "mobi") || strings.Contains(lo, "iphone") ||
		strings.Contains(lo, "ipod") || strings.Contains(lo, "android"):
		d.Device = "mobile"
	case d.OS == "Other" && d.Browser == "Other":
		d.Device = "other"
	}
	return d
}
