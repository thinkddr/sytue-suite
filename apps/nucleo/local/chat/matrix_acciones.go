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
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Lo que una cuenta Matrix hace además de leer y escribir: crear salas, directos,
// invitar, explorar el directorio del servidor, adjuntos sin cifrar y sus hilos.

var (
	_ Creador      = (*Matrix)(nil)
	_ Directos     = (*Matrix)(nil)
	_ Personas     = (*Matrix)(nil)
	_ Invitador    = (*Matrix)(nil)
	_ Explorador   = (*Matrix)(nil)
	_ Adjuntos     = (*Matrix)(nil)
	_ HilosPropios = (*Matrix)(nil)
)

// infoMx es el `info` de un m.image, m.file, m.video o m.audio.
type infoMx struct {
	Tipo   string `json:"mimetype,omitempty"`
	Tamano int64  `json:"size,omitempty"`
}

// adjuntoMx es el adjunto de un mensaje con fichero SIN cifrar (`url` mxc://). Los
// cifrados llevan `file` en vez de `url`, y aquí ni llegan: van en m.room.encrypted.
func adjuntoMx(c contenidoMx) (api.RoomsAttachment, bool) {
	switch c.Tipo {
	case "m.image", "m.file", "m.video", "m.audio":
	default:
		return api.RoomsAttachment{}, false
	}
	if !strings.HasPrefix(c.URL, "mxc://") {
		return api.RoomsAttachment{}, false
	}
	a := api.RoomsAttachment{FileID: c.URL, Name: cmp.Or(c.Fichero, c.Cuerpo, "fichero")}
	if c.Info != nil {
		a.ContentType, a.SizeBytes = c.Info.Tipo, c.Info.Tamano
	}
	return a, true
}

// tipoMx es el msgtype de un fichero por su tipo MIME.
func tipoMx(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "m.image"
	case strings.HasPrefix(mime, "video/"):
		return "m.video"
	case strings.HasPrefix(mime, "audio/"):
		return "m.audio"
	}
	return "m.file"
}

// salaNueva pone en la caché una sala recién creada o a la que se acaba de entrar,
// para poder abrirla antes de que llegue por /sync (que la rellena). Con m.mu cogido.
func (m *Matrix) salaNueva(id, nombre, asunto, reglas string) *salaMx {
	s := m.salas[id]
	if s == nil {
		s = &salaMx{id: id, miembros: map[string]string{m.Usuario: localpart(m.Usuario)}, reacciones: map[string]reaccionMx{}, creada: time.Now(), completa: true}
		m.salas[id] = s
	}
	s.nombre, s.asunto, s.reglas = cmp.Or(nombre, s.nombre), cmp.Or(asunto, s.asunto), cmp.Or(reglas, s.reglas)
	return s
}

// CrearCanal crea una sala (createRoom). Pública: en el directorio del servidor y con
// entrada libre; privada: solo por invitación. No se pide cifrado: esta edición no lo
// lee. Si el servidor lo pone por su cuenta a las salas privadas, la sala sale como
// cifrada, igual que cualquier otra.
func (m *Matrix) CrearCanal(ctx context.Context, nombre, asunto string, privado bool) (api.RoomsChannel, error) {
	if err := m.asegurar(ctx); err != nil {
		return api.RoomsChannel{}, err
	}
	cuerpo := map[string]any{"name": strings.TrimSpace(nombre), "preset": "public_chat", "visibility": "public"}
	reglas := "public"
	if privado {
		cuerpo["preset"], cuerpo["visibility"], reglas = "private_chat", "private", "invite"
	}
	if asunto = strings.TrimSpace(asunto); asunto != "" {
		cuerpo["topic"] = asunto
	}
	var r struct {
		ID string `json:"room_id"`
	}
	if err := m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/createRoom", nil, cuerpo, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.salaNueva(r.ID, strings.TrimSpace(nombre), asunto, reglas)
	s.creador = m.Usuario
	return m.canalDe(s), nil
}

