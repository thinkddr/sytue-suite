// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// ProveedorTeams es una cuenta de Microsoft Teams (de trabajo o de centro educativo).
const ProveedorTeams = "teams"

// APIGraph es Microsoft Graph. Se cambia en las pruebas (y con `-ldflags -X` en la
// prueba visual contra un Graph de mentira).
var APIGraph = "https://graph.microsoft.com/v1.0"

// Intervalos del sondeo. Ni Teams ni Google Chat avisan a una app que no tiene servidor
// (sus notificaciones de cambio piden una dirección pública que las reciba), así que se
// pregunta: cada SondeoBase mientras llega algo y, cuando no, cada vez más despacio hasta
// SondeoTope. Lo que se cae lo reintenta el Agregador con su propia espera creciente.
const (
	SondeoBase = 15 * time.Second
	SondeoTope = 2 * time.Minute
	// cacheChats y cacheEquipos es lo que vale la lista de chats y la de equipos y
	// canales: la pantalla vuelve a pedir los pendientes con cada mensaje que llega.
	cacheChats   = 30 * time.Second
	cacheEquipos = 5 * time.Minute
)

// ErrSinHilosTeams: los chats de Teams no tienen hilos (solo los canales de un equipo).
var ErrSinHilosTeams = errors.New("chat: en los chats de Teams no hay hilos; escribe en el chat")

// Avisos son los textos que una cuenta pone en la pantalla cuando el proveedor no deja
// hacer algo, ya en el idioma de la app (los da `cmd-local/rooms`).
type Avisos struct {
	// SinConsentimiento: leer los canales de Teams pide el consentimiento del
	// administrador de la organización.
	SinConsentimiento string
	// SinLectura: Google Chat sin un cliente OAuth propio del Workspace (D-22).
	SinLectura string
	// PersonaGoogle es el nombre de quien escribe en Google Chat cuando Google no lo da
	// ni se puede saber por la People API («Persona de Google»).
	PersonaGoogle string
}

// AutorSistema es quien firma los avisos de Sytue Office en una conversación.
const AutorSistema = "sytue-office"

// avisoSistema es un mensaje de Sytue Office (no del proveedor) que explica por qué no
// se ve lo que hay en una conversación: va al final de lo que se ve, que es donde se
// mira, con la hora en que se supo (en).
func avisoSistema(canal, texto string, en time.Time) api.RoomsMessage {
	return api.RoomsMessage{ID: "aviso-" + canal, ChannelID: canal, AuthorSub: AutorSistema, AuthorName: "Sytue Office", Body: texto, IsBot: true, CreatedAt: en}
}

// avisos recuerda cuándo se avisó en cada conversación, para que el aviso no cambie de
// hora con cada recarga.
type avisos struct {
	mu sync.Mutex
	en map[string]time.Time
}

func (a *avisos) de(canal, texto string, ahora time.Time) api.RoomsMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.en == nil {
		a.en = map[string]time.Time{}
	}
	en, ok := a.en[canal]
	if !ok {
		en = ahora
		a.en[canal] = en
	}
	return avisoSistema(canal, texto, en)
}

// Teams es una cuenta de Microsoft Teams por Microsoft Graph (RO-05): los chats (1:1, de
// grupo y de reuniones) y, si se pidió al entrar y el administrador lo consiente, los
// canales de sus equipos.
//
// Los identificadores son los del servicio de integraciones: un chat es `chat/<id>` y un
// canal `<equipo>/<canal>`. Los chats no tienen hilos; editar y borrar solo en los chats
// (en los canales piden un permiso de administrador).
type Teams struct {
	G *cchat.Graph
	// Nombre es el rótulo de la cuenta («Teams · ana@empresa.com»).
	Nombre string
	// ConCanales lista también los canales de los equipos.
	ConCanales bool
	// Ficheros: la cuenta entró con el permiso de los adjuntos (subirlos al OneDrive de
	// la persona y abrir los que le comparten).
	Ficheros bool
	Avisos   Avisos
	// Sondeo y SondeoMax son la espera del sondeo (cero: SondeoBase y SondeoTope).
	Sondeo, SondeoMax time.Duration
	// Ahora es el reloj; las pruebas lo fijan.
	Ahora func() time.Time

	mu       sync.Mutex
	yo       cchat.UsuarioGraph
	chats    map[string]cchat.ChatGraph
	equipos  []equipoTeams
	listado  time.Time // cuándo se pidieron los chats
	listadoE time.Time // y los equipos
	previa   map[string]string
	ultimo   map[string]time.Time
	sinLeer  map[string]int
	abiertos map[string]bool
	deltas   map[string]string
	paginas  map[string]string
	// sinConsentimiento: Graph dijo 403 al leer un canal.
	sinConsentimiento bool
	avisados          avisos
	// cargada: la lista de chats se ha traído alguna vez (antes, ningún chat es nuevo);
	// anunciados son los nuevos de los que ya se avisó.
	cargada    bool
	anunciados map[string]bool
	adjuntos   map[string]adjuntoTeams
	cargados   cargados
}

