// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// scheduledCols son las columnas de scheduled_messages en el orden de scanScheduled.
// file_ids se proyecta como text[] (cast) para escanear a []string y parsear a UUID
// sin depender del códec binario de uuid[] de pgx; en la escritura se pasa []string
// con cast $n::uuid[]. Simétrico y portable.
const scheduledCols = `id, channel_id, parent_id, author_sub, author_name, body, file_ids::text[], send_at, created_at`

func scanScheduled(row pgx.Row, tenant string) (domain.ScheduledMessage, error) {
	var sm domain.ScheduledMessage
	var fileIDs []string
	if err := row.Scan(&sm.ID, &sm.ChannelID, &sm.ParentID, &sm.AuthorSub, &sm.AuthorName,
		&sm.Body, &fileIDs, &sm.SendAt, &sm.CreatedAt); err != nil {
		return domain.ScheduledMessage{}, err
	}
	sm.FileIDs = parseUUIDList(fileIDs)
	sm.Tenant = tenant
	return sm, nil
}

// uuidStrings serializa los UUID a texto para pasarlos como text[] (con cast a
// uuid[] en SQL). parseUUIDList hace la inversa al leer, descartando basura.
func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func parseUUIDList(ss []string) []uuid.UUID {
	if len(ss) == 0 {
		return nil
	}
	out := make([]uuid.UUID, 0, len(ss))
	for _, s := range ss {
		if id, err := uuid.Parse(s); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// NewScheduledRecord son los datos para programar un mensaje.
type NewScheduledRecord struct {
	ChannelID  uuid.UUID
	ParentID   *uuid.UUID
	AuthorSub  string
	AuthorName string
	Body       string
	FileIDs    []uuid.UUID
	SendAt     time.Time
}

// CreateScheduled programa un mensaje para publicarse en SendAt. Valida acceso de
// ESCRITURA al canal en el momento de programar (feedback inmediato al usuario); el
// worker revalida al enviar. Devuelve la fila creada.
func (s *Store) CreateScheduled(ctx context.Context, rec NewScheduledRecord) (domain.ScheduledMessage, error) {
	t := tenancy.MustFrom(ctx)
	var sm domain.ScheduledMessage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, rec.ChannelID)
		if err != nil {
			return err
		}
		if err := requireWriteAccess(ctx, tx, t.Slug, ch, rec.AuthorSub, rec.AuthorName); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO scheduled_messages
			   (tenant, channel_id, parent_id, author_sub, author_name, body, file_ids, send_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7::uuid[], $8) RETURNING `+scheduledCols,
			t.Slug, rec.ChannelID, rec.ParentID, rec.AuthorSub, rec.AuthorName,
			rec.Body, uuidStrings(rec.FileIDs), rec.SendAt.UTC())
		sm, err = scanScheduled(row, t.Slug)
		return err
	})
	return sm, err
}

// ListScheduled lista los mensajes programados del usuario (los suyos), ordenados
// por SendAt ascendente (los más próximos primero). Opcionalmente acotados a un
// canal.
//
// ponytail: sin keyset — los programados pendientes de un usuario son un conjunto
// acotado por su propia redacción (como los fijados de un canal, ListPins); se capa
// a 200. Si crecieran, portar a keyset por (send_at, id).
func (s *Store) ListScheduled(ctx context.Context, subject string, channelID *uuid.UUID) ([]domain.ScheduledMessage, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.ScheduledMessage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+scheduledCols+` FROM scheduled_messages
			 WHERE tenant=$1 AND author_sub=$2 AND ($3::uuid IS NULL OR channel_id=$3)
			 ORDER BY send_at ASC, id ASC LIMIT 200`,
			t.Slug, subject, channelID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sm, err := scanScheduled(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, sm)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateScheduledRecord son los campos editables de un programado (nil = sin cambio).
type UpdateScheduledRecord struct {
	Body    *string
	SendAt  *time.Time
	FileIDs *[]uuid.UUID
}

// UpdateScheduled edita el cuerpo/adjuntos/instante de un programado del usuario
// (solo el autor). ErrNotFound si no existe o no es suyo. Devuelve la fila.
func (s *Store) UpdateScheduled(ctx context.Context, id uuid.UUID, subject string, upd UpdateScheduledRecord) (domain.ScheduledMessage, error) {
	t := tenancy.MustFrom(ctx)
	var sm domain.ScheduledMessage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var body *string
		var sendAt *time.Time
		if upd.Body != nil {
			body = upd.Body
		}
		if upd.SendAt != nil {
			u := upd.SendAt.UTC()
			sendAt = &u
		}
		var fileIDs *[]string
		if upd.FileIDs != nil {
			ss := uuidStrings(*upd.FileIDs)
			fileIDs = &ss
		}
		// COALESCE conserva el valor actual cuando el campo llega nil.
		row := tx.QueryRow(ctx,
			`UPDATE scheduled_messages SET
			   body     = COALESCE($3, body),
			   send_at  = COALESCE($4, send_at),
			   file_ids = COALESCE($5::uuid[], file_ids)
			 WHERE tenant=$1 AND id=$2 AND author_sub=$6 RETURNING `+scheduledCols,
			t.Slug, id, body, sendAt, fileIDs, subject)
		var err error
		sm, err = scanScheduled(row, t.Slug)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ScheduledMessage{}, domain.ErrNotFound
	}
	return sm, err
}

// CancelScheduled cancela (borra) un programado del usuario. ErrNotFound si no
// existe o no es suyo.
func (s *Store) CancelScheduled(ctx context.Context, id uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM scheduled_messages WHERE tenant=$1 AND id=$2 AND author_sub=$3`,
			t.Slug, id, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// dueScheduled es un programado vencido reclamado por el worker.
type dueScheduled struct {
	id     uuid.UUID
	tenant string
	rec    NewMessageRecord
}

// ClaimDueScheduled reclama hasta `limit` programados vencidos (send_at<=now), los
// publica reusando createMessageTx (misma validación de escritura/hilo, reply_count,
// read_state y auto-follow que un envío normal) + `publish` (evento WS al outbox), y
// borra sus filas — TODO en UNA transacción de mantenimiento (exactly-once: si el
// commit falla, ni se insertan mensajes ni se borran las filas). Cross-tenant vía
// `app.maintenance` (política PERMISSIVE), sin BYPASSRLS; app.tenant se fija por fila
// para que la inserción en `messages` pase su RLS por tenant. FOR UPDATE SKIP LOCKED
// permite varias réplicas del worker sin doble envío. Devuelve cuántas filas reclamó.
//
// ponytail: los adjuntos se envían como file_id «pelado» (sin denormalizar
// nombre/tipo/tamaño desde Drive), porque resolverlos exige I/O de red que no cabe
// dentro de la tx que sostiene los locks; el file_id basta para servir/resolver el
// adjunto en la lectura (mismo estado degradado que resolveAttachments con Drive
// caído). Las menciones @usuario no viajan (la tabla solo guarda el cuerpo);
// @channel/@here/@all sí, recomputadas de HasChannelMention.
func (s *Store) ClaimDueScheduled(ctx context.Context, now time.Time, limit int, publish func(context.Context, pg.Querier, domain.Message) error) (int, error) {
	var claimed int
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, tenant, channel_id, parent_id, author_sub, author_name, body, file_ids::text[]
			   FROM scheduled_messages
			  WHERE send_at <= $1
			  ORDER BY send_at
			  FOR UPDATE SKIP LOCKED
			  LIMIT $2`, now.UTC(), limit)
		if err != nil {
			return err
		}
		var due []dueScheduled
		for rows.Next() {
			var d dueScheduled
			var fileIDs []string
			if err := rows.Scan(&d.id, &d.tenant, &d.rec.ChannelID, &d.rec.ParentID,
				&d.rec.AuthorSub, &d.rec.AuthorName, &d.rec.Body, &fileIDs); err != nil {
				rows.Close()
				return err
			}
			d.rec.Attachments = bareAttachments(parseUUIDList(fileIDs))
			d.rec.MentionsAll = domain.HasChannelMention(d.rec.Body)
			due = append(due, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		claimed = len(due)

		var deleteIDs []uuid.UUID
		for _, d := range due {
			if err := pg.SetTenant(ctx, tx, d.tenant); err != nil {
				return err
			}
			m, err := enviarProgramadoTx(ctx, tx, d)
			if err != nil {
				if isPermanentSendError(err) {
					// No se podrá enviar nunca (canal archivado/borrado, sin acceso de
					// escritura, hilo inválido): descartar la fila para no reintentar en bucle.
					log.From(ctx).Warn("programado descartado: no se puede enviar",
						"id", d.id, "tenant", d.tenant, "error", err)
					deleteIDs = append(deleteIDs, d.id)
					continue
				}
				return err // transitorio: rollback del lote, reintento en el próximo tick
			}
			if err := publish(ctx, tx, m); err != nil {
				return err
			}
			deleteIDs = append(deleteIDs, d.id)
		}
		if len(deleteIDs) > 0 {
			if _, err := tx.Exec(ctx,
				`DELETE FROM scheduled_messages WHERE id = ANY($1)`, deleteIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return claimed, err
}

// bareAttachments referencia cada fichero por su id, sin metadatos de Drive (ver la
// nota ponytail de ClaimDueScheduled).
func bareAttachments(ids []uuid.UUID) []domain.Attachment {
	if len(ids) == 0 {
		return nil
	}
	out := make([]domain.Attachment, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil {
			out = append(out, domain.Attachment{FileID: id})
		}
	}
	return out
}

// enviarProgramadoTx publica un programado vencido. Antes mira el bloqueo: un
// programado no se salta uno puesto después de programarlo.
func enviarProgramadoTx(ctx context.Context, tx pgx.Tx, d dueScheduled) (domain.Message, error) {
	if err := bloqueoEnDirectoTx(ctx, tx, d.tenant, d.rec.ChannelID, d.rec.AuthorSub); err != nil {
		return domain.Message{}, err
	}
	return createMessageTx(ctx, tx, d.tenant, d.rec)
}

// isPermanentSendError indica si el fallo al publicar un programado es definitivo
// (nunca se resolverá reintentando): descarta la fila en vez de reintentar en bucle.
func isPermanentSendError(err error) bool {
	return errors.Is(err, domain.ErrForbidden) ||
		errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrNotMember) ||
		errors.Is(err, domain.ErrBlocked) ||
		errors.Is(err, domain.ErrInvalidInput) ||
		errors.Is(err, domain.ErrEmptyMessage)
}