// AbrirDirecto reutiliza el directo con esa persona si ya hay uno (m.direct); si no,
// crea una sala directa (`is_direct`, `trusted_private_chat`), invita a las personas y
// la apunta en el m.direct de la cuenta para que los demás clientes la vean como
// directo.
func (m *Matrix) AbrirDirecto(ctx context.Context, personas []string) (api.RoomsChannel, error) {
	if err := m.asegurar(ctx); err != nil {
		return api.RoomsChannel{}, err
	}
	personas = slices.DeleteFunc(slices.Clone(personas), func(p string) bool { return p == m.Usuario || p == Yo || p == "" })
	if len(personas) == 0 {
		return api.RoomsChannel{}, errors.New("chat: falta con quién")
	}
	for _, p := range personas {
		if !strings.HasPrefix(p, "@") || !strings.Contains(p, ":") {
			return api.RoomsChannel{}, fmt.Errorf("chat: %q no es un usuario de Matrix (@nombre:servidor)", p)
		}
	}
	m.mu.Lock()
	if len(personas) == 1 {
		for _, id := range m.directosDe[personas[0]] {
			if s := m.salas[id]; s != nil {
				c := m.canalDe(s)
				m.mu.Unlock()
				return c, nil
			}
		}
	}
	m.mu.Unlock()
	var r struct {
		ID string `json:"room_id"`
	}
	cuerpo := map[string]any{"is_direct": true, "preset": "trusted_private_chat", "invite": personas}
	if err := m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/createRoom", nil, cuerpo, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	m.mu.Lock()
	d := map[string][]string{}
	for k, v := range m.directosDe {
		d[k] = slices.Clone(v)
	}
	for _, p := range personas {
		d[p] = append(d[p], r.ID)
	}
	m.directosDe, m.directos[r.ID] = d, true
	s := m.salaNueva(r.ID, "", "", "invite")
	for _, p := range personas {
		s.miembros[p] = localpart(p)
	}
	c := m.canalDe(s)
	m.mu.Unlock()
	// La sala ya existe y es directa (`is_direct`): si no se guarda el m.direct, los
	// demás clientes la enseñan como sala, pero no se deshace.
	_ = m.pedir(ctx, http.MethodPut, ruta("/_matrix/client/v3/user/", m.Usuario, "/account_data/m.direct"), nil, d, nil)
	return c, nil
}

// Gente es quien sale en las salas cargadas, sin repetir y sin quien mira.
func (m *Matrix) Gente(ctx context.Context) ([]api.RoomsMember, error) {
	if err := m.asegurar(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	vistos := map[string]string{}
	for _, s := range m.salas {
		for mx, n := range s.miembros {
			if mx != m.Usuario {
				vistos[mx] = cmp.Or(vistos[mx], n)
			}
		}
	}
	out := make([]api.RoomsMember, 0, len(vistos))
	for mx, n := range vistos {
		out = append(out, api.RoomsMember{Subject: mx, Name: n, Role: "member"})
	}
	slices.SortFunc(out, func(a, b api.RoomsMember) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.Subject, b.Subject))
	})
	return out, nil
}

// ErrNoEncontrada: no hay nadie con eso.
var ErrNoEncontrada = errors.New("chat: no se encontró a nadie con eso")

// Resolver busca a alguien: un `@nombre:servidor` por su perfil; lo demás, en el
// directorio de usuarios del servidor (el primero que salga).
func (m *Matrix) Resolver(ctx context.Context, q string) (api.RoomsPerson, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return api.RoomsPerson{}, ErrNoEncontrada
	}
	if strings.HasPrefix(q, "@") && strings.Contains(q, ":") {
		var r struct {
			Nombre string `json:"displayname"`
		}
		if err := m.pedir(ctx, http.MethodGet, ruta("/_matrix/client/v3/profile/", q), nil, nil, &r); err != nil {
			var e *ErrMatrix
			if errors.As(err, &e) && e.Estado == http.StatusNotFound {
				return api.RoomsPerson{}, ErrNoEncontrada
			}
			return api.RoomsPerson{}, err
		}
		return api.RoomsPerson{UserID: q, Name: cmp.Or(r.Nombre, localpart(q)), Username: localpart(q)}, nil
	}
	var r struct {
		Resultados []struct {
			ID     string `json:"user_id"`
			Nombre string `json:"display_name"`
		} `json:"results"`
	}
	if err := m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/user_directory/search", nil,
		map[string]any{"search_term": strings.TrimPrefix(q, "@"), "limit": 10}, &r); err != nil {
		return api.RoomsPerson{}, err
	}
	for _, x := range r.Resultados {
		if x.ID != m.Usuario {
			return api.RoomsPerson{UserID: x.ID, Name: cmp.Or(x.Nombre, localpart(x.ID)), Username: localpart(x.ID)}, nil
		}
	}
	return api.RoomsPerson{}, ErrNoEncontrada
}

