// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// ProveedorSlack es una cuenta de Slack.
const ProveedorSlack = "slack"

// APISlack es la Web API de Slack. Se cambia en las pruebas.
var APISlack = "https://slack.com/api"

// Slack es un espacio de Slack conectado con la «app propia del espacio» (D-20): la
// persona instala en SU espacio una app interna con el manifiesto que damos
// ([ManifiestoSlack]) y pega aquí su token de usuario (`xoxp-…`). Así no le alcanza el
// límite que Slack pone desde 2025 a las apps distribuidas fuera de su Marketplace
// (una petición por minuto y 15 mensajes en `conversations.history`).
//
// Lo que llega en vivo va por Socket Mode si la persona da también el token de la app
// (`xapp-…`, con `connections:write`); sin él, se sondean los canales abiertos cada
// [SondeoSlack], que es más lento pero no pide nada más. La app nunca abre un puerto.
type Slack struct {
	Token, TokenApp string
	HTTP            *http.Client
	// Sondeo es cada cuánto se miran los canales sin Socket Mode.
	Sondeo time.Duration

	mu       sync.Mutex
	yo       string
	equipo   string
	nombres  map[string]string // usuario → nombre
	canales  map[string]canalSlack
	ultimo   map[string]string // canal → ts del último mensaje visto
	sinLeer  map[string]int
	abiertos map[string]bool // canales cuyos mensajes se han pedido (los que se sondean)
	ficheros map[string]ficheroSlack
	raices   map[string]raizSlack // canal|ts → raíz con respuestas vista
	gente    []api.RoomsMember
}

type canalSlack struct {
	ID, Nombre, Asunto, Proposito string
	Privado, Directo, Archivado   bool
	Usuario                       string // el otro, en un directo
	Creado                        int64
}

// SondeoSlack es el intervalo de sondeo sin Socket Mode.
const SondeoSlack = 20 * time.Second

// NuevaSlack prepara la cuenta; no habla con la red hasta que se le pide algo.
func NuevaSlack(token, tokenApp string, hc *http.Client) *Slack {
	if hc == nil {
		hc = &http.Client{Transport: red.Denegar{}, Timeout: 30 * time.Second} // sin el del portero, ninguno
	}
	return &Slack{
		Token: token, TokenApp: tokenApp, HTTP: hc, Sondeo: SondeoSlack,
		nombres: map[string]string{}, canales: map[string]canalSlack{}, ultimo: map[string]string{}, sinLeer: map[string]int{}, abiertos: map[string]bool{},
		ficheros: map[string]ficheroSlack{}, raices: map[string]raizSlack{},
	}
}

var (
	_ Proveedor    = (*Slack)(nil)
	_ Editor       = (*Slack)(nil)
	_ Reaccionador = (*Slack)(nil)
	_ Hilos        = (*Slack)(nil)
	_ Miembros     = (*Slack)(nil)
	_ Buscador     = (*Slack)(nil)
)

// ErrSlack es un error que devuelve la API de Slack (`ok: false`).
type ErrSlack = cchat.ErrSlack

// cliente es el de la Web API (`cuentas/chat`, el mismo que usa el servicio de
// integraciones) con el token dado y el transporte de la cuenta.
func (s *Slack) cliente(token string) *cchat.Slack {
	return &cchat.Slack{Base: APISlack, Token: cchat.TokenFijo(token), HTTP: s.HTTP}
}

// llamar hace una petición a la Web API con el token dado y decodifica en out. Un 429
// es el `ratelimited` de Slack.
func (s *Slack) llamar(ctx context.Context, token, metodo string, params url.Values, out any) error {
	err := s.cliente(token).Llamar(ctx, metodo, params, out)
	if cchat.Estado(err) == http.StatusTooManyRequests {
		return &ErrSlack{Codigo: "ratelimited"}
	}
	return err
}

func (s *Slack) api(ctx context.Context, metodo string, params url.Values, out any) error {
	return s.llamar(ctx, s.Token, metodo, params, out)
}

// QuienSoy comprueba el token (`auth.test`) y devuelve «persona · espacio».
func (s *Slack) QuienSoy(ctx context.Context) (string, error) {
	var r struct {
		User   string `json:"user"`
		UserID string `json:"user_id"`
		Team   string `json:"team"`
	}
	if err := s.api(ctx, "auth.test", url.Values{}, &r); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.yo, s.equipo = r.UserID, r.Team
	s.mu.Unlock()
	return strings.TrimSpace(r.User + " · " + r.Team), nil
}

