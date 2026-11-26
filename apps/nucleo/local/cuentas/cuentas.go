// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cuentas son las cuentas externas de TODA la edición en un solo modelo
// (PLAN-EDICION-LOCAL §5.10, D-17): las de correo de Mail, las de mensajería de Rooms y
// las de nube de Ajustes, para enseñarlas juntas en Ajustes → Cuentas y para que el
// portero de red (`local/red`) sepa a qué servidores puede ir cada una.
//
// # Migración suave
//
// Cada app sigue guardando sus cuentas donde siempre, y sigue siendo la dueña de su
// alta y de su baja:
//
//   - Mail en `<Datos>/Correo/cuentas.json` (`local/correo`);
//   - Rooms en `<Ajustes>/chat-cuentas.json` (`local/chat`);
//   - la nube en `<Ajustes>/nube.json` (`local/nube`);
//   - el calendario y los contactos de Calendar y Contacts en `<Ajustes>/agenda.json`
//     (`local/agenda`).
//
// Este paquete las LEE (solo los campos que no son secretos, y sin enlazar esos
// paquetes), las junta en el modelo común y lo deja escrito en `<Ajustes>/cuentas.json`.
// Cuando un fichero de origen cambia —otra app añadió una cuenta—, la lista se vuelve a
// leer sola. Los secretos siguen en el llavero con su clave `cuenta-<ID>-<clase>`: aquí
// no se tocan.
package cuentas

import (
	"cmp"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Servicio es lo que da una cuenta.
type Servicio string

// Servicios de una cuenta.
const (
	Correo     Servicio = "correo"
	Calendario Servicio = "calendario"
	Contactos  Servicio = "contactos"
	Chat       Servicio = "chat"
	Reuniones  Servicio = "reuniones"
	Nube       Servicio = "nube"
)

// Estados de una cuenta.
const (
	EstadoOK          = "ok"
	EstadoReconectar  = "reconectar"   // el servidor rechazó la contraseña o el token
	EstadoSinServicio = "sin-servicio" // no da ninguno de los servicios que se usan
)

// Origen es la app que da de alta y de baja la cuenta.
const (
	OrigenCorreo = "correo" // Sytue Mail
	OrigenChat   = "chat"   // Sytue Rooms
	OrigenNube   = "nube"   // Ajustes → Nube
	OrigenAgenda = "agenda" // Sytue Calendar y Sytue Contacts
)

// Servidor es un extremo de una cuenta.
type Servidor struct {
	Servicio Servicio `json:"servicio"`
	// Protocolo: imap, smtp, https…
	Protocolo string `json:"protocolo"`
	Host      string `json:"host"`
	Puerto    int    `json:"puerto,omitempty"`
	// Seguridad: tls, starttls o ninguna (solo la que la persona marca a mano).
	Seguridad string `json:"seguridad,omitempty"`
}

// Cuenta es una cuenta externa, sin secretos.
type Cuenta struct {
	ID        string     `json:"id"`
	Proveedor string     `json:"proveedor"` // microsoft | google | icloud | yahoo | imap | matrix | webdav | sytue…
	Rotulo    string     `json:"rotulo"`
	Direccion string     `json:"direccion,omitempty"` // correo o usuario
	Servicios []Servicio `json:"servicios"`
	// Servidores son los extremos de la cuenta; vacío si el proveedor los fija (Google
	// Drive, OneDrive: los pone `local/nube`).
	Servidores []Servidor `json:"servidores,omitempty"`
	Estado     string     `json:"estado"`
	// Origen es quién la gestiona (OrigenCorreo…).
	Origen string `json:"origen"`
	// OAuth es el servidor de tokens que renueva el acceso, si entra por OAuth.
	OAuth string `json:"oauth,omitempty"`
}

// Registro es el modelo común, leído de los ficheros de cada app.
type Registro struct {
	ajustes, datos string

	mu      sync.Mutex
	cuentas []Cuenta
	firmas  map[string]time.Time
}

// Abrir junta las cuentas de la edición. ajustes es `<Ajustes>` y datos `<Datos>`
// (espacio.Rutas).
func Abrir(ajustes, datos string) *Registro {
	r := &Registro{ajustes: ajustes, datos: datos}
	r.Cuentas()
	return r
}

// Fichero de cada origen.
func (r *Registro) origenes() map[string]string {
	return map[string]string{
		OrigenCorreo: filepath.Join(r.datos, "Correo", "cuentas.json"),
		OrigenChat:   filepath.Join(r.ajustes, "chat-cuentas.json"),
		OrigenNube:   filepath.Join(r.ajustes, "nube.json"),
		OrigenAgenda: filepath.Join(r.ajustes, "agenda.json"),
	}
}

// Cuentas son todas, en el orden de Ajustes: correo, chat, nube, agenda. Si un fichero de
// origen cambió desde la última vez, se vuelve a leer.
func (r *Registro) Cuentas() []Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	firmas := map[string]time.Time{}
	for o, ruta := range r.origenes() {
		if fi, err := os.Stat(ruta); err == nil {
			firmas[o] = fi.ModTime()
		}
	}
	if r.firmas != nil && reflect.DeepEqual(firmas, r.firmas) {
		return slices.Clone(r.cuentas)
	}
	r.firmas = firmas
	o := r.origenes()
	cs := slices.Concat(leerCorreo(o[OrigenCorreo]), leerChat(o[OrigenChat]), leerNube(o[OrigenNube]), leerAgenda(o[OrigenAgenda]))
	if !reflect.DeepEqual(cs, r.cuentas) {
		r.cuentas = cs
		// El modelo común queda escrito para quien lo quiera leer; si no se puede, la
		// lista en memoria vale igual.
		_ = espacio.EscribirJSON(filepath.Join(r.ajustes, "cuentas.json"), struct {
			Cuentas []Cuenta `json:"cuentas"`
		}{cs})
	}
	return slices.Clone(r.cuentas)
}

