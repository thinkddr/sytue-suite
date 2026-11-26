// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package dav es el cliente CalDAV (RFC 4791) y CardDAV (RFC 6352) que SINCRONIZA:
// lista las colecciones de una cuenta, baja lo que cambió desde la última vez
// (`sync-collection`, RFC 6578; o `getctag` y las ETag de cada objeto donde el servidor
// no la tiene) y sube cada objeto con su ETag (`If-Match`, `If-None-Match: *`) para no
// pisar nunca un cambio que no se ha visto (PLAN-EDICION-LOCAL §5.10, §6.11; A-10, K-05).
//
// Trabaja con el iCalendar y la vCard CRUDOS: lo que el servidor guarda es lo que se
// guarda en el equipo, sin pasar por un modelo intermedio que pierda propiedades.
//
// La red la pone quien lo usa ([Nuevo] recibe el *http.Client): en Sytue Office es el
// portero (`local/red`). Además, este paquete solo habla con los hosts de la cuenta
// (el de su dirección y los que se apuntaron al darla de alta, [Cuenta.Hosts]): un href
// que apunta a otro sitio se ignora, y la contraseña no sale nunca hacia él.
//
// No reutiliza el cliente de `go-webdav` porque devuelve las rutas sin el host (iCloud
// reparte las cuentas en servidores `pNN-caldav.icloud.com` con hrefs absolutos), no
// manda `If-Match` y no tiene `sync-collection` en CalDAV; el XML de cada petición es
// corto y está aquí. El servidor de `go-webdav` sí se usa en las pruebas.
package dav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Clase es lo que guarda una colección.
type Clase string

// Clases de colección.
const (
	Calendario Clase = "calendario"
	Libreta    Clase = "libreta"
)

// Errores del servidor que el sincronizador distingue.
var (
	// ErrConflicto: el objeto cambió en el servidor desde la ETag que se mandó (412).
	ErrConflicto = errors.New("dav: el objeto cambió en el servidor")
	// ErrAcceso: el servidor rechazó la contraseña (401) o el permiso (403).
	ErrAcceso = errors.New("dav: el servidor rechazó el acceso")
	// ErrNoEsta: el objeto o la colección no existe (404, 410).
	ErrNoEsta = errors.New("dav: no existe en el servidor")
	// ErrFichaCaducada: el servidor ya no reconoce la ficha de sincronización.
	ErrFichaCaducada = errors.New("dav: ficha de sincronización caducada")
	// ErrServidor: cualquier otra respuesta que no es un éxito.
	ErrServidor = errors.New("dav: el servidor respondió con un error")
)

// MaxRespuesta acota lo que se lee de una respuesta: una libreta de varios miles de
// tarjetas con foto cabe de sobra.
const MaxRespuesta = 64 << 20

const (
	nsDAV    = "DAV:"
	nsCalDAV = "urn:ietf:params:xml:ns:caldav"
	nsCard   = "urn:ietf:params:xml:ns:carddav"
	nsCS     = "http://calendarserver.org/ns/"
	nsApple  = "http://apple.com/ns/ical/"
)

// Cuenta es lo que hace falta para hablar con un servidor DAV.
type Cuenta struct {
	// Direccion es la URL de contexto del servicio («https://caldav.fastmail.com/dav/»):
	// desde ella se busca el principal.
	Direccion string
	Usuario   string
	Clave     string
	// Hosts son otros servidores de la misma cuenta a los que el servidor manda
	// (iCloud: `p42-caldav.icloud.com`). Se apuntan al descubrir; sin ellos, solo se
	// habla con el host de Direccion.
	Hosts []string
}

// Cliente habla con un servidor DAV de una cuenta.
type Cliente struct {
	base  *url.URL
	hosts []string
	hc    *http.Client
}

// Nuevo prepara el cliente; no abre ninguna conexión. hc es la red (el portero).
func Nuevo(c Cuenta, hc *http.Client) (*Cliente, error) {
	if hc == nil {
		return nil, errors.New("dav: falta el cliente de red")
	}
	u, err := url.Parse(strings.TrimSpace(c.Direccion))
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, fmt.Errorf("dav: dirección no válida %q", c.Direccion)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	hosts := []string{strings.ToLower(u.Host)}
	for _, h := range c.Hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	cl := &Cliente{base: u, hosts: hosts}
	guardado := *hc
	guardado.Transport = &autenticar{base: transporteDe(hc), usuario: c.Usuario, clave: c.Clave, hosts: hosts}
	guardado.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !cl.permitido(req.URL) {
			return errors.New("dav: redirección a un servidor que no es de la cuenta")
		}
		return nil
	}
	cl.hc = &guardado
	return cl, nil
}