func (s *Slack) asegurar(ctx context.Context) error {
	s.mu.Lock()
	listo := s.yo != ""
	s.mu.Unlock()
	if listo {
		return nil
	}
	_, err := s.QuienSoy(ctx)
	return err
}

// Rotulo es el nombre del espacio.
func (s *Slack) Rotulo() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cmp.Or(s.equipo, "Slack")
}

// Canales son las conversaciones de las que la persona es miembro.
func (s *Slack) Canales(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	if err := s.asegurar(ctx); err != nil {
		return nil, nil, err
	}
	var todos []canalSlack
	cursor := ""
	for range 20 {
		var r struct {
			Channels []canalAPI `json:"channels"`
			Meta     struct {
				Next string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		p := url.Values{"types": {"public_channel,private_channel,mpim,im"}, "exclude_archived": {"true"}, "limit": {"200"}}
		if cursor != "" {
			p.Set("cursor", cursor)
		}
		if err := s.api(ctx, "users.conversations", p, &r); err != nil {
			return nil, nil, err
		}
		for _, c := range r.Channels {
			todos = append(todos, canalDeAPI(c))
		}
		if cursor = r.Meta.Next; cursor == "" {
			break
		}
	}
	var cs []api.RoomsChannel
	var us []api.RoomsUnread
	for _, c := range todos {
		cs = append(cs, s.aCanal(ctx, c))
	}
	s.mu.Lock()
	for _, c := range todos {
		s.canales[c.ID] = c
		if n := s.sinLeer[c.ID]; n > 0 {
			us = append(us, api.RoomsUnread{ChannelID: c.ID, Unread: n})
		}
	}
	s.mu.Unlock()
	return cs, us, nil
}

// nombre resuelve el nombre de una persona (`users.info`), con caché.
func (s *Slack) nombre(ctx context.Context, usuario string) string {
	if usuario == "" {
		return ""
	}
	s.mu.Lock()
	if n, ok := s.nombres[usuario]; ok {
		s.mu.Unlock()
		return n
	}
	s.mu.Unlock()
	var r struct {
		User struct {
			Name     string `json:"name"`
			RealName string `json:"real_name"`
			Profile  struct {
				DisplayName string `json:"display_name"`
			} `json:"profile"`
		} `json:"user"`
	}
	n := usuario
	if s.api(ctx, "users.info", url.Values{"user": {usuario}}, &r) == nil {
		n = cmp.Or(r.User.Profile.DisplayName, r.User.RealName, r.User.Name, usuario)
	}
	s.mu.Lock()
	s.nombres[usuario] = n
	s.mu.Unlock()
	return n
}

// mensajeSlack es un mensaje tal como lo da la API.
type mensajeSlack = cchat.MensajeSlack

// hora pasa un `ts` de Slack («1700000000.000200») a tiempo, en la hora del equipo.
func hora(ts string) time.Time { return cchat.HoraSlack(ts).Local() }

// tsDe pasa un tiempo a `ts`, para pedir «antes de».
func tsDe(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

func (s *Slack) aMensaje(ctx context.Context, canal string, m mensajeSlack) api.RoomsMessage {
	autor := m.User
	var nombre string
	if autor == "" && m.BotID != "" {
		autor, nombre = m.BotID, cmp.Or(m.Username, "bot")
	} else {
		nombre = s.nombre(ctx, autor)
	}
	s.mu.Lock()
	if autor != "" && autor == s.yo {
		autor = Yo
	}
	s.mu.Unlock()
	out := api.RoomsMessage{ID: m.TS, ChannelID: canal, AuthorSub: autor, AuthorName: nombre, Body: m.Text, CreatedAt: hora(m.TS), ReplyCount: m.ReplyCount, IsBot: m.BotID != ""}
	if m.ThreadTS != "" && m.ThreadTS != m.TS {
		padre := m.ThreadTS
		out.ParentID = &padre
		out.AlsoInChannel = m.Subtype == "thread_broadcast"
	}
	if m.LatestRep != "" {
		t := hora(m.LatestRep)
		out.LastReplyAt = &t
	}
	if m.Edited != nil {
		t := hora(m.Edited.TS)
		out.EditedAt = &t
	}
	s.adjuntosDe(&out, m)
	for _, r := range m.Reactions {
		subs := slices.Clone(r.Users)
		for i, u := range subs {
			if u == s.yo {
				subs[i] = Yo
			}
		}
		out.Reactions = append(out.Reactions, api.RoomsReaction{Emoji: ":" + r.Name + ":", Count: r.Count, Subs: subs})
	}
	s.apuntarRaiz(canal, m, out)
	return out
}

// Mensajes es una página de raíces anteriores a antes, en orden de lectura.
func (s *Slack) Mensajes(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	if err := s.asegurar(ctx); err != nil {
		return nil, err
	}
	p := url.Values{"channel": {canal}, "limit": {strconv.Itoa(cmp.Or(limite, 50))}}
	if !antes.IsZero() {
		p.Set("latest", tsDe(antes))
	}
	var r struct {
		Messages []mensajeSlack `json:"messages"`
	}
	if err := s.api(ctx, "conversations.history", p, &r); err != nil {
		return nil, err
	}
	var out []api.RoomsMessage
	for i := len(r.Messages) - 1; i >= 0; i-- { // Slack da el más nuevo primero
		m := r.Messages[i]
		if m.ThreadTS != "" && m.ThreadTS != m.TS && m.Subtype != "thread_broadcast" {
			continue // las respuestas van en su hilo
		}
		out = append(out, s.aMensaje(ctx, canal, m))
	}
	s.mu.Lock()
	s.abiertos[canal] = true
	if antes.IsZero() && len(r.Messages) > 0 && r.Messages[0].TS > s.ultimo[canal] {
		s.ultimo[canal] = r.Messages[0].TS
	}
	s.mu.Unlock()
	return out, nil
}

// Enviar publica texto; con raiz, en su hilo.
func (s *Slack) Enviar(ctx context.Context, canal, raiz, texto string) (api.RoomsMessage, error) {
	if err := s.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	p := url.Values{"channel": {canal}, "text": {texto}}
	if raiz != "" {
		p.Set("thread_ts", raiz)
	}
	var r struct {
		TS      string       `json:"ts"`
		Message mensajeSlack `json:"message"`
	}
	if err := s.api(ctx, "chat.postMessage", p, &r); err != nil {
		return api.RoomsMessage{}, err
	}
	m := r.Message
	m.TS = cmp.Or(m.TS, r.TS)
	if m.ThreadTS == "" && raiz != "" {
		m.ThreadTS = raiz
	}
	s.mu.Lock()
	if m.User == "" {
		m.User = s.yo
	}
	if raiz == "" && m.TS > s.ultimo[canal] {
		s.ultimo[canal] = m.TS
	}
	if r, ok := s.raices[canal+sep+raiz]; ok && raiz != "" {
		r.participa = true
		s.raices[canal+sep+raiz] = r
	}
	s.mu.Unlock()
	return s.aMensaje(ctx, canal, m), nil
}

// MarcarLeido marca el canal hasta el último mensaje visto.
func (s *Slack) MarcarLeido(ctx context.Context, canal string) error {
	s.mu.Lock()
	ts := s.ultimo[canal]
	delete(s.sinLeer, canal)
	s.mu.Unlock()
	if ts == "" {
		return nil
	}
	return s.api(ctx, "conversations.mark", url.Values{"channel": {canal}, "ts": {ts}}, nil)
}

// Editar cambia el texto de un mensaje propio.
func (s *Slack) Editar(ctx context.Context, canal, mensaje, texto string) (api.RoomsMessage, error) {
	var r struct {
		Message mensajeSlack `json:"message"`
	}
	if err := s.api(ctx, "chat.update", url.Values{"channel": {canal}, "ts": {mensaje}, "text": {texto}}, &r); err != nil {
		return api.RoomsMessage{}, err
	}
	m := r.Message
	m.TS = cmp.Or(m.TS, mensaje)
	if m.Text == "" {
		m.Text = texto
	}
	s.mu.Lock()
	if m.User == "" {
		m.User = s.yo
	}
	s.mu.Unlock()
	return s.aMensaje(ctx, canal, m), nil
}

// Borrar borra un mensaje propio.
func (s *Slack) Borrar(ctx context.Context, canal, mensaje string) error {
	return s.api(ctx, "chat.delete", url.Values{"channel": {canal}, "ts": {mensaje}}, nil)
}

// Reaccionar pone o quita una reacción (`:pulgar:` o `pulgar`).
func (s *Slack) Reaccionar(ctx context.Context, canal, mensaje, emoji string, poner bool) error {
	metodo := "reactions.remove"
	if poner {
		metodo = "reactions.add"
	}
	return s.api(ctx, metodo, url.Values{"channel": {canal}, "timestamp": {mensaje}, "name": {strings.Trim(emoji, ":")}}, nil)
}

// Hilo da la raíz y sus respuestas (`conversations.replies`).
func (s *Slack) Hilo(ctx context.Context, canal, raiz string) (api.RoomsThread, error) {
	var r struct {
		Messages []mensajeSlack `json:"messages"`
	}
	if err := s.api(ctx, "conversations.replies", url.Values{"channel": {canal}, "ts": {raiz}, "limit": {"200"}}, &r); err != nil {
		return api.RoomsThread{}, err
	}
	var h api.RoomsThread
	for _, m := range r.Messages {
		if m.TS == raiz {
			h.Root = s.aMensaje(ctx, canal, m)
			continue
		}
		h.Replies = append(h.Replies, s.aMensaje(ctx, canal, m))
	}
	if h.Root.ID == "" {
		return h, errors.New("chat: Slack no devolvió la raíz del hilo")
	}
	return h, nil
}

// Miembros lista quién está en una conversación.
func (s *Slack) Miembros(ctx context.Context, canal string) ([]api.RoomsMember, error) {
	var r struct {
		Members []string `json:"members"`
	}
	if err := s.api(ctx, "conversations.members", url.Values{"channel": {canal}, "limit": {"500"}}, &r); err != nil {
		return nil, err
	}
	out := make([]api.RoomsMember, 0, len(r.Members))
	for _, u := range r.Members {
		out = append(out, api.RoomsMember{Subject: u, Name: s.nombre(ctx, u), Role: "member"})
	}
	return out, nil
}

// Buscar usa `search.messages` (pide el permiso `search:read` del token de usuario).
func (s *Slack) Buscar(ctx context.Context, q string, limite int) ([]api.RoomsMessage, error) {
	var r struct {
		Messages struct {
			Matches []struct {
				mensajeSlack
				Channel struct {
					ID string `json:"id"`
				} `json:"channel"`
			} `json:"matches"`
		} `json:"messages"`
	}
	if err := s.api(ctx, "search.messages", url.Values{"query": {q}, "count": {strconv.Itoa(cmp.Or(limite, 20))}}, &r); err != nil {
		return nil, err
	}
	var out []api.RoomsMessage
	for _, m := range r.Messages.Matches {
		out = append(out, s.aMensaje(ctx, m.Channel.ID, m.mensajeSlack))
	}
	return out, nil
}

// Escuchar entrega lo que llega: por Socket Mode con token de app y, si no, sondeando.
func (s *Slack) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	if err := s.asegurar(ctx); err != nil {
		return err
	}
	if s.TokenApp != "" {
		return s.socketMode(ctx, alLlegar)
	}
	return s.sondear(ctx, alLlegar)
}

// sondear mira los canales abiertos (los que la persona ha leído) cada s.Sondeo.
func (s *Slack) sondear(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	t := time.NewTicker(cmp.Or(s.Sondeo, SondeoSlack))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
		s.mu.Lock()
		canales := make([]string, 0, len(s.abiertos))
		for c := range s.abiertos {
			canales = append(canales, c)
		}
		s.mu.Unlock()
		slices.Sort(canales)
		for _, canal := range canales {
			s.mu.Lock()
			desde := s.ultimo[canal]
			s.mu.Unlock()
			if desde == "" {
				continue
			}
			var r struct {
				Messages []mensajeSlack `json:"messages"`
			}
			if err := s.api(ctx, "conversations.history", url.Values{"channel": {canal}, "oldest": {desde}, "limit": {"100"}}, &r); err != nil {
				return err
			}
			for i := len(r.Messages) - 1; i >= 0; i-- {
				// `oldest` no siempre deja fuera el propio `desde` (`inclusive`, relojes
				// iguales): lo ya visto no se entrega ni se cuenta como sin leer otra vez.
				if r.Messages[i].TS <= desde {
					continue
				}
				s.entregar(ctx, canal, r.Messages[i], "message", alLlegar)
			}
		}
	}
}

func (s *Slack) entregar(ctx context.Context, canal string, m mensajeSlack, tipo string, alLlegar func(api.RoomsEvent)) {
	msg := s.aMensaje(ctx, canal, m)
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.mu.Lock()
	if tipo == "message" && m.TS > s.ultimo[canal] && (m.ThreadTS == "" || m.ThreadTS == m.TS) {
		s.ultimo[canal] = m.TS
	}
	if tipo == "message" && msg.AuthorSub != Yo {
		s.sinLeer[canal]++
	}
	s.mu.Unlock()
	alLlegar(api.RoomsEvent{Type: tipo, ChannelID: canal, Subject: msg.AuthorSub, Name: msg.AuthorName, Payload: raw})
}

// socketMode abre la conexión de Socket Mode (`apps.connections.open`) y entrega los
// eventos de mensaje; cada sobre se confirma con su `envelope_id`.
func (s *Slack) socketMode(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	var r struct {
		URL string `json:"url"`
	}
	if err := s.llamar(ctx, s.TokenApp, "apps.connections.open", url.Values{}, &r); err != nil {
		return err
	}
	c, resp, err := websocket.Dial(ctx, r.URL, &websocket.DialOptions{HTTPClient: s.HTTP})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close() // la biblioteca ya lo dejó leído (o nil): cerrarlo no cuesta nada
	}
	if err != nil {
		return err
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(4 << 20)
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			return err
		}
		var sobre struct {
			Tipo     string `json:"type"`
			Envelope string `json:"envelope_id"`
			Payload  struct {
				Event eventoSocket `json:"event"`
			} `json:"payload"`
		}
		if json.Unmarshal(raw, &sobre) != nil {
			continue
		}
		if sobre.Envelope != "" {
			ack, _ := json.Marshal(map[string]string{"envelope_id": sobre.Envelope})
			if err := c.Write(ctx, websocket.MessageText, ack); err != nil {
				return err
			}
		}
		switch sobre.Tipo {
		case "disconnect":
			return errors.New("chat: Slack pidió reconectar")
		case "events_api":
		default:
			continue
		}
		s.despachar(ctx, sobre.Payload.Event, alLlegar)
	}
}