type equipoTeams struct {
	equipo  cchat.EquipoGraph
	canales []cchat.CanalGraph
}

// NuevaTeams prepara la cuenta; no habla con la red hasta que se le pide algo.
func NuevaTeams(g *cchat.Graph, nombre string, conCanales bool, av Avisos) *Teams {
	return &Teams{
		G: g, Nombre: nombre, ConCanales: conCanales, Avisos: av,
		chats: map[string]cchat.ChatGraph{}, previa: map[string]string{}, ultimo: map[string]time.Time{}, sinLeer: map[string]int{},
		abiertos: map[string]bool{}, deltas: map[string]string{}, paginas: map[string]string{},
		anunciados: map[string]bool{}, adjuntos: map[string]adjuntoTeams{},
	}
}

var (
	_ Proveedor    = (*Teams)(nil)
	_ Editor       = (*Teams)(nil)
	_ Reaccionador = (*Teams)(nil)
	_ Hilos        = (*Teams)(nil)
	_ Miembros     = (*Teams)(nil)
	_ Reuniones    = (*Teams)(nil)
	_ Presentes    = (*Teams)(nil)
	_ Directos     = (*Teams)(nil)
	_ Personas     = (*Teams)(nil)
	_ Adjuntos     = (*Teams)(nil)
	_ Buscador     = (*Teams)(nil)
)

func (t *Teams) ahora() time.Time {
	if t.Ahora != nil {
		return t.Ahora()
	}
	return time.Now()
}

// QuienSoy comprueba la sesión (`/me`) y la recuerda.
func (t *Teams) QuienSoy(ctx context.Context) (cchat.UsuarioGraph, error) {
	u, err := t.G.Yo(ctx)
	if err != nil {
		return u, err
	}
	t.mu.Lock()
	t.yo = u
	t.mu.Unlock()
	return u, nil
}

func (t *Teams) asegurar(ctx context.Context) (string, error) {
	t.mu.Lock()
	yo := t.yo.ID
	t.mu.Unlock()
	if yo != "" {
		return yo, nil
	}
	u, err := t.QuienSoy(ctx)
	return u.ID, err
}

// Rotulo es el de la cuenta.
func (t *Teams) Rotulo() string { return cmp.Or(t.Nombre, "Microsoft Teams") }

// ----- Canales -----

// cargarChats pide los chats si la lista que hay es vieja; la nueva sustituye a la de
// antes (un chat que ya no está, sale).
func (t *Teams) cargarChats(ctx context.Context) error {
	t.mu.Lock()
	fresca := !t.listado.IsZero() && t.ahora().Sub(t.listado) < cacheChats
	t.mu.Unlock()
	if fresca {
		return nil
	}
	cs, err := t.G.Chats(ctx)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.listado, t.cargada = t.ahora(), true
	t.chats = make(map[string]cchat.ChatGraph, len(cs))
	for _, c := range cs {
		if !cchat.IDChatTeamsValido(c.ID) {
			continue
		}
		t.chats[c.ID] = c
		if c.Ultimo != nil {
			if _, ok := t.previa[c.ID]; !ok {
				t.previa[c.ID] = c.Ultimo.ID
			}
		}
	}
	return nil
}

