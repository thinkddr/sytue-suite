// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const messageCols = `id, channel_id, parent_id, author_sub, author_name, body, mentions, attachments, reply_count, edited_at, created_at, type, priority, requires_ack, mentions_all, card, matrix_room_id, matrix_event_id`

// scanMessage escanea las columnas de messageCols en ese orden. `extra` recibe los
// destinos de las columnas que la consulta añade DESPUÉS de messageCols (cursores,
// contadores): único punto de escaneo de mensajes, para que ampliar messageCols no
// deje scanners duplicados desincronizados.
func scanMessage(row pgx.Row, t string, extra ...any) (domain.Message, error) {
	var m domain.Message
	var attachments, card []byte
	var matrixRoom, matrixEvent string
	dest := append([]any{
		&m.ID, &m.ChannelID, &m.ParentID, &m.AuthorSub, &m.AuthorName,
		&m.Body, &m.Mentions, &attachments, &m.ReplyCount, &m.EditedAt, &m.CreatedAt, &m.Type,
		&m.Priority, &m.RequiresAck, &m.MentionsAll, &card, &matrixRoom, &matrixEvent,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Message{}, err
	}
	if len(attachments) > 0 {
		if err := json.Unmarshal(attachments, &m.Attachments); err != nil {
			return domain.Message{}, err
		}
	}
	// card es '{}' en la inmensa mayoría de mensajes: solo se materializa el puntero
	// cuando de verdad hay tarjeta, para no colgar un objeto vacío de cada mensaje.
	if len(card) > 0 {
		var c domain.Card
		if err := json.Unmarshal(card, &c); err != nil {
			return domain.Message{}, err
		}
		if c.HasCard() {
			m.Card = &c
		}
	}
	m.Tenant = t
	m.Encryption = domain.LegacyEncryption
	if matrixEvent != "" {
		m.MatrixEvent = &domain.MatrixEventRef{RoomID: matrixRoom, EventID: matrixEvent}
		m.Encryption = domain.MatrixEncryption
	}
	// Badge «BOT» (Ola 15): derivado del prefijo del autor sintético, sin columna.
	m.IsBot = domain.IsBotAuthor(m.AuthorSub)
	return m, nil
}

// getMessageTx lee un mensaje vivo completo por ID dentro de una transacción.
func getMessageTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (domain.Message, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+messageCols+` FROM messages WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, tenant, id)
	m, err := scanMessage(row, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, domain.ErrNotFound
	}
	return m, err
}

// NewMessageRecord son los datos para publicar un mensaje.
type NewMessageRecord struct {
	ChannelID      uuid.UUID
	ParentID       *uuid.UUID
	AuthorSub      string
	AuthorName     string
	Body           string
	MatrixEvent    *domain.MatrixEventRef
	ValidateMatrix func(context.Context, MatrixChannel) error
	// Type es el tipo de mensaje ("user" o "call"). Vacío se persiste como "user".
	Type     string
	Mentions []string
	// MentionsAll marca que el mensaje menciona a todo el canal (@channel/@all/
	// @here). El contador de menciones lo cuenta por consulta indexada, sin una
	// fila de mención por miembro (sin fan-out N). Ver UnreadByChannel.
	MentionsAll bool
	Attachments []domain.Attachment
	// Card es la tarjeta del mensaje (encuesta/tarea); nil = mensaje normal. Llega
	// YA normalizada por el servicio (domain.NormalizeCard).
	Card *domain.Card
	// Priority es la prioridad ("normal"|"important"|"urgent"); vacío = "normal".
	Priority string
	// RequiresAck pide acuse de recibo explícito a los lectores (Ola 12).
	RequiresAck bool
}

// CreateMessage publica un mensaje en un canal (o respuesta en hilo), validando
// acceso de escritura, actualizando reply_count del padre y last_message_at del
// canal, todo en una transacción.
//
//nolint:gocognit // message publishing transaction coordinates moderation, threads, mentions, attachments and last_message_at
func (s *Store) CreateMessage(ctx context.Context, rec NewMessageRecord) (domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.Message
	// El bloqueo se mira ANTES y en su propia transacción: es de la persona y cruza
	// organizaciones, así que solo se ve con la política de mantenimiento, que la
	// transacción de negocio de abajo no debe llevar (ver moderacion.go).
	if err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, tenant string) error {
		return bloqueoEnDirectoTx(ctx, tx, tenant, rec.ChannelID, rec.AuthorSub)
	}); err != nil {
		return domain.Message{}, err
	}
	runTx := s.pool.InTenantTx
	if rec.MatrixEvent != nil {
		runTx = func(ctx context.Context, fn func(pgx.Tx) error) error {
			return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error { return fn(tx) })
		}
	}
	err := runTx(ctx, func(tx pgx.Tx) error {
		var err error
		if rec.MatrixEvent != nil {
			if rec.ValidateMatrix == nil {
				return domain.ErrEncryptedRequired
			}
			if err := lockMatrixChannelTx(ctx, tx, t.Slug, rec.ChannelID); err != nil {
				return err
			}
			state, err := matrixChannelTx(ctx, tx, t.Slug, rec.ChannelID)
			if err != nil {
				return err
			}
			if err := requireWriteAccess(ctx, tx, t.Slug, state.Channel, rec.AuthorSub, rec.AuthorName); err != nil {
				return err
			}
			if state.RoomID == "" || state.RoomID != rec.MatrixEvent.RoomID {
				return domain.ErrForbidden
			}
			if err := rec.ValidateMatrix(matrixContext(ctx, tx), state); err != nil {
				return err
			}
			previous, lookupErr := scanMessage(tx.QueryRow(ctx, `SELECT `+messageCols+` FROM messages WHERE tenant=$1 AND matrix_room_id=$2 AND matrix_event_id=$3 AND deleted_at IS NULL`, t.Slug, rec.MatrixEvent.RoomID, rec.MatrixEvent.EventID), t.Slug)
			if lookupErr == nil {
				sameParent := previous.ParentID == nil && rec.ParentID == nil || previous.ParentID != nil && rec.ParentID != nil && *previous.ParentID == *rec.ParentID
				if previous.AuthorSub != rec.AuthorSub || previous.ChannelID != rec.ChannelID || !sameParent {
					return domain.ErrConflict
				}
				m = previous
				return nil
			}
			if !errors.Is(lookupErr, pgx.ErrNoRows) {
				return lookupErr
			}
		}
		m, err = createMessageTx(ctx, tx, t.Slug, rec)
		return err
	})
	return m, err
}

// createMessageTx inserta un mensaje dentro de una transacción YA acotada al
// tenant (app.tenant fijado). Lo comparten CreateMessage (camino de usuario) y el
// worker de programados (que fija app.tenant por fila dentro de su tx de
// mantenimiento): así el envío programado reusa EXACTAMENTE la validación de
// escritura, la validación de hilo, reply_count/last_message_at, el read_state del
// autor y el auto-follow — sin duplicar el flujo.
//
//nolint:gocognit // transacción con validación de canal/hilo/adjuntos; trocearla ocultaría el flujo atómico.
func createMessageTx(ctx context.Context, tx pgx.Tx, tenant string, rec NewMessageRecord) (domain.Message, error) {
	var m domain.Message
	ch, err := getChannelTx(ctx, tx, tenant, rec.ChannelID)
	if err != nil {
		return domain.Message{}, err
	}
	if err := requireWriteAccess(ctx, tx, tenant, ch, rec.AuthorSub, rec.AuthorName); err != nil {
		return domain.Message{}, err
	}

	// Validar el padre (debe existir, vivo, en el MISMO canal y ser raíz). Su
	// autor y fecha alimentan el auto-follow del hilo tras insertar la respuesta.
	var parentAuthor string
	var parentCreatedAt time.Time
	if rec.ParentID != nil {
		var parentChannel uuid.UUID
		var parentParent *uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT channel_id, parent_id, author_sub, created_at FROM messages
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			tenant, *rec.ParentID).Scan(&parentChannel, &parentParent, &parentAuthor, &parentCreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Message{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Message{}, err
		}
		if parentChannel != rec.ChannelID || parentParent != nil {
			return domain.Message{}, domain.ErrInvalidInput
		}
	}

	mentions := rec.Mentions
	if mentions == nil {
		mentions = []string{}
	}
	attachments, err := json.Marshal(orEmptyAttachments(rec.Attachments))
	if err != nil {
		return domain.Message{}, err
	}
	msgType := rec.Type
	if msgType == "" {
		msgType = domain.MessageTypeUser
	}
	priority := rec.Priority
	if priority == "" {
		priority = domain.PriorityNormal
	}
	card, err := cardJSON(rec.Card)
	if err != nil {
		return domain.Message{}, err
	}

	matrixRoom, matrixEvent := "", ""
	if rec.MatrixEvent != nil {
		matrixRoom, matrixEvent = rec.MatrixEvent.RoomID, rec.MatrixEvent.EventID
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO messages (tenant, channel_id, parent_id, author_sub, author_name, body, mentions, mentions_all, attachments, type, priority, requires_ack, card, matrix_room_id, matrix_event_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING `+messageCols,
		tenant, rec.ChannelID, rec.ParentID, rec.AuthorSub, rec.AuthorName,
		rec.Body, mentions, rec.MentionsAll, attachments, msgType, priority, rec.RequiresAck, card, matrixRoom, matrixEvent)
	m, err = scanMessage(row, tenant)
	if err != nil {
		return domain.Message{}, err
	}

	if rec.ParentID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE messages SET reply_count = reply_count + 1 WHERE tenant=$1 AND id=$2`,
			tenant, *rec.ParentID); err != nil {
			return domain.Message{}, err
		}
		// Auto-follow del hilo: el que responde y el autor de la raíz (Ola 9).
		if err := autoFollowOnReply(ctx, tx, tenant, *rec.ParentID, parentCreatedAt, rec.AuthorSub, parentAuthor); err != nil {
			return domain.Message{}, err
		}
	}
	if err := actividadDelCanalTx(ctx, tx, tenant, ch); err != nil {
		return domain.Message{}, err
	}
	// El autor ha leído su propio mensaje.
	if err := upsertReadState(ctx, tx, tenant, rec.ChannelID, rec.AuthorSub, m.ID); err != nil {
		return domain.Message{}, err
	}
	return m, nil
}

func orEmptyAttachments(a []domain.Attachment) []domain.Attachment {
	if a == nil {
		return []domain.Attachment{}
	}
	return a
}

// vaciadoAntesDe devuelve el instante hasta el que subject borró para sí el
// historial de un canal (cleared_before, mig 0058; solo lo fija HideDirect en
// directos), o nil si nunca lo borró o no es miembro. Las lecturas de mensajes
// muestran solo lo POSTERIOR.
func vaciadoAntesDe(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) (*time.Time, error) {
	var corte *time.Time
	err := tx.QueryRow(ctx,
		`SELECT cleared_before FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
		tenant, channelID, subject).Scan(&corte)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil // no es miembro: no hay corte
	}
	return corte, err
}

