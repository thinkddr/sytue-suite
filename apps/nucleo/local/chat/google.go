// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// ProveedorGoogle es una cuenta de Google Workspace con Google Chat y Google Meet.
const ProveedorGoogle = "google-chat"

// Las API de Google que usa la cuenta. Se cambian en las pruebas (y con `-ldflags -X` en
// la prueba visual contra un Google de mentira).
var (
	APIChatGoogle    = "https://chat.googleapis.com/v1"
	APIMeet          = "https://meet.googleapis.com/v2"
	APIUsuarioGoogle = "https://openidconnect.googleapis.com/v1/userinfo"
	// APIPersonasGoogle es la People API: el nombre de quien escribe, que Chat no da
	// con la autenticación de la persona.
	APIPersonasGoogle = "https://people.googleapis.com/v1"
)

// GoogleChat es una cuenta de Google Chat (RO-06) que además crea enlaces de Meet.
//
// D-22: enviar vale con el cliente OAuth de la compilación, pero LEER los mensajes es un
// permiso restringido de Google que una app de terceros solo tiene tras una auditoría
// externa; una app «interna» de un Workspace está exenta. Por eso Lectura solo va con el
// cliente OAuth propio que pone la persona. Sin ella, cada espacio dice cómo leerlo y
// enseña lo que se mandó desde aquí en esta sesión.
//
// Un espacio es `spaces/<id>` y un mensaje, la última parte de su nombre (el espacio ya
// va en el canal).
type GoogleChat struct {
	C *cchat.GoogleChat
	M *cchat.Meet
	// P es la People API, para los nombres de quien escribe (nil: no se preguntan).
	P *cchat.PersonasGoogle
	// Usuario dice quién es la persona (el `userinfo` de OpenID Connect).
	Usuario func(ctx context.Context) (cchat.UsuarioGoogle, error)
	Nombre  string
	Lectura bool
	// Directos: la cuenta entró con el permiso de abrir mensajes directos nuevos
	// (`chat.spaces.create`); sin él, se abren los que ya hay.
	Directos bool
	Avisos   Avisos
	// Sondeo y SondeoMax son la espera del sondeo (cero: SondeoBase y SondeoTope).
	Sondeo, SondeoMax time.Duration
	Ahora             func() time.Time

	mu       sync.Mutex
	yo       cchat.UsuarioGoogle
	espacios map[string]cchat.EspacioChat
	activos  map[string]time.Time // espacio → última actividad vista
	ultimo   map[string]time.Time // espacio abierto → último mensaje visto
	abiertos map[string]bool
	sinLeer  map[string]int
	hilos    map[string]string // mensaje → su hilo
	raices   map[string]string // hilo → mensaje raíz
	enviados map[string][]api.RoomsMessage
	avisados avisos
	// nombres son los de las personas (`users/<id>` → nombre o correo) que se han
	// sabido; intentados, a quién ya se preguntó sin respuesta.
	nombres    map[string]string
	intentados map[string]bool
	// sinPersonas: la People API dijo que no (403) y no se le pregunta más.
	sinPersonas bool
	// directos son los nombres de los mensajes directos que Google no nombra (el de
	// la otra persona).
	directos   map[string]string
	adjuntos   map[string]cchat.AdjuntoGoogle
	cargados   cargados
	cargada    bool
	anunciados map[string]bool
}

// NuevaGoogleChat prepara la cuenta; no habla con la red hasta que se le pide algo.
func NuevaGoogleChat(c *cchat.GoogleChat, m *cchat.Meet, usuario func(context.Context) (cchat.UsuarioGoogle, error), nombre string, lectura bool, av Avisos) *GoogleChat {
	return &GoogleChat{
		C: c, M: m, Usuario: usuario, Nombre: nombre, Lectura: lectura, Avisos: av,
		espacios: map[string]cchat.EspacioChat{}, activos: map[string]time.Time{}, ultimo: map[string]time.Time{}, abiertos: map[string]bool{},
		sinLeer: map[string]int{}, hilos: map[string]string{}, raices: map[string]string{}, enviados: map[string][]api.RoomsMessage{},
		nombres: map[string]string{}, intentados: map[string]bool{}, directos: map[string]string{}, adjuntos: map[string]cchat.AdjuntoGoogle{},
		anunciados: map[string]bool{},
	}
}