// Destinos son los servidores de todas las cuentas, para el portero.
func (r *Registro) Destinos() []red.Destino {
	cs := r.Cuentas()
	ds := make([]red.Destino, 0, len(cs))
	for _, c := range cs {
		ds = append(ds, DestinosDe(c)...)
	}
	return ds
}

// DestinosDe son los servidores de una cuenta como destinos del portero. Los de la nube
// de Google y Microsoft no salen de aquí: los conoce `local/nube`.
func DestinosDe(c Cuenta) []red.Destino {
	motivo := c.Rotulo
	if c.Direccion != "" && c.Direccion != c.Rotulo {
		motivo += " — " + c.Direccion
	}
	var ds []red.Destino
	for _, s := range c.Servidores {
		if s.Host == "" {
			continue
		}
		ds = append(ds, red.Destino{Proposito: proposito(s.Servicio), Host: strings.TrimLeft(s.Host, "."), Puerto: s.Puerto, Claro: s.Seguridad == "ninguna", Motivo: motivo})
	}
	if c.Proveedor == ProveedorSlack {
		ds = append(ds, DestinosSlack(motivo)...)
	}
	if c.OAuth != "" {
		ds = append(ds, red.Destino{Proposito: red.OAuth, Host: c.OAuth, Motivo: motivo})
	}
	return ds
}

// ProveedorSlack es el de un espacio de Slack (`local/chat`).
const ProveedorSlack = "slack"

// DestinosSlack son los de Slack: su API (`slack.com`) y sus subdominios, que son el
// canal en tiempo real de Socket Mode (`wss-primary.slack.com`…) y los ficheros
// (`files.slack.com`). Es el proveedor quien los fija, no la persona.
func DestinosSlack(motivo string) []red.Destino {
	return []red.Destino{
		{Proposito: red.Chat, Host: "slack.com", Motivo: motivo},
		{Proposito: red.Chat, Host: ".slack.com", Motivo: motivo},
	}
}

func proposito(s Servicio) red.Proposito {
	switch s {
	case Correo:
		return red.Correo
	case Calendario:
		return red.Calendario
	case Contactos:
		return red.Contactos
	case Chat, Reuniones:
		return red.Chat
	default: // Nube
		return red.Nube
	}
}

// ----- Lo que guarda cada app (solo lo que no es secreto) -----

type servidorCorreo struct {
	Host      string `json:"host"`
	Puerto    int    `json:"puerto"`
	Seguridad string `json:"seguridad"`
}

type cuentaCorreo struct {
	ID        string         `json:"id"`
	Proveedor string         `json:"proveedor"`
	Rotulo    string         `json:"rotulo"`
	Direccion string         `json:"direccion"`
	IMAP      servidorCorreo `json:"imap"`
	SMTP      servidorCorreo `json:"smtp"`
	// Sieve es el servidor de filtros (ManageSieve) que la persona escribió; vacío, el
	// de IMAP en el puerto 4190 si la cuenta es de un IMAP cualquiera (ver servidorSieve).
	Sieve  servidorCorreo `json:"sieve"`
	Acceso string         `json:"acceso"`
	Estado string         `json:"estado"`
}