func transporteDe(hc *http.Client) http.RoundTripper {
	if hc.Transport != nil {
		return hc.Transport
	}
	return http.DefaultTransport
}

// autenticar pone el usuario y la contraseña SOLO hacia los hosts de la cuenta.
type autenticar struct {
	base           http.RoundTripper
	usuario, clave string
	hosts          []string
}

func (a *autenticar) RoundTrip(r *http.Request) (*http.Response, error) {
	if a.usuario != "" || a.clave != "" {
		if deLaCuenta(a.hosts, r.URL.Host) {
			r = r.Clone(r.Context())
			r.SetBasicAuth(a.usuario, a.clave)
		}
	}
	return a.base.RoundTrip(r)
}

// Hosts son los servidores con los que habla este cliente.
func (c *Cliente) Hosts() []string { return slices.Clone(c.hosts) }

func (c *Cliente) permitido(u *url.URL) bool {
	return u != nil && u.Scheme == c.base.Scheme && deLaCuenta(c.hosts, u.Host) && u.User == nil
}

// deLaCuenta dice si host (con su puerto, si lo lleva) es de la cuenta. Un host con un
// punto delante («.icloud.com») vale para sus subdominios: solo lo ponen los preajustes
// de un proveedor, nunca lo que escribe la persona.
func deLaCuenta(hosts []string, host string) bool {
	host = strings.ToLower(host)
	for _, h := range hosts {
		if h == host || (strings.HasPrefix(h, ".") && strings.HasSuffix(host, h) && len(host) > len(h) && !strings.Contains(host, ":")) {
			return true
		}
	}
	return false
}

// resolver pasa un href del servidor a URL absoluta, o "" si no es de la cuenta.
func (c *Cliente) resolver(desde *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	r, err := url.Parse(href)
	if err != nil {
		return ""
	}
	u := desde.ResolveReference(r)
	u.Fragment, u.RawQuery = "", ""
	if !c.permitido(u) {
		return ""
	}
	return u.String()
}

// ----- Peticiones -----

type peticion struct {
	metodo, destino string
	cuerpo          []byte
	tipo            string
	cabeceras       map[string]string
}

func (c *Cliente) hacer(ctx context.Context, p peticion) (*http.Response, []byte, error) {
	u, err := url.Parse(p.destino)
	if err != nil || !c.permitido(u) {
		return nil, nil, fmt.Errorf("dav: %q no es de la cuenta", p.destino)
	}
	var cuerpo io.Reader = http.NoBody
	if p.cuerpo != nil {
		cuerpo = bytes.NewReader(p.cuerpo)
	}
	req, err := http.NewRequestWithContext(ctx, p.metodo, u.String(), cuerpo)
	if err != nil {
		return nil, nil, err
	}
	if p.tipo != "" {
		req.Header.Set("Content-Type", p.tipo)
	}
	for k, v := range p.cabeceras {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "SytueOffice")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, MaxRespuesta+1))
	if err != nil {
		return resp, nil, err
	}
	if len(datos) > MaxRespuesta {
		return resp, nil, fmt.Errorf("%w: respuesta demasiado grande", ErrServidor)
	}
	return resp, datos, nil
}

func errorDe(resp *http.Response, cuerpo []byte) error {
	if formatoRechazado(resp.StatusCode, cuerpo) {
		return fmt.Errorf("%w: %s", ErrFormato, resp.Status)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrAcceso
	case http.StatusForbidden:
		if bytes.Contains(cuerpo, []byte("valid-sync-token")) {
			return ErrFichaCaducada
		}
		return ErrAcceso
	case http.StatusNotFound, http.StatusGone:
		return ErrNoEsta
	case http.StatusPreconditionFailed:
		return ErrConflicto
	case http.StatusConflict:
		if bytes.Contains(cuerpo, []byte("valid-sync-token")) {
			return ErrFichaCaducada
		}
		return ErrConflicto
	}
	return fmt.Errorf("%w: %s", ErrServidor, resp.Status)
}

// multiestado pide un PROPFIND o un REPORT y lee la respuesta 207.
func (c *Cliente) multiestado(ctx context.Context, metodo, destino, profundidad, cuerpo string) (*multistatus, *url.URL, error) {
	resp, datos, err := c.hacer(ctx, peticion{metodo: metodo, destino: destino, cuerpo: []byte(cuerpo), tipo: "application/xml; charset=utf-8", cabeceras: map[string]string{"Depth": profundidad}}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, nil, errorDe(resp, datos)
	}
	var ms multistatus
	if err := xml.Unmarshal(datos, &ms); err != nil {
		return nil, nil, fmt.Errorf("%w: XML ilegible: %w", ErrServidor, err)
	}
	return &ms, resp.Request.URL, nil
}