// cargarEquipos pide los equipos y sus canales si la lista es vieja. Un 403 (no se
// concedió leer los equipos) deja la cuenta con sus chats.
func (t *Teams) cargarEquipos(ctx context.Context) error {
	t.mu.Lock()
	fresca := !t.listadoE.IsZero() && t.ahora().Sub(t.listadoE) < cacheEquipos
	t.mu.Unlock()
	if !t.ConCanales || fresca {
		return nil
	}
	es, err := t.G.Equipos(ctx)
	if cchat.Estado(err) == http.StatusForbidden {
		t.mu.Lock()
		t.listadoE = t.ahora()
		t.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	var out []equipoTeams
	for _, e := range es {
		cs, err := t.G.Canales(ctx, e.ID)
		if err != nil && cchat.Estado(err) != http.StatusForbidden {
			return err
		}
		out = append(out, equipoTeams{equipo: e, canales: cs})
	}
	t.mu.Lock()
	t.equipos, t.listadoE = out, t.ahora()
	t.mu.Unlock()
	return nil
}

// Canales son los chats visibles y, con ConCanales, los canales de los equipos.
func (t *Teams) Canales(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	if _, err := t.asegurar(ctx); err != nil {
		return nil, nil, err
	}
	if err := t.cargarChats(ctx); err != nil {
		return nil, nil, err
	}
	if err := t.cargarEquipos(ctx); err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var cs []api.RoomsChannel
	var us []api.RoomsUnread
	ids := make([]string, 0, len(t.chats))
	for id := range t.chats {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		c := t.chats[id]
		if c.Vista != nil && c.Vista.Oculto {
			continue
		}
		cs = append(cs, t.aCanalChat(c))
		n := t.sinLeer[cchat.RemotoChat(id)]
		if n == 0 && t.noLeido(c) {
			n = 1
		}
		if n > 0 {
			us = append(us, api.RoomsUnread{ChannelID: cchat.RemotoChat(id), Unread: n})
		}
	}
	for _, e := range t.equipos {
		for _, c := range e.canales {
			ch := api.RoomsChannel{
				ID: e.equipo.ID + "/" + c.ID, Slug: c.ID, Name: e.equipo.Nombre + " / " + c.Nombre, Purpose: c.Descripcion,
				Kind: "public", CreatedAt: c.Creado, NotifyLevel: "all",
			}
			if c.Tipo == "private" || c.Tipo == "shared" {
				ch.Kind = "private"
			}
			cs = append(cs, ch)
			if n := t.sinLeer[ch.ID]; n > 0 {
				us = append(us, api.RoomsUnread{ChannelID: ch.ID, Unread: n})
			}
		}
	}
	return cs, us, nil
}

// noLeido: el último mensaje no es propio y es posterior a la última lectura.
func (t *Teams) noLeido(c cchat.ChatGraph) bool {
	u := c.Ultimo
	if u == nil || u.Borrado() || u.From.User.ID == t.yo.ID || u.Type != "" && u.Type != "message" {
		return false
	}
	return c.Vista == nil || c.Vista.Leido == nil || u.Created.After(*c.Vista.Leido)
}

// aCanalChat es un chat como canal: un 1:1 o un grupo sin tema es un directo con los
// demás; uno con tema (o el de una reunión), una conversación privada con ese nombre.
func (t *Teams) aCanalChat(c cchat.ChatGraph) api.RoomsChannel {
	ch := api.RoomsChannel{ID: cchat.RemotoChat(c.ID), Slug: c.ID, CreatedAt: c.Creado, NotifyLevel: "all", Kind: "direct"}
	var nombres, subs []string
	for _, m := range c.Miembros {
		if m.Usuario == t.yo.ID {
			continue
		}
		nombres = append(nombres, cmp.Or(m.Nombre, m.Correo, m.Usuario))
		subs = append(subs, m.Usuario)
	}
	if c.Tema != "" {
		ch.Kind, ch.Name = "private", c.Tema
		return ch
	}
	ch.Name, ch.Peers, ch.PeerSubs = strings.Join(nombres, ", "), nombres, subs
	if ch.Name == "" {
		ch.Name = t.Rotulo()
	}
	return ch
}

// ----- Mensajes -----

var emojisTeams = map[string]string{"like": "👍", "heart": "❤️", "laugh": "😆", "surprised": "😮", "sad": "😢", "angry": "😠"}

// emojiDeTeams pasa una reacción de Teams («like» o un emoji) al emoji que se enseña.
func emojiDeTeams(tipo string) string { return cmp.Or(emojisTeams[tipo], tipo) }

// tipoDeTeams es lo contrario: los seis clásicos vuelven a su nombre, para que quitar
// una reacción puesta desde Teams quite esa.
func tipoDeTeams(emoji string) string {
	emoji = strings.Trim(emoji, ":")
	for k, v := range emojisTeams {
		if v == emoji || k == emoji {
			return k
		}
	}
	return emoji
}

func (t *Teams) sub(id string) string {
	if id != "" && id == t.yo.ID {
		return Yo
	}
	return id
}

// aMensaje pasa un mensaje de Graph al modelo de la pantalla (sin bloquear t.mu).
func (t *Teams) aMensaje(remoto string, m cchat.MensajeGraph) api.RoomsMessage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.aMensajeBloqueado(remoto, m)
}

