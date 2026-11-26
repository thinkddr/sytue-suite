// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
)

// Lo que una cuenta de Google Chat hace además de leer y escribir: los nombres de quien
// escribe, abrir un mensaje directo, mandar y abrir ficheros, y buscar en lo cargado.

// ErrSinDirectosGoogle: abrir un mensaje directo con quien no se tenía pide un permiso que
// no se marcó al entrar (los que ya hay se abren sin él).
var ErrSinDirectosGoogle = errors.New("chat: abrir un mensaje directo nuevo en Google Chat pide volver a entrar en la cuenta marcando «Mensajes directos nuevos»")

// maxPreguntas es a cuántas personas se pregunta su nombre de una vez.
const maxPreguntas = 20

// nombreBloqueado es el nombre de usuario (`users/<id>`) con g.mu tomado: el que dio
// Google, el que se supo por la People API, «Persona de Google» o, sin ese texto, el id.
func (g *GoogleChat) nombreBloqueado(usuario, dado string) string {
	if dado != "" {
		return dado
	}
	if n := g.nombres[usuario]; n != "" {
		return n
	}
	if g.Avisos.PersonaGoogle != "" {
		return g.Avisos.PersonaGoogle
	}
	return strings.TrimPrefix(usuario, "users/")
}

// resolverNombres pregunta a la People API por quien escribe en ms sin nombre (con la
// autenticación de la persona, Chat solo da `users/<id>`), y guarda el que venga.
func (g *GoogleChat) resolverNombres(ctx context.Context, ms []cchat.MensajeGoogle) {
	var faltan []string
	g.mu.Lock()
	for _, m := range ms {
		faltan = g.apuntarNombre(faltan, m.Sender.Name, m.Sender.DisplayName, m.Sender.Type)
	}
	g.mu.Unlock()
	g.preguntar(ctx, faltan)
}

// resolverMiembros es resolverNombres para los miembros de un espacio.
func (g *GoogleChat) resolverMiembros(ctx context.Context, ms []cchat.MiembroGoogle) {
	var faltan []string
	g.mu.Lock()
	for _, m := range ms {
		faltan = g.apuntarNombre(faltan, m.Member.Name, m.Member.DisplayName, m.Member.Type)
	}
	g.mu.Unlock()
	g.preguntar(ctx, faltan)
}

// apuntarNombre guarda el nombre que dé Google y, si no lo da, añade usuario a los que
// hay que preguntar (con g.mu tomado).
func (g *GoogleChat) apuntarNombre(faltan []string, usuario, dado, tipo string) []string {
	if usuario == "" || tipo == "BOT" || usuario == g.yo.Recurso() {
		return faltan
	}
	if dado != "" {
		g.nombres[usuario] = dado
		return faltan
	}
	if g.nombres[usuario] != "" || g.intentados[usuario] || slices.Contains(faltan, usuario) {
		return faltan
	}
	return append(faltan, usuario)
}

// preguntar pide a la People API el nombre (o el correo) de esos usuarios. Un 403 o un
// 404 es que Google no lo enseña a quien pregunta: no se vuelve a preguntar por él, y
// tras un 403 no se pregunta por nadie más (la API no está abierta para esta cuenta). Sin
// red se deja para la próxima vez.
func (g *GoogleChat) preguntar(ctx context.Context, usuarios []string) {
	for i, u := range usuarios {
		g.mu.Lock()
		cerrada := g.sinPersonas
		g.mu.Unlock()
		if g.P == nil || cerrada || i >= maxPreguntas {
			return
		}
		p, err := g.P.Persona(ctx, u)
		estado := cchat.Estado(err)
		if err != nil && estado == 0 && !errors.Is(err, cchat.ErrEntrada) && !errors.Is(err, cchat.ErrRespuesta) {
			return
		}
		g.mu.Lock()
		g.intentados[u] = true
		if n := cmp.Or(p.Nombre, p.Correo); err == nil && n != "" {
			g.nombres[u] = n
		}
		if estado == http.StatusForbidden || estado == http.StatusUnauthorized {
			g.sinPersonas = true
		}
		g.mu.Unlock()
		if estado == http.StatusTooManyRequests {
			return
		}
	}
}