// actividadDelCanalTx anota un mensaje nuevo en el canal: refresca su actividad
// (orden de la barra lateral) y, si es un directo que alguien había borrado para
// sí (hidden_at), se lo devuelve a la barra. cleared_before se conserva, así que
// solo ve lo de después (mig 0058).
func actividadDelCanalTx(ctx context.Context, tx pgx.Tx, tenant string, ch domain.Channel) error {
	if _, err := tx.Exec(ctx,
		`UPDATE channels SET last_message_at=now() WHERE tenant=$1 AND id=$2`,
		tenant, ch.ID); err != nil {
		return err
	}
	if ch.Kind != domain.ChannelDirect {
		return nil
	}
	_, err := tx.Exec(ctx,
		`UPDATE channel_members SET hidden_at=NULL
		 WHERE tenant=$1 AND channel_id=$2 AND hidden_at IS NOT NULL`,
		tenant, ch.ID)
	return err
}

// filtroVaciado es la condición de las búsquedas que esconde lo que el llamante
// (subj, su placeholder) borró para sí en un directo: lo anterior a su
// cleared_before. Va correlacionada con `msg`.
func filtroVaciado(subj string) string {
	return ` AND NOT EXISTS (SELECT 1 FROM channel_members cb
	        WHERE cb.tenant=msg.tenant AND cb.channel_id=msg.channel_id AND cb.subject=` + subj + `
	          AND cb.cleared_before IS NOT NULL AND msg.created_at <= cb.cleared_before)`
}