// eventoSocket es el `event` de un sobre de Socket Mode.
type eventoSocket struct {
	mensajeSlack
	Channel string        `json:"channel"`
	Message *mensajeSlack `json:"message"`
	Deleted string        `json:"deleted_ts"`
	Item    struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	} `json:"item"`
}

// despachar entrega un evento de Socket Mode si es de un mensaje: nuevo, editado o
// borrado.
func (s *Slack) despachar(ctx context.Context, ev eventoSocket, alLlegar func(api.RoomsEvent)) {
	switch {
	case ev.Type == "message" && ev.Subtype == "message_changed" && ev.Message != nil:
		s.entregar(ctx, ev.Channel, *ev.Message, "message_edited", alLlegar)
	case ev.Type == "message" && ev.Subtype == "message_deleted":
		raw, _ := json.Marshal(api.RoomsMessage{ID: ev.Deleted, ChannelID: ev.Channel})
		alLlegar(api.RoomsEvent{Type: "message_deleted", ChannelID: ev.Channel, Payload: raw})
	case ev.Type == "message" && (ev.Subtype == "" || ev.Subtype == "thread_broadcast" || ev.Subtype == "bot_message" || ev.Subtype == "file_share"):
		s.entregar(ctx, ev.Channel, ev.mensajeSlack, "message", alLlegar)
	}
}

// ManifiestoSlack es el manifiesto de la app interna que la persona instala en su
// espacio (api.slack.com/apps → «Create New App» → «From a manifest»). Solo pide lo que
// el cliente usa; Socket Mode viene activado para el aviso en vivo sin abrir puertos.
const ManifiestoSlack = `display_information:
  name: Sytue Office
features:
  bot_user:
    display_name: Sytue Office
oauth_config:
  scopes:
    user:
      - channels:history
      - channels:read
      - channels:write
      - groups:history
      - groups:read
      - groups:write
      - im:history
      - im:read
      - im:write
      - mpim:history
      - mpim:read
      - mpim:write
      - chat:write
      - files:read
      - files:write
      - reactions:read
      - reactions:write
      - users:read
      - users:read.email
      - search:read
settings:
  event_subscriptions:
    user_events:
      - message.channels
      - message.groups
      - message.im
      - message.mpim
  socket_mode_enabled: true
`