// maxDirectosSinNombre es a cuántos directos sin nombre se busca la otra persona en cada
// lista.
const maxDirectosSinNombre = 20

// nombrarDirectos pone a los mensajes directos sin nombre (Google no los nombra) el de
// la otra persona, preguntando por sus miembros: pide poder verlos (Lectura).
func (g *GoogleChat) nombrarDirectos(ctx context.Context, es []cchat.EspacioChat) {
	if !g.Lectura {
		return
	}
	n := 0
	for _, e := range es {
		g.mu.Lock()
		hecho := e.DisplayName != "" || !e.Directo() || g.directos[e.Name] != "" || g.intentados[e.Name]
		g.mu.Unlock()
		if hecho || n >= maxDirectosSinNombre {
			continue
		}
		n++
		ms, err := g.C.Miembros(ctx, e.Name)
		if cchat.Estado(err) == 0 && err != nil {
			return
		}
		g.resolverMiembros(ctx, ms)
		g.mu.Lock()
		g.intentados[e.Name] = true
		var nombres []string
		for _, m := range ms {
			if m.Member.Name != g.yo.Recurso() && m.Member.Type != "BOT" {
				nombres = append(nombres, g.nombreBloqueado(m.Member.Name, m.Member.DisplayName))
			}
		}
		if len(nombres) > 0 {
			g.directos[e.Name] = strings.Join(nombres, ", ")
		}
		g.mu.Unlock()
	}
}

// ----- Personas y directos -----

// usuarioGoogle es como Chat nombra a una persona: `users/<id>` o `users/<correo>`.
func usuarioGoogle(p string) string {
	p = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p), "@"))
	if !strings.HasPrefix(p, "users/") {
		p = "users/" + p
	}
	if !cchat.UsuarioChat.MatchString(p) {
		return ""
	}
	if strings.Contains(p, "@") {
		return strings.ToLower(p)
	}
	return p
}