// ProveedorIMAPGenerico es el de una cuenta de un IMAP cualquiera en Mail
// (`correo.ProveedorIMAP`).
const ProveedorIMAPGenerico = "imap"

// PuertoSieve es el de ManageSieve (RFC 5804).
const PuertoSieve = 4190

// servidorSieve es el servidor de la respuesta automática de una cuenta de Mail (M-10),
// con la MISMA regla que `correo.Cuenta.ServidorSieve`: el que escribió la persona o, en
// una cuenta de un IMAP cualquiera, el host de IMAP en el 4190. Los proveedores conocidos
// (Gmail, Outlook, iCloud, Yahoo, Sytue Mail) no tienen ManageSieve: no abren nada. Va
// con STARTTLS, y sin cifrar solo con un servidor de este equipo.
func servidorSieve(c cuentaCorreo) (Servidor, bool) {
	host, puerto := strings.TrimSpace(c.Sieve.Host), c.Sieve.Puerto
	if host == "" {
		if c.Proveedor != ProveedorIMAPGenerico {
			return Servidor{}, false
		}
		host = c.IMAP.Host
	}
	if host == "" {
		return Servidor{}, false
	}
	if puerto <= 0 || puerto > 65535 {
		puerto = PuertoSieve
	}
	s := Servidor{Servicio: Correo, Protocolo: "managesieve", Host: host, Puerto: puerto, Seguridad: "starttls"}
	if red.EsLocal(host) {
		s.Seguridad = "ninguna"
	}
	return s, true
}

// HostOAuthMicrosoft es el servidor de tokens de Microsoft (`nube.OAuthMicrosoft`), que
// renueva el acceso de una cuenta de Outlook.
const HostOAuthMicrosoft = "login.microsoftonline.com"

// DeCorreo pasa una cuenta de `local/correo` (su JSON) al modelo común: es lo que usa
// Mail para abrir al portero los servidores de una cuenta que aún está probando.
func DeCorreo(raw []byte) (Cuenta, error) {
	var c cuentaCorreo
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cuenta{}, err
	}
	return deCorreo(c), nil
}

func deCorreo(c cuentaCorreo) Cuenta {
	n := Cuenta{
		ID: c.ID, Proveedor: c.Proveedor, Rotulo: cmp.Or(c.Rotulo, c.Direccion), Direccion: c.Direccion,
		Servicios: []Servicio{Correo}, Estado: EstadoOK, Origen: OrigenCorreo,
		Servidores: []Servidor{
			{Servicio: Correo, Protocolo: "imap", Host: c.IMAP.Host, Puerto: c.IMAP.Puerto, Seguridad: c.IMAP.Seguridad},
			{Servicio: Correo, Protocolo: "smtp", Host: c.SMTP.Host, Puerto: c.SMTP.Puerto, Seguridad: c.SMTP.Seguridad},
		},
	}
	// El servidor de la respuesta automática es uno más de la cuenta (aditivo: los de
	// IMAP y SMTP no cambian).
	if s, ok := servidorSieve(c); ok {
		n.Servidores = append(n.Servidores, s)
	}
	if c.Estado == EstadoReconectar {
		n.Estado = EstadoReconectar
	}
	if c.Acceso == "oauth-microsoft" {
		n.OAuth = HostOAuthMicrosoft
	}
	return n
}

func leerCorreo(ruta string) []Cuenta {
	var cs []cuentaCorreo
	if espacio.LeerJSON(ruta, &cs) != nil {
		return nil
	}
	var out []Cuenta
	for _, c := range cs {
		out = append(out, deCorreo(c))
	}
	return out
}

type cuentaChat struct {
	ID        string `json:"id"`
	Proveedor string `json:"proveedor"`
	Rotulo    string `json:"rotulo"`
	Servidor  string `json:"servidor"`
	Usuario   string `json:"usuario"`
	// Las de Teams y Google Chat (OAuth): las direcciones de sus API, su servidor de
	// tokens y si hay que volver a entrar.
	Servidores []string `json:"servidores"`
	OAuth      string   `json:"oauth"`
	Estado     string   `json:"estado"`
}

// Proveedores de mensajería que además crean reuniones (Teams y Google Meet).
const (
	ProveedorTeams      = "teams"
	ProveedorGoogleChat = "google-chat"
)