// Anadir invita a alguien a la sala: en Matrix se entra aceptando, así que vuelve como
// pendiente.
func (m *Matrix) Anadir(ctx context.Context, canal, persona string) (api.RoomsMember, error) {
	if !strings.HasPrefix(persona, "@") || !strings.Contains(persona, ":") {
		return api.RoomsMember{}, fmt.Errorf("chat: %q no es un usuario de Matrix (@nombre:servidor)", persona)
	}
	if err := m.pedir(ctx, http.MethodPost, ruta("/_matrix/client/v3/rooms/", canal, "/invite"), nil, map[string]string{"user_id": persona}, nil); err != nil {
		return api.RoomsMember{}, err
	}
	m.mu.Lock()
	nombre := localpart(persona)
	if s := m.salas[canal]; s != nil {
		s.miembros[persona] = cmp.Or(s.miembros[persona], nombre)
		nombre = s.miembros[persona]
	}
	m.mu.Unlock()
	return api.RoomsMember{Subject: persona, Name: nombre, Role: "member", Pending: true}, nil
}

// Explorar busca en el directorio de salas públicas del servidor de la cuenta; las
// salas en las que ya se está no salen.
func (m *Matrix) Explorar(ctx context.Context, q string) ([]api.RoomsChannel, error) {
	if err := m.asegurar(ctx); err != nil {
		return nil, err
	}
	cuerpo := map[string]any{"limit": 50}
	if q = strings.TrimSpace(q); q != "" {
		cuerpo["filter"] = map[string]string{"generic_search_term": q}
	}
	var r struct {
		Trozo []struct {
			ID      string `json:"room_id"`
			Nombre  string `json:"name"`
			Alias   string `json:"canonical_alias"`
			Asunto  string `json:"topic"`
			Dentro  int    `json:"num_joined_members"`
			Entrada string `json:"join_rule"`
		} `json:"chunk"`
	}
	if err := m.pedir(ctx, http.MethodPost, "/_matrix/client/v3/publicRooms", nil, cuerpo, &r); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []api.RoomsChannel
	for _, x := range r.Trozo {
		if _, ya := m.salas[x.ID]; ya || (x.Entrada != "" && x.Entrada != "public") {
			continue
		}
		out = append(out, api.RoomsChannel{ID: x.ID, Slug: x.ID, Name: cmp.Or(x.Nombre, x.Alias, x.ID), Topic: x.Asunto, Purpose: x.Asunto, Kind: "public", NotifyLevel: "all"})
	}
	return out, nil
}