func (t *Teams) aMensajeBloqueado(remoto string, m cchat.MensajeGraph) api.RoomsMessage {
	autor, nombre, bot := m.From.User.ID, m.From.User.Name, false
	if autor == "" && m.From.Application != nil {
		autor, nombre, bot = m.From.Application.ID, m.From.Application.Name, true
	}
	out := api.RoomsMessage{ID: m.ID, ChannelID: remoto, AuthorSub: t.sub(autor), AuthorName: nombre, Body: m.Texto(), CreatedAt: m.Created, IsBot: bot}
	if m.Edited != nil {
		e := *m.Edited
		out.EditedAt = &e
	}
	if m.ReplyTo != "" && m.ReplyTo != m.ID {
		p := m.ReplyTo
		out.ParentID = &p
	}
	vistos := map[string]bool{}
	for i := len(m.Replies) - 1; i >= 0; i-- {
		r := m.Replies[i]
		if r.Borrado() {
			continue
		}
		out.ReplyCount++
		if out.LastReplyAt == nil || r.Created.After(*out.LastReplyAt) {
			c := r.Created
			out.LastReplyAt = &c
		}
		if s := t.sub(r.From.User.ID); s != "" && !vistos[s] && len(out.ThreadParticipants) < 3 {
			vistos[s] = true
			out.ThreadParticipants = append(out.ThreadParticipants, api.RoomsThreadParticipant{Sub: s, Name: r.From.User.Name})
		}
	}
	for _, r := range m.Reactions {
		e := emojiDeTeams(r.Tipo)
		i := slices.IndexFunc(out.Reactions, func(x api.RoomsReaction) bool { return x.Emoji == e })
		if i < 0 {
			out.Reactions = append(out.Reactions, api.RoomsReaction{Emoji: e})
			i = len(out.Reactions) - 1
		}
		out.Reactions[i].Count++
		out.Reactions[i].Subs = append(out.Reactions[i].Subs, t.sub(r.User.User.ID))
	}
	t.adjuntosDe(&out, remoto, m)
	if !m.Borrado() {
		t.cargados.apuntar(out)
	}
	return out
}

// visible dice si un mensaje de Graph se enseña: no los borrados ni los de sistema.
func visible(m cchat.MensajeGraph) bool {
	return !m.Borrado() && (m.Type == "" || m.Type == "message")
}

// Mensajes es una página de raíces anteriores a antes, en orden de lectura.
func (t *Teams) Mensajes(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	if _, err := t.asegurar(ctx); err != nil {
		return nil, err
	}
	var ms []cchat.MensajeGraph
	var err error
	if chat, ok := cchat.EsChat(canal); ok {
		ms, err = t.G.MensajesChat(ctx, chat, antes, cmp.Or(limite, 50))
	} else {
		t.mu.Lock()
		siguiente := t.paginas[canal]
		sinPermiso := t.sinConsentimiento
		t.mu.Unlock()
		switch {
		case sinPermiso:
			return []api.RoomsMessage{t.avisados.de(canal, t.Avisos.SinConsentimiento, t.ahora())}, nil
		case !antes.IsZero() && siguiente == "":
			return nil, nil // no hay más atrás
		case antes.IsZero():
			siguiente = ""
		}
		var sig string
		ms, sig, err = t.G.MensajesCanal(ctx, canal, siguiente)
		if cchat.Estado(err) == http.StatusForbidden {
			// El administrador no ha consentido leer los canales: se dice y los chats
			// siguen.
			t.mu.Lock()
			t.sinConsentimiento = true
			t.mu.Unlock()
			return []api.RoomsMessage{t.avisados.de(canal, t.Avisos.SinConsentimiento, t.ahora())}, nil
		}
		t.mu.Lock()
		t.paginas[canal] = sig
		t.mu.Unlock()
	}
	if err != nil {
		return nil, err
	}
	out := make([]api.RoomsMessage, 0, len(ms))
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := len(ms) - 1; i >= 0; i-- { // Graph da el más nuevo primero
		if visible(ms[i]) {
			out = append(out, t.aMensajeBloqueado(canal, ms[i]))
		}
	}
	t.abiertos[canal] = true
	if antes.IsZero() {
		ultimo := t.ahora()
		for _, m := range ms {
			if c := m.Cambiado(); c.After(ultimo) {
				ultimo = c
			}
		}
		if ultimo.After(t.ultimo[canal]) {
			t.ultimo[canal] = ultimo
		}
	}
	return out, nil
}

