// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// Lo que una cuenta de Teams hace además de leer y escribir: abrir un chat nuevo con
// alguien, mandar y abrir ficheros, y buscar en lo cargado.

// ErrSinFicherosTeams: los adjuntos de Teams piden el permiso de los ficheros, que no se
// marcó al entrar.
var ErrSinFicherosTeams = errors.New("chat: los adjuntos de Teams piden volver a entrar en la cuenta marcando «Adjuntos»")

// adjuntoTeams es de dónde sale un adjunto que se ha visto: un contenido alojado en el
// mensaje (una imagen pegada) o un fichero de OneDrive o SharePoint enlazado.
type adjuntoTeams struct {
	remoto, mensaje string
	alojado, enlace string
}

// idFichero es el `FileID` de un adjunto: corto y sin nada del proveedor dentro.
func idFichero(prefijo string, partes ...string) string {
	h := sha256.Sum256([]byte(strings.Join(partes, "\x00")))
	return prefijo + hex.EncodeToString(h[:10])
}

// tipoDe es el tipo de un fichero por su nombre.
func tipoDe(nombre string) string {
	t := mime.TypeByExtension(strings.ToLower(path.Ext(nombre)))
	if i := strings.Index(t, ";"); i >= 0 {
		t = t[:i]
	}
	return cmp.Or(t, "application/octet-stream")
}

// adjuntosDe pone los ficheros de m como adjuntos y se apunta de dónde bajarlos (con
// t.mu tomado).
func (t *Teams) adjuntosDe(out *api.RoomsMessage, remoto string, m cchat.MensajeGraph) {
	for _, a := range m.Attachments {
		if !a.Referencia() {
			continue
		}
		id := idFichero("t", remoto, m.ID, a.ID, a.Enlace)
		nombre := cmp.Or(a.Nombre, path.Base(a.Enlace))
		t.adjuntos[id] = adjuntoTeams{remoto: remoto, mensaje: m.ID, enlace: a.Enlace}
		out.Attachments = append(out.Attachments, api.RoomsAttachment{FileID: id, Name: nombre, ContentType: tipoDe(nombre)})
	}
	for i, h := range m.Alojados() {
		id := idFichero("t", remoto, m.ID, h)
		t.adjuntos[id] = adjuntoTeams{remoto: remoto, mensaje: m.ID, alojado: h}
		out.Attachments = append(out.Attachments, api.RoomsAttachment{FileID: id, Name: fmt.Sprintf("imagen-%d.png", i+1), ContentType: "image/png"})
	}
}

// EnviarFicheros sube los ficheros al OneDrive de quien manda (en la carpeta de los
// ficheros de los chats de Teams), los comparte y los enlaza en el mensaje, como hace
// Teams. En un chat se comparten con sus miembros; en un canal, con la organización.
func (t *Teams) EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error) {
	if len(fs) == 0 {
		return t.Enviar(ctx, canal, raiz, texto)
	}
	if !t.Ficheros {
		return api.RoomsMessage{}, ErrSinFicherosTeams
	}
	yo, err := t.asegurar(ctx)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	if _, ok := cchat.EsChat(canal); ok && raiz != "" {
		return api.RoomsMessage{}, ErrSinHilosTeams
	}
	es := make([]cchat.ElementoDrive, 0, len(fs))
	for _, f := range fs {
		e, err := t.G.SubirOneDrive(ctx, f.Nombre, cchat.Fichero{Tipo: f.Tipo, Tamano: f.Tamano, Abrir: f.Abrir})
		if err != nil {
			return api.RoomsMessage{}, err
		}
		es = append(es, e)
	}
	t.compartir(ctx, canal, yo, es)
	m, err := t.G.EnviarConAdjuntos(ctx, canal, raiz, texto, es)
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
	if len(m.Attachments) == 0 {
		for _, e := range es {
			m.Attachments = append(m.Attachments, cchat.AdjuntoGraph{ID: e.IDAdjunto(), Tipo: "reference", Enlace: e.Web, Nombre: e.Nombre})
		}
	}
	return t.aMensaje(canal, m), nil
}

// compartir da acceso a los ficheros subidos: a los miembros del chat o, en un canal, a
// la organización. Si no se puede, el mensaje va igual (quien lo reciba pedirá acceso).
func (t *Teams) compartir(ctx context.Context, canal, yo string, es []cchat.ElementoDrive) {
	chat, esChat := cchat.EsChat(canal)
	var otros []string
	if esChat {
		t.mu.Lock()
		ms := slices.Clone(t.chats[chat].Miembros)
		t.mu.Unlock()
		if len(ms) == 0 {
			if c, err := t.G.Chat(ctx, chat); err == nil {
				ms = c.Miembros
			}
		}
		for _, m := range ms {
			if m.Usuario != "" && m.Usuario != yo {
				otros = append(otros, m.Usuario)
			}
		}
	}
	for _, e := range es {
		var err error
		if esChat {
			err = t.G.Compartir(ctx, e.ID, otros)
		} else {
			_, err = t.G.EnlaceOrganizacion(ctx, e.ID)
		}
		if err != nil {
			slog.Warn("chat: no se pudo compartir un fichero de Teams", "error", err)
		}
	}
}