// MessagePage son los parámetros de paginación keyset de un feed.
type MessagePage struct {
	// Before pagina hacia mensajes más antiguos que este instante (exclusivo).
	Before *time.Time
	Limit  int
}

// ListMessages devuelve la página de mensajes raíz de un canal (orden
// cronológico inverso por created_at), con paginación keyset. Adjunta las
// reacciones agregadas en una sola pasada (sin N+1).
func (s *Store) ListMessages(ctx context.Context, channelID uuid.UUID, subject string, page MessagePage) ([]domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	if page.Limit <= 0 || page.Limit > 200 {
		page.Limit = 50
	}
	before := time.Now().Add(time.Hour) // futuro: primera página
	if page.Before != nil {
		before = *page.Before
	}
	var msgs []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		corte, err := vaciadoAntesDe(ctx, tx, t.Slug, channelID, subject)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND channel_id=$2 AND parent_id IS NULL AND deleted_at IS NULL
			   AND created_at < $3
			   AND ($5::timestamptz IS NULL OR created_at > $5)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			t.Slug, channelID, before, page.Limit, corte)
		if err != nil {
			return err
		}
		msgs, err = collectMessages(rows, t.Slug)
		if err != nil {
			return err
		}
		if err := attachReactions(ctx, tx, t.Slug, msgs); err != nil {
			return err
		}
		if err := attachAcks(ctx, tx, t.Slug, subject, msgs); err != nil {
			return err
		}
		return attachCardVotes(ctx, tx, t.Slug, msgs)
	})
	return msgs, err
}