// Enviar publica texto; con raiz, como respuesta en el hilo de un canal.
func (t *Teams) Enviar(ctx context.Context, canal, raiz, texto string) (api.RoomsMessage, error) {
	if _, err := t.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	if _, ok := cchat.EsChat(canal); ok && raiz != "" {
		return api.RoomsMessage{}, ErrSinHilosTeams
	}
	m, err := t.G.Enviar(ctx, canal, raiz, texto)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	if m.From.User.ID == "" {
		t.mu.Lock()
		m.From.User.ID, m.From.User.Name = t.yo.ID, t.yo.Nombre
		t.mu.Unlock()
	}
	if raiz != "" && m.ReplyTo == "" {
		m.ReplyTo = raiz
	}
	if m.Body.Content == "" {
		m.Body.Type, m.Body.Content = "text", texto
	}
	return t.aMensaje(canal, m), nil
}

// MarcarLeido da el chat por leído (en Teams también); en un canal, solo aquí.
func (t *Teams) MarcarLeido(ctx context.Context, canal string) error {
	t.mu.Lock()
	delete(t.sinLeer, canal)
	chat, esChat := cchat.EsChat(canal)
	c, conocido := t.chats[chat]
	yo := t.yo.ID
	if conocido && c.Ultimo != nil {
		leido := c.Ultimo.Created
		v := cchat.VistaChat{Leido: &leido}
		if c.Vista != nil {
			v.Oculto = c.Vista.Oculto
		}
		c.Vista = &v
		t.chats[chat] = c
	}
	t.mu.Unlock()
	if !esChat || !conocido || c.Inquilino == "" || yo == "" {
		return nil
	}
	return t.G.MarcarLeido(ctx, chat, yo, c.Inquilino)
}

// Editar cambia el texto de un mensaje propio de un chat.
func (t *Teams) Editar(ctx context.Context, canal, mensaje, texto string) (api.RoomsMessage, error) {
	if _, ok := cchat.EsChat(canal); !ok {
		return api.RoomsMessage{}, ErrNoDisponible
	}
	if err := t.G.Editar(ctx, canal, mensaje, texto); err != nil {
		return api.RoomsMessage{}, err
	}
	m, err := t.G.Mensaje(ctx, canal, mensaje)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	return t.aMensaje(canal, m), nil
}

// Borrar borra un mensaje propio de un chat.
func (t *Teams) Borrar(ctx context.Context, canal, mensaje string) error {
	yo, err := t.asegurar(ctx)
	if err != nil {
		return err
	}
	if _, ok := cchat.EsChat(canal); !ok {
		return ErrNoDisponible
	}
	return t.G.Borrar(ctx, yo, canal, mensaje)
}

// Reaccionar pone o quita una reacción.
func (t *Teams) Reaccionar(ctx context.Context, canal, mensaje, emoji string, poner bool) error {
	return t.G.Reaccionar(ctx, canal, mensaje, tipoDeTeams(emoji), poner)
}

// Hilo es la raíz de un mensaje de canal con sus respuestas (los chats no tienen).
func (t *Teams) Hilo(ctx context.Context, canal, raiz string) (api.RoomsThread, error) {
	if _, ok := cchat.EsChat(canal); ok {
		return api.RoomsThread{}, ErrSinHilosTeams
	}
	if _, err := t.asegurar(ctx); err != nil {
		return api.RoomsThread{}, err
	}
	r, err := t.G.Mensaje(ctx, canal, raiz)
	if err != nil {
		return api.RoomsThread{}, err
	}
	rs, err := t.G.Respuestas(ctx, canal, raiz)
	if err != nil {
		return api.RoomsThread{}, err
	}
	slices.SortFunc(rs, func(a, b cchat.MensajeGraph) int { return a.Created.Compare(b.Created) })
	r.Replies = rs
	h := api.RoomsThread{Root: t.aMensaje(canal, r)}
	for _, m := range rs {
		if visible(m) {
			if m.ReplyTo == "" {
				m.ReplyTo = raiz
			}
			h.Replies = append(h.Replies, t.aMensaje(canal, m))
		}
	}
	return h, nil
}