var (
	_ Proveedor = (*GoogleChat)(nil)
	_ Hilos     = (*GoogleChat)(nil)
	_ Miembros  = (*GoogleChat)(nil)
	_ Reuniones = (*GoogleChat)(nil)
	_ Directos  = (*GoogleChat)(nil)
	_ Personas  = (*GoogleChat)(nil)
	_ Adjuntos  = (*GoogleChat)(nil)
	_ Buscador  = (*GoogleChat)(nil)
)

func (g *GoogleChat) ahora() time.Time {
	if g.Ahora != nil {
		return g.Ahora()
	}
	return time.Now()
}

// QuienSoy comprueba la sesión y la recuerda.
func (g *GoogleChat) QuienSoy(ctx context.Context) (cchat.UsuarioGoogle, error) {
	u, err := g.Usuario(ctx)
	if err != nil {
		return u, err
	}
	g.mu.Lock()
	g.yo = u
	g.mu.Unlock()
	return u, nil
}

func (g *GoogleChat) asegurar(ctx context.Context) error {
	g.mu.Lock()
	listo := g.yo.Sujeto != ""
	g.mu.Unlock()
	if listo {
		return nil
	}
	_, err := g.QuienSoy(ctx)
	return err
}

// Rotulo es el de la cuenta.
func (g *GoogleChat) Rotulo() string { return cmp.Or(g.Nombre, "Google Chat") }

// Canales son los espacios, los mensajes directos y los chats de grupo.
func (g *GoogleChat) Canales(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	if err := g.asegurar(ctx); err != nil {
		return nil, nil, err
	}
	es, err := g.C.Espacios(ctx)
	if err != nil {
		return nil, nil, err
	}
	g.nombrarDirectos(ctx, es)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cargada = true
	var cs []api.RoomsChannel
	var us []api.RoomsUnread
	for _, e := range es {
		g.espacios[e.Name] = e
		if _, ok := g.activos[e.Name]; !ok && e.LastActiveTime != nil {
			g.activos[e.Name] = *e.LastActiveTime
		}
		cs = append(cs, g.aCanal(e))
		if n := g.sinLeer[e.Name]; n > 0 {
			us = append(us, api.RoomsUnread{ChannelID: e.Name, Unread: n})
		}
	}
	return cs, us, nil
}

func (g *GoogleChat) aCanal(e cchat.EspacioChat) api.RoomsChannel {
	ch := api.RoomsChannel{
		ID: e.Name, Slug: strings.TrimPrefix(e.Name, "spaces/"), Name: cmp.Or(e.DisplayName, g.Rotulo()), Purpose: e.Details.Description,
		Kind: "public", NotifyLevel: "all",
	}
	if e.CreateTime != nil {
		ch.CreatedAt = *e.CreateTime
	}
	if e.Directo() {
		ch.Kind = "direct"
		if n := cmp.Or(e.DisplayName, g.directos[e.Name]); n != "" {
			ch.Name, ch.Peers = n, []string{n}
		}
	}
	return ch
}

// idMensaje es la última parte del nombre de un mensaje.
func idMensaje(nombre string) string {
	_, id, _ := strings.Cut(nombre, "/messages/")
	return id
}

func (g *GoogleChat) sub(nombre string) string {
	if nombre != "" && nombre == g.yo.Recurso() {
		return Yo
	}
	return nombre
}

func (g *GoogleChat) aMensaje(espacio string, m cchat.MensajeGoogle) api.RoomsMessage {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.aMensajeBloqueado(espacio, m)
}