// MessagesAround devuelve una ventana de mensajes raíz CENTRADA en messageID: hasta
// limit/2 anteriores + el objetivo + hasta limit/2 posteriores, en orden cronológico
// ASCENDENTE (viejo→nuevo, como el feed ya pintado). Respalda el permalink / scroll-
// to-message (Ola 16): abrir un canal en un mensaje concreto sin cargar todo el
// historial. Valida acceso al canal y que el objetivo sea un mensaje raíz vivo del
// canal (ErrNotFound si no).
//
// ESCALA: fetch-around por KEYSET sobre (created_at, id) —el mismo orden e índice que
// ListMessages—; dos consultas acotadas por LIMIT, sin escanear ni contar filas. El
// objetivo cae en el bucket "posterior" (>=), así que no hay solape ni hueco.
// Devuelve además la RAÍZ del hilo cuando el mensaje pedido era una respuesta
// (uuid.Nil si ya era raíz): el feed se ancla en la raíz y quien llama abre el
// hilo para resaltar dentro el mensaje que se pidió de verdad.
// anclaDelFeed resuelve el `created_at` sobre el que anclar el keyset del feed, y
// la raíz del hilo si el mensaje pedido era una respuesta (uuid.Nil si no lo era).
//
// Si el objetivo es una RESPUESTA, el ancla es su raíz: el feed del canal solo
// pinta raíces, así que anclar en la respuesta no encuentra nada. Exigir
// `parent_id IS NULL` aquí devolvía 404 y el cliente pintaba «No se pudieron
// cargar los mensajes» para TODO el canal —un resultado de búsqueda dentro de un
// hilo dejaba el canal en blanco, y la búsqueda no filtra respuestas, así que
// pasaba constantemente—.
//
// Está fuera de MessagesAround porque esa función pasó de 30 de complejidad
// cognitiva y el CI la rechaza.
func anclaDelFeed(ctx context.Context, tx pgx.Tx, tenant string, channelID, messageID uuid.UUID) (time.Time, uuid.UUID, error) {
	var anchor time.Time
	var parent *uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT created_at, parent_id FROM messages
		 WHERE tenant=$1 AND id=$2 AND channel_id=$3 AND deleted_at IS NULL`,
		tenant, messageID, channelID).Scan(&anchor, &parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	if parent == nil {
		return anchor, uuid.Nil, nil
	}
	err = tx.QueryRow(ctx,
		`SELECT created_at FROM messages
		 WHERE tenant=$1 AND id=$2 AND channel_id=$3 AND deleted_at IS NULL`,
		tenant, *parent, channelID).Scan(&anchor)
	if errors.Is(err, pgx.ErrNoRows) {
		// Raíz borrada: la respuesta ya no es alcanzable por el feed.
		return time.Time{}, uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return anchor, *parent, nil
}

func (s *Store) MessagesAround(ctx context.Context, channelID uuid.UUID, subject string, messageID uuid.UUID, limit int) ([]domain.Message, uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	half := limit / 2
	if half < 1 {
		half = 1
	}
	var out []domain.Message
	var rootID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		anchor, raiz, err := anclaDelFeed(ctx, tx, t.Slug, channelID, messageID)
		if err != nil {
			return err
		}
		if raiz != uuid.Nil {
			// Sale a quien llama para que abra el hilo y resalte dentro el mensaje
			// pedido; y el keyset se ancla en la raíz, no en la respuesta.
			rootID = raiz
			messageID = raiz
		}
		corte, err := vaciadoAntesDe(ctx, tx, t.Slug, channelID, subject)
		if err != nil {
			return err
		}
		if corte != nil && !anchor.After(*corte) {
			return domain.ErrNotFound // lo borró para sí
		}
		// Anteriores estrictos por keyset (created_at, id) < (anchor, messageID), DESC.
		beforeRows, err := tx.Query(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND channel_id=$2 AND parent_id IS NULL AND deleted_at IS NULL
			   AND (created_at < $3 OR (created_at = $3 AND id < $4))
			   AND ($6::timestamptz IS NULL OR created_at > $6)
			 ORDER BY created_at DESC, id DESC LIMIT $5`,
			t.Slug, channelID, anchor, messageID, half, corte)
		if err != nil {
			return err
		}
		before, err := collectMessages(beforeRows, t.Slug)
		if err != nil {
			return err
		}
		// Los anteriores salen DESC; se invierten a ASC para el orden final.
		for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
			before[i], before[j] = before[j], before[i]
		}
		// Objetivo + posteriores por keyset (created_at, id) >= (anchor, messageID), ASC.
		afterRows, err := tx.Query(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND channel_id=$2 AND parent_id IS NULL AND deleted_at IS NULL
			   AND (created_at > $3 OR (created_at = $3 AND id >= $4))
			 ORDER BY created_at ASC, id ASC LIMIT $5`,
			t.Slug, channelID, anchor, messageID, half+1)
		if err != nil {
			return err
		}
		after, err := collectMessages(afterRows, t.Slug)
		if err != nil {
			return err
		}
		out = append(before, after...) //nolint:gocritic // appendAssign: 'before' no se reutiliza tras esto
		if err := attachReactions(ctx, tx, t.Slug, out); err != nil {
			return err
		}
		if err := attachAcks(ctx, tx, t.Slug, subject, out); err != nil {
			return err
		}
		return attachCardVotes(ctx, tx, t.Slug, out)
	})
	return out, rootID, err
}

// ListThread devuelve el mensaje raíz y sus respuestas en orden cronológico.
func (s *Store) ListThread(ctx context.Context, rootID uuid.UUID, subject string) (domain.Message, []domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var root domain.Message
	var replies []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND id=$2 AND parent_id IS NULL AND deleted_at IS NULL`,
			t.Slug, rootID)
		var err error
		root, err = scanMessage(row, t.Slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, root.ChannelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, subject); err != nil {
			return err
		}
		corte, err := vaciadoAntesDe(ctx, tx, t.Slug, root.ChannelID, subject)
		if err != nil {
			return err
		}
		if corte != nil && !root.CreatedAt.After(*corte) {
			return domain.ErrNotFound // lo borró para sí
		}
		rows, err := tx.Query(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND parent_id=$2 AND deleted_at IS NULL
			   AND ($3::timestamptz IS NULL OR created_at > $3)
			 ORDER BY created_at ASC, id ASC`, t.Slug, rootID, corte)
		if err != nil {
			return err
		}
		replies, err = collectMessages(rows, t.Slug)
		if err != nil {
			return err
		}
		all := append([]domain.Message{root}, replies...)
		if err := attachReactions(ctx, tx, t.Slug, all); err != nil {
			return err
		}
		if err := attachAcks(ctx, tx, t.Slug, subject, all); err != nil {
			return err
		}
		if err := attachCardVotes(ctx, tx, t.Slug, all); err != nil {
			return err
		}
		root = all[0]
		replies = all[1:]
		// Abrir el hilo lo marca como visto para el usuario si lo sigue (Ola 9):
		// pone a cero su contador de respuestas no leídas. No-op si no lo sigue.
		return markThreadViewed(ctx, tx, t.Slug, rootID, subject)
	})
	return root, replies, err
}

// ListOwnMessages devuelve, por páginas y en orden cronológico, los mensajes que
// ESCRIBIÓ subject en cualquier canal, respuestas de hilo incluidas: lo suyo para
// «Exportar tus datos». Lo que escribieron los demás es de ellos. La página
// siguiente empieza después de (afterAt, afterID), el último de la anterior.
//
// ponytail: sin índice por autor, cada página recorre los mensajes del tenant;
// añadir (tenant, author_sub, created_at, id) cuando el volumen lo pida.
func (s *Store) ListOwnMessages(ctx context.Context, subject string, afterAt time.Time, afterID uuid.UUID, limit int) ([]domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var msgs []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+messageCols+` FROM messages
			 WHERE tenant=$1 AND author_sub=$2 AND deleted_at IS NULL
			   AND (created_at, id) > ($3, $4)
			 ORDER BY created_at, id LIMIT $5`,
			t.Slug, subject, afterAt, afterID, limit)
		if err != nil {
			return err
		}
		msgs, err = collectMessages(rows, t.Slug)
		return err
	})
	return msgs, err
}

// collectMessages drena un cursor de mensajes.
func collectMessages(rows pgx.Rows, tenant string) ([]domain.Message, error) {
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		m, err := scanMessage(rows, tenant)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EditMessage cambia el cuerpo de un mensaje (solo su autor).
func (s *Store) EditMessage(ctx context.Context, id uuid.UUID, actor, body string) (domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE messages SET body=$3, edited_at=now()
			 WHERE tenant=$1 AND id=$2 AND author_sub=$4 AND deleted_at IS NULL
			 RETURNING `+messageCols,
			t.Slug, id, body, actor)
		var err error
		m, err = scanMessage(row, t.Slug)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// O no existe, o el actor no es el autor.
		return domain.Message{}, domain.ErrForbidden
	}
	return m, err
}

