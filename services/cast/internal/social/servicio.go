// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package social son los comentarios bajo un vídeo y el chat de un directo de
// Sytue Cast: el servicio, su store sobre pg.Pool y el hub de WebSocket que
// difunde el chat a quienes lo tienen abierto.
//
// El tenant de cada transacción es el del VÍDEO, no el del token: una persona
// invitada de otra organización escribe filas en la organización del vídeo,
// como los externos de Rooms (ADR 0040). Quién puede ver el vídeo lo decide
// `internal/acceso` (Acceso); aquí solo se aplican las reglas sociales: escribir
// exige sesión y que el vídeo tenga la función encendida, borrar lo puede hacer
// el autor o quien modera (dueño del vídeo, admin u owner de la organización).
package social

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Errores del servicio. La capa HTTP los traduce al sobre canónico.
var (
	// ErrNoExiste: el vídeo, el comentario o el mensaje no existe o no se puede ver.
	ErrNoExiste = errors.New("social: no existe")
	// ErrProhibido: quien llama no puede hacer eso (borrar lo de otra persona,
	// escribir sin sesión).
	ErrProhibido = errors.New("social: prohibido")
	// ErrDesactivado: el vídeo tiene apagados los comentarios o el chat.
	ErrDesactivado = errors.New("social: desactivado")
	// ErrEntradaInvalida: cuerpo vacío o demasiado largo, cursor ilegible.
	ErrEntradaInvalida = errors.New("social: entrada inválida")
	// ErrDemasiadoRapido: la persona manda mensajes más deprisa de lo permitido.
	ErrDemasiadoRapido = errors.New("social: demasiado rápido")
)

// Topes del cuerpo, los mismos que los CHECK de la migración 0003.
const (
	MaxComentario = 2000
	MaxMensaje    = 500
)

// Quien es quien pregunta: lo que dice su token, o nadie.
type Quien struct {
	Sub     string
	Tenant  string
	Nombre  string
	Usuario string
	// Admin: owner o admin en SU organización. Lo necesita el resolutor de acceso
	// para dar la moderación a quien administra la organización del vídeo.
	Admin bool
	// Operador: opera la plataforma ahora mismo. Solo lo pone la capa HTTP de la
	// cola del operador, tras el sí de Sytue ID (platform/pkg/operador).
	Operador bool
	Anonimo  bool
	// IP es de quien pregunta. Solo la usa la denuncia sin sesión, troceada, para
	// no contar dos veces a la misma persona; no se guarda.
	IP string
}

// Video es lo que lo social necesita saber del vídeo. Lo rellena la capa HTTP a
// partir de `domain.Video` tras resolver el acceso.
type Video struct {
	ID       uuid.UUID
	Tenant   string
	OwnerSub string
	// OwnerName y Titulo son lo que una denuncia guarda del vídeo.
	OwnerName string
	Titulo    string
	// ComentariosActivos y ChatActivo son `comments_enabled` y `chat_enabled`.
	ComentariosActivos bool
	ChatActivo         bool
}

// Permisos es lo que quien pregunta puede hacer con el vídeo (`acceso.Permisos`).
type Permisos struct {
	Ver, Editar, Moderar bool
}

// Acceso resuelve el vídeo y los permisos de quien pregunta. Es la firma de
// `acceso.Resolver` sin el store: la capa HTTP la cierra sobre el suyo. Debe
// devolver ErrNoExiste (o un error que lo envuelva) cuando no se puede ver.
type Acceso func(ctx context.Context, quien Quien, videoID uuid.UUID) (Video, Permisos, error)

// Autor es quien escribió un comentario o un mensaje.
type Autor struct {
	Sub      string `json:"sub"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// Comentario es un comentario bajo el vídeo, tal como sale por el cable.
type Comentario struct {
	ID        uuid.UUID  `json:"id"`
	Author    Autor      `json:"author"`
	Body      string     `json:"body"`
	ReplyTo   *uuid.UUID `json:"reply_to,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	// Deleted marca el hueco de un comentario borrado que conserva respuestas.
	Deleted bool `json:"deleted,omitempty"`
	// Can dice lo que quien pregunta puede hacer con él.
	Can PuedeComentario `json:"can"`
}

// PuedeComentario son las acciones de quien pregunta sobre un comentario.
type PuedeComentario struct {
	Delete bool `json:"delete"`
}

