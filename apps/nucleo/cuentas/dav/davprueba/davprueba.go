// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package davprueba es un servidor CalDAV y CardDAV en memoria para las pruebas de la
// sincronización: el de `go-webdav` (principal, casas, GET, PUT con If-Match, consultas
// y multiget) con lo que no trae puesto encima —`sync-collection`, `getctag`, el
// conjunto de privilegios (colecciones de solo lectura), `If-Match` en DELETE, la
// planificación implícita en OPTIONS, el buzón de salida con la ocupación de otras
// personas (VFREEBUSY, RFC 6638 §5), las direcciones de la persona, crear libretas con
// el MKCOL extendido y la contraseña—, para probar contra algo que se porta como un
// servidor de verdad sin salir del proceso.
//
// Solo lo importan pruebas.
package davprueba

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
)

// Rutas del servidor.
const (
	PrincipalCal  = "/cal/u/"
	CasaCal       = "/cal/u/calendars/"
	PrincipalCard = "/card/u/"
	CasaCard      = "/card/u/contacts/"
	// Buzon es el buzón de salida de la planificación (`schedule-outbox-URL`).
	Buzon = "/cal/u/outbox/"
)

type objeto struct {
	datos []byte
	etag  string
	ver   int64
}

type coleccion struct {
	nombre      string
	color       string // «#RRGGBBAA»; vacío, el de siempre
	soloLectura bool
	objetos     map[string]*objeto
	borrados    map[string]int64
}

// Servidor es el servidor de pruebas.
type Servidor struct {
	*httptest.Server
	Usuario, Clave string
	// SinSync quita `sync-collection` (y el sync-token): obliga a ir por ETag.
	SinSync bool
	// Planifica anuncia `calendar-auto-schedule`.
	Planifica bool
	// Caido devuelve 503 a todo (la red cortada no se puede simular desde aquí; esto es
	// lo más parecido que ve el cliente).
	Caido bool
	// Solo3 rechaza con 415 las vCard 4.0 (como algunos servidores CardDAV antiguos):
	// solo guarda las 3.0.
	Solo3 bool
	// Correo es la dirección de la persona (`calendar-user-address-set`).
	Correo string
	// SinBuzon quita el buzón de salida aunque planifique: el servidor no da la
	// ocupación de nadie.
	SinBuzon bool

	mu       sync.Mutex
	cols     map[string]*coleccion
	version  int64
	ocupados map[string][][2]time.Time
	Peticion []string // «MÉTODO ruta» de cada petición, para las pruebas
	// Preguntas son los VFREEBUSY que llegaron al buzón de salida, tal cual.
	Preguntas []string
}

// Nuevo arranca el servidor con TLS.
func Nuevo() *Servidor { return nuevo(httptest.NewTLSServer) }

// NuevoClaro lo arranca sin TLS, en localhost: el portero de red solo deja ir sin
// cifrar a este equipo, y así se puede probar la app entera contra él.
func NuevoClaro() *Servidor { return nuevo(httptest.NewServer) }

func nuevo(arrancar func(http.Handler) *httptest.Server) *Servidor {
	s := &Servidor{Usuario: "ana", Clave: "secreta", Correo: "ana@example.org", cols: map[string]*coleccion{}, ocupados: map[string][][2]time.Time{}}
	cal := &caldav.Handler{Backend: calBackend{s}, Prefix: "/cal"}
	card := &carddav.Handler{Backend: cardBackend{s}, Prefix: "/card"}
	s.Server = arrancar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.servir(w, r, cal, card)
	}))
	return s
}

// Crear añade una colección (ruta completa, acabada en «/»).
func (s *Servidor) Crear(ruta, nombre string, soloLectura bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cols[ruta] = &coleccion{nombre: nombre, soloLectura: soloLectura, objetos: map[string]*objeto{}, borrados: map[string]int64{}}
}

// Ocupado apunta un rato ocupado de otra persona (la que el buzón de salida conoce).
func (s *Servidor) Ocupado(correo string, desde, hasta time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := strings.ToLower(correo)
	s.ocupados[c] = append(s.ocupados[c], [2]time.Time{desde.UTC(), hasta.UTC()})
}