// DeleteMessage marca un mensaje como borrado (autor o admin del canal) y, si es
// una respuesta, decrementa el reply_count del padre.
func (s *Store) DeleteMessage(ctx context.Context, id uuid.UUID, actor string) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var author string
		err := tx.QueryRow(ctx,
			`SELECT channel_id, author_sub FROM messages
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id).Scan(&channelID, &author)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if author != actor {
			ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
			if err != nil {
				return err
			}
			if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
				return err
			}
		}
		if err := s.redactMatrixMessageTx(ctx, tx, t.Slug, id); err != nil {
			return err
		}
		return borrarMensajeTx(ctx, tx, t.Slug, id)
	})
	return channelID, err
}

// borrarMensajeTx marca un mensaje vivo como borrado y descuenta la respuesta de
// su hilo. NO autoriza: quien llama ya ha decidido que el actor puede (el autor o
// un admin del canal en DeleteMessage; quien modera la denuncia en ResolveReport).
// ErrNotFound si no existe o ya estaba borrado.
func borrarMensajeTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) error {
	var parentID *uuid.UUID
	err := tx.QueryRow(ctx,
		`UPDATE messages SET deleted_at=now()
		 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL RETURNING parent_id`,
		tenant, id).Scan(&parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil || parentID == nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE messages SET reply_count = GREATEST(0, reply_count - 1)
		 WHERE tenant=$1 AND id=$2`, tenant, *parentID)
	return err
}

