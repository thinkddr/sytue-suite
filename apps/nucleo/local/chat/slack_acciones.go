// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
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
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// Lo que un espacio de Slack hace además de leer y escribir: crear canales, directos,
// invitar, explorar los canales públicos, ficheros y los hilos en los que se escribió.

var (
	_ Creador      = (*Slack)(nil)
	_ Directos     = (*Slack)(nil)
	_ Personas     = (*Slack)(nil)
	_ Invitador    = (*Slack)(nil)
	_ Explorador   = (*Slack)(nil)
	_ Adjuntos     = (*Slack)(nil)
	_ HilosPropios = (*Slack)(nil)
)

// canalAPI es una conversación tal como la da la API.
type canalAPI = cchat.CanalSlack

func canalDeAPI(c canalAPI) canalSlack {
	return canalSlack{
		ID: c.ID, Nombre: c.Name, Asunto: c.Topic.Value, Proposito: c.Purpose.Value,
		Privado: c.Private || c.Mpim, Directo: c.IM, Archivado: c.Archived, Usuario: c.User, Creado: c.Created,
	}
}

// aCanal es la conversación como la ve la pantalla.
func (s *Slack) aCanal(ctx context.Context, c canalSlack) api.RoomsChannel {
	ch := api.RoomsChannel{
		ID: c.ID, Slug: c.ID, Name: c.Nombre, Topic: c.Asunto, Purpose: c.Proposito, Archived: c.Archivado,
		CreatedAt: time.Unix(c.Creado, 0), NotifyLevel: "all", Kind: "public",
	}
	switch {
	case c.Directo:
		ch.Kind = "direct"
		nombre := s.nombre(ctx, c.Usuario)
		ch.Name, ch.Peers, ch.PeerSubs = nombre, []string{nombre}, []string{c.Usuario}
	case c.Privado:
		ch.Kind = "private"
	}
	return ch
}

// apuntar guarda la conversación en la caché y la devuelve como la ve la pantalla.
func (s *Slack) apuntar(ctx context.Context, c canalAPI) api.RoomsChannel {
	cs := canalDeAPI(c)
	s.mu.Lock()
	s.canales[cs.ID] = cs
	s.mu.Unlock()
	return s.aCanal(ctx, cs)
}

// info pide una conversación (`conversations.info`).
func (s *Slack) info(ctx context.Context, canal string) (canalAPI, error) {
	var r struct {
		Channel canalAPI `json:"channel"`
	}
	err := s.api(ctx, "conversations.info", url.Values{"channel": {canal}}, &r)
	return r.Channel, err
}

// NombreCanalSlack es el nombre que Slack admite: minúsculas, cifras, guiones y guiones
// bajos, sin espacios y hasta 80.
func NombreCanalSlack(nombre string) string {
	var b strings.Builder
	guion := false
	for _, c := range strings.ToLower(strings.TrimSpace(nombre)) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			b.WriteRune(c)
			guion = false
		case c == ' ' || c == '-' || c == '.':
			if !guion && b.Len() > 0 {
				b.WriteByte('-')
				guion = true
			}
		case c > 127:
			// Slack admite letras de otros alfabetos en los nombres.
			b.WriteRune(c)
			guion = false
		}
	}
	out := strings.Trim(b.String(), "-")
	if r := []rune(out); len(r) > 80 {
		out = strings.Trim(string(r[:80]), "-")
	}
	return out
}