func leerChat(ruta string) []Cuenta {
	var cs []cuentaChat
	if espacio.LeerJSON(ruta, &cs) != nil {
		return nil
	}
	var out []Cuenta
	for _, c := range cs {
		n := Cuenta{
			ID: c.ID, Proveedor: c.Proveedor, Rotulo: cmp.Or(c.Rotulo, c.Usuario), Direccion: c.Usuario,
			Servicios: []Servicio{Chat}, Estado: EstadoOK, Origen: OrigenChat,
		}
		if c.Proveedor == ProveedorTeams || c.Proveedor == ProveedorGoogleChat {
			n.Servicios = append(n.Servicios, Reuniones)
		}
		if c.Estado == EstadoReconectar {
			n.Estado = EstadoReconectar
		}
		for _, dir := range append([]string{c.Servidor}, c.Servidores...) {
			if s, ok := servidorHTTPS(Chat, dir); ok && !slices.Contains(n.Servidores, s) {
				n.Servidores = append(n.Servidores, s)
			}
		}
		// Solo nombres exactos: un host con punto delante no lo escribe nadie aquí.
		if h := strings.TrimSpace(c.OAuth); h != "" && !strings.HasPrefix(h, ".") && !strings.ContainsAny(h, "/:") {
			n.OAuth = h
		}
		out = append(out, n)
	}
	return out
}

type cuentaNube struct {
	ID      string `json:"id"`
	Tipo    string `json:"tipo"`
	Rotulo  string `json:"rotulo"`
	URL     string `json:"url"`
	Usuario string `json:"usuario"`
}

func leerNube(ruta string) []Cuenta {
	var g struct {
		Cuentas []cuentaNube `json:"cuentas"`
	}
	if espacio.LeerJSON(ruta, &g) != nil {
		return nil
	}
	var out []Cuenta
	for _, c := range g.Cuentas {
		n := Cuenta{
			ID: c.ID, Proveedor: c.Tipo, Rotulo: c.Rotulo, Direccion: c.Usuario,
			Servicios: []Servicio{Nube}, Estado: EstadoOK, Origen: OrigenNube,
		}
		if s, ok := servidorHTTPS(Nube, c.URL); ok {
			n.Servidores = []Servidor{s}
		}
		out = append(out, n)
	}
	return out
}

func servidorHTTPS(sv Servicio, direccion string) (Servidor, bool) {
	if strings.TrimSpace(direccion) == "" {
		return Servidor{}, false
	}
	u, err := url.Parse(direccion)
	if err != nil || u.Hostname() == "" {
		return Servidor{}, false
	}
	s := Servidor{Servicio: sv, Protocolo: u.Scheme, Host: u.Hostname(), Seguridad: "tls"}
	if u.Scheme == "http" {
		s.Seguridad = "ninguna"
		if !red.EsLocal(s.Host) {
			// Fuera del equipo, una dirección http no abre nada (solo TLS, §5.8).
			return Servidor{}, false
		}
	}
	if p := u.Port(); p != "" {
		s.Puerto, _ = strconv.Atoi(p)
	}
	return s, true
}

// cuentaAgenda es lo que guarda `local/agenda` (sin secretos).
type cuentaAgenda struct {
	ID         string     `json:"id"`
	Proveedor  string     `json:"proveedor"`
	Rotulo     string     `json:"rotulo"`
	Usuario    string     `json:"usuario"`
	Servicios  []Servicio `json:"servicios"`
	Servidores []struct {
		Servicio Servicio `json:"servicio"`
		Host     string   `json:"host"`
		Puerto   int      `json:"puerto"`
	} `json:"servidores"`
	OAuth  string `json:"oauth"`
	Estado string `json:"estado"`
}

func leerAgenda(ruta string) []Cuenta {
	var g struct {
		Cuentas []cuentaAgenda `json:"cuentas"`
	}
	if espacio.LeerJSON(ruta, &g) != nil {
		return nil
	}
	var out []Cuenta
	for _, c := range g.Cuentas {
		n := Cuenta{
			ID: c.ID, Proveedor: c.Proveedor, Rotulo: cmp.Or(c.Rotulo, c.Usuario), Direccion: c.Usuario,
			Servicios: slices.Clone(c.Servicios), Estado: EstadoOK, Origen: OrigenAgenda, OAuth: c.OAuth,
		}
		if c.Estado == EstadoReconectar {
			n.Estado = EstadoReconectar
		}
		for _, s := range c.Servidores {
			// Solo nombres exactos: un host con punto delante no lo escribe la persona.
			if s.Host == "" || strings.HasPrefix(s.Host, ".") || (s.Servicio != Calendario && s.Servicio != Contactos) {
				continue
			}
			n.Servidores = append(n.Servidores, Servidor{Servicio: s.Servicio, Protocolo: "https", Host: s.Host, Puerto: s.Puerto, Seguridad: "tls"})
		}
		out = append(out, n)
	}
	return out
}