// Mensaje es un mensaje del chat del directo.
type Mensaje struct {
	ID        uuid.UUID `json:"id"`
	Author    Autor     `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Servicio son los comentarios y el chat.
type Servicio struct {
	store  *store
	acceso Acceso
	hub    *Hub
	// ritmo acota los mensajes de chat por persona (tenant:sub) en esta réplica.
	ritmo *httpx.TokenBucket
	// ritmoDenuncias acota las denuncias por persona (o por IP troceada).
	ritmoDenuncias *httpx.TokenBucket
	// mod son los avisos y la clave de las denuncias (ConModeracion).
	mod Moderacion
}

// New construye el servicio sobre el pool y el resolutor de acceso.
func New(pool *pg.Pool, acceso Acceso) *Servicio {
	return &Servicio{
		store:  &store{pool: pool},
		acceso: acceso,
		hub:    NewHub(),
		// Un mensaje por segundo sostenido, cinco de golpe: lo que teclea una
		// persona, no un bucle.
		ritmo:          httpx.NewTokenBucket(1, 5),
		ritmoDenuncias: ritmoDeDenuncias(),
	}
}

// Hub es el difusor del chat, para que la capa HTTP cuelgue de él las conexiones.
func (s *Servicio) Hub() *Hub { return s.hub }

// autorDe convierte a quien pregunta en autor.
func autorDe(q Quien) Autor {
	return Autor{Sub: q.Sub, Name: q.Nombre, Username: q.Usuario}
}

// cuerpoValido recorta y comprueba el cuerpo: ni vacío ni más largo que el tope
// (en runas, como `length()` en Postgres).
func cuerpoValido(body string, tope int) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("%w: cuerpo vacío", ErrEntradaInvalida)
	}
	if n := utf8.RuneCountInString(body); n > tope {
		return "", fmt.Errorf("%w: %d caracteres, el tope es %d", ErrEntradaInvalida, n, tope)
	}
	return body, nil
}

// resolver aplica el acceso y exige poder ver el vídeo.
func (s *Servicio) resolver(ctx context.Context, quien Quien, videoID uuid.UUID) (Video, Permisos, error) {
	v, p, err := s.acceso(ctx, quien, videoID)
	if err != nil {
		return Video{}, Permisos{}, err
	}
	if !p.Ver {
		return Video{}, Permisos{}, ErrNoExiste
	}
	return v, p, nil
}

// ── Comentarios ─────────────────────────────────────────────────────────────

// Comentarios lista los comentarios del vídeo, más antiguos primero, desde el
// cursor. Sin sesión se leen por la política pública (solo vídeos public o
// unlisted); con sesión, en la organización del vídeo y sin los de las personas
// que quien pregunta tiene bloqueadas.
func (s *Servicio) Comentarios(ctx context.Context, quien Quien, videoID uuid.UUID, cursor string, limite int) ([]Comentario, string, error) {
	v, p, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return nil, "", err
	}
	desde, err := descifrarCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	limite = acotar(limite, 50, 200)
	var filas []Comentario
	if quien.Anonimo {
		filas, err = s.store.comentariosPublicos(ctx, v.ID, desde, limite+1)
	} else {
		filas, err = s.store.comentarios(ctx, v, quien.Sub, desde, limite+1)
	}
	if err != nil {
		return nil, "", err
	}
	siguiente := ""
	if len(filas) > limite {
		filas = filas[:limite]
		ultimo := filas[len(filas)-1]
		siguiente = cifrarCursor(cursorDe{ultimo.CreatedAt, ultimo.ID})
	}
	for i := range filas {
		filas[i].Can.Delete = !filas[i].Deleted && puedeBorrar(quien, p, filas[i].Author.Sub)
	}
	return filas, siguiente, nil
}

// Comentar publica un comentario (o una respuesta) en el vídeo.
func (s *Servicio) Comentar(ctx context.Context, quien Quien, videoID uuid.UUID, body string, replyTo *uuid.UUID) (Comentario, error) {
	if quien.Anonimo || quien.Sub == "" {
		return Comentario{}, ErrProhibido
	}
	v, p, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return Comentario{}, err
	}
	if !v.ComentariosActivos {
		return Comentario{}, ErrDesactivado
	}
	body, err = cuerpoValido(body, MaxComentario)
	if err != nil {
		return Comentario{}, err
	}
	c, err := s.store.insertarComentario(ctx, v, autorDe(quien), body, replyTo)
	if err != nil {
		return Comentario{}, err
	}
	c.Can.Delete = puedeBorrar(quien, p, c.Author.Sub)
	return c, nil
}

// BorrarComentario retira un comentario: el autor, o quien modera el vídeo.
func (s *Servicio) BorrarComentario(ctx context.Context, quien Quien, videoID, comentarioID uuid.UUID) error {
	if quien.Anonimo || quien.Sub == "" {
		return ErrProhibido
	}
	v, p, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return err
	}
	return s.store.borrarComentario(ctx, v, comentarioID, quien.Sub, p.Moderar)
}

// ── Chat ────────────────────────────────────────────────────────────────────

// HistorialChat devuelve los mensajes posteriores a `desde` (o los últimos 100
// si es cero), más antiguos primero, sin los de las personas que quien pregunta
// tiene bloqueadas. Siempre con sesión.
func (s *Servicio) HistorialChat(ctx context.Context, quien Quien, videoID uuid.UUID, desde time.Time) ([]Mensaje, error) {
	if quien.Anonimo {
		return nil, ErrProhibido
	}
	v, _, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return nil, err
	}
	return s.store.historialChat(ctx, v, quien.Sub, desde, 100)
}

// EnviarChat guarda un mensaje y lo difunde a quienes tienen el chat abierto.
func (s *Servicio) EnviarChat(ctx context.Context, quien Quien, videoID uuid.UUID, body string) (Mensaje, error) {
	if quien.Anonimo || quien.Sub == "" {
		return Mensaje{}, ErrProhibido
	}
	v, _, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return Mensaje{}, err
	}
	return s.enviarChat(ctx, quien, v, body)
}

// enviarChat es EnviarChat con el vídeo ya resuelto (la conexión WebSocket lo
// resolvió al abrirse y no vuelve a preguntar por cada mensaje).
func (s *Servicio) enviarChat(ctx context.Context, quien Quien, v Video, body string) (Mensaje, error) {
	if !v.ChatActivo {
		return Mensaje{}, ErrDesactivado
	}
	body, err := cuerpoValido(body, MaxMensaje)
	if err != nil {
		return Mensaje{}, err
	}
	if res, err := s.ritmo.Allow(ctx, quien.Tenant+":"+quien.Sub); err != nil || !res.Allowed {
		return Mensaje{}, ErrDemasiadoRapido
	}
	m, err := s.store.insertarChat(ctx, v, autorDe(quien), body)
	if err != nil {
		return Mensaje{}, err
	}
	s.hub.Difundir(v.Tenant, v.ID, tramaMensaje(m))
	return m, nil
}

// BorrarChat retira un mensaje del chat (autor o moderación) y avisa a la sala.
func (s *Servicio) BorrarChat(ctx context.Context, quien Quien, videoID, mensajeID uuid.UUID) error {
	if quien.Anonimo || quien.Sub == "" {
		return ErrProhibido
	}
	v, p, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return err
	}
	if err := s.store.borrarChat(ctx, v, mensajeID, quien.Sub, p.Moderar); err != nil {
		return err
	}
	s.hub.Difundir(v.Tenant, v.ID, Trama{Type: TramaBorrado, ID: &mensajeID})
	return nil
}

// puedeBorrar: el autor, o quien modera el vídeo (dueño, admin u owner).
func puedeBorrar(quien Quien, p Permisos, autorSub string) bool {
	if quien.Anonimo || quien.Sub == "" {
		return false
	}
	return p.Moderar || quien.Sub == autorSub
}

// acotar deja el límite entre 1 y `max`, con `porDefecto` cuando no viene.
func acotar(n, porDefecto, maximo int) int {
	if n <= 0 {
		return porDefecto
	}
	return min(n, maximo)
}

// ComentariosDe son los comentarios que escribió quien pregunta, en cualquier
// vídeo y organización, para «Exportar tus datos».
func (s *Servicio) ComentariosDe(ctx context.Context, quien Quien) ([]ComentarioExportado, error) {
	if quien.Anonimo || quien.Sub == "" {
		return nil, ErrProhibido
	}
	return s.store.comentariosDe(ctx, quien.Sub)
}
