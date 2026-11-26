// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // la API cliente-servidor de Matrix usa estas claves fijas con punto (m.relates_to…).
package chat

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Matrix es una cuenta de un servidor Matrix por la API cliente-servidor, SIN cifrado de
// extremo a extremo: no enlaza ningún motor de Olm/Megolm. Lee y escribe en las salas sin
// cifrar; las cifradas salen en la lista, avisan de que no se pueden leer y no admiten
// envíos (mandar en claro a una sala cifrada rompería lo que sus miembros esperan).
//
// Sytue Rooms online es Matrix: con la dirección de su servidor y un token o una
// contraseña, sus salas sin cifrar se leen aquí como las de cualquier otro servidor.
type Matrix struct {
	Servidor string // https://matrix.example.org
	Usuario  string // @ana:example.org
	token    string
	cli      *http.Client
	txn      atomic.Int64

	mu       sync.Mutex
	salas    map[string]*salaMx
	directos map[string]bool
	// directosDe es el `m.direct` tal cual (persona → salas): se reescribe entero al
	// abrir un directo nuevo.
	directosDe map[string][]string
	siguient   string
	sincro     bool
}

var (
	_ Proveedor    = (*Matrix)(nil)
	_ Editor       = (*Matrix)(nil)
	_ Reaccionador = (*Matrix)(nil)
	_ Hilos        = (*Matrix)(nil)
	_ Abandonador  = (*Matrix)(nil)
	_ Miembros     = (*Matrix)(nil)
)

type salaMx struct {
	id, nombre, alias, asunto string
	reglas                    string
	cifrada                   bool
	creada                    time.Time
	creador                   string            // mxid de quien la creó
	miembros                  map[string]string // mxid → nombre
	heroes                    []string
	noLeidos, menciones       int
	eventos                   []api.RoomsMessage // raíces y respuestas, en orden de lectura
	reacciones                map[string]reaccionMx
	atras                     string // token para paginar hacia atrás
	completa                  bool
}

type reaccionMx struct{ objetivo, clave, autor string }

// TextoCifrado es lo que se enseña en lugar de un mensaje cifrado.
var TextoCifrado = "🔒 Mensaje cifrado de extremo a extremo: Sytue Office no puede leerlo."

// TextoSalaCifrada es el propósito que se enseña de una sala cifrada.
var TextoSalaCifrada = "Sala cifrada de extremo a extremo: Sytue Office no puede leerla ni escribir en ella."

// NuevaMatrix abre una cuenta con su token. cli es el cliente HTTP (el del portero de la
// edición); nil, ninguno: falla cerrado.
func NuevaMatrix(servidor, usuario, token string, cli *http.Client) *Matrix {
	if cli == nil {
		cli = &http.Client{Transport: red.Denegar{}, Timeout: 90 * time.Second} // sin el del portero, ninguno
	}
	m := &Matrix{Servidor: strings.TrimRight(servidor, "/"), Usuario: usuario, token: token, cli: cli, salas: map[string]*salaMx{}, directos: map[string]bool{}}
	m.txn.Store(time.Now().UnixNano())
	return m
}

// SesionMatrix es lo que deja un inicio de sesión.
type SesionMatrix struct {
	Servidor, Usuario, Token, Dispositivo string
}

// IniciarMatrix entra con usuario y contraseña (m.login.password). La contraseña no se
// guarda: solo el token que devuelve el servidor.
func IniciarMatrix(ctx context.Context, cli *http.Client, servidor, usuario, clave, dispositivo string) (SesionMatrix, error) {
	m := NuevaMatrix(servidor, "", "", cli)
	cuerpo := map[string]any{
		"type":                        "m.login.password",
		"identifier":                  map[string]string{"type": "m.id.user", "user": usuario},
		"password":                    clave,
		"initial_device_display_name": dispositivo,
	}
	var r struct {
		UserID      string `json:"user_id"`
		AccessToken string `json:"access_token"`
		DeviceID    string `json:"device_id"`
	}
	if err := m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/login", nil, cuerpo, &r); err != nil {
		return SesionMatrix{}, err
	}
	if r.AccessToken == "" {
		return SesionMatrix{}, errors.New("chat: el servidor Matrix no devolvió token")
	}
	return SesionMatrix{Servidor: m.Servidor, Usuario: r.UserID, Token: r.AccessToken, Dispositivo: r.DeviceID}, nil
}