// ----- XML -----

type multistatus struct {
	XMLName   xml.Name    `xml:"DAV: multistatus"`
	Responses []respuesta `xml:"DAV: response"`
	SyncToken string      `xml:"DAV: sync-token"`
}

type respuesta struct {
	Href     []string   `xml:"DAV: href"`
	Status   string     `xml:"DAV: status"`
	Propstat []propstat `xml:"DAV: propstat"`
}

type propstat struct {
	Status string `xml:"DAV: status"`
	Prop   prop   `xml:"DAV: prop"`
}

type enlace struct {
	Href string `xml:"DAV: href"`
}

type vacio struct{}

type prop struct {
	ResourceType *struct {
		Collection  *vacio `xml:"DAV: collection"`
		Calendar    *vacio `xml:"urn:ietf:params:xml:ns:caldav calendar"`
		AddressBook *vacio `xml:"urn:ietf:params:xml:ns:carddav addressbook"`
	} `xml:"DAV: resourcetype"`
	DisplayName string  `xml:"DAV: displayname"`
	ETag        string  `xml:"DAV: getetag"`
	CTag        string  `xml:"http://calendarserver.org/ns/ getctag"`
	SyncToken   string  `xml:"DAV: sync-token"`
	Principal   *enlace `xml:"DAV: current-user-principal"`
	CalHome     *enlace `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	CardHome    *enlace `xml:"urn:ietf:params:xml:ns:carddav addressbook-home-set"`
	// Outbox y Direcciones son de la planificación (RFC 6638 §2.1, §2.4.1).
	Outbox      *enlace `xml:"urn:ietf:params:xml:ns:caldav schedule-outbox-URL"`
	Direcciones *struct {
		Href []string `xml:"DAV: href"`
	} `xml:"urn:ietf:params:xml:ns:caldav calendar-user-address-set"`
	Privilegios *struct {
		Privilegio []struct {
			Write        *vacio `xml:"DAV: write"`
			WriteContent *vacio `xml:"DAV: write-content"`
			All          *vacio `xml:"DAV: all"`
			Bind         *vacio `xml:"DAV: bind"`
		} `xml:"DAV: privilege"`
	} `xml:"DAV: current-user-privilege-set"`
	Color       string `xml:"http://apple.com/ns/ical/ calendar-color"`
	CalDesc     string `xml:"urn:ietf:params:xml:ns:caldav calendar-description"`
	CardDesc    string `xml:"urn:ietf:params:xml:ns:carddav addressbook-description"`
	Componentes *struct {
		Comp []struct {
			Name string `xml:"name,attr"`
		} `xml:"urn:ietf:params:xml:ns:caldav comp"`
	} `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
	CalData  string `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	CardData string `xml:"urn:ietf:params:xml:ns:carddav address-data"`
}

// bien es si un estado HTTP de la respuesta es 2xx (sin estado, se da por bueno).
func bien(status string) bool {
	if status == "" {
		return true
	}
	campos := strings.Fields(status)
	return len(campos) >= 2 && strings.HasPrefix(campos[1], "2")
}

// props junta las propiedades de los propstat buenos: una propiedad que el servidor
// no tiene llega vacía en un propstat 404 y no debe pisar a la buena.
func (r respuesta) props() (prop, bool) {
	var p prop
	alguna := false
	for _, ps := range r.Propstat {
		if !bien(ps.Status) {
			continue
		}
		alguna = true
		q := ps.Prop
		if q.ResourceType != nil {
			p.ResourceType = q.ResourceType
		}
		for _, par := range [][2]*string{
			{&p.DisplayName, &q.DisplayName},
			{&p.ETag, &q.ETag},
			{&p.CTag, &q.CTag},
			{&p.SyncToken, &q.SyncToken},
			{&p.Color, &q.Color},
			{&p.CalDesc, &q.CalDesc},
			{&p.CardDesc, &q.CardDesc},
			{&p.CalData, &q.CalData},
			{&p.CardData, &q.CardData},
		} {
			if *par[1] != "" {
				*par[0] = *par[1]
			}
		}
		for _, par := range [][2]**enlace{{&p.Principal, &q.Principal}, {&p.CalHome, &q.CalHome}, {&p.CardHome, &q.CardHome}, {&p.Outbox, &q.Outbox}} {
			if *par[1] != nil {
				*par[0] = *par[1]
			}
		}
		if q.Privilegios != nil {
			p.Privilegios = q.Privilegios
		}
		if q.Componentes != nil {
			p.Componentes = q.Componentes
		}
		if q.Direcciones != nil {
			p.Direcciones = q.Direcciones
		}
	}
	return p, alguna
}

func escapar(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// ETag normaliza una ETag tal como se manda en If-Match: entre comillas.
func ETag(e string) string {
	e = strings.TrimSpace(e)
	if e == "" || strings.HasPrefix(e, `"`) || strings.HasPrefix(e, "W/") {
		return e
	}
	return `"` + e + `"`
}

