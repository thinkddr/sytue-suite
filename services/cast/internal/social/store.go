// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// store son las consultas de comentarios y chat. Cada función es UNA transacción
// con `app.tenant` fijado al tenant del vídeo (o `app.cast_publico` para la
// lectura sin sesión): aquí no se lee ni escribe nada sin decidir antes en qué
// organización.
type store struct {
	pool *pg.Pool
}

// enTenant abre una transacción en la organización del vídeo.
func (s *store) enTenant(ctx context.Context, v Video, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, v.Tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// cursorDe es la posición de paginación: (created_at, id) del último devuelto.
type cursorDe struct {
	fecha time.Time
	id    uuid.UUID
}

func cifrarCursor(c cursorDe) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.fecha.UTC().Format(time.RFC3339Nano) + "|" + c.id.String()))
}

// descifrarCursor acepta el cursor vacío (desde el principio).
func descifrarCursor(s string) (*cursorDe, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // sin cursor = desde el principio; el llamante lo comprueba.
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor ilegible", ErrEntradaInvalida)
	}
	fecha, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return nil, fmt.Errorf("%w: cursor incompleto", ErrEntradaInvalida)
	}
	t, err := time.Parse(time.RFC3339Nano, fecha)
	if err != nil {
		return nil, fmt.Errorf("%w: fecha del cursor ilegible", ErrEntradaInvalida)
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: id del cursor ilegible", ErrEntradaInvalida)
	}
	return &cursorDe{fecha: t, id: u}, nil
}

// ── Comentarios ─────────────────────────────────────────────────────────────

// Un comentario borrado se devuelve como hueco (`deleted`) SOLO si conserva
// respuestas vivas; sin ellas, desaparece. Así el hilo no pierde el sentido y
// tampoco se enseñan huecos sueltos.
const (
	comentariosBase = `
SELECT c.id, c.author_sub, c.author_name, c.author_username, c.body, c.reply_to, c.created_at,
       c.deleted_at IS NOT NULL
  FROM comments c
 WHERE c.video_id = $1
   AND (c.deleted_at IS NULL
        OR EXISTS (SELECT 1 FROM comments r WHERE r.reply_to = c.id AND r.deleted_at IS NULL))
   AND ($2::timestamptz IS NULL OR (c.created_at, c.id) > ($2::timestamptz, $3::uuid))`
	comentariosOrden = `
 ORDER BY c.created_at, c.id
 LIMIT $4`

	// sqlComentarios es la lectura con sesión: en la organización del vídeo ($5) y
	// sin lo de las personas que quien pregunta ($6) tiene bloqueadas, ni las
	// respuestas a un comentario suyo, que se quedarían sin hilo.
	sqlComentarios = comentariosBase + `
   AND c.tenant = $5
   AND NOT EXISTS (SELECT 1 FROM person_blocks b
                    WHERE b.blocker_sub = $6
                      AND (b.blocked_sub = c.author_sub
                           OR b.blocked_sub = (SELECT r.author_sub FROM comments r WHERE r.id = c.reply_to)))` + comentariosOrden

	// sqlComentariosPublicos es la lectura sin sesión, cruzando organizaciones: solo
	// los de un vídeo que se puede ver sin sesión. Es la condición de la política
	// `comments_publico`, escrita también aquí: una conexión que se salte la RLS no
	// la evalúa, y esta consulta no lleva organización que la acote.
	sqlComentariosPublicos = comentariosBase + `
   AND EXISTS (SELECT 1 FROM videos v
                WHERE v.id = c.video_id AND v.deleted_at IS NULL
                  AND v.visibility IN ('public', 'unlisted') AND v.status IN ('ready', 'live'))` + comentariosOrden
)