// ChannelOfMessage devuelve el canal de un mensaje vivo (para fan-out).
func (s *Store) ChannelOfMessage(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var channelID uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT channel_id FROM messages WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id).Scan(&channelID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	return channelID, err
}

// sqlArgs acumula los argumentos posicionales de una consulta y entrega el
// placeholder ($N) de cada uno, para ensamblar filtros opcionales sin numerar los
// $ a mano. Uso interno de las búsquedas con operadores.
type sqlArgs struct{ vals []any }

// add registra un valor y devuelve su placeholder ($N, 1-indexado).
func (a *sqlArgs) add(v any) string {
	a.vals = append(a.vals, v)
	return "$" + strconv.Itoa(len(a.vals))
}

// likePattern envuelve un valor del usuario en un patrón de subcadena para ILIKE
// ESCAPANDO los comodines (% y _) y el propio carácter de escape (\, el que usa
// PostgreSQL por defecto). Sin esto, buscar «50%» dispara un comodín silencioso y
// buscar «%» devuelve TODO el historial accesible. Un único Replacer de una pasada:
// la barra que se inserta al escapar no se vuelve a escapar.
func likePattern(v string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v) + "%"
}

// searchFilters traduce una SearchQuery (texto + operadores) a condiciones SQL extra
// —cada una un AND que solo ACOTA el conjunto— y registra sus args en qb; subj es el
// placeholder del llamante, que is:saved necesita. La misma forma la comparten
// SearchAll y SearchMessages. El filtro de ACCESO y el LIMIT de la consulta base
// siguen mandando (nada de lo que se añada aquí amplía lo visible), así que sigue
// siendo UNA sola consulta indexada, sin N+1.
//
// ponytail: el texto se resuelve con un ILIKE de subcadena POR TÉRMINO sobre el
// índice trigram de body (mig. 0002) en lugar de tsvector: AND indexado con cero
// migración. Techo conocido: sin stemming («corrió» no encuentra «correr»; sí casa
// «reunion» dentro de «reuniones», que es subcadena) y sin relevancia (el orden
// sigue siendo cronológico). Ranking o stemming ya obligarían a tsvector + su índice
// + elegir la configuración de idioma, y eso es otra ola.
//
// ponytail: from:/in: usan ILIKE de subcadena sin índice propio; a escala el peso
// lo llevan el filtro de acceso + el trigram de body + LIMIT, y estos operadores
// solo refinan hacia abajo. Si from:/in: se volvieran calientes en solitario,
// añadir índices trigram sobre author_name / channels.name.
func searchFilters(qb *sqlArgs, sq domain.SearchQuery, subj string) string {
	var b strings.Builder
	// Texto libre: TODOS los términos deben aparecer (AND), en cualquier orden y
	// posición. Un término con espacios (venía entrecomillado) casa la secuencia.
	for _, term := range sq.Terms {
		fmt.Fprintf(&b, " AND msg.body_plegado LIKE plegar(%s)", qb.add(likePattern(term)))
	}
	for _, term := range sq.Not {
		fmt.Fprintf(&b, " AND msg.body_plegado NOT LIKE plegar(%s)", qb.add(likePattern(term)))
	}
	if sq.From != "" {
		// Autor por nombre (subcadena) o por sub exacto: el patrón escapado para el
		// ILIKE y el valor CRUDO para la igualdad (un sub puede llevar «_»).
		fmt.Fprintf(&b, " AND (plegar(msg.author_name) LIKE plegar(%s) OR msg.author_sub = %s)",
			qb.add(likePattern(sq.From)), qb.add(sq.From))
	}
	if sq.In != "" {
		// Canal por slug o por nombre (subcadena).
		p := qb.add(likePattern(sq.In))
		fmt.Fprintf(&b, " AND (plegar(c.slug) LIKE plegar(%s) OR plegar(c.name) LIKE plegar(%s))", p, p)
	}
	if sq.After != nil {
		fmt.Fprintf(&b, " AND msg.created_at >= %s", qb.add(*sq.After))
	}
	if sq.Before != nil {
		fmt.Fprintf(&b, " AND msg.created_at < %s", qb.add(*sq.Before))
	}
	if sq.HasLink {
		// El prefiltro ILIKE '%http%' SÍ usa el trigram y la regex confirma el
		// esquema, así que «hablamos de http» no cuela como enlace. link_unfurls
		// (mig. 0015) no sirve aquí: es una caché por (tenant, url_hash) sin
		// message_id, no un índice mensaje→enlace.
		b.WriteString(` AND msg.body_plegado LIKE '%http%' AND msg.body ~* 'https?://'`)
	}
	if sq.HasFile {
		// attachments es NOT NULL DEFAULT '[]', así que basta comparar con el vacío.
		b.WriteString(` AND msg.attachments <> '[]'::jsonb`)
	}
	if sq.Saved {
		// Semi-join por la PK (tenant, subject, message_id) de message_saves: sin
		// escaneo y sin multiplicar filas. Los guardados son POR-USUARIO, así que
		// filtra por el LLAMANTE (nunca por el autor del mensaje).
		fmt.Fprintf(&b, ` AND EXISTS (SELECT 1 FROM message_saves ms
		        WHERE ms.tenant=msg.tenant AND ms.message_id=msg.id AND ms.subject=%s)`, subj)
	}
	if sq.Pinned {
		// Semi-join por la PK (tenant, message_id) de message_pins.
		b.WriteString(` AND EXISTS (SELECT 1 FROM message_pins mp
		        WHERE mp.tenant=msg.tenant AND mp.message_id=msg.id)`)
	}
	return b.String()
}