// ----- Principal y colecciones -----

// Principal encuentra la URL de la persona y las de sus casas de calendarios y de
// libretas (las que haya). Sirve también de prueba de la cuenta: una contraseña mala
// da ErrAcceso.
func (c *Cliente) Principal(ctx context.Context) (principal, calendarios, libretas string, err error) {
	ms, desde, err := c.multiestado(ctx, "PROPFIND", c.base.String(), "0",
		`<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`)
	if err != nil {
		return "", "", "", err
	}
	for _, r := range ms.Responses {
		if p, ok := r.props(); ok && p.Principal != nil {
			principal = c.resolver(desde, p.Principal.Href)
		}
	}
	if principal == "" {
		return "", "", "", fmt.Errorf("%w: sin current-user-principal", ErrServidor)
	}
	ms, desde, err = c.multiestado(ctx, "PROPFIND", principal, "0",
		`<d:propfind xmlns:d="DAV:" xmlns:c="`+nsCalDAV+`" xmlns:card="`+nsCard+`"><d:prop><c:calendar-home-set/><card:addressbook-home-set/></d:prop></d:propfind>`)
	if err != nil {
		return principal, "", "", err
	}
	for _, r := range ms.Responses {
		p, ok := r.props()
		if !ok {
			continue
		}
		if p.CalHome != nil {
			calendarios = c.resolver(desde, p.CalHome.Href)
		}
		if p.CardHome != nil {
			libretas = c.resolver(desde, p.CardHome.Href)
		}
	}
	return principal, calendarios, libretas, nil
}

// Coleccion es un calendario o una libreta del servidor.
type Coleccion struct {
	URL    string
	Clase  Clase
	Nombre string
	// NombreServidor es el displayname tal como llega (vacío si el servidor no lo da;
	// Nombre lleva entonces el último trozo de la ruta).
	NombreServidor string
	Descripcion    string
	Color          string
	SoloLectura    bool
	CTag           string
	// SyncToken no vacío: el servidor sabe `sync-collection`.
	SyncToken string
	// Componentes: los del calendario (VEVENT, VTODO…); vacío, cualquiera.
	Componentes []string
}

// Colecciones lista los calendarios (o las libretas) de una casa. Un calendario solo de
// tareas (VTODO) no sale: no es de Calendar.
func (c *Cliente) Colecciones(ctx context.Context, casa string, clase Clase) ([]Coleccion, error) {
	return c.colecciones(ctx, casa, clase, true)
}

func (c *Cliente) colecciones(ctx context.Context, casa string, clase Clase, soloEventos bool) ([]Coleccion, error) {
	cuerpo := `<d:propfind xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `" xmlns:card="` + nsCard + `" xmlns:cs="` + nsCS + `" xmlns:ic="` + nsApple + `"><d:prop>` +
		`<d:resourcetype/><d:displayname/><cs:getctag/><d:sync-token/><d:current-user-privilege-set/>` +
		`<ic:calendar-color/><c:calendar-description/><card:addressbook-description/><c:supported-calendar-component-set/>` +
		`</d:prop></d:propfind>`
	ms, desde, err := c.multiestado(ctx, "PROPFIND", casa, "1", cuerpo)
	if err != nil {
		return nil, err
	}
	var out []Coleccion
	for _, r := range ms.Responses {
		p, ok := r.props()
		if !ok || p.ResourceType == nil || len(r.Href) == 0 {
			continue
		}
		if (clase == Calendario && p.ResourceType.Calendar == nil) || (clase == Libreta && p.ResourceType.AddressBook == nil) {
			continue
		}
		u := c.resolver(desde, r.Href[0])
		if u == "" {
			continue
		}
		col := Coleccion{URL: u, Clase: clase, Nombre: strings.TrimSpace(p.DisplayName), NombreServidor: p.DisplayName, CTag: p.CTag, SyncToken: p.SyncToken}
		col.Descripcion = p.CalDesc
		if clase == Libreta {
			col.Descripcion = p.CardDesc
		}
		if col.Nombre == "" {
			col.Nombre = ultimo(u)
		}
		col.Color = colorCSS(p.Color)
		col.Componentes = p.componentes()
		col.SoloLectura = p.soloLectura()
		if soloEventos && clase == Calendario && len(col.Componentes) > 0 && !slices.Contains(col.Componentes, "VEVENT") {
			// Un calendario solo de tareas (VTODO) no es de Calendar.
			continue
		}
		out = append(out, col)
	}
	return out, nil
}

