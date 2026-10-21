// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Changes es el delta incremental estilo JMAP Email/changes (RFC 8621 §4.3) para
// el refresco en vivo de la bandeja abierta: dado un cursor de estado
// (updated_at, id), devuelve los sobres CAMBIADOS (vivos) y los ids DESTRUIDOS
// (borrados) con updated_at posterior al cursor, en orden (updated_at, id) y
// acotados a limit. Owner-scoped por account_id (+ RLS por tenant): solo entrega
// cambios de la propia cuenta. El índice messages_changes_idx resuelve el rango
// en O(nº de cambios) —no vuelca la bandeja—, así que sondear cuando no hay nada
// nuevo es O(1). lastTS/lastID son el nuevo cursor; hasMore indica si se truncó
// por limit (el cliente debe volver a sondear enseguida).
//
// since en cero (primer arranque) NO devuelve cambios: solo fija el estado actual
// del servidor (now()) para que el cliente empiece a sondear desde «ahora» sin una
// recarga, y sin depender de su reloj (que podría ir adelantado y perder cambios).
//
// ponytail: el orden por updated_at (now() = inicio de la transacción) asume que
// las mutaciones del sobre son transacciones cortas (insertar/flag/mover/borrar lo
// son). Bajo una transacción larga concurrente un cambio podría no verse hasta el
// siguiente sondeo del cliente; si alguna vez importa, promover a una secuencia
// monotónica por cuenta (un contador bumpeado por mutación) en lugar del timestamp.
//
//nolint:gocritic // la firma ES el contrato JMAP Email/changes (created+updated, destroyed, cursor {state,id}, hasMoreChanges).
func (s *Store) Changes(ctx context.Context, accountID uuid.UUID, since time.Time, sinceID uuid.UUID, limit int) (changed []domain.Message, destroyed []uuid.UUID, lastTS time.Time, lastID uuid.UUID, hasMore bool, err error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if since.IsZero() {
			if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&lastTS); err != nil {
				return fmt.Errorf("store: estado actual: %w", err)
			}
			lastID = uuid.Nil
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, account_id, mailbox_id, thread_id, uid, rfc822_msg_id, in_reply_to,
			       subject, from_addr, from_name, to_addrs, cc_addrs, bcc_addrs, reply_to,
			       snippet, blob_key, size_bytes, flags, has_attach,
			       spf_result, dkim_result, dmarc_result, received_at, sent_at,
			       updated_at, deleted_at
			FROM messages
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND (updated_at, id) > ($2, $3)
			ORDER BY updated_at, id
			LIMIT $4`, accountID, since, sinceID, limit)
		if err != nil {
			return fmt.Errorf("store: changes: %w", err)
		}
		defer rows.Close()
		lastTS, lastID = since, sinceID
		count := 0
		for rows.Next() {
			m, ts, deleted, scanErr := scanChangeRow(rows)
			if scanErr != nil {
				return scanErr
			}
			if deleted {
				destroyed = append(destroyed, m.ID)
			} else {
				changed = append(changed, m)
			}
			lastTS, lastID = ts, m.ID
			count++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		hasMore = count == limit
		return nil
	})
	return changed, destroyed, lastTS, lastID, hasMore, err
}

// scanChangeRow escanea una fila del delta: el sobre completo del mensaje más
// updated_at (el nuevo cursor) y si está borrado (deleted_at IS NOT NULL). Espeja
// scanMessageRow con las dos columnas extra que Changes selecciona al final.
func scanChangeRow(s scanner) (domain.Message, time.Time, bool, error) {
	var m domain.Message
	var uid int64
	var spf, dkim, dmarc string
	var sentAt *time.Time
	var updatedAt time.Time
	var deletedAt *time.Time
	err := s.Scan(&m.ID, &m.Tenant, &m.AccountID, &m.MailboxID, &m.ThreadID, &uid,
		&m.MessageID, &m.InReplyTo, &m.Subject, &m.FromAddr, &m.FromName,
		&m.To, &m.Cc, &m.Bcc, &m.ReplyTo, &m.Snippet, &m.BlobKey, &m.SizeBytes,
		&m.Flags, &m.HasAttach, &spf, &dkim, &dmarc, &m.ReceivedAt, &sentAt,
		&updatedAt, &deletedAt)
	if err != nil {
		return m, time.Time{}, false, err
	}
	m.UID = u32(uid)
	if sentAt != nil {
		m.SentAt = *sentAt
	}
	m.SPFResult = domain.AuthResult(spf)
	m.DKIMResult = domain.AuthResult(dkim)
	m.DMARCResult = domain.AuthResult(dmarc)
	return m, updatedAt, deletedAt != nil, nil
}