func (g *GoogleChat) aMensajeBloqueado(espacio string, m cchat.MensajeGoogle) api.RoomsMessage {
	id := idMensaje(m.Name)
	out := api.RoomsMessage{
		ID: id, ChannelID: espacio, AuthorSub: g.sub(m.Sender.Name), AuthorName: g.nombreBloqueado(m.Sender.Name, m.Sender.DisplayName),
		Body: m.Text, CreatedAt: m.CreateTime, IsBot: m.Sender.Type == "BOT",
	}
	if out.AuthorSub == Yo {
		out.AuthorName = cmp.Or(g.yo.Nombre, g.yo.Correo, out.AuthorName)
	}
	if m.UpdateTime != nil && m.UpdateTime.After(m.CreateTime) {
		e := *m.UpdateTime
		out.EditedAt = &e
	}
	if h := m.Thread.Name; h != "" {
		g.hilos[id] = h
		if !m.ThreadReply {
			if _, ok := g.raices[h]; !ok {
				g.raices[h] = id
			}
		} else if r := g.raices[h]; r != "" && r != id {
			out.ParentID = &r
		}
	}
	for _, r := range m.Reactions {
		if r.Emoji.Unicode != "" {
			out.Reactions = append(out.Reactions, api.RoomsReaction{Emoji: r.Emoji.Unicode, Count: r.Count})
		}
	}
	for _, a := range m.Attachment {
		fid := idFichero("g", espacio, id, a.Name, a.ContentName)
		g.adjuntos[fid] = a
		out.Attachments = append(out.Attachments, api.RoomsAttachment{FileID: fid, Name: cmp.Or(a.ContentName, path.Base(a.Name)), ContentType: cmp.Or(a.ContentType, tipoDe(a.ContentName))})
	}
	if m.DeleteTime == nil {
		g.cargados.apuntar(out)
	}
	return out
}

// Mensajes es una página de raíces anteriores a antes, en orden de lectura. Sin Lectura,
// el aviso de cómo leer y lo que se mandó desde aquí.
func (g *GoogleChat) Mensajes(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error) {
	if err := g.asegurar(ctx); err != nil {
		return nil, err
	}
	if !g.Lectura {
		if !antes.IsZero() {
			return nil, nil
		}
		aviso := g.avisados.de(canal, g.Avisos.SinLectura, g.ahora())
		g.mu.Lock()
		defer g.mu.Unlock()
		return append([]api.RoomsMessage{aviso}, g.enviados[canal]...), nil
	}
	ms, err := g.C.Mensajes(ctx, canal, antes, cmp.Or(limite, 50))
	if err != nil {
		return nil, err
	}
	g.resolverNombres(ctx, ms)
	slices.SortFunc(ms, func(a, b cchat.MensajeGoogle) int { return a.CreateTime.Compare(b.CreateTime) })
	g.mu.Lock()
	defer g.mu.Unlock()
	// Primero las raíces, para que cada respuesta sepa la suya.
	for _, m := range ms {
		if !m.ThreadReply && m.Thread.Name != "" {
			if _, ok := g.raices[m.Thread.Name]; !ok {
				g.raices[m.Thread.Name] = idMensaje(m.Name)
			}
		}
	}
	var out []api.RoomsMessage
	respuestas := map[string][]cchat.MensajeGoogle{}
	for _, m := range ms {
		if m.DeleteTime != nil {
			continue
		}
		if m.ThreadReply {
			respuestas[m.Thread.Name] = append(respuestas[m.Thread.Name], m)
			g.aMensajeBloqueado(canal, m) // apunta su hilo
			continue
		}
		out = append(out, g.aMensajeBloqueado(canal, m))
	}
	for i := range out {
		h := g.hilos[out[i].ID]
		for _, r := range respuestas[h] {
			out[i].ReplyCount++
			c := r.CreateTime
			out[i].LastReplyAt = &c
		}
	}
	g.abiertos[canal] = true
	if antes.IsZero() {
		ultimo := g.ahora()
		if n := len(ms); n > 0 && ms[n-1].CreateTime.After(ultimo) {
			ultimo = ms[n-1].CreateTime
		}
		if ultimo.After(g.ultimo[canal]) {
			g.ultimo[canal] = ultimo
		}
	}
	return out, nil
}

// hiloDe es el hilo de un mensaje: el apuntado o, si no, el que dice Google.
func (g *GoogleChat) hiloDe(ctx context.Context, canal, mensaje string) (string, error) {
	g.mu.Lock()
	h := g.hilos[mensaje]
	g.mu.Unlock()
	if h != "" {
		return h, nil
	}
	if !g.Lectura {
		return "", ErrNoDisponible
	}
	m, err := g.C.Mensaje(ctx, canal+"/messages/"+mensaje)
	if err != nil {
		return "", err
	}
	g.aMensaje(canal, m)
	return m.Thread.Name, nil
}

// Enviar publica texto; con raiz, en su hilo.
func (g *GoogleChat) Enviar(ctx context.Context, canal, raiz, texto string) (api.RoomsMessage, error) {
	if err := g.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	hilo := ""
	if raiz != "" {
		var err error
		if hilo, err = g.hiloDe(ctx, canal, raiz); err != nil {
			return api.RoomsMessage{}, err
		}
	}
	m, err := g.C.Enviar(ctx, canal, hilo, texto)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	return g.trasEnviar(canal, raiz, hilo, texto, m), nil
}