// SearchAll busca mensajes vivos en TODOS los canales accesibles al usuario
// (cualquiera de sus teams): miembro del canal (privados/DMs/públicos a los que se
// unió) o público de un team del que es miembro. Para la búsqueda federada del Hub y
// la búsqueda global de Rooms. Respeta el acceso: nunca devuelve mensajes de canales
// privados ajenos ni de públicos de teams a los que no perteneces. La query admite
// texto (AND de términos, frase entrecomillada, exclusión con «-») y los operadores
// from:/in:/before:/after:/on:/has:/is: (ver domain.ParseSearchQuery), que solo acotan.
func (s *Store) SearchAll(ctx context.Context, subject, query string, limit int) ([]domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	sq := domain.ParseSearchQuery(query)
	var qb sqlArgs
	tenant := qb.add(t.Slug)
	subj := qb.add(subject)
	filters := searchFilters(&qb, sq, subj)
	lim := qb.add(limit)
	sql := `SELECT ` + prefixed("msg", messageCols) + ` FROM messages msg
		 JOIN channels c ON c.tenant=msg.tenant AND c.id=msg.channel_id
		 WHERE msg.tenant=` + tenant + ` AND msg.deleted_at IS NULL AND c.deleted_at IS NULL` + filters + filtroVaciado(subj) + `
		   AND (
		        EXISTS (SELECT 1 FROM channel_members m
		                WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.subject=` + subj + `)
		        OR (c.kind='public' AND EXISTS (SELECT 1 FROM team_members tm
		                WHERE tm.tenant=c.tenant AND tm.team_id=c.team_id AND tm.subject=` + subj + `))
		   )
		 ORDER BY msg.created_at DESC LIMIT ` + lim
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, qb.vals...)
		if err != nil {
			return err
		}
		out, err = collectMessages(rows, t.Slug)
		return err
	})
	return out, err
}