// componentes son los del conjunto de componentes del calendario (VEVENT, VTODO…), en
// mayúsculas; nil si no lo da.
func (p prop) componentes() []string {
	if p.Componentes == nil || len(p.Componentes.Comp) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Componentes.Comp))
	for _, x := range p.Componentes.Comp {
		out = append(out, strings.ToUpper(x.Name))
	}
	return out
}

// soloLectura dice si ningún privilegio deja escribir en la colección. Sin conjunto de
// privilegios se da por escribible: el PUT dirá que no.
func (p prop) soloLectura() bool {
	if p.Privilegios == nil {
		return false
	}
	for _, pv := range p.Privilegios.Privilegio {
		if pv.Write != nil || pv.WriteContent != nil || pv.All != nil || pv.Bind != nil {
			return false
		}
	}
	return true
}

func ultimo(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	partes := strings.Split(strings.Trim(p.Path, "/"), "/")
	n, _ := url.PathUnescape(partes[len(partes)-1])
	return n
}

// colorCSS pasa «#RRGGBBAA» (el de Apple) a «#RRGGBB».
func colorCSS(c string) string {
	c = strings.TrimSpace(c)
	if len(c) == 9 && c[0] == '#' {
		return strings.ToUpper(c[:7])
	}
	if len(c) == 7 && c[0] == '#' {
		return strings.ToUpper(c)
	}
	return ""
}

// ----- Objetos -----

// Objeto es un recurso de una colección: un VCALENDAR con un UID o una vCard.
type Objeto struct {
	URL  string
	ETag string
	// Datos es el iCalendar o la vCard; vacío si solo se pidió la ETag.
	Datos []byte
	// Tipo es el Content-Type con que llegó (solo en LeerComo).
	Tipo string
}

// Cambios es lo que cambió en una colección desde una ficha.
type Cambios struct {
	// Puestos son los objetos nuevos o cambiados, con sus datos.
	Puestos []Objeto
	// Fuera son objetos que existen pero no se bajan: en la primera bajada de un
	// calendario, los que acabaron antes de la ventana (URL y ETag, sin datos).
	Fuera []Objeto
	// Borrados son las URL de los que ya no están.
	Borrados []string
	// Completo: Vistos es la colección ENTERA; lo conocido que no está, se borró.
	Completo bool
	Vistos   []string
	// Ficha es la que se guarda para la próxima vez.
	Ficha string
}

// Fichas: con `sync-collection`, su sync-token; sin él, el getctag.
const (
	prefijoSync = "sync:"
	prefijoCTag = "ctag:"
)

// Cambios baja lo que cambió en col desde ficha ("" la primera vez). etagDe dice la
// ETag que se tiene de un objeto, para no bajar lo que no cambió cuando el servidor no
// sabe `sync-collection`. desde, si no es cero, es la ventana de la PRIMERA bajada de un
// calendario: solo se bajan los eventos que acaban después (`calendar-query` con
// time-range); el resto queda en Fuera y se baja si cambia.
func (c *Cliente) Cambios(ctx context.Context, col Coleccion, ficha string, etagDe func(url string) (string, bool), desde time.Time) (Cambios, error) {
	if etagDe == nil {
		etagDe = func(string) (string, bool) { return "", false }
	}
	if col.SyncToken != "" && !strings.HasPrefix(ficha, prefijoCTag) {
		ca, err := c.porSync(ctx, col, strings.TrimPrefix(ficha, prefijoSync), etagDe, desde)
		if errors.Is(err, ErrFichaCaducada) && ficha != "" {
			// Con la ficha caducada se vuelve a empezar: lo que no venga, se borró.
			ca, err = c.porSync(ctx, col, "", etagDe, time.Time{})
			if err == nil {
				ca.Completo = true
			}
		}
		if err == nil || !esNoSabe(err) {
			return ca, err
		}
	}
	return c.porETags(ctx, col, ficha, etagDe, desde)
}