// trasEnviar completa lo que Google no devuelve de un mensaje mandado desde aquí y lo
// apunta.
func (g *GoogleChat) trasEnviar(canal, raiz, hilo, texto string, m cchat.MensajeGoogle) api.RoomsMessage {
	g.mu.Lock()
	defer g.mu.Unlock()
	if m.Sender.Name == "" {
		m.Sender.Name = g.yo.Recurso()
	}
	if m.Text == "" {
		m.Text = texto
	}
	if m.CreateTime.IsZero() {
		m.CreateTime = g.ahora()
	}
	if raiz != "" {
		m.ThreadReply = true
		if m.Thread.Name == "" {
			m.Thread.Name = hilo
		}
		g.raices[hilo] = cmp.Or(g.raices[hilo], raiz)
	}
	out := g.aMensajeBloqueado(canal, m)
	if !g.Lectura && raiz == "" {
		g.enviados[canal] = append(g.enviados[canal], out)
	}
	if m.CreateTime.After(g.ultimo[canal]) && g.abiertos[canal] {
		g.ultimo[canal] = m.CreateTime
	}
	return out
}

// MarcarLeido se queda aquí: la API de Chat no lleva el estado de lectura.
func (g *GoogleChat) MarcarLeido(_ context.Context, canal string) error {
	g.mu.Lock()
	delete(g.sinLeer, canal)
	g.mu.Unlock()
	return nil
}

// Hilo es la raíz con sus respuestas.
func (g *GoogleChat) Hilo(ctx context.Context, canal, raiz string) (api.RoomsThread, error) {
	if !g.Lectura {
		return api.RoomsThread{}, ErrNoDisponible
	}
	h, err := g.hiloDe(ctx, canal, raiz)
	if err != nil {
		return api.RoomsThread{}, err
	}
	ms, err := g.C.Hilo(ctx, canal, h)
	if err != nil {
		return api.RoomsThread{}, err
	}
	g.resolverNombres(ctx, ms)
	slices.SortFunc(ms, func(a, b cchat.MensajeGoogle) int { return a.CreateTime.Compare(b.CreateTime) })
	g.mu.Lock()
	defer g.mu.Unlock()
	g.raices[h] = raiz
	var t api.RoomsThread
	for _, m := range ms {
		if m.DeleteTime != nil {
			continue
		}
		x := g.aMensajeBloqueado(canal, m)
		if x.ID == raiz {
			x.ParentID = nil
			t.Root = x
			continue
		}
		if x.ParentID == nil {
			r := raiz
			x.ParentID = &r
		}
		t.Replies = append(t.Replies, x)
	}
	t.Root.ReplyCount = len(t.Replies)
	if t.Root.ID == "" {
		return t, cchat.ErrRespuesta
	}
	return t, nil
}

// Miembros son los del espacio; sin leer, solo quien mira.
func (g *GoogleChat) Miembros(ctx context.Context, canal string) ([]api.RoomsMember, error) {
	if !g.Lectura {
		return []api.RoomsMember{{Subject: Yo, Role: "member"}}, nil
	}
	if err := g.asegurar(ctx); err != nil {
		return nil, err
	}
	ms, err := g.C.Miembros(ctx, canal)
	if err != nil {
		return nil, err
	}
	g.resolverMiembros(ctx, ms)
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]api.RoomsMember, 0, len(ms))
	for _, m := range ms {
		rol := "member"
		if m.Role == "ROLE_MANAGER" {
			rol = "admin"
		}
		nombre := g.nombreBloqueado(m.Member.Name, m.Member.DisplayName)
		if g.sub(m.Member.Name) == Yo {
			nombre = cmp.Or(g.yo.Nombre, g.yo.Correo, nombre)
		}
		out = append(out, api.RoomsMember{Subject: g.sub(m.Member.Name), Name: nombre, Role: rol})
	}
	return out, nil
}

// ProveedorReunion es el nombre del servicio de reuniones.
func (g *GoogleChat) ProveedorReunion() string { return "Google Meet" }

// CrearReunion crea un espacio de Meet y da su enlace (Meet no tiene asunto).
func (g *GoogleChat) CrearReunion(ctx context.Context, _ string) (string, error) {
	return g.M.Crear(ctx)
}