// Miembros son los del chat o el canal. Los de un canal piden un permiso que no siempre
// se tiene: sin él, solo quien mira.
func (t *Teams) Miembros(ctx context.Context, canal string) ([]api.RoomsMember, error) {
	if _, err := t.asegurar(ctx); err != nil {
		return nil, err
	}
	var ms []cchat.MiembroGraph
	t.mu.Lock()
	if chat, ok := cchat.EsChat(canal); ok {
		ms = slices.Clone(t.chats[chat].Miembros)
	}
	t.mu.Unlock()
	if len(ms) == 0 {
		var err error
		ms, err = t.G.MiembrosDe(ctx, canal)
		if cchat.Estado(err) == http.StatusForbidden {
			return []api.RoomsMember{{Subject: Yo, Role: "member"}}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	out := make([]api.RoomsMember, 0, len(ms))
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range ms {
		rol := "member"
		if slices.Contains(m.Roles, "owner") {
			rol = "admin"
		}
		out = append(out, api.RoomsMember{Subject: t.sub(m.Usuario), Name: cmp.Or(m.Nombre, m.Correo), Role: rol})
	}
	return out, nil
}

// ----- Reuniones y presencia -----

// ProveedorReunion es el nombre del servicio de reuniones.
func (t *Teams) ProveedorReunion() string { return "Microsoft Teams" }

// CrearReunion crea una reunión de Teams de una hora desde ahora y da su enlace.
func (t *Teams) CrearReunion(ctx context.Context, asunto string) (string, error) {
	inicio := t.ahora()
	return t.G.CrearReunion(ctx, cmp.Or(asunto, t.Rotulo()), inicio, inicio.Add(time.Hour))
}

// Presencias es la presencia de esas personas de la organización.
func (t *Teams) Presencias(ctx context.Context, personas []string) ([]api.Presencia, error) {
	ps, err := t.G.Presencias(ctx, personas)
	out := make([]api.Presencia, 0, len(ps))
	for _, p := range ps {
		out = append(out, api.Presencia{Subject: p.ID, Estado: presenciaDeTeams(p.Disponibilidad)})
	}
	return out, err
}

// presenciaDeTeams pasa la disponibilidad de Teams a los cuatro estados de la suite.
func presenciaDeTeams(d string) api.EstadoPresencia {
	switch d {
	case "Available", "AvailableIdle":
		return api.EnLinea
	case "Away", "BeRightBack":
		return api.Ausente
	case "Busy", "BusyIdle", "DoNotDisturb":
		return api.NoMolestar
	}
	return api.Desconectado
}

// ----- Lo que llega -----

// Escuchar sondea hasta que el contexto muera o algo falle: los chats con mensajes nuevos
// (por la vista previa del último de cada uno), los cambios de los chats abiertos (por
// fecha: Graph no da `delta` de los chats) y los de los canales abiertos (con `delta`).
// Si no llega nada, cada vez espera más, hasta SondeoMax; lo que llega la devuelve a
// Sondeo. Un 429 espera lo que pida Graph.
func (t *Teams) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	if _, err := t.asegurar(ctx); err != nil {
		return err
	}
	base, tope := cmp.Or(t.Sondeo, SondeoBase), cmp.Or(t.SondeoMax, SondeoTope)
	espera := base
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(espera):
		}
		n, err := t.sondear(ctx, alLlegar)
		switch {
		case cchat.Estado(err) == http.StatusTooManyRequests:
			espera = max(cchat.Espera(err), base)
			continue
		case err != nil:
			return err
		case n > 0:
			espera = base
		default:
			espera = min(2*espera, tope)
		}
	}
}