func escanearComentarios(rows pgx.Rows) ([]Comentario, error) {
	defer rows.Close()
	var out []Comentario
	for rows.Next() {
		var c Comentario
		if err := rows.Scan(&c.ID, &c.Author.Sub, &c.Author.Name, &c.Author.Username, &c.Body, &c.ReplyTo, &c.CreatedAt, &c.Deleted); err != nil {
			return nil, fmt.Errorf("social: leer comentario: %w", err)
		}
		if c.Deleted {
			// El hueco no cuenta quién escribió ni qué.
			c.Body, c.Author = "", Autor{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func argumentosCursor(desde *cursorDe) (fecha *time.Time, id uuid.UUID) {
	if desde == nil {
		return nil, uuid.Nil
	}
	return &desde.fecha, desde.id
}

// comentarios lista en la organización del vídeo, para `quien`.
func (s *store) comentarios(ctx context.Context, v Video, quien string, desde *cursorDe, limite int) ([]Comentario, error) {
	var out []Comentario
	fecha, id := argumentosCursor(desde)
	err := s.enTenant(ctx, v, func(tx pgx.Tx) error {
		if err := comoPersona(ctx, tx, quien); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sqlComentarios, v.ID, fecha, id, limite, v.Tenant, quien)
		if err != nil {
			return fmt.Errorf("social: listar comentarios: %w", err)
		}
		out, err = escanearComentarios(rows)
		return err
	})
	return out, err
}

// comentariosPublicos lista SIN sesión. Es la ÚNICA función del paquete que fija
// `app.cast_publico`: con ella la política `comments_publico` deja ver los
// comentarios de un vídeo public o unlisted, y nada más.
func (s *store) comentariosPublicos(ctx context.Context, videoID uuid.UUID, desde *cursorDe, limite int) ([]Comentario, error) {
	var out []Comentario
	fecha, id := argumentosCursor(desde)
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_publico', 'on', true)"); err != nil {
			return fmt.Errorf("social: abrir la lectura pública: %w", err)
		}
		rows, err := tx.Query(ctx, sqlComentariosPublicos, videoID, fecha, id, limite)
		if err != nil {
			return fmt.Errorf("social: listar comentarios públicos: %w", err)
		}
		out, err = escanearComentarios(rows)
		return err
	})
	return out, err
}

// insertarComentario guarda el comentario. Una respuesta se cuelga siempre de un
// comentario RAÍZ del mismo vídeo: si `replyTo` es a su vez una respuesta, se
// aplana a su raíz (hilos de un nivel).
func (s *store) insertarComentario(ctx context.Context, v Video, autor Autor, body string, replyTo *uuid.UUID) (Comentario, error) {
	c := Comentario{Author: autor, Body: body}
	err := s.enTenant(ctx, v, func(tx pgx.Tx) error {
		if err := vetadoTx(ctx, tx, v, autor.Sub); err != nil {
			return err
		}
		if replyTo != nil {
			var raiz uuid.UUID
			err := tx.QueryRow(ctx, `
				SELECT coalesce(reply_to, id) FROM comments
				 WHERE id = $1 AND video_id = $2 AND tenant = $3 AND deleted_at IS NULL`, *replyTo, v.ID, v.Tenant).Scan(&raiz)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: el comentario al que se responde", ErrNoExiste)
			}
			if err != nil {
				return fmt.Errorf("social: resolver la raíz del hilo: %w", err)
			}
			c.ReplyTo = &raiz
		}
		return tx.QueryRow(ctx, `
			INSERT INTO comments (tenant, video_id, author_sub, author_name, author_username, body, reply_to)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at`,
			v.Tenant, v.ID, autor.Sub, autor.Name, autor.Username, body, c.ReplyTo,
		).Scan(&c.ID, &c.CreatedAt)
	})
	if err != nil {
		return Comentario{}, traducirPG(err, "social: guardar comentario")
	}
	return c, nil
}

// borrarConPermiso hace el borrado blando en UNA sentencia y distingue «no existe»
// de «no es tuyo» sin segunda consulta: cuenta el objetivo y lo borrado.
const sqlBorrar = `
WITH objetivo AS (
    SELECT 1 FROM %[1]s WHERE id = $1 AND video_id = $2 AND tenant = $5 AND deleted_at IS NULL
), borrado AS (
    UPDATE %[1]s SET deleted_at = now()
     WHERE id = $1 AND video_id = $2 AND tenant = $5 AND deleted_at IS NULL AND ($3 OR author_sub = $4)
    RETURNING 1
)
SELECT (SELECT count(*) FROM objetivo), (SELECT count(*) FROM borrado)`

func (s *store) borrarConPermiso(ctx context.Context, tabla string, v Video, id uuid.UUID, sub string, modera bool) error {
	return s.enTenant(ctx, v, func(tx pgx.Tx) error {
		var habia, borrados int
		if err := tx.QueryRow(ctx, fmt.Sprintf(sqlBorrar, tabla), id, v.ID, modera, sub, v.Tenant).Scan(&habia, &borrados); err != nil {
			return fmt.Errorf("social: borrar en %s: %w", tabla, err)
		}
		switch {
		case habia == 0:
			return ErrNoExiste
		case borrados == 0:
			return ErrProhibido
		}
		return nil
	})
}