// Escuchar sondea, con Lectura, los espacios que han tenido actividad y los mensajes
// nuevos de los abiertos (por fecha: Chat no da `delta`), con la misma espera creciente
// que Teams. Sin Lectura no hay nada que mirar y espera a que se cierre.
func (g *GoogleChat) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	if err := g.asegurar(ctx); err != nil {
		return err
	}
	if !g.Lectura {
		return g.vigilarEspacios(ctx, alLlegar)
	}
	base, tope := cmp.Or(g.Sondeo, SondeoBase), cmp.Or(g.SondeoMax, SondeoTope)
	espera := base
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(espera):
		}
		n, err := g.sondear(ctx, alLlegar)
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

// vigilarEspacios es la escucha sin Lectura: no hay mensajes que mirar, pero sí si
// aparece un espacio nuevo (los espacios se ven con el permiso de siempre). Despacio:
// cada SondeoMax.
func (g *GoogleChat) vigilarEspacios(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cmp.Or(g.SondeoMax, SondeoTope)):
		}
		es, err := g.C.Espacios(ctx)
		if cchat.Estado(err) == http.StatusTooManyRequests {
			continue
		}
		if err != nil {
			return err
		}
		if g.hayNuevos(es) {
			alLlegar(api.RoomsEvent{Type: EventoCanales})
		}
	}
}

// hayNuevos dice si en es hay un espacio que no estaba en la lista (y que no se
// anunció ya), una vez traída la lista.
func (g *GoogleChat) hayNuevos(es []cchat.EspacioChat) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	nuevo := false
	for _, e := range es {
		if _, ok := g.espacios[e.Name]; !ok && g.cargada && !g.anunciados[e.Name] {
			g.anunciados[e.Name] = true
			nuevo = true
		}
	}
	return nuevo
}

func (g *GoogleChat) sondear(ctx context.Context, alLlegar func(api.RoomsEvent)) (int, error) {
	es, err := g.C.Espacios(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	if g.hayNuevos(es) {
		defer alLlegar(api.RoomsEvent{Type: EventoCanales})
		n++
	}
	for _, e := range es {
		g.mu.Lock()
		antes, visto := g.activos[e.Name]
		abierto := g.abiertos[e.Name]
		desde := g.ultimo[e.Name]
		g.espacios[e.Name] = e
		if e.LastActiveTime != nil {
			g.activos[e.Name] = *e.LastActiveTime
		}
		g.mu.Unlock()
		cambio := e.LastActiveTime != nil && (!visto || e.LastActiveTime.After(antes))
		if !abierto && (!visto || !cambio) {
			continue
		}
		if !abierto {
			// Solo el último, para el aviso y el contador.
			ms, err := g.C.Mensajes(ctx, e.Name, time.Time{}, 1)
			if err != nil {
				return n, err
			}
			g.resolverNombres(ctx, ms)
			for _, m := range ms {
				n += g.entregar(e.Name, m, alLlegar)
			}
			continue
		}
		if desde.IsZero() {
			desde = g.ahora()
		}
		ms, err := g.C.Nuevos(ctx, e.Name, desde)
		if err != nil {
			return n, err
		}
		g.resolverNombres(ctx, ms)
		slices.SortFunc(ms, func(a, b cchat.MensajeGoogle) int { return a.CreateTime.Compare(b.CreateTime) })
		for _, m := range ms {
			n += g.entregar(e.Name, m, alLlegar)
			g.mu.Lock()
			if m.CreateTime.After(g.ultimo[e.Name]) {
				g.ultimo[e.Name] = m.CreateTime
			}
			g.mu.Unlock()
		}
	}
	return n, nil
}

func (g *GoogleChat) entregar(espacio string, m cchat.MensajeGoogle, alLlegar func(api.RoomsEvent)) int {
	if m.DeleteTime != nil {
		return 0
	}
	msg := g.aMensaje(espacio, m)
	raw, err := json.Marshal(msg)
	if err != nil {
		return 0
	}
	g.mu.Lock()
	if msg.AuthorSub != Yo && !g.abiertos[espacio] {
		g.sinLeer[espacio]++
	}
	g.mu.Unlock()
	alLlegar(api.RoomsEvent{Type: "message", ChannelID: espacio, Subject: msg.AuthorSub, Name: msg.AuthorName, Payload: raw})
	return 1
}