// CrearCanal crea un canal (`conversations.create`) y le pone el tema si lo hay.
func (s *Slack) CrearCanal(ctx context.Context, nombre, asunto string, privado bool) (api.RoomsChannel, error) {
	if err := s.asegurar(ctx); err != nil {
		return api.RoomsChannel{}, err
	}
	n := NombreCanalSlack(nombre)
	if n == "" {
		return api.RoomsChannel{}, &ErrSlack{Codigo: "invalid_name"}
	}
	var r struct {
		Channel canalAPI `json:"channel"`
	}
	if err := s.api(ctx, "conversations.create", url.Values{"name": {n}, "is_private": {strconv.FormatBool(privado)}}, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	if asunto = strings.TrimSpace(asunto); asunto != "" {
		if s.api(ctx, "conversations.setTopic", url.Values{"channel": {r.Channel.ID}, "topic": {asunto}}, nil) == nil {
			r.Channel.Topic.Value = asunto
		}
	}
	return s.apuntar(ctx, r.Channel), nil
}

// AbrirDirecto abre (o reutiliza: Slack devuelve el que ya hay) el directo con una
// persona, o el de grupo con varias (`conversations.open`).
func (s *Slack) AbrirDirecto(ctx context.Context, personas []string) (api.RoomsChannel, error) {
	if err := s.asegurar(ctx); err != nil {
		return api.RoomsChannel{}, err
	}
	s.mu.Lock()
	yo := s.yo
	s.mu.Unlock()
	personas = slices.DeleteFunc(slices.Clone(personas), func(p string) bool { return p == "" || p == Yo || p == yo })
	if len(personas) == 0 {
		return api.RoomsChannel{}, errors.New("chat: falta con quién")
	}
	var r struct {
		Channel canalAPI `json:"channel"`
	}
	if err := s.api(ctx, "conversations.open", url.Values{"users": {strings.Join(personas, ",")}, "return_im": {"true"}}, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	c := r.Channel
	if c.Name == "" && !c.IM {
		if completo, err := s.info(ctx, c.ID); err == nil {
			c = completo
		}
	}
	if c.IM && c.User == "" && len(personas) == 1 {
		c.User = personas[0]
	}
	if !c.IM && !c.Mpim && len(personas) == 1 {
		c.IM, c.User = true, personas[0]
	}
	return s.apuntar(ctx, c), nil
}

// Gente es la del espacio (`users.list`), sin bots, sin cuentas borradas y sin quien
// mira. Se pide una vez y se guarda.
func (s *Slack) Gente(ctx context.Context) ([]api.RoomsMember, error) {
	if err := s.asegurar(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.gente != nil {
		out := slices.Clone(s.gente)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	var out []api.RoomsMember
	cursor := ""
	for range 10 {
		var r struct {
			Members []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				RealName string `json:"real_name"`
				Deleted  bool   `json:"deleted"`
				Bot      bool   `json:"is_bot"`
				Profile  struct {
					DisplayName string `json:"display_name"`
				} `json:"profile"`
			} `json:"members"`
			Meta struct {
				Next string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		p := url.Values{"limit": {"200"}}
		if cursor != "" {
			p.Set("cursor", cursor)
		}
		if err := s.api(ctx, "users.list", p, &r); err != nil {
			return nil, err
		}
		s.mu.Lock()
		for _, u := range r.Members {
			if u.Deleted || u.Bot || u.ID == "USLACKBOT" {
				continue
			}
			n := cmp.Or(u.Profile.DisplayName, u.RealName, u.Name, u.ID)
			s.nombres[u.ID] = n
			if u.ID != s.yo {
				out = append(out, api.RoomsMember{Subject: u.ID, Name: n, Role: "member"})
			}
		}
		s.mu.Unlock()
		if cursor = r.Meta.Next; cursor == "" {
			break
		}
	}
	slices.SortFunc(out, func(a, b api.RoomsMember) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	s.mu.Lock()
	s.gente = out
	s.mu.Unlock()
	return slices.Clone(out), nil
}

// Resolver busca a alguien del espacio: por correo (`users.lookupByEmail`) o por su
// nombre, su @usuario o su identificador.
func (s *Slack) Resolver(ctx context.Context, q string) (api.RoomsPerson, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return api.RoomsPerson{}, ErrNoEncontrada
	}
	if strings.Contains(q, "@") && strings.Contains(q, ".") && !strings.HasPrefix(q, "@") {
		var r struct {
			User struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				RealName string `json:"real_name"`
			} `json:"user"`
		}
		if err := s.api(ctx, "users.lookupByEmail", url.Values{"email": {q}}, &r); err != nil {
			var e *ErrSlack
			if errors.As(err, &e) && e.Codigo == "users_not_found" {
				return api.RoomsPerson{}, ErrNoEncontrada
			}
			return api.RoomsPerson{}, err
		}
		return api.RoomsPerson{UserID: r.User.ID, Name: cmp.Or(r.User.RealName, r.User.Name), Username: r.User.Name}, nil
	}
	gente, err := s.Gente(ctx)
	if err != nil {
		return api.RoomsPerson{}, err
	}
	b := strings.ToLower(strings.TrimPrefix(q, "@"))
	for _, g := range gente {
		if strings.EqualFold(g.Subject, b) || strings.EqualFold(g.Name, b) {
			return api.RoomsPerson{UserID: g.Subject, Name: g.Name}, nil
		}
	}
	for _, g := range gente {
		if buscar.Contiene(g.Name, b) {
			return api.RoomsPerson{UserID: g.Subject, Name: g.Name}, nil
		}
	}
	return api.RoomsPerson{}, ErrNoEncontrada
}

// Anadir mete a alguien del espacio en el canal (`conversations.invite`).
func (s *Slack) Anadir(ctx context.Context, canal, persona string) (api.RoomsMember, error) {
	if err := s.api(ctx, "conversations.invite", url.Values{"channel": {canal}, "users": {persona}}, nil); err != nil {
		var e *ErrSlack
		if !errors.As(err, &e) || e.Codigo != "already_in_channel" {
			return api.RoomsMember{}, err
		}
	}
	return api.RoomsMember{Subject: persona, Name: s.nombre(ctx, persona), Role: "member"}, nil
}

// Explorar son los canales públicos del espacio en los que no se está, con q en el
// nombre o en el propósito.
func (s *Slack) Explorar(ctx context.Context, q string) ([]api.RoomsChannel, error) {
	if err := s.asegurar(ctx); err != nil {
		return nil, err
	}
	q = buscar.Plegar(strings.TrimSpace(q))
	var out []api.RoomsChannel
	cursor := ""
	for range 5 {
		var r struct {
			Channels []canalAPI `json:"channels"`
			Meta     struct {
				Next string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		p := url.Values{"types": {"public_channel"}, "exclude_archived": {"true"}, "limit": {"200"}}
		if cursor != "" {
			p.Set("cursor", cursor)
		}
		if err := s.api(ctx, "conversations.list", p, &r); err != nil {
			return nil, err
		}
		for _, c := range r.Channels {
			if c.Member || (q != "" && !strings.Contains(buscar.Plegar(c.Name), q) && !strings.Contains(buscar.Plegar(c.Purpose.Value), q)) {
				continue
			}
			out = append(out, s.aCanal(ctx, canalDeAPI(c)))
		}
		if cursor = r.Meta.Next; cursor == "" {
			break
		}
	}
	return out, nil
}

// Unirse entra en un canal público (`conversations.join`).
func (s *Slack) Unirse(ctx context.Context, canal string) (api.RoomsChannel, error) {
	var r struct {
		Channel canalAPI `json:"channel"`
	}
	if err := s.api(ctx, "conversations.join", url.Values{"channel": {canal}}, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	return s.apuntar(ctx, r.Channel), nil
}

// ----- Ficheros -----

// ficheroSlack es un fichero de un mensaje.
type ficheroSlack = cchat.FicheroSlack

// adjuntosDe pone los ficheros del mensaje como adjuntos y se apunta sus direcciones.
func (s *Slack) adjuntosDe(out *api.RoomsMessage, m mensajeSlack) {
	if len(m.Files) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range m.Files {
		if f.ID == "" {
			continue
		}
		s.ficheros[f.ID] = f
		out.Attachments = append(out.Attachments, api.RoomsAttachment{FileID: f.ID, Name: cmp.Or(f.Name, f.Title, f.ID), ContentType: f.Mimetype, SizeBytes: f.Size})
	}
}

// subirUno sube un fichero por el camino de 2024 (`files.getUploadURLExternal` y la
// dirección que da) y devuelve su identificador; se comparte después, al completar.
func (s *Slack) subirUno(ctx context.Context, f Fichero) (string, error) {
	var r struct {
		URL string `json:"upload_url"`
		ID  string `json:"file_id"`
	}
	if err := s.api(ctx, "files.getUploadURLExternal", url.Values{"filename": {f.Nombre}, "length": {strconv.FormatInt(f.Tamano, 10)}}, &r); err != nil {
		return "", err
	}
	rc, err := f.Abrir()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, rc)
	if err != nil {
		return "", err
	}
	req.ContentLength = f.Tamano
	req.Header.Set("Content-Type", cmp.Or(f.Tipo, "application/octet-stream"))
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat: Slack no aceptó el fichero (%d)", resp.StatusCode)
	}
	return r.ID, nil
}

// EnviarFicheros sube los ficheros y los comparte en el canal (o en el hilo) con el
// texto como comentario (`files.completeUploadExternal`). Slack publica el mensaje por
// su cuenta; se busca su `ts` en el fichero compartido para devolverlo.
func (s *Slack) EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error) {
	if len(fs) == 0 {
		return s.Enviar(ctx, canal, raiz, texto)
	}
	if err := s.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	type uno struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	var subidos []uno
	for _, f := range fs {
		id, err := s.subirUno(ctx, f)
		if err != nil {
			return api.RoomsMessage{}, err
		}
		subidos = append(subidos, uno{ID: id, Title: f.Nombre})
	}
	raw, _ := json.Marshal(subidos)
	p := url.Values{"files": {string(raw)}, "channel_id": {canal}}
	if strings.TrimSpace(texto) != "" {
		p.Set("initial_comment", texto)
	}
	if raiz != "" {
		p.Set("thread_ts", raiz)
	}
	if err := s.api(ctx, "files.completeUploadExternal", p, nil); err != nil {
		return api.RoomsMessage{}, err
	}
	// Compartir es asíncrono: se mira unas pocas veces dónde quedó.
	for i := range 5 {
		var r struct {
			File ficheroSlack `json:"file"`
		}
		if s.api(ctx, "files.info", url.Values{"file": {subidos[0].ID}}, &r) == nil {
			ts := ""
			if v := r.File.Shares.Public[canal]; len(v) > 0 {
				ts = v[0].TS
			} else if v := r.File.Shares.Private[canal]; len(v) > 0 {
				ts = v[0].TS
			}
			if ts != "" {
				if m, ok := s.uno(ctx, canal, raiz, ts); ok {
					return m, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return api.RoomsMessage{}, ctx.Err()
		case <-time.After(time.Duration(200*(i+1)) * time.Millisecond):
		}
	}
	// No se supo a tiempo: un mensaje provisional; el de verdad llega por la escucha.
	m := api.RoomsMessage{ID: "subida-" + subidos[0].ID, ChannelID: canal, AuthorSub: Yo, Body: texto, CreatedAt: time.Now()}
	for i, f := range fs {
		m.Attachments = append(m.Attachments, api.RoomsAttachment{FileID: subidos[i].ID, Name: f.Nombre, ContentType: f.Tipo, SizeBytes: f.Tamano})
	}
	if raiz != "" {
		m.ParentID = &raiz
	}
	return m, nil
}

// uno lee el mensaje ts del canal (o del hilo de raiz).
func (s *Slack) uno(ctx context.Context, canal, raiz, ts string) (api.RoomsMessage, bool) {
	var r struct {
		Messages []mensajeSlack `json:"messages"`
	}
	metodo, p := "conversations.history", url.Values{"channel": {canal}, "latest": {ts}, "inclusive": {"true"}, "limit": {"1"}}
	if raiz != "" {
		metodo, p = "conversations.replies", url.Values{"channel": {canal}, "ts": {raiz}, "latest": {ts}, "inclusive": {"true"}, "limit": {"200"}}
	}
	if s.api(ctx, metodo, p, &r) != nil {
		return api.RoomsMessage{}, false
	}
	for _, m := range r.Messages {
		if m.TS == ts {
			s.mu.Lock()
			if raiz == "" && ts > s.ultimo[canal] {
				s.ultimo[canal] = ts
			}
			s.mu.Unlock()
			return s.aMensaje(ctx, canal, m), true
		}
	}
	return api.RoomsMessage{}, false
}

// Bajar trae un fichero (o su miniatura) con el token de la persona. Slack contesta
// con su página de entrada, en HTML, si el token no tiene `files:read`: eso es un fallo.
func (s *Slack) Bajar(ctx context.Context, _, fichero string, miniatura bool) (io.ReadCloser, error) {
	s.mu.Lock()
	f, ok := s.ficheros[fichero]
	s.mu.Unlock()
	if !ok {
		var r struct {
			File ficheroSlack `json:"file"`
		}
		if err := s.api(ctx, "files.info", url.Values{"file": {fichero}}, &r); err != nil {
			return nil, err
		}
		f = r.File
		s.mu.Lock()
		s.ficheros[fichero] = f
		s.mu.Unlock()
	}
	u := cmp.Or(f.URLDescarga, f.URLPrivada)
	if miniatura {
		u = f.Miniatura
	}
	if u == "" {
		return nil, ErrNoDisponible
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || (strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") && !strings.HasPrefix(f.Mimetype, "text/html")) {
		resp.Body.Close()
		return nil, fmt.Errorf("chat: Slack no dio el fichero (%d; ¿falta el permiso files:read?)", resp.StatusCode)
	}
	return resp.Body, nil
}

// ----- Hilos -----

// raizSlack es una raíz con respuestas vista al leer un canal.
type raizSlack struct {
	msg       api.RoomsMessage
	participa bool
}

// apuntarRaiz guarda una raíz con respuestas y si quien mira escribió en ella.
func (s *Slack) apuntarRaiz(canal string, m mensajeSlack, out api.RoomsMessage) {
	if out.ParentID != nil || m.ReplyCount == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clave := canal + sep + m.TS
	r := s.raices[clave]
	r.msg = out
	r.participa = r.participa || out.AuthorSub == Yo || slices.Contains(m.ReplyUsers, s.yo)
	s.raices[clave] = r
}

// MisHilos son las raíces vistas en las que quien mira escribió.
func (s *Slack) MisHilos(context.Context) ([]api.RoomsThreadSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]api.RoomsThreadSummary, 0, len(s.raices))
	for _, r := range s.raices {
		m := r.msg
		if r.participa {
			m.ThreadParticipants = append(slices.Clone(m.ThreadParticipants), api.RoomsThreadParticipant{Sub: Yo})
		}
		h := api.RoomsThreadSummary{Root: m, LastActivity: m.CreatedAt}
		if m.LastReplyAt != nil {
			h.LastActivity = *m.LastReplyAt
		}
		out = append(out, h)
	}
	return out, nil
}