func (s *store) borrarComentario(ctx context.Context, v Video, id uuid.UUID, sub string, modera bool) error {
	return s.borrarConPermiso(ctx, "comments", v, id, sub, modera)
}

// ── Chat ────────────────────────────────────────────────────────────────────

// historialChat devuelve los últimos `limite` mensajes vivos posteriores a
// `desde` (todo el historial reciente si es cero), más antiguos primero.
func (s *store) historialChat(ctx context.Context, v Video, quien string, desde time.Time, limite int) ([]Mensaje, error) {
	var out []Mensaje
	err := s.enTenant(ctx, v, func(tx pgx.Tx) error {
		if err := comoPersona(ctx, tx, quien); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, author_sub, author_name, body, created_at
			  FROM (SELECT id, author_sub, author_name, body, created_at
			          FROM chat_messages
			         WHERE video_id = $1 AND tenant = $4 AND deleted_at IS NULL AND created_at > $2
			           AND NOT EXISTS (SELECT 1 FROM person_blocks b
			                            WHERE b.blocker_sub = $5 AND b.blocked_sub = chat_messages.author_sub)
			         ORDER BY created_at DESC, id DESC
			         LIMIT $3) ultimos
			 ORDER BY created_at, id`, v.ID, desde, limite, v.Tenant, quien)
		if err != nil {
			return fmt.Errorf("social: historial del chat: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m Mensaje
			if err := rows.Scan(&m.ID, &m.Author.Sub, &m.Author.Name, &m.Body, &m.CreatedAt); err != nil {
				return fmt.Errorf("social: leer mensaje: %w", err)
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

func (s *store) insertarChat(ctx context.Context, v Video, autor Autor, body string) (Mensaje, error) {
	m := Mensaje{Author: autor, Body: body}
	err := s.enTenant(ctx, v, func(tx pgx.Tx) error {
		if err := vetadoTx(ctx, tx, v, autor.Sub); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO chat_messages (tenant, video_id, author_sub, author_name, body)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at`, v.Tenant, v.ID, autor.Sub, autor.Name, body,
		).Scan(&m.ID, &m.CreatedAt)
	})
	if err != nil {
		return Mensaje{}, traducirPG(err, "social: guardar mensaje")
	}
	return m, nil
}

func (s *store) borrarChat(ctx context.Context, v Video, id uuid.UUID, sub string, modera bool) error {
	return s.borrarConPermiso(ctx, "chat_messages", v, id, sub, modera)
}

// traducirPG convierte las violaciones de CHECK y de clave ajena en errores del
// servicio: un cuerpo que se cuela por encima del tope o un vídeo que dejó de
// existir entre resolver el acceso y escribir.
func traducirPG(err error, contexto string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", ErrEntradaInvalida, pgErr.ConstraintName)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", ErrNoExiste, pgErr.ConstraintName)
		}
	}
	if errors.Is(err, ErrNoExiste) || errors.Is(err, ErrProhibido) || errors.Is(err, ErrEntradaInvalida) || errors.Is(err, ErrBloqueado) {
		return err
	}
	return fmt.Errorf("%s: %w", contexto, err)
}

// ComentarioExportado es un comentario de quien exporta sus datos.
type ComentarioExportado struct {
	ID        uuid.UUID  `json:"id"`
	VideoID   uuid.UUID  `json:"video_id"`
	Body      string     `json:"body"`
	ReplyTo   *uuid.UUID `json:"reply_to,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Deleted   bool       `json:"deleted"`
}

// comentariosDe lee todos los comentarios de un autor, de cualquier organización,
// por la política `comments_autor` (`app.cast_persona` = su sub). Es la ÚNICA
// función de lo social que fija esa variable.
func (s *store) comentariosDe(ctx context.Context, sub string) ([]ComentarioExportado, error) {
	var out []ComentarioExportado
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_persona', $1, true)", sub); err != nil {
			return fmt.Errorf("social: set_config app.cast_persona: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT id, video_id, body, reply_to, created_at, deleted_at IS NOT NULL
			  FROM comments WHERE author_sub = $1 ORDER BY created_at, id`, sub)
		if err != nil {
			return fmt.Errorf("social: comentarios del autor: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c ComentarioExportado
			if err := rows.Scan(&c.ID, &c.VideoID, &c.Body, &c.ReplyTo, &c.CreatedAt, &c.Deleted); err != nil {
				return fmt.Errorf("social: leer comentario del autor: %w", err)
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}