// Bajar trae un adjunto visto: una imagen pegada (vale con poder leer el chat) o un
// fichero enlazado (pide el permiso de los ficheros).
func (t *Teams) Bajar(ctx context.Context, _, fichero string, _ bool) (io.ReadCloser, error) {
	t.mu.Lock()
	a, ok := t.adjuntos[fichero]
	t.mu.Unlock()
	if !ok {
		return nil, ErrNoDisponible
	}
	var c cchat.Contenido
	var err error
	switch {
	case a.alojado != "":
		c, err = t.G.BajarAlojado(ctx, a.remoto, a.mensaje, a.alojado)
	case !t.Ficheros:
		return nil, ErrSinFicherosTeams
	default:
		c, err = t.G.BajarCompartido(ctx, a.enlace)
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ----- Personas y directos -----

// Gente es la de los chats cargados (Teams no deja listar la organización con los
// permisos que se piden), sin quien mira y por nombre.
func (t *Teams) Gente(ctx context.Context) ([]api.RoomsMember, error) {
	if _, err := t.asegurar(ctx); err != nil {
		return nil, err
	}
	if err := t.cargarChats(ctx); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	vistos := map[string]bool{}
	var out []api.RoomsMember
	for _, c := range t.chats {
		for _, m := range c.Miembros {
			if m.Usuario == "" || m.Usuario == t.yo.ID || vistos[m.Usuario] {
				continue
			}
			vistos[m.Usuario] = true
			out = append(out, api.RoomsMember{Subject: m.Usuario, Name: cmp.Or(m.Nombre, m.Correo, m.Usuario), Role: "member"})
		}
	}
	slices.SortFunc(out, func(a, b api.RoomsMember) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// Resolver busca a alguien: por su correo de la organización (vale aunque no se haya
// hablado nunca con él: Teams abre el chat con su nombre principal) o por su nombre entre
// la gente de los chats.
func (t *Teams) Resolver(ctx context.Context, q string) (api.RoomsPerson, error) {
	q = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(q), "@"))
	if q == "" {
		return api.RoomsPerson{}, ErrNoEncontrada
	}
	gente, err := t.Gente(ctx)
	if err != nil {
		return api.RoomsPerson{}, err
	}
	if cchat.PersonaTeams.MatchString(q) {
		t.mu.Lock()
		defer t.mu.Unlock()
		for _, c := range t.chats {
			for _, m := range c.Miembros {
				if m.Usuario != "" && strings.EqualFold(m.Correo, q) {
					return api.RoomsPerson{UserID: m.Usuario, Name: cmp.Or(m.Nombre, m.Correo)}, nil
				}
			}
		}
		return api.RoomsPerson{UserID: strings.ToLower(q), Name: q, Username: q}, nil
	}
	b := strings.ToLower(q)
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

// AbrirDirecto abre el chat con esas personas (por su id o su correo): con una, el 1:1
// que ya hay si lo hay; si no, Graph lo crea (y si ya existía, lo devuelve).
func (t *Teams) AbrirDirecto(ctx context.Context, personas []string) (api.RoomsChannel, error) {
	yo, err := t.asegurar(ctx)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	personas = slices.DeleteFunc(slices.Clone(personas), func(p string) bool { return p == "" || p == Yo || p == yo })
	if len(personas) == 0 {
		return api.RoomsChannel{}, errors.New("chat: falta con quién")
	}
	if len(personas) == 1 {
		_ = t.cargarChats(ctx)
		t.mu.Lock()
		for _, c := range t.chats {
			if c.Tipo != "oneOnOne" {
				continue
			}
			for _, m := range c.Miembros {
				if m.Usuario != yo && (m.Usuario == personas[0] || m.Correo != "" && strings.EqualFold(m.Correo, personas[0])) {
					ch := t.aCanalChat(c)
					t.mu.Unlock()
					return ch, nil
				}
			}
		}
		t.mu.Unlock()
	}
	c, err := t.G.CrearChat(ctx, yo, personas)
	if err != nil {
		return api.RoomsChannel{}, err
	}
	if completo, err := t.G.Chat(ctx, c.ID); err == nil && completo.ID == c.ID {
		c = completo
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.chats[c.ID]; ok && c.Ultimo == nil {
		c.Ultimo, c.Vista = prev.Ultimo, prev.Vista
	}
	t.chats[c.ID] = c
	t.anunciados[c.ID] = true
	return t.aCanalChat(c), nil
}

// Buscar busca en los mensajes cargados en esta sesión: Graph solo busca en los chats
// con Microsoft Search, que pide otros permisos.
func (t *Teams) Buscar(_ context.Context, q string, limite int) ([]api.RoomsMessage, error) {
	return t.cargados.buscar(q, limite), nil
}