// esNoSabe: un servidor que anuncia sync-token pero no atiende el REPORT.
func esNoSabe(err error) bool {
	return errors.Is(err, ErrServidor)
}

func (c *Cliente) porSync(ctx context.Context, col Coleccion, token string, etagDe func(string) (string, bool), desde time.Time) (Cambios, error) {
	var ca Cambios
	cambiados := map[string]string{}
	var orden []string
	primera := token == ""
	for vuelta := 0; vuelta < 50; vuelta++ {
		cuerpo := `<d:sync-collection xmlns:d="DAV:"><d:sync-token>` + escapar(token) + `</d:sync-token><d:sync-level>1</d:sync-level><d:prop><d:getetag/></d:prop></d:sync-collection>`
		ms, desdeURL, err := c.multiestado(ctx, "REPORT", col.URL, "0", cuerpo)
		if err != nil {
			return ca, err
		}
		truncada := c.apuntarSync(col, ms, desdeURL, cambiados, &orden, &ca)
		if ms.SyncToken == "" {
			return ca, fmt.Errorf("%w: sync-collection sin sync-token", ErrServidor)
		}
		token = ms.SyncToken
		if !truncada || ms.SyncToken == "" {
			break
		}
	}
	ca.Ficha = prefijoSync + token
	pedir := porPedir(orden, cambiados, etagDe, primera, &ca)
	if primera {
		ca.Completo = true
	}
	// bajar rellena ca: se llama antes de devolverla.
	err := c.bajar(ctx, col, pedir, cambiados, primera, desde, &ca)
	return ca, err
}

// apuntarSync anota una página de `sync-collection`: en cambiados, la ETag de lo que
// cambió (y en orden, la primera vez que sale); en ca.Borrados, lo que ya no está.
// truncada: el servidor cortó la respuesta (507 sobre la propia colección) y hay más.
func (c *Cliente) apuntarSync(col Coleccion, ms *multistatus, desdeURL *url.URL, cambiados map[string]string, orden *[]string, ca *Cambios) (truncada bool) {
	for _, r := range ms.Responses {
		if len(r.Href) == 0 {
			continue
		}
		u := c.resolver(desdeURL, r.Href[0])
		if u == "" {
			continue
		}
		if mismo(u, col.URL) {
			// 507 sobre la propia colección: hay más páginas.
			if strings.Contains(r.Status, "507") {
				truncada = true
			}
			continue
		}
		if strings.Contains(r.Status, "404") {
			delete(cambiados, u)
			ca.Borrados = append(ca.Borrados, u)
			continue
		}
		p, ok := r.props()
		if !ok {
			continue
		}
		if _, ya := cambiados[u]; !ya {
			*orden = append(*orden, u)
		}
		cambiados[u] = p.ETag
	}
	return truncada
}

// porPedir son, en orden, los cambiados (y no borrados después) cuya ETag no es la que
// ya se tiene. En la primera bajada van todos, además, a ca.Vistos.
func porPedir(orden []string, cambiados map[string]string, etagDe func(string) (string, bool), primera bool, ca *Cambios) []string {
	var pedir []string
	for _, u := range orden {
		e, ok := cambiados[u]
		if !ok {
			continue
		}
		if primera {
			ca.Vistos = append(ca.Vistos, u)
		}
		if conocida, ok := etagDe(u); ok && conocida == e && e != "" {
			continue
		}
		pedir = append(pedir, u)
	}
	return pedir
}