// QuienSoy comprueba el token y devuelve el usuario (whoami).
func (m *Matrix) QuienSoy(ctx context.Context) (string, error) {
	var r struct {
		UserID string `json:"user_id"`
	}
	if err := m.pedir(ctx, http.MethodGet, "/_matrix/client/v3/account/whoami", nil, nil, &r); err != nil {
		return "", err
	}
	m.Usuario = r.UserID
	return r.UserID, nil
}

// CerrarSesion invalida el token en el servidor (logout).
func (m *Matrix) CerrarSesion(ctx context.Context) error {
	return m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/logout", nil, map[string]any{}, nil)
}

// ErrMatrix es un error devuelto por el servidor.
type ErrMatrix struct {
	Estado  int
	Codigo  string
	Mensaje string
}

func (e *ErrMatrix) Error() string {
	return fmt.Sprintf("matrix: %d %s: %s", e.Estado, e.Codigo, e.Mensaje)
}

func (m *Matrix) pedir(ctx context.Context, metodo, ruta string, q url.Values, cuerpo, out any) error {
	u := m.Servidor + ruta
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if cuerpo != nil {
		raw, err := json.Marshal(cuerpo)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, rd)
	if err != nil {
		return err
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		e := &ErrMatrix{Estado: resp.StatusCode}
		var cuerpoErr struct {
			Errcode string `json:"errcode"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(raw, &cuerpoErr)
		e.Codigo, e.Mensaje = cuerpoErr.Errcode, cuerpoErr.Error
		return e
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func ruta(partes ...string) string {
	var b strings.Builder
	for i, p := range partes {
		if i%2 == 1 {
			b.WriteString(url.PathEscape(p))
		} else {
			b.WriteString(p)
		}
	}
	return b.String()
}

// ----- Sincronización -----

type eventoMx struct {
	Tipo     string          `json:"type"`
	ID       string          `json:"event_id"`
	Remite   string          `json:"sender"`
	Hora     int64           `json:"origin_server_ts"`
	Estado   *string         `json:"state_key"`
	Contenid json.RawMessage `json:"content"`
	Redacta  string          `json:"redacts"`
}

type lineaMx struct {
	Eventos []eventoMx `json:"events"`
	Atras   string     `json:"prev_batch"`
	Cortada bool       `json:"limited"`
}

type salaSync struct {
	Estado struct {
		Eventos []eventoMx `json:"events"`
	} `json:"state"`
	Linea   lineaMx `json:"timeline"`
	Resumen struct {
		Heroes []string `json:"m.heroes"`
	} `json:"summary"`
	Pendiente struct {
		Avisos    int `json:"notification_count"`
		Destacado int `json:"highlight_count"`
	} `json:"unread_notifications"`
}

type respSync struct {
	Siguiente string `json:"next_batch"`
	Cuenta    struct {
		Eventos []eventoMx `json:"events"`
	} `json:"account_data"`
	Salas struct {
		Dentro map[string]salaSync `json:"join"`
		Fuera  map[string]struct{} `json:"leave"`
	} `json:"rooms"`
}

const filtroSync = `{"room":{"timeline":{"limit":30},"state":{"lazy_load_members":true}}}`

// sincronizar pide /sync y aplica lo que trae; devuelve los eventos que hay que avisar.
func (m *Matrix) sincronizar(ctx context.Context, espera time.Duration) ([]api.RoomsEvent, error) {
	m.mu.Lock()
	desde := m.siguient
	m.mu.Unlock()
	q := url.Values{"filter": {filtroSync}, "timeout": {strconv.FormatInt(espera.Milliseconds(), 10)}}
	if desde != "" {
		q.Set("since", desde)
	}
	var r respSync
	if err := m.pedir(ctx, http.MethodGet, "/_matrix/client/v3/sync", q, nil, &r); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.siguient != desde { // otra sincronización llegó antes: lo suyo ya está aplicado
		return nil, nil
	}
	m.siguient, m.sincro = r.Siguiente, true
	for _, ev := range r.Cuenta.Eventos {
		if ev.Tipo == "m.direct" {
			m.aplicarDirectos(ev)
		}
	}
	for id := range r.Salas.Fuera {
		delete(m.salas, id)
	}
	var avisos []api.RoomsEvent
	for id, s := range r.Salas.Dentro {
		avisos = append(avisos, m.aplicarSala(id, s, desde)...)
	}
	return avisos, nil
}

// aplicarDirectos toma el m.direct de la cuenta: qué salas son mensajes directos y con
// quién. Uno que no se lee no cambia nada. Con m.mu cogido.
func (m *Matrix) aplicarDirectos(ev eventoMx) {
	var d map[string][]string
	if json.Unmarshal(ev.Contenid, &d) != nil {
		return
	}
	clear(m.directos)
	m.directosDe = d
	for _, salas := range d {
		for _, s := range salas {
			m.directos[s] = true
		}
	}
}

// aplicarSala mete en la caché lo que trae /sync de una sala en la que se está (desde
// es el since de la petición: "" la primera vez) y devuelve los eventos que hay que
// avisar. Con m.mu cogido.
func (m *Matrix) aplicarSala(id string, s salaSync, desde string) []api.RoomsEvent {
	sala := m.salas[id]
	if sala == nil {
		sala = &salaMx{id: id, miembros: map[string]string{}, reacciones: map[string]reaccionMx{}}
		m.salas[id] = sala
	}
	for _, ev := range s.Estado.Eventos {
		sala.aplicarEstado(ev)
	}
	if len(s.Resumen.Heroes) > 0 {
		sala.heroes = s.Resumen.Heroes
	}
	sala.noLeidos, sala.menciones = s.Pendiente.Avisos, s.Pendiente.Destacado
	if desde == "" || s.Linea.Cortada || len(sala.eventos) == 0 {
		sala.eventos = nil
		sala.atras = s.Linea.Atras
		sala.completa = s.Linea.Atras == ""
	}
	var avisos []api.RoomsEvent
	for _, ev := range s.Linea.Eventos {
		if ev.Estado != nil {
			sala.aplicarEstado(ev)
		}
		tipo, msg, ok := m.aplicar(sala, ev, false)
		if ok && desde != "" {
			raw, _ := json.Marshal(msg)
			avisos = append(avisos, api.RoomsEvent{Type: tipo, ChannelID: id, Subject: m.autor(ev.Remite), Payload: raw})
		}
	}
	return avisos
}

func (s *salaMx) aplicarEstado(ev eventoMx) {
	var c struct {
		Name        string `json:"name"`
		Alias       string `json:"alias"`
		Topic       string `json:"topic"`
		JoinRule    string `json:"join_rule"`
		Membership  string `json:"membership"`
		Displayname string `json:"displayname"`
		Algorithm   string `json:"algorithm"`
	}
	_ = json.Unmarshal(ev.Contenid, &c)
	switch ev.Tipo {
	case "m.room.name":
		s.nombre = c.Name
	case "m.room.canonical_alias":
		s.alias = c.Alias
	case "m.room.topic":
		s.asunto = c.Topic
	case "m.room.join_rules":
		s.reglas = c.JoinRule
	case "m.room.encryption":
		s.cifrada = true
	case "m.room.create":
		s.creada, s.creador = time.UnixMilli(ev.Hora), ev.Remite
	case "m.room.member":
		if ev.Estado == nil {
			return
		}
		if c.Membership == "join" || c.Membership == "invite" {
			s.miembros[*ev.Estado] = cmp.Or(c.Displayname, localpart(*ev.Estado))
		} else {
			delete(s.miembros, *ev.Estado)
		}
	}
}

func localpart(mxid string) string {
	s := strings.TrimPrefix(mxid, "@")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}

func (m *Matrix) autor(mxid string) string {
	if mxid == m.Usuario {
		return Yo
	}
	return mxid
}

type contenidoMx struct {
	Tipo       string          `json:"msgtype"`
	Cuerpo     string          `json:"body"`
	URL        string          `json:"url,omitempty"`
	Fichero    string          `json:"filename,omitempty"`
	Info       *infoMx         `json:"info,omitempty"`
	Nuevo      *contenidoMx    `json:"m.new_content"`
	Relacion   *relacionMx     `json:"m.relates_to"`
	Mencionado json.RawMessage `json:"m.mentions"`
}

type relacionMx struct {
	Tipo    string `json:"rel_type,omitempty"`
	Evento  string `json:"event_id,omitempty"`
	Clave   string `json:"key,omitempty"`
	Recurre bool   `json:"is_falling_back,omitempty"`
	Respues *struct {
		Evento string `json:"event_id"`
	} `json:"m.in_reply_to,omitempty"`
}

// aplicar mete un evento de la línea de tiempo en la caché de la sala. alPrincipio es
// para la paginación hacia atrás. Devuelve el tipo de trama y el mensaje afectado.
// Con m.mu cogido.
func (m *Matrix) aplicar(s *salaMx, ev eventoMx, alPrincipio bool) (string, api.RoomsMessage, bool) {
	cuando := time.UnixMilli(ev.Hora)
	switch ev.Tipo {
	case "m.room.encrypted":
		msg := api.RoomsMessage{ID: ev.ID, ChannelID: s.id, AuthorSub: m.autor(ev.Remite), AuthorName: s.nombreDe(ev.Remite), Body: TextoCifrado, CreatedAt: cuando, Type: "notice"}
		return "message", msg, s.poner(msg, alPrincipio)
	case "m.room.message":
		return m.aplicarMensaje(s, ev, cuando, alPrincipio)
	case "m.reaction":
		return m.aplicarReaccion(s, ev)
	case "m.room.redaction":
		return aplicarRedaccion(s, ev)
	}
	return "", api.RoomsMessage{}, false
}

// aplicarMensaje es aplicar para un m.room.message: uno nuevo (o de un hilo) o la
// edición de uno que ya está.
func (m *Matrix) aplicarMensaje(s *salaMx, ev eventoMx, cuando time.Time, alPrincipio bool) (string, api.RoomsMessage, bool) {
	var c contenidoMx
	if json.Unmarshal(ev.Contenid, &c) != nil {
		return "", api.RoomsMessage{}, false
	}
	if c.Relacion != nil && c.Relacion.Tipo == "m.replace" && c.Nuevo != nil {
		i := s.indice(c.Relacion.Evento)
		if i < 0 || s.eventos[i].AuthorSub != m.autor(ev.Remite) {
			return "", api.RoomsMessage{}, false
		}
		s.eventos[i].Body = cuerpoDe(*c.Nuevo)
		s.eventos[i].EditedAt = &cuando
		return "message_edited", s.eventos[i], true
	}
	msg := api.RoomsMessage{ID: ev.ID, ChannelID: s.id, AuthorSub: m.autor(ev.Remite), AuthorName: s.nombreDe(ev.Remite), Body: cuerpoDe(c), CreatedAt: cuando}
	if c.Tipo == "m.notice" {
		msg.IsBot = true
	}
	if a, ok := adjuntoMx(c); ok {
		msg.Attachments = []api.RoomsAttachment{a}
		// Con `filename`, el `body` es el pie (Matrix 1.10); sin él, es el nombre.
		msg.Body = ""
		if c.Fichero != "" && c.Cuerpo != c.Fichero {
			msg.Body = c.Cuerpo
		}
	}
	if c.Relacion != nil && c.Relacion.Tipo == "m.thread" && c.Relacion.Evento != "" {
		raiz := c.Relacion.Evento
		msg.ParentID = &raiz
		if !s.poner(msg, alPrincipio) {
			return "", api.RoomsMessage{}, false
		}
		s.contarRespuesta(raiz, msg, cuando)
		return "message", msg, true
	}
	return "message", msg, s.poner(msg, alPrincipio)
}

// contarRespuesta apunta en la raíz de un hilo, si está en la caché, la respuesta msg.
func (s *salaMx) contarRespuesta(raiz string, msg api.RoomsMessage, cuando time.Time) {
	i := s.indice(raiz)
	if i < 0 {
		return
	}
	r := &s.eventos[i]
	r.ReplyCount++
	if r.LastReplyAt == nil || cuando.After(*r.LastReplyAt) {
		r.LastReplyAt = &cuando
	}
	if !slices.ContainsFunc(r.ThreadParticipants, func(p api.RoomsThreadParticipant) bool { return p.Sub == msg.AuthorSub }) {
		r.ThreadParticipants = append(r.ThreadParticipants, api.RoomsThreadParticipant{Sub: msg.AuthorSub, Name: msg.AuthorName})
	}
}

// aplicarReaccion es aplicar para un m.reaction (una m.annotation sobre un mensaje).
func (m *Matrix) aplicarReaccion(s *salaMx, ev eventoMx) (string, api.RoomsMessage, bool) {
	var c contenidoMx
	if json.Unmarshal(ev.Contenid, &c) != nil || c.Relacion == nil || c.Relacion.Tipo != "m.annotation" {
		return "", api.RoomsMessage{}, false
	}
	if _, ya := s.reacciones[ev.ID]; ya {
		return "", api.RoomsMessage{}, false
	}
	i := s.indice(c.Relacion.Evento)
	if i < 0 {
		return "", api.RoomsMessage{}, false
	}
	s.reacciones[ev.ID] = reaccionMx{objetivo: c.Relacion.Evento, clave: c.Relacion.Clave, autor: m.autor(ev.Remite)}
	sumarReaccion(&s.eventos[i], c.Relacion.Clave, m.autor(ev.Remite), true)
	return "reaction", s.eventos[i], true
}

// aplicarRedaccion es aplicar para un m.room.redaction: quita una reacción o un mensaje.
func aplicarRedaccion(s *salaMx, ev eventoMx) (string, api.RoomsMessage, bool) {
	objetivo := ev.Redacta
	if objetivo == "" {
		var c struct {
			Redacts string `json:"redacts"`
		}
		_ = json.Unmarshal(ev.Contenid, &c)
		objetivo = c.Redacts
	}
	if r, ok := s.reacciones[objetivo]; ok {
		delete(s.reacciones, objetivo)
		if i := s.indice(r.objetivo); i >= 0 {
			sumarReaccion(&s.eventos[i], r.clave, r.autor, false)
			return "reaction", s.eventos[i], true
		}
		return "", api.RoomsMessage{}, false
	}
	if i := s.indice(objetivo); i >= 0 {
		msg := s.eventos[i]
		s.eventos = slices.Delete(s.eventos, i, i+1)
		return "message_deleted", msg, true
	}
	return "", api.RoomsMessage{}, false
}

func cuerpoDe(c contenidoMx) string {
	switch c.Tipo {
	case "m.emote":
		return "* " + c.Cuerpo
	case "m.image", "m.file", "m.video", "m.audio":
		return "📎 " + c.Cuerpo
	}
	return c.Cuerpo
}

func sumarReaccion(m *api.RoomsMessage, clave, autor string, poner bool) {
	i := slices.IndexFunc(m.Reactions, func(r api.RoomsReaction) bool { return r.Emoji == clave })
	if poner {
		if i < 0 {
			m.Reactions = append(m.Reactions, api.RoomsReaction{Emoji: clave})
			i = len(m.Reactions) - 1
		}
		if !slices.Contains(m.Reactions[i].Subs, autor) {
			m.Reactions[i].Subs = append(m.Reactions[i].Subs, autor)
			m.Reactions[i].Count++
		}
		return
	}
	if i < 0 {
		return
	}
	r := &m.Reactions[i]
	r.Subs = slices.DeleteFunc(r.Subs, func(s string) bool { return s == autor })
	r.Count = max(0, r.Count-1)
	if r.Count == 0 {
		m.Reactions = slices.Delete(m.Reactions, i, i+1)
	}
}

func (s *salaMx) indice(id string) int {
	return slices.IndexFunc(s.eventos, func(m api.RoomsMessage) bool { return m.ID == id })
}

// poner añade msg si no estaba (el eco de un envío propio llega también por /sync).
func (s *salaMx) poner(msg api.RoomsMessage, alPrincipio bool) bool {
	if s.indice(msg.ID) >= 0 {
		return false
	}
	if alPrincipio {
		s.eventos = slices.Insert(s.eventos, 0, msg)
	} else {
		s.eventos = append(s.eventos, msg)
	}
	return true
}

func (s *salaMx) nombreDe(mxid string) string {
	return cmp.Or(s.miembros[mxid], localpart(mxid))
}

func (m *Matrix) asegurar(ctx context.Context) error {
	m.mu.Lock()
	hecho := m.sincro
	m.mu.Unlock()
	if hecho {
		return nil
	}
	_, err := m.sincronizar(ctx, 0)
	return err
}

// ----- Proveedor -----

// Rotulo es el usuario de la cuenta.
func (m *Matrix) Rotulo() string { return m.Usuario }

// Canales son las salas en las que se está.
func (m *Matrix) Canales(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	if err := m.asegurar(ctx); err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var cs []api.RoomsChannel
	var us []api.RoomsUnread
	for _, s := range m.salas {
		c := m.canalDe(s)
		cs = append(cs, c)
		if s.noLeidos > 0 || s.menciones > 0 {
			us = append(us, api.RoomsUnread{ChannelID: s.id, Unread: s.noLeidos, Mentions: s.menciones})
		}
	}
	slices.SortFunc(cs, func(a, b api.RoomsChannel) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return cs, us, nil
}

// canalDe es la sala como la ve la pantalla. Con m.mu cogido.
func (m *Matrix) canalDe(s *salaMx) api.RoomsChannel {
	c := api.RoomsChannel{ID: s.id, Slug: s.id, Name: m.nombreSala(s), Topic: s.asunto, CreatedAt: s.creada, NotifyLevel: "all"}
	if s.creador != "" {
		c.CreatedBy = m.autor(s.creador)
	}
	switch {
	case m.directos[s.id]:
		c.Kind = "direct"
		for mx, n := range s.miembros {
			if mx != m.Usuario {
				c.Peers, c.PeerSubs = append(c.Peers, n), append(c.PeerSubs, mx)
			}
		}
	case s.reglas == "public":
		c.Kind = "public"
	default:
		c.Kind = "private"
	}
	if s.cifrada {
		c.Purpose = TextoSalaCifrada
		c.Topic = strings.TrimSpace("🔒 " + s.asunto)
	}
	return c
}

func (m *Matrix) nombreSala(s *salaMx) string {
	if s.nombre != "" {
		return s.nombre
	}
	if s.alias != "" {
		return s.alias
	}
	var ns []string
	for _, h := range s.heroes {
		if h != m.Usuario {
			ns = append(ns, s.nombreDe(h))
		}
	}
	if len(ns) == 0 {
		for mx, n := range s.miembros {
			if mx != m.Usuario {
				ns = append(ns, n)
			}
		}
		slices.Sort(ns)
	}
	if len(ns) > 0 {
		return strings.Join(ns, ", ")
	}
	return s.id
}

func (m *Matrix) sala(canal string) (*salaMx, error) {
	s := m.salas[canal]
	if s == nil {
		return nil, fmt.Errorf("chat: sala %q desconocida", canal)
	}
	return s, nil
}

// Mensajes pagina hacia atrás con /messages lo que haga falta.
func (m *Matrix) Mensajes(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	if err := m.asegurar(ctx); err != nil {
		return nil, err
	}
	limite = cmp.Or(limite, 50)
	for range 10 { // tope de vueltas: no se baja el historial entero por una página
		m.mu.Lock()
		s, err := m.sala(canal)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		out := raicesAntes(s.eventos, antes)
		desde, completa := s.atras, s.completa
		m.mu.Unlock()
		if len(out) >= limite || completa || desde == "" {
			if len(out) > limite {
				out = out[len(out)-limite:]
			}
			return out, nil
		}
		if err := m.atras(ctx, canal, desde); err != nil {
			return nil, err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil {
		return nil, err
	}
	out := raicesAntes(s.eventos, antes)
	if len(out) > limite {
		out = out[len(out)-limite:]
	}
	return out, nil
}

func raicesAntes(ms []api.RoomsMessage, antes time.Time) []api.RoomsMessage {
	var out []api.RoomsMessage
	for _, m := range ms {
		if m.ParentID == nil && (antes.IsZero() || m.CreatedAt.Before(antes)) {
			out = append(out, m)
		}
	}
	return out
}

func (m *Matrix) atras(ctx context.Context, canal, desde string) error {
	var r struct {
		Trozo []eventoMx `json:"chunk"`
		Fin   string     `json:"end"`
	}
	q := url.Values{"dir": {"b"}, "limit": {"50"}, "from": {desde}}
	if err := m.pedir(ctx, http.MethodGet, ruta("/_matrix/client/v3/rooms/", canal, "/messages"), q, nil, &r); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil || s.atras != desde {
		return err
	}
	// El trozo viene del más nuevo al más viejo: se inserta cada uno al principio.
	for _, ev := range r.Trozo {
		m.aplicar(s, ev, true)
	}
	s.atras = r.Fin
	s.completa = r.Fin == "" || len(r.Trozo) == 0
	return nil
}

func (m *Matrix) mandar(ctx context.Context, canal, tipo string, contenido any) (string, error) {
	var r struct {
		ID string `json:"event_id"`
	}
	txn := "sytueoffice-" + strconv.FormatInt(m.txn.Add(1), 10)
	err := m.pedir(ctx, http.MethodPut, ruta("/_matrix/client/v3/rooms/", canal, "/send/", tipo, "/", txn), nil, contenido, &r)
	return r.ID, err
}

// cifrada da ErrSalaCifrada si la sala está cifrada de extremo a extremo (desde aquí no
// se puede escribir en ella), o el error si no se conoce la sala.
func (m *Matrix) cifrada(canal string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil {
		return err
	}
	if s.cifrada {
		return ErrSalaCifrada
	}
	return nil
}

// Enviar manda un m.text; con raiz, en su hilo (m.thread).
func (m *Matrix) Enviar(ctx context.Context, canal, raiz, texto string) (api.RoomsMessage, error) {
	if err := m.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	if err := m.cifrada(canal); err != nil {
		return api.RoomsMessage{}, err
	}
	c := contenidoMx{Tipo: "m.text", Cuerpo: texto}
	if raiz != "" {
		c.Relacion = &relacionMx{Tipo: "m.thread", Evento: raiz, Recurre: true}
		c.Relacion.Respues = &struct {
			Evento string `json:"event_id"`
		}{raiz}
	}
	id, err := m.mandar(ctx, canal, "m.room.message", c)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	raw, _ := json.Marshal(c)
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	_, msg, _ := m.aplicar(s, eventoMx{Tipo: "m.room.message", ID: id, Remite: m.Usuario, Hora: time.Now().UnixMilli(), Contenid: raw}, false)
	if msg.ID == "" {
		if i := s.indice(id); i >= 0 {
			msg = s.eventos[i]
		}
	}
	return msg, nil
}

// Editar manda un m.replace.
func (m *Matrix) Editar(ctx context.Context, canal, mensaje, texto string) (api.RoomsMessage, error) {
	if err := m.cifrada(canal); err != nil {
		return api.RoomsMessage{}, err
	}
	c := contenidoMx{Tipo: "m.text", Cuerpo: "* " + texto, Nuevo: &contenidoMx{Tipo: "m.text", Cuerpo: texto}, Relacion: &relacionMx{Tipo: "m.replace", Evento: mensaje}}
	id, err := m.mandar(ctx, canal, "m.room.message", c)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	raw, _ := json.Marshal(c)
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	_, msg, _ := m.aplicar(s, eventoMx{Tipo: "m.room.message", ID: id, Remite: m.Usuario, Hora: time.Now().UnixMilli(), Contenid: raw}, false)
	return msg, nil
}

func (m *Matrix) redactar(ctx context.Context, canal, evento string) error {
	txn := "sytueoffice-" + strconv.FormatInt(m.txn.Add(1), 10)
	return m.pedir(ctx, http.MethodPut, ruta("/_matrix/client/v3/rooms/", canal, "/redact/", evento, "/", txn), nil, map[string]any{}, nil)
}

// Borrar redacta el mensaje.
func (m *Matrix) Borrar(ctx context.Context, canal, mensaje string) error {
	if err := m.redactar(ctx, canal, mensaje); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, err := m.sala(canal); err == nil {
		m.aplicar(s, eventoMx{Tipo: "m.room.redaction", Redacta: mensaje}, false)
	}
	return nil
}

// Reaccionar manda un m.annotation, o redacta la reacción propia.
func (m *Matrix) Reaccionar(ctx context.Context, canal, mensaje, emoji string, poner bool) error {
	if err := m.cifrada(canal); err != nil {
		return err
	}
	if !poner {
		m.mu.Lock()
		propia := ""
		if s, err := m.sala(canal); err == nil {
			for id, r := range s.reacciones {
				if r.objetivo == mensaje && r.clave == emoji && r.autor == Yo {
					propia = id
				}
			}
		}
		m.mu.Unlock()
		if propia == "" {
			return nil
		}
		return m.Borrar(ctx, canal, propia)
	}
	c := map[string]any{"m.relates_to": relacionMx{Tipo: "m.annotation", Evento: mensaje, Clave: emoji}}
	id, err := m.mandar(ctx, canal, "m.reaction", c)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(c)
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, err := m.sala(canal); err == nil {
		m.aplicar(s, eventoMx{Tipo: "m.reaction", ID: id, Remite: m.Usuario, Hora: time.Now().UnixMilli(), Contenid: raw}, false)
	}
	return nil
}

// Hilo es la raíz con las respuestas que hay en la caché.
func (m *Matrix) Hilo(_ context.Context, canal, raiz string) (api.RoomsThread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sala(canal)
	if err != nil {
		return api.RoomsThread{}, err
	}
	i := s.indice(raiz)
	if i < 0 {
		return api.RoomsThread{}, fmt.Errorf("chat: mensaje %q no está entre los cargados", raiz)
	}
	t := api.RoomsThread{Root: s.eventos[i], Following: true}
	for _, e := range s.eventos {
		if e.ParentID != nil && *e.ParentID == raiz {
			t.Replies = append(t.Replies, e)
		}
	}
	return t, nil
}

// MarcarLeido manda el recibo de lectura del último evento.
func (m *Matrix) MarcarLeido(ctx context.Context, canal string) error {
	m.mu.Lock()
	s, err := m.sala(canal)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	s.noLeidos, s.menciones = 0, 0
	ultimo := ""
	if n := len(s.eventos); n > 0 {
		ultimo = s.eventos[n-1].ID
	}
	m.mu.Unlock()
	if ultimo == "" {
		return nil
	}
	return m.pedir(ctx, http.MethodPost, ruta("/_matrix/client/v3/rooms/", canal, "/read_markers"), nil,
		map[string]string{"m.fully_read": ultimo, "m.read": ultimo}, nil)
}

// Salir abandona la sala.
func (m *Matrix) Salir(ctx context.Context, canal string) error {
	if err := m.pedir(ctx, http.MethodPost, ruta("/_matrix/client/v3/rooms/", canal, "/leave"), nil, map[string]any{}, nil); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.salas, canal)
	m.mu.Unlock()
	return nil
}

// Miembros son los de la sala (joined_members).
func (m *Matrix) Miembros(ctx context.Context, canal string) ([]api.RoomsMember, error) {
	var r struct {
		Dentro map[string]struct {
			Nombre string `json:"display_name"`
		} `json:"joined"`
	}
	if err := m.pedir(ctx, http.MethodGet, ruta("/_matrix/client/v3/rooms/", canal, "/joined_members"), nil, nil, &r); err != nil {
		return nil, err
	}
	var out []api.RoomsMember
	for mx, d := range r.Dentro {
		out = append(out, api.RoomsMember{Subject: m.autor(mx), Name: cmp.Or(d.Nombre, localpart(mx)), Role: "member"})
	}
	slices.SortFunc(out, func(a, b api.RoomsMember) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Escuchar sincroniza en bucle (sondeo largo de /sync) y entrega lo nuevo.
func (m *Matrix) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	if err := m.asegurar(ctx); err != nil {
		return err
	}
	for {
		avisos, err := m.sincronizar(ctx, 30*time.Second)
		if err != nil {
			return err
		}
		for _, ev := range avisos {
			alLlegar(ev)
		}
	}
}