// Unirse entra en una sala pública (o en una a la que se está invitado).
func (m *Matrix) Unirse(ctx context.Context, canal string) (api.RoomsChannel, error) {
	var r struct {
		ID string `json:"room_id"`
	}
	if err := m.pedir(ctx, http.MethodPost, ruta("/_matrix/client/v3/join/", canal), nil, map[string]any{}, &r); err != nil {
		return api.RoomsChannel{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.canalDe(m.salaNueva(cmp.Or(r.ID, canal), "", "", "public")), nil
}

// ----- Adjuntos -----

// crudo hace una petición sin JSON (subir o bajar medios). El que llama cierra el
// cuerpo de la respuesta.
func (m *Matrix) crudo(ctx context.Context, metodo, ruta string, q url.Values, cuerpo io.Reader, tipo string, tamano int64) (*http.Response, error) {
	u := m.Servidor + ruta
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, cuerpo)
	if err != nil {
		return nil, err
	}
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	if tamano > 0 {
		req.ContentLength = tamano
	}
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.cli.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		e := &ErrMatrix{Estado: resp.StatusCode}
		var c struct {
			Errcode string `json:"errcode"`
			Error   string `json:"error"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = json.Unmarshal(raw, &c)
		e.Codigo, e.Mensaje = c.Errcode, c.Error
		return nil, e
	}
	return resp, nil
}

// subir sube un fichero al repositorio de medios y devuelve su mxc://. Sin cifrar: la
// sala tampoco lo está.
func (m *Matrix) subir(ctx context.Context, f Fichero) (string, error) {
	rc, err := f.Abrir()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	resp, err := m.crudo(ctx, http.MethodPost, "/_matrix/media/v3/upload", url.Values{"filename": {f.Nombre}}, rc,
		cmp.Or(f.Tipo, "application/octet-stream"), f.Tamano)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		URI string `json:"content_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&r); err != nil {
		return "", err
	}
	if !strings.HasPrefix(r.URI, "mxc://") {
		return "", errors.New("chat: el servidor Matrix no devolvió el mxc:// del fichero")
	}
	return r.URI, nil
}

// EnviarFicheros sube cada fichero y lo manda como su propio mensaje (m.image, m.file…);
// el texto va de pie del primero (`filename` + `body`, Matrix 1.10). Devuelve el
// primero; los demás llegan por /sync como cualquier mensaje.
func (m *Matrix) EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error) {
	if len(fs) == 0 {
		return m.Enviar(ctx, canal, raiz, texto)
	}
	if err := m.asegurar(ctx); err != nil {
		return api.RoomsMessage{}, err
	}
	if err := m.cifrada(canal); err != nil {
		return api.RoomsMessage{}, err
	}
	var primero api.RoomsMessage
	for i, f := range fs {
		mxc, err := m.subir(ctx, f)
		if err != nil {
			return primero, err
		}
		c := contenidoMx{Tipo: tipoMx(f.Tipo), Cuerpo: f.Nombre, URL: mxc, Info: &infoMx{Tipo: f.Tipo, Tamano: f.Tamano}}
		if i == 0 && strings.TrimSpace(texto) != "" {
			c.Cuerpo, c.Fichero = texto, f.Nombre
		}
		if raiz != "" {
			c.Relacion = &relacionMx{Tipo: "m.thread", Evento: raiz, Recurre: true}
			c.Relacion.Respues = &struct {
				Evento string `json:"event_id"`
			}{raiz}
		}
		id, err := m.mandar(ctx, canal, "m.room.message", c)
		if err != nil {
			return primero, err
		}
		if i > 0 {
			continue
		}
		raw, _ := json.Marshal(c)
		m.mu.Lock()
		if s, err := m.sala(canal); err == nil {
			_, primero, _ = m.aplicar(s, eventoMx{Tipo: "m.room.message", ID: id, Remite: m.Usuario, Hora: time.Now().UnixMilli(), Contenid: raw}, false)
			if primero.ID == "" {
				if j := s.indice(id); j >= 0 {
					primero = s.eventos[j]
				}
			}
		}
		m.mu.Unlock()
	}
	return primero, nil
}

// partirMxc parte mxc://servidor/medio.
func partirMxc(mxc string) (servidor, medio string, err error) {
	resto, ok := strings.CutPrefix(mxc, "mxc://")
	servidor, medio, ok2 := strings.Cut(resto, "/")
	if !ok || !ok2 || servidor == "" || medio == "" || strings.Contains(medio, "/") {
		return "", "", fmt.Errorf("chat: %q no es un mxc://", mxc)
	}
	return servidor, medio, nil
}

// Bajar trae un medio: primero por la API autenticada (Matrix 1.11) y, si el servidor
// no la tiene, por la vieja. La miniatura la escala el servidor.
func (m *Matrix) Bajar(ctx context.Context, _, fichero string, miniatura bool) (io.ReadCloser, error) {
	servidor, medio, err := partirMxc(fichero)
	if err != nil {
		return nil, err
	}
	clase, q := "/download/", url.Values(nil)
	if miniatura {
		clase, q = "/thumbnail/", url.Values{"width": {"320"}, "height": {"320"}, "method": {"scale"}}
	}
	resp, err := m.crudo(ctx, http.MethodGet, ruta("/_matrix/client/v1/media"+clase, servidor, "/", medio), q, nil, "", 0)
	var e *ErrMatrix
	if errors.As(err, &e) && (e.Estado == http.StatusNotFound || e.Estado == http.StatusMethodNotAllowed) && e.Codigo != "M_NOT_FOUND" {
		resp, err = m.crudo(ctx, http.MethodGet, ruta("/_matrix/media/v3"+clase, servidor, "/", medio), q, nil, "", 0)
	}
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// MisHilos son las raíces con respuestas de las salas cargadas.
func (m *Matrix) MisHilos(context.Context) ([]api.RoomsThreadSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []api.RoomsThreadSummary
	for _, s := range m.salas {
		for _, e := range s.eventos {
			if e.ParentID != nil || e.ReplyCount == 0 {
				continue
			}
			h := api.RoomsThreadSummary{Root: e, LastActivity: e.CreatedAt}
			if e.LastReplyAt != nil {
				h.LastActivity = *e.LastReplyAt
			}
			out = append(out, h)
		}
	}
	return out, nil
}