func mismo(a, b string) bool { return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/") }

func (c *Cliente) porETags(ctx context.Context, col Coleccion, ficha string, etagDe func(string) (string, bool), desde time.Time) (Cambios, error) {
	if col.CTag != "" && ficha == prefijoCTag+col.CTag {
		// Nada cambió en la colección.
		return Cambios{Ficha: ficha}, nil
	}
	ms, desdeURL, err := c.multiestado(ctx, "PROPFIND", col.URL, "1", `<d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:resourcetype/></d:prop></d:propfind>`)
	if err != nil {
		return Cambios{}, err
	}
	ca := Cambios{Completo: true, Ficha: ficha}
	if col.CTag != "" {
		ca.Ficha = prefijoCTag + col.CTag
	} else {
		ca.Ficha = ""
	}
	var pedir []string
	etags := map[string]string{}
	for _, r := range ms.Responses {
		p, ok := r.props()
		if !ok || len(r.Href) == 0 {
			continue
		}
		u := c.resolver(desdeURL, r.Href[0])
		if u == "" || mismo(u, col.URL) || (p.ResourceType != nil && p.ResourceType.Collection != nil) {
			continue
		}
		ca.Vistos = append(ca.Vistos, u)
		etags[u] = p.ETag
		if conocida, ok := etagDe(u); ok && conocida == p.ETag && p.ETag != "" {
			continue
		}
		pedir = append(pedir, u)
	}
	// bajar rellena ca: se llama antes de devolverla.
	err = c.bajar(ctx, col, pedir, etags, ficha == "", desde, &ca)
	return ca, err
}

// bajar trae los datos de urls. En la primera bajada de un calendario con ventana, una
// sola `calendar-query` trae los de dentro, y los demás quedan Fuera; en la de una
// libreta, una `addressbook-query` trae todo de una vez. Si no, `multiget` por tandas.
func (c *Cliente) bajar(ctx context.Context, col Coleccion, urls []string, etags map[string]string, primera bool, desde time.Time, ca *Cambios) error {
	if len(urls) == 0 {
		return nil
	}
	pendientes := map[string]bool{}
	for _, u := range urls {
		pendientes[u] = true
	}
	if primera && len(urls) > 1 {
		c.bajarDeUnaVez(ctx, col, urls, etags, desde, pendientes, ca)
	}
	var resto []string
	for _, u := range urls {
		if pendientes[u] {
			resto = append(resto, u)
		}
	}
	for len(resto) > 0 {
		n := min(len(resto), 100)
		objs, err := c.Multiget(ctx, col, resto[:n])
		if err != nil {
			return err
		}
		ca.Puestos = append(ca.Puestos, objs...)
		resto = resto[n:]
	}
	return nil
}

// bajarDeUnaVez es la primera bajada con una sola consulta: la `calendar-query` de lo
// que cae en la ventana de un calendario (lo demás queda Fuera) o la `addressbook-query`
// de una libreta. Quita de pendientes lo que resuelve. Si la consulta falla, o no cabe,
// no resuelve nada: lo pendiente se baja por multiget.
func (c *Cliente) bajarDeUnaVez(ctx context.Context, col Coleccion, urls []string, etags map[string]string, desde time.Time, pendientes map[string]bool, ca *Cambios) {
	var objs []Objeto
	var err error
	switch {
	case col.Clase == Calendario && !desde.IsZero():
		objs, err = c.ConsultarCalendario(ctx, col.URL, desde)
	case col.Clase == Libreta:
		objs, err = c.ConsultarLibreta(ctx, col.URL)
	}
	if err != nil || objs == nil {
		return
	}
	for _, o := range objs {
		if pendientes[o.URL] {
			delete(pendientes, o.URL)
			ca.Puestos = append(ca.Puestos, o)
		}
	}
	if col.Clase != Calendario {
		return
	}
	for _, u := range urls {
		if pendientes[u] {
			ca.Fuera = append(ca.Fuera, Objeto{URL: u, ETag: etags[u]})
			delete(pendientes, u)
		}
	}
}

func (c *Cliente) objetos(ms *multistatus, desde *url.URL, clase Clase) []Objeto {
	var out []Objeto
	for _, r := range ms.Responses {
		p, ok := r.props()
		if !ok || len(r.Href) == 0 {
			continue
		}
		u := c.resolver(desde, r.Href[0])
		datos := p.CalData
		if clase == Libreta {
			datos = p.CardData
		}
		if u == "" || strings.TrimSpace(datos) == "" {
			continue
		}
		out = append(out, Objeto{URL: u, ETag: p.ETag, Datos: []byte(datos)})
	}
	return out
}

// Multiget trae varios objetos de una colección por su URL.
func (c *Cliente) Multiget(ctx context.Context, col Coleccion, urls []string) ([]Objeto, error) {
	var b strings.Builder
	if col.Clase == Libreta {
		b.WriteString(`<card:addressbook-multiget xmlns:d="DAV:" xmlns:card="` + nsCard + `"><d:prop><d:getetag/><card:address-data/></d:prop>`)
	} else {
		b.WriteString(`<c:calendar-multiget xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `"><d:prop><d:getetag/><c:calendar-data/></d:prop>`)
	}
	for _, u := range urls {
		p, err := url.Parse(u)
		if err != nil {
			continue
		}
		b.WriteString(`<d:href>` + escapar(p.EscapedPath()) + `</d:href>`)
	}
	if col.Clase == Libreta {
		b.WriteString(`</card:addressbook-multiget>`)
	} else {
		b.WriteString(`</c:calendar-multiget>`)
	}
	ms, desde, err := c.multiestado(ctx, "REPORT", col.URL, "1", b.String())
	if err != nil {
		return nil, err
	}
	return c.objetos(ms, desde, col.Clase), nil
}

// ConsultarCalendario es la `calendar-query` de los eventos que acaban después de desde,
// con sus datos (la ventana de la primera bajada).
func (c *Cliente) ConsultarCalendario(ctx context.Context, coleccion string, desde time.Time) ([]Objeto, error) {
	cuerpo := `<c:calendar-query xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `"><d:prop><d:getetag/><c:calendar-data/></d:prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="` + desde.UTC().Format("20060102T150405Z") + `"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`
	ms, u, err := c.multiestado(ctx, "REPORT", coleccion, "1", cuerpo)
	if err != nil {
		return nil, err
	}
	return c.objetos(ms, u, Calendario), nil
}

// ConsultarLibreta es la `addressbook-query` de todas las tarjetas, con sus datos.
func (c *Cliente) ConsultarLibreta(ctx context.Context, coleccion string) ([]Objeto, error) {
	cuerpo := `<card:addressbook-query xmlns:d="DAV:" xmlns:card="` + nsCard + `"><d:prop><d:getetag/><card:address-data/></d:prop></card:addressbook-query>`
	ms, u, err := c.multiestado(ctx, "REPORT", coleccion, "1", cuerpo)
	if err != nil {
		return nil, err
	}
	return c.objetos(ms, u, Libreta), nil
}

// Leer trae un objeto por su URL.
func (c *Cliente) Leer(ctx context.Context, u string) (Objeto, error) {
	resp, datos, err := c.hacer(ctx, peticion{metodo: http.MethodGet, destino: u}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return Objeto{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Objeto{}, errorDe(resp, datos)
	}
	return Objeto{URL: u, ETag: resp.Header.Get("ETag"), Datos: datos}, nil
}

// Poner sube un objeto. etag "" lo crea (If-None-Match: *: si ya hay uno con esa URL,
// ErrConflicto); si no, lo reemplaza solo si sigue en esa ETag (If-Match). Devuelve la
// ETag nueva, o "" si el servidor no la da (porque lo retocó: se baja en la próxima).
func (c *Cliente) Poner(ctx context.Context, u, etag string, datos []byte, clase Clase) (string, error) {
	tipo := "text/calendar; charset=utf-8"
	if clase == Libreta {
		tipo = "text/vcard; charset=utf-8"
	}
	cab := map[string]string{"If-None-Match": "*"}
	if etag != "" {
		cab = map[string]string{"If-Match": ETag(etag)}
	}
	resp, cuerpo, err := c.hacer(ctx, peticion{metodo: http.MethodPut, destino: u, cuerpo: datos, tipo: tipo, cabeceras: cab}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errorDe(resp, cuerpo)
	}
	return resp.Header.Get("ETag"), nil
}

// Borrar borra un objeto si sigue en esa ETag. Si ya no estaba, no es un error.
func (c *Cliente) Borrar(ctx context.Context, u, etag string) error {
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = ETag(etag)
	}
	resp, cuerpo, err := c.hacer(ctx, peticion{metodo: http.MethodDelete, destino: u, cabeceras: cab}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorDe(resp, cuerpo)
	}
	return nil
}

// Planifica dice si el servidor envía él las invitaciones de lo que se guarda en esta
// colección (planificación implícita, RFC 6638: `calendar-auto-schedule` en la cabecera
// DAV). Si no, las tiene que mandar quien organiza, por correo (iMIP).
func (c *Cliente) Planifica(ctx context.Context, coleccion string) (bool, error) {
	resp, cuerpo, err := c.hacer(ctx, peticion{metodo: http.MethodOptions, destino: coleccion}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, errorDe(resp, cuerpo)
	}
	for _, v := range resp.Header.Values("DAV") {
		for _, x := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(x), "calendar-auto-schedule") {
				return true, nil
			}
		}
	}
	return false, nil
}

// URLNueva es la URL de un objeto nuevo en una colección, a partir de su UID: los
// caracteres que no son seguros en una ruta se cambian, para que el servidor no tenga
// que interpretar nada.
func URLNueva(coleccion, uid string, clase Clase) string {
	limpio := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '@':
			return r
		}
		return '_'
	}, uid)
	if len(limpio) > 120 || limpio == "" || limpio == "." || limpio == ".." {
		limpio = fmt.Sprintf("%x", []byte(uid))
		if len(limpio) > 120 {
			limpio = limpio[:120]
		}
	}
	ext := ".ics"
	if clase == Libreta {
		ext = ".vcf"
	}
	return strings.TrimSuffix(coleccion, "/") + "/" + limpio + ext
}