// Libretas son las rutas de las libretas del servidor, en orden.
func (s *Servidor) Libretas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for r := range s.cols {
		if strings.HasPrefix(r, CasaCard) {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

// Nombre es el nombre de una colección, o "".
func (s *Servidor) Nombre(ruta string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.cols[ruta]; c != nil {
		return c.nombre
	}
	return ""
}

// Poner deja un objeto en el servidor como si lo hubiera escrito otro cliente.
func (s *Servidor) Poner(ruta string, datos []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poner(ruta, datos)
}

// Quitar borra un objeto como si lo hubiera hecho otro cliente.
func (s *Servidor) Quitar(ruta string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quitar(ruta)
}

// Datos devuelve un objeto del servidor, o nil.
func (s *Servidor) Datos(ruta string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cols[path.Dir(ruta)+"/"]
	if c == nil || c.objetos[ruta] == nil {
		return nil
	}
	return bytes.Clone(c.objetos[ruta].datos)
}

// Rutas son las de los objetos de una colección, en orden.
func (s *Servidor) Rutas(col string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	if c := s.cols[col]; c != nil {
		for r := range c.objetos {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Servidor) poner(ruta string, datos []byte) *objeto {
	c := s.cols[path.Dir(ruta)+"/"]
	if c == nil {
		return nil
	}
	s.version++
	h := sha256.Sum256(append([]byte(strconv.FormatInt(s.version, 10)), datos...))
	o := &objeto{datos: bytes.Clone(datos), etag: hex.EncodeToString(h[:8]), ver: s.version}
	c.objetos[ruta] = o
	delete(c.borrados, ruta)
	return o
}

func (s *Servidor) quitar(ruta string) {
	c := s.cols[path.Dir(ruta)+"/"]
	if c == nil || c.objetos[ruta] == nil {
		return
	}
	s.version++
	delete(c.objetos, ruta)
	c.borrados[ruta] = s.version
}

func (s *Servidor) servir(w http.ResponseWriter, r *http.Request, cal, card http.Handler) {
	if !s.admitir(w, r) {
		return
	}
	cuerpo, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(cuerpo))
	if s.atender(w, r, cuerpo) {
		return
	}
	if strings.HasPrefix(r.URL.Path, "/card") {
		card.ServeHTTP(w, r)
		return
	}
	cal.ServeHTTP(w, r)
}

// admitir apunta la petición y la deja pasar si el servidor no está caído y trae las
// credenciales; si no, contesta él.
func (s *Servidor) admitir(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	s.Peticion = append(s.Peticion, r.Method+" "+r.URL.Path)
	caido := s.Caido
	s.mu.Unlock()
	if caido {
		http.Error(w, "caído", http.StatusServiceUnavailable)
		return false
	}
	if u, c, ok := r.BasicAuth(); !ok || u != s.Usuario || c != s.Clave {
		w.Header().Set("WWW-Authenticate", `Basic realm="prueba"`)
		http.Error(w, "sin acceso", http.StatusUnauthorized)
		return false
	}
	return true
}

// atender contesta lo que el servidor WebDAV de debajo no hace (o se simula que hace
// otro servidor) y dice si ya contestó; si no, la petición sigue a él.
func (s *Servidor) atender(w http.ResponseWriter, r *http.Request, cuerpo []byte) bool {
	switch {
	case r.Method == "PROPFIND" && (r.URL.Path == CasaCal || r.URL.Path == CasaCard) && r.Header.Get("Depth") == "1":
		s.listarCasa(w, r.URL.Path)
		return true
	case r.Method == "REPORT" && bytes.Contains(cuerpo, []byte("sync-collection")):
		s.sincronizar(w, r.URL.Path, cuerpo)
		return true
	case r.Method == http.MethodDelete:
		s.borrar(w, r)
		return true
	case r.Method == "MKCALENDAR":
		s.crearCalendario(w, r.URL.Path, cuerpo)
		return true
	case r.Method == "MKCOL" && strings.HasPrefix(r.URL.Path, CasaCard):
		s.crearLibreta(w, r.URL.Path, cuerpo)
		return true
	case r.Method == "PROPFIND" && r.URL.Path == PrincipalCal && (bytes.Contains(cuerpo, []byte("schedule-outbox-URL")) || bytes.Contains(cuerpo, []byte("calendar-user-address-set"))):
		s.planificacion(w)
		return true
	case r.Method == http.MethodPost && r.URL.Path == Buzon:
		s.ocupacion(w, r, cuerpo)
		return true
	case r.Method == http.MethodPut:
		return s.rechazarPut(w, r, cuerpo)
	case r.Method == http.MethodOptions && s.Planifica:
		w.Header().Add("DAV", "calendar-auto-schedule")
	}
	return false
}

// rechazarPut contesta él a un PUT que el servidor de verdad no aceptaría: en una
// colección de solo lectura, o una vCard 4.0 con Solo3. Dice si lo rechazó.
func (s *Servidor) rechazarPut(w http.ResponseWriter, r *http.Request, cuerpo []byte) bool {
	s.mu.Lock()
	c := s.cols[path.Dir(r.URL.Path)+"/"]
	solo3 := s.Solo3
	s.mu.Unlock()
	if c != nil && c.soloLectura {
		http.Error(w, "solo lectura", http.StatusForbidden)
		return true
	}
	if solo3 && strings.HasPrefix(r.URL.Path, "/card") && bytes.Contains(cuerpo, []byte("VERSION:4.0")) {
		http.Error(w, "solo vCard 3.0", http.StatusUnsupportedMediaType)
		return true
	}
	return false
}

// crearCalendario atiende MKCALENDAR: una colección nueva dentro de la casa, con el
// displayname del cuerpo.
func (s *Servidor) crearCalendario(w http.ResponseWriter, ruta string, cuerpo []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ruta = strings.TrimSuffix(ruta, "/") + "/"
	if !strings.HasPrefix(ruta, CasaCal) || ruta == CasaCal || strings.Contains(strings.TrimPrefix(ruta, CasaCal), "/") && !strings.HasSuffix(strings.TrimPrefix(ruta, CasaCal), "/") {
		http.Error(w, "fuera de la casa", http.StatusForbidden)
		return
	}
	if s.cols[ruta] != nil {
		http.Error(w, "ya existe", http.StatusMethodNotAllowed)
		return
	}
	valor := func(etiqueta string) string {
		if i := bytes.Index(cuerpo, []byte(etiqueta+">")); i >= 0 {
			resto := cuerpo[i+len(etiqueta)+1:]
			if j := bytes.Index(resto, []byte("</")); j >= 0 {
				return string(resto[:j])
			}
		}
		return ""
	}
	nombre := valor("displayname")
	if nombre == "" {
		nombre = path.Base(ruta)
	}
	s.version++
	s.cols[ruta] = &coleccion{nombre: nombre, color: valor("calendar-color"), objetos: map[string]*objeto{}, borrados: map[string]int64{}}
	w.WriteHeader(http.StatusCreated)
}

// crearLibreta atiende el MKCOL extendido de una libreta (RFC 5689): una colección
// nueva dentro de la casa de libretas, con el displayname del cuerpo.
func (s *Servidor) crearLibreta(w http.ResponseWriter, ruta string, cuerpo []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ruta = strings.TrimSuffix(ruta, "/") + "/"
	resto := strings.TrimSuffix(strings.TrimPrefix(ruta, CasaCard), "/")
	if resto == "" || strings.Contains(resto, "/") || !bytes.Contains(cuerpo, []byte("addressbook")) {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	if s.cols[ruta] != nil {
		http.Error(w, "ya existe", http.StatusMethodNotAllowed)
		return
	}
	nombre := path.Base(ruta)
	if i := bytes.Index(cuerpo, []byte("displayname>")); i >= 0 {
		if j := bytes.Index(cuerpo[i+len("displayname>"):], []byte("</")); j >= 0 {
			nombre = string(cuerpo[i+len("displayname>") : i+len("displayname>")+j])
		}
	}
	s.version++
	s.cols[ruta] = &coleccion{nombre: nombre, objetos: map[string]*objeto{}, borrados: map[string]int64{}}
	w.WriteHeader(http.StatusCreated)
}

// planificacion contesta el PROPFIND del principal por el buzón de salida y las
// direcciones de la persona. Sin planificación no hay buzón.
func (s *Servidor) planificacion(w http.ResponseWriter) {
	s.mu.Lock()
	buzon := s.Planifica && !s.SinBuzon
	correo := s.Correo
	s.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>` + PrincipalCal + `</d:href>`)
	b.WriteString(`<d:propstat><d:prop><c:calendar-user-address-set><d:href>/cal/u/</d:href><d:href>mailto:` + correo + `</d:href></c:calendar-user-address-set>`)
	if buzon {
		b.WriteString(`<c:schedule-outbox-URL><d:href>` + Buzon + `</d:href></c:schedule-outbox-URL>`)
	}
	b.WriteString(`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>`)
	if !buzon {
		b.WriteString(`<d:propstat><d:prop><c:schedule-outbox-URL/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>`)
	}
	b.WriteString(`</d:response></d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, b.String())
}

// ocupacion atiende el VFREEBUSY del buzón de salida: por cada ATTENDEE, sus ratos
// ocupados del intervalo (los apuntados con Ocupado); a quien no conoce, 3.7.
func (s *Servidor) ocupacion(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Planifica || s.SinBuzon {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	s.Preguntas = append(s.Preguntas, string(cuerpo))
	cal, err := ical.NewDecoder(bytes.NewReader(cuerpo)).Decode()
	if err != nil || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/calendar") {
		http.Error(w, "no es un calendario", http.StatusBadRequest)
		return
	}
	var fb *ical.Component
	for _, c := range cal.Children {
		if c.Name == ical.CompFreeBusy {
			fb = c
		}
	}
	if fb == nil {
		http.Error(w, "sin VFREEBUSY", http.StatusBadRequest)
		return
	}
	desde, _ := fb.Props.DateTime(ical.PropDateTimeStart, time.UTC)
	hasta, _ := fb.Props.DateTime(ical.PropDateTimeEnd, time.UTC)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><C:schedule-response xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">`)
	for _, a := range fb.Props.Values(ical.PropAttendee) {
		quien := strings.ToLower(strings.TrimPrefix(a.Value, "mailto:"))
		b.WriteString(`<C:response><C:recipient><D:href>mailto:` + quien + `</D:href></C:recipient>`)
		tramos, conocido := s.ocupados[quien]
		if !conocido {
			b.WriteString(`<C:request-status>3.7;Invalid calendar user</C:request-status></C:response>`)
			continue
		}
		var cal strings.Builder
		cal.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//davprueba//ES\r\nMETHOD:REPLY\r\nBEGIN:VFREEBUSY\r\nUID:x\r\nDTSTAMP:20260101T000000Z\r\n")
		for _, t := range tramos {
			if t[1].After(desde) && t[0].Before(hasta) {
				cal.WriteString("FREEBUSY;FBTYPE=BUSY:" + t[0].Format("20060102T150405Z") + "/" + t[1].Format("20060102T150405Z") + "\r\n")
			}
		}
		cal.WriteString("END:VFREEBUSY\r\nEND:VCALENDAR\r\n")
		b.WriteString(`<C:request-status>2.0;Success</C:request-status><C:calendar-data>`)
		_ = xml.EscapeText(&b, []byte(cal.String()))
		b.WriteString(`</C:calendar-data></C:response>`)
	}
	b.WriteString(`</C:schedule-response>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, b.String())
}

func (s *Servidor) borrar(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cols[path.Dir(r.URL.Path)+"/"]
	if c == nil || c.objetos[r.URL.Path] == nil {
		http.Error(w, "no está", http.StatusNotFound)
		return
	}
	if c.soloLectura {
		http.Error(w, "solo lectura", http.StatusForbidden)
		return
	}
	if m := r.Header.Get("If-Match"); m != "" && strings.Trim(m, `"`) != c.objetos[r.URL.Path].etag {
		http.Error(w, "cambió", http.StatusPreconditionFailed)
		return
	}
	s.quitar(r.URL.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) listarCasa(w http.ResponseWriter, casa string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	libreta := casa == CasaCard
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:card="urn:ietf:params:xml:ns:carddav" xmlns:cs="http://calendarserver.org/ns/" xmlns:ic="http://apple.com/ns/ical/">`)
	b.WriteString(`<d:response><d:href>` + casa + `</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
	var rutas []string
	for r := range s.cols {
		if strings.HasPrefix(r, casa) {
			rutas = append(rutas, r)
		}
	}
	sort.Strings(rutas)
	for _, r := range rutas {
		c := s.cols[r]
		tipo := `<c:calendar/>`
		if libreta {
			tipo = `<card:addressbook/>`
		}
		priv := `<d:privilege><d:read/></d:privilege>`
		if !c.soloLectura {
			priv += `<d:privilege><d:write/></d:privilege>`
		}
		b.WriteString(`<d:response><d:href>` + r + `</d:href><d:propstat><d:prop><d:resourcetype><d:collection/>` + tipo + `</d:resourcetype>`)
		b.WriteString(`<d:displayname>` + c.nombre + `</d:displayname><cs:getctag>` + strconv.FormatInt(s.version, 10) + `</cs:getctag>`)
		if !s.SinSync {
			b.WriteString(`<d:sync-token>` + s.ficha() + `</d:sync-token>`)
		}
		b.WriteString(`<d:current-user-privilege-set>` + priv + `</d:current-user-privilege-set>`)
		if !libreta {
			color := c.color
			if color == "" {
				color = "#33AA55FF"
			}
			b.WriteString(`<ic:calendar-color>` + color + `</ic:calendar-color><c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set>`)
		}
		b.WriteString(`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
	}
	b.WriteString(`</d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, b.String())
}

func (s *Servidor) ficha() string { return "http://prueba/sync/" + strconv.FormatInt(s.version, 10) }

// sincronizar atiende el REPORT `sync-collection`; con SinSync, el servidor no lo sabe.
func (s *Servidor) sincronizar(w http.ResponseWriter, ruta string, cuerpo []byte) {
	if s.SinSync {
		http.Error(w, "no", http.StatusNotImplemented)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cols[ruta]
	if c == nil {
		http.Error(w, "no está", http.StatusNotFound)
		return
	}
	token := ""
	if i := bytes.Index(cuerpo, []byte("<d:sync-token>")); i >= 0 {
		resto := cuerpo[i+len("<d:sync-token>"):]
		if j := bytes.Index(resto, []byte("</d:sync-token>")); j >= 0 {
			token = string(resto[:j])
		}
	}
	var desde int64
	if token != "" {
		v, err := strconv.ParseInt(strings.TrimPrefix(token, "http://prueba/sync/"), 10, 64)
		if err != nil || !strings.HasPrefix(token, "http://prueba/sync/") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<d:error xmlns:d="DAV:"><d:valid-sync-token/></d:error>`)
			return
		}
		desde = v
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:">`)
	var rutas []string
	for r, o := range c.objetos {
		if o.ver > desde {
			rutas = append(rutas, r)
		}
	}
	sort.Strings(rutas)
	for _, r := range rutas {
		b.WriteString(`<d:response><d:href>` + r + `</d:href><d:propstat><d:prop><d:getetag>"` + c.objetos[r].etag + `"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
	}
	if desde > 0 {
		for r, v := range c.borrados {
			if v > desde {
				b.WriteString(`<d:response><d:href>` + r + `</d:href><d:status>HTTP/1.1 404 Not Found</d:status></d:response>`)
			}
		}
	}
	b.WriteString(`<d:sync-token>` + s.ficha() + `</d:sync-token></d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, b.String())
}

// ----- Los backends de go-webdav -----

func noEsta() error { return webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no está")) }

func condicion(actual *objeto, siNo, si webdav.ConditionalMatch) error {
	if siNo.IsWildcard() && actual != nil {
		return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("ya existe"))
	}
	if si.IsSet() {
		if actual == nil {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("no existe"))
		}
		if ok, _ := si.MatchETag(actual.etag); !ok {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("cambió"))
		}
	}
	return nil
}

type calBackend struct{ s *Servidor }

func (b calBackend) CurrentUserPrincipal(context.Context) (string, error) { return PrincipalCal, nil }
func (b calBackend) CalendarHomeSetPath(context.Context) (string, error)  { return CasaCal, nil }
func (b calBackend) CreateCalendar(context.Context, *caldav.Calendar) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("no"))
}

func (b calBackend) ListCalendars(context.Context) ([]caldav.Calendar, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	var out []caldav.Calendar
	for r, c := range b.s.cols {
		if strings.HasPrefix(r, CasaCal) {
			out = append(out, caldav.Calendar{Path: r, Name: c.nombre, SupportedComponentSet: []string{"VEVENT"}})
		}
	}
	return out, nil
}

func (b calBackend) GetCalendar(_ context.Context, p string) (*caldav.Calendar, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	if c := b.s.cols[strings.TrimSuffix(p, "/")+"/"]; c != nil {
		return &caldav.Calendar{Path: strings.TrimSuffix(p, "/") + "/", Name: c.nombre, SupportedComponentSet: []string{"VEVENT"}}, nil
	}
	return nil, noEsta()
}

func calObjeto(r string, o *objeto) (caldav.CalendarObject, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(o.datos)).Decode()
	if err != nil {
		return caldav.CalendarObject{}, err
	}
	return caldav.CalendarObject{Path: r, ETag: o.etag, Data: cal, ModTime: time.Unix(o.ver, 0), ContentLength: int64(len(o.datos))}, nil
}

func (b calBackend) GetCalendarObject(_ context.Context, p string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[path.Dir(p)+"/"]
	if c == nil || c.objetos[p] == nil {
		return nil, noEsta()
	}
	co, err := calObjeto(p, c.objetos[p])
	return &co, err
}

func (b calBackend) ListCalendarObjects(_ context.Context, p string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[strings.TrimSuffix(p, "/")+"/"]
	if c == nil {
		return nil, noEsta()
	}
	var out []caldav.CalendarObject
	for r, o := range c.objetos {
		if co, err := calObjeto(r, o); err == nil {
			out = append(out, co)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (b calBackend) QueryCalendarObjects(ctx context.Context, p string, q *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	todos, err := b.ListCalendarObjects(ctx, p, &q.CompRequest)
	if err != nil {
		return nil, err
	}
	return caldav.Filter(q, todos)
}

func (b calBackend) PutCalendarObject(_ context.Context, p string, cal *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[path.Dir(p)+"/"]
	if c == nil {
		return nil, noEsta()
	}
	if err := condicion(c.objetos[p], opts.IfNoneMatch, opts.IfMatch); err != nil {
		return nil, err
	}
	o := b.s.poner(p, buf.Bytes())
	co, err := calObjeto(p, o)
	return &co, err
}

func (b calBackend) DeleteCalendarObject(_ context.Context, p string) error {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	b.s.quitar(p)
	return nil
}

type cardBackend struct{ s *Servidor }

func (b cardBackend) CurrentUserPrincipal(context.Context) (string, error) { return PrincipalCard, nil }

func (b cardBackend) AddressBookHomeSetPath(context.Context) (string, error) { return CasaCard, nil }

func (b cardBackend) CreateAddressBook(context.Context, *carddav.AddressBook) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("no"))
}

func (b cardBackend) DeleteAddressBook(context.Context, string) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("no"))
}

func (b cardBackend) ListAddressBooks(context.Context) ([]carddav.AddressBook, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	var out []carddav.AddressBook
	for r, c := range b.s.cols {
		if strings.HasPrefix(r, CasaCard) {
			out = append(out, carddav.AddressBook{Path: r, Name: c.nombre})
		}
	}
	return out, nil
}

func (b cardBackend) GetAddressBook(_ context.Context, p string) (*carddav.AddressBook, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	if c := b.s.cols[strings.TrimSuffix(p, "/")+"/"]; c != nil {
		return &carddav.AddressBook{Path: strings.TrimSuffix(p, "/") + "/", Name: c.nombre}, nil
	}
	return nil, noEsta()
}

func cardObjeto(r string, o *objeto) (carddav.AddressObject, error) {
	card, err := vcard.NewDecoder(bytes.NewReader(o.datos)).Decode()
	if err != nil {
		return carddav.AddressObject{}, err
	}
	return carddav.AddressObject{Path: r, ETag: o.etag, Card: card, ModTime: time.Unix(o.ver, 0), ContentLength: int64(len(o.datos))}, nil
}

func (b cardBackend) GetAddressObject(_ context.Context, p string, _ *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[path.Dir(p)+"/"]
	if c == nil || c.objetos[p] == nil {
		return nil, noEsta()
	}
	ao, err := cardObjeto(p, c.objetos[p])
	return &ao, err
}

func (b cardBackend) ListAddressObjects(_ context.Context, p string, _ *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[strings.TrimSuffix(p, "/")+"/"]
	if c == nil {
		return nil, noEsta()
	}
	var out []carddav.AddressObject
	for r, o := range c.objetos {
		if ao, err := cardObjeto(r, o); err == nil {
			out = append(out, ao)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (b cardBackend) QueryAddressObjects(ctx context.Context, p string, q *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	todos, err := b.ListAddressObjects(ctx, p, &q.DataRequest)
	if err != nil {
		return nil, err
	}
	return carddav.Filter(q, todos)
}

func (b cardBackend) PutAddressObject(_ context.Context, p string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	c := b.s.cols[path.Dir(p)+"/"]
	if c == nil {
		return nil, noEsta()
	}
	if err := condicion(c.objetos[p], opts.IfNoneMatch, opts.IfMatch); err != nil {
		return nil, err
	}
	o := b.s.poner(p, buf.Bytes())
	ao, err := cardObjeto(p, o)
	return &ao, err
}

func (b cardBackend) DeleteAddressObject(_ context.Context, p string) error {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	b.s.quitar(p)
	return nil
}