// sondear es una vuelta del sondeo; devuelve cuántas cosas entregó.
func (t *Teams) sondear(ctx context.Context, alLlegar func(api.RoomsEvent)) (int, error) {
	n := 0
	rec, err := t.G.ChatsRecientes(ctx)
	if err != nil {
		return 0, err
	}
	nuevos := false
	for _, c := range rec {
		if c.Ultimo == nil || !cchat.IDChatTeamsValido(c.ID) {
			continue
		}
		remoto := cchat.RemotoChat(c.ID)
		t.mu.Lock()
		antes, visto := t.previa[c.ID]
		abierto := t.abiertos[remoto]
		t.previa[c.ID] = c.Ultimo.ID
		if prev, ok := t.chats[c.ID]; ok {
			prev.Ultimo = c.Ultimo
			t.chats[c.ID] = prev
		} else {
			// Un chat nuevo (lo abrió otra persona, o esta en otro sitio): la lista se
			// vuelve a pedir la próxima vez y se avisa, una vez, de que hay uno más.
			t.listado = time.Time{}
			if t.cargada && !t.anunciados[c.ID] {
				t.anunciados[c.ID] = true
				nuevos = true
			}
		}
		t.mu.Unlock()
		if abierto || (visto && antes == c.Ultimo.ID) || !visible(*c.Ultimo) {
			continue
		}
		// Un chat que no está abierto: basta con su último mensaje para el aviso y el
		// contador.
		n += t.entregar(remoto, *c.Ultimo, "message", alLlegar)
	}
	if nuevos {
		alLlegar(api.RoomsEvent{Type: EventoCanales})
		n++
	}
	t.mu.Lock()
	var abiertos []string
	for r := range t.abiertos {
		abiertos = append(abiertos, r)
	}
	sinPermiso := t.sinConsentimiento
	t.mu.Unlock()
	slices.Sort(abiertos)
	for _, remoto := range abiertos {
		k, err := t.cambiosDe(ctx, remoto, sinPermiso, alLlegar)
		if err != nil {
			return n, err
		}
		n += k
	}
	return n, nil
}

// cambiosDe entrega lo nuevo, lo editado y lo borrado de una conversación abierta.
func (t *Teams) cambiosDe(ctx context.Context, remoto string, sinPermiso bool, alLlegar func(api.RoomsEvent)) (int, error) {
	t.mu.Lock()
	desde := t.ultimo[remoto]
	enlace := t.deltas[remoto]
	t.mu.Unlock()
	if desde.IsZero() {
		desde = t.ahora()
	}
	var ms []cchat.MensajeGraph
	var err error
	if chat, ok := cchat.EsChat(remoto); ok {
		ms, err = t.G.CambiosChat(ctx, chat, desde)
	} else {
		if sinPermiso {
			return 0, nil
		}
		var delta string
		ms, delta, err = t.G.DeltaCanal(ctx, remoto, enlace, desde)
		if cchat.Estado(err) == http.StatusForbidden {
			t.mu.Lock()
			t.sinConsentimiento = true
			t.mu.Unlock()
			return 0, nil
		}
		if err == nil {
			t.mu.Lock()
			t.deltas[remoto] = delta
			t.mu.Unlock()
		}
	}
	if err != nil {
		return 0, err
	}
	slices.SortFunc(ms, func(a, b cchat.MensajeGraph) int { return a.Cambiado().Compare(b.Cambiado()) })
	n := 0
	for _, m := range ms {
		tipo := "message"
		switch {
		case m.Borrado():
			tipo = "message_deleted"
		case !m.Created.After(desde):
			tipo = "message_edited"
		case m.Type != "" && m.Type != "message":
			continue
		}
		n += t.entregar(remoto, m, tipo, alLlegar)
		t.mu.Lock()
		if c := m.Cambiado(); c.After(t.ultimo[remoto]) {
			t.ultimo[remoto] = c
		}
		t.mu.Unlock()
	}
	return n, nil
}

func (t *Teams) entregar(remoto string, m cchat.MensajeGraph, tipo string, alLlegar func(api.RoomsEvent)) int {
	msg := t.aMensaje(remoto, m)
	if tipo == "message_deleted" {
		msg = api.RoomsMessage{ID: m.ID, ChannelID: remoto}
		t.cargados.quitar(remoto, m.ID)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return 0
	}
	t.mu.Lock()
	if tipo == "message" && msg.AuthorSub != Yo && !t.abiertos[remoto] {
		t.sinLeer[remoto]++
	}
	t.mu.Unlock()
	alLlegar(api.RoomsEvent{Type: tipo, ChannelID: remoto, Subject: msg.AuthorSub, Name: msg.AuthorName, Payload: raw})
	return 1
}