// Gente es la que se conoce de los espacios cargados (con su nombre), sin quien mira.
func (g *GoogleChat) Gente(ctx context.Context) ([]api.RoomsMember, error) {
	if err := g.asegurar(ctx); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []api.RoomsMember
	for u, n := range g.nombres {
		if u != g.yo.Recurso() && n != "" {
			out = append(out, api.RoomsMember{Subject: u, Name: n, Role: "member"})
		}
	}
	slices.SortFunc(out, func(a, b api.RoomsMember) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// Resolver busca a alguien: por su correo (Chat lo admite para abrir un directo) o por
// su nombre entre la gente conocida.
func (g *GoogleChat) Resolver(ctx context.Context, q string) (api.RoomsPerson, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return api.RoomsPerson{}, ErrNoEncontrada
	}
	if u := usuarioGoogle(q); u != "" && (strings.Contains(q, "@") || strings.HasPrefix(q, "users/")) {
		g.preguntar(ctx, []string{u})
		g.mu.Lock()
		defer g.mu.Unlock()
		nombre := g.nombres[u]
		if nombre == "" {
			nombre = strings.TrimPrefix(u, "users/")
		}
		return api.RoomsPerson{UserID: u, Name: nombre}, nil
	}
	gente, err := g.Gente(ctx)
	if err != nil {
		return api.RoomsPerson{}, err
	}
	b := strings.ToLower(q)
	for _, p := range gente {
		if strings.EqualFold(p.Name, b) {
			return api.RoomsPerson{UserID: p.Subject, Name: p.Name}, nil
		}
	}
	for _, p := range gente {
		if buscar.Contiene(p.Name, b) {
			return api.RoomsPerson{UserID: p.Subject, Name: p.Name}, nil
		}
	}
	return api.RoomsPerson{}, ErrNoEncontrada
}

// AbrirDirecto abre el mensaje directo con una persona (el que ya hay, que Google busca
// con el permiso de siempre) o, si no lo hay o son varias, lo crea: eso pide el permiso
// de los directos nuevos.
func (g *GoogleChat) AbrirDirecto(ctx context.Context, personas []string) (api.RoomsChannel, error) {
	if err := g.asegurar(ctx); err != nil {
		return api.RoomsChannel{}, err
	}
	g.mu.Lock()
	yo, correo := g.yo.Recurso(), strings.ToLower("users/"+g.yo.Correo)
	g.mu.Unlock()
	var usuarios []string
	for _, p := range personas {
		if p == "" || p == Yo {
			continue
		}
		u := usuarioGoogle(p)
		if u == "" {
			return api.RoomsChannel{}, ErrNoEncontrada
		}
		if u != yo && u != correo && !slices.Contains(usuarios, u) {
			usuarios = append(usuarios, u)
		}
	}
	if len(usuarios) == 0 {
		return api.RoomsChannel{}, errors.New("chat: falta con quién")
	}
	var e cchat.EspacioChat
	var err error
	if len(usuarios) == 1 {
		e, err = g.C.BuscarDirecto(ctx, usuarios[0])
	}
	if len(usuarios) > 1 || cchat.Estado(err) == http.StatusNotFound {
		if !g.Directos {
			return api.RoomsChannel{}, ErrSinDirectosGoogle
		}
		e, err = g.C.CrearConversacion(ctx, usuarios)
	}
	if err != nil {
		return api.RoomsChannel{}, err
	}
	g.preguntar(ctx, usuarios)
	g.mu.Lock()
	defer g.mu.Unlock()
	if e.Directo() && e.DisplayName == "" {
		var nombres []string
		for _, u := range usuarios {
			nombre := g.nombreBloqueado(u, "")
			if correo := strings.TrimPrefix(u, "users/"); g.nombres[u] == "" && strings.Contains(correo, "@") {
				nombre = correo // a quien se escribió por su correo
			}
			nombres = append(nombres, nombre)
		}
		g.directos[e.Name] = strings.Join(nombres, ", ")
	}
	g.espacios[e.Name] = e
	g.anunciados[e.Name] = true
	return g.aCanal(e), nil
}

// ----- Ficheros -----

// EnviarFicheros sube los ficheros al espacio (`media.upload`, con el permiso de
// escribir) y los manda con el texto; con raiz, en su hilo.
func (g *GoogleChat) EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error) {
	if len(fs) == 0 {
		return g.Enviar(ctx, canal, raiz, texto)
	}
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
	refs := make([]cchat.RefAdjuntoGoogle, 0, len(fs))
	for _, f := range fs {
		r, err := g.C.Subir(ctx, canal, f.Nombre, cchat.Fichero{Tipo: f.Tipo, Tamano: f.Tamano, Abrir: f.Abrir})
		if err != nil {
			return api.RoomsMessage{}, err
		}
		refs = append(refs, r)
	}
	m, err := g.C.EnviarConAdjuntos(ctx, canal, hilo, texto, refs)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	if len(m.Attachment) == 0 {
		for i, f := range fs {
			m.Attachment = append(m.Attachment, cchat.AdjuntoGoogle{Name: refs[i].Recurso, ContentName: f.Nombre, ContentType: f.Tipo, Datos: &refs[i], Source: "UPLOADED_CONTENT"})
		}
	}
	return g.trasEnviar(canal, raiz, hilo, texto, m), nil
}

// Bajar trae un fichero subido a Chat por su referencia (`media.download`, que pide
// poder leer los mensajes). Los de Drive van con Drive, que esta cuenta no tiene.
func (g *GoogleChat) Bajar(ctx context.Context, _, fichero string, _ bool) (io.ReadCloser, error) {
	g.mu.Lock()
	a, ok := g.adjuntos[fichero]
	g.mu.Unlock()
	if !ok || a.Datos == nil || a.Datos.Recurso == "" {
		return nil, ErrNoDisponible
	}
	c, err := g.C.Bajar(ctx, a.Datos.Recurso)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Buscar busca en los mensajes cargados en esta sesión: la API de Chat no busca en los
// mensajes con la autenticación de la persona.
func (g *GoogleChat) Buscar(_ context.Context, q string, limite int) ([]api.RoomsMessage, error) {
	return g.cargados.buscar(q, limite), nil
}