// SearchMessages busca mensajes vivos en canales accesibles al usuario dentro de un
// team. Comparte con SearchAll el parser de operadores y el ensamblado de filtros.
func (s *Store) SearchMessages(ctx context.Context, teamID uuid.UUID, subject, query string, limit int) ([]domain.Message, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	sq := domain.ParseSearchQuery(query)
	var qb sqlArgs
	tenant := qb.add(t.Slug)
	team := qb.add(teamID)
	subj := qb.add(subject)
	filters := searchFilters(&qb, sq, subj)
	lim := qb.add(limit)
	sql := `SELECT ` + prefixed("msg", messageCols) + ` FROM messages msg
		 JOIN channels c ON c.tenant=msg.tenant AND c.id=msg.channel_id
		 WHERE msg.tenant=` + tenant + ` AND c.team_id=` + team + ` AND msg.deleted_at IS NULL
		   AND c.deleted_at IS NULL` + filters + filtroVaciado(subj) + `
		   AND (
		        c.kind='public'
		        OR EXISTS (SELECT 1 FROM channel_members m
		                   WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.subject=` + subj + `)
		   )
		 ORDER BY msg.created_at DESC LIMIT ` + lim
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, teamID, subject); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, qb.vals...)
		if err != nil {
			return err
		}
		out, err = collectMessages(rows, t.Slug)
		return err
	})
	return out, err
}
