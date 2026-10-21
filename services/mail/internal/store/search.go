// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// ListMessagesByUID devuelve los sobres de un buzón ordenados por UID ascendente.
// Es el orden estable que exige IMAP (RFC 9051 §2.3.1.1): el número de secuencia
// del mensaje es su posición 1-based en esta lista, y su UID el valor de la
// columna. FETCH/STORE/MOVE resuelven los conjuntos de secuencia/UID sobre él.
func (s *Store) ListMessagesByUID(ctx context.Context, mailboxID uuid.UUID) ([]domain.Message, error) {
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, selectMessageCols+`
			  AND mailbox_id = $1 AND deleted_at IS NULL
			ORDER BY uid ASC`, mailboxID)
		if err != nil {
			return fmt.Errorf("store: list by uid: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			msg, err := scanMessage(rows)
			if err != nil {
				return err
			}
			out = append(out, msg)
		}
		return rows.Err()
	})
	return out, err
}

// SearchFilter son los criterios de búsqueda soportados (subconjunto útil de
// IMAP SEARCH y del filtro Email/query de JMAP). Los campos vacíos no filtran.
type SearchFilter struct {
	// MailboxID restringe a un buzón; uuid.Nil busca en toda la cuenta.
	MailboxID uuid.UUID
	// Text es una consulta full-text sobre asunto+remitente+snippet (FTS Postgres).
	Text string
	// From filtra por subcadena del remitente (case-insensitive).
	From string
	// To filtra por subcadena de CUALQUIERA de los destinatarios (case-insensitive).
	To string
	// Subject filtra por subcadena del asunto (case-insensitive).
	Subject string
	// After/Before acotan received_at: >= After y < Before. Cero = sin acotar.
	After  time.Time
	Before time.Time
	// Unseen, Seen, Flagged restringen por flag (mutuamente combinables).
	Unseen  bool
	Seen    bool
	Flagged bool
	// HasAttachment restringe a mensajes con adjunto (operador has:attachment).
	HasAttachment bool
	// Limit/Offset paginan por posición (semántica `position` de Email/query,
	// RFC 8620 §5.5; orden received_at DESC). El listado de buzón (ListMessages)
	// usa keyset; aquí el offset lo exige el protocolo JMAP.
	Limit  int
	Offset int
}

// ftsMensaje es el tsvector GUARDADO del mensaje (mig. 0019), ya plegado: se calcula
// al escribir y no en cada búsqueda. Lo comparten la búsqueda y el eDiscovery.
const ftsMensaje = "busqueda"

// SearchMessages busca sobres de mensaje en una cuenta según el filtro. Usa el
// índice GIN de FTS (to_tsvector) cuando hay texto libre y consultas
// parametrizadas para el resto (sin SQL dinámico inseguro: los predicados se
// componen con placeholders posicionales). Devuelve los sobres, más recientes
// primero. Corre bajo RLS (tenant del contexto).
func (s *Store) SearchMessages(ctx context.Context, accountID uuid.UUID, f SearchFilter) ([]domain.Message, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// Construcción parametrizada del WHERE (sin interpolación de valores).
	args := []any{accountID}
	var preds []string
	preds = append(preds, "account_id = $1", "deleted_at IS NULL")

	add := func(clause string, val any) {
		args = append(args, val)
		preds = append(preds, fmt.Sprintf(clause, len(args)))
	}
	if f.MailboxID != uuid.Nil {
		add("mailbox_id = $%d", f.MailboxID)
	}
	if t := strings.TrimSpace(f.Text); t != "" {
		// plainto_tsquery evita inyección de operadores FTS por el usuario.
		add(ftsMensaje+" @@ plainto_tsquery('simple', plegar($%d))", t)
	}
	if f.From != "" {
		add("plegar(from_addr) LIKE '%%' || plegar($%d) || '%%'", f.From)
	}
	if f.Subject != "" {
		add("plegar(subject) LIKE '%%' || plegar($%d) || '%%'", f.Subject)
	}
	if f.To != "" {
		// to_addrs es un array: se compara elemento a elemento. EXISTS con el
		// unnest deja que baste con que UNO de los destinatarios encaje.
		add("EXISTS (SELECT 1 FROM unnest(to_addrs) AS d WHERE plegar(d) LIKE '%%' || plegar($%d) || '%%')", f.To)
	}
	if !f.After.IsZero() {
		add("received_at >= $%d", f.After)
	}
	if !f.Before.IsZero() {
		// Excluyente por el final del día: `before:2026-03-01` no trae ese día.
		add("received_at < $%d", f.Before)
	}
	if f.Unseen {
		preds = append(preds, `NOT (flags @> ARRAY['\Seen'])`)
	}
	if f.Seen {
		preds = append(preds, `flags @> ARRAY['\Seen']`)
	}
	if f.Flagged {
		preds = append(preds, `flags @> ARRAY['\Flagged']`)
	}
	if f.HasAttachment {
		preds = append(preds, `has_attach`)
	}

	args = append(args, limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	query := selectMessageCols + " AND " + strings.Join(preds, " AND ") +
		fmt.Sprintf(" ORDER BY received_at DESC LIMIT $%d OFFSET $%d", limitPos, offsetPos)

	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("store: search: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			msg, err := scanMessage(rows)
			if err != nil {
				return err
			}
			out = append(out, msg)
		}
		return rows.Err()
	})
	return out, err
}

// GetMessages devuelve los sobres de un conjunto de ids (para Email/get de JMAP),
// preservando solo los que existen y no están borrados. El orden de salida sigue
// el de ids de entrada; los ausentes se omiten (el llamador los reporta como
// notFound).
func (s *Store) GetMessages(ctx context.Context, ids []uuid.UUID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[uuid.UUID]domain.Message, len(ids))
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, selectMessageCols+`
			  AND id = ANY($1) AND deleted_at IS NULL`, ids)
		if err != nil {
			return fmt.Errorf("store: get messages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMessage(rows)
			if err != nil {
				return err
			}
			byID[m.ID] = m
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// CopyMessage copia un mensaje a otro buzón (IMAP COPY), duplicando el sobre y
// reutilizando el mismo blob S3 (el .eml es idéntico). Asigna un UID nuevo en el
// destino y devuelve ese UID. El uso de almacenamiento se acumula porque hay una
// nueva referencia lógica al mensaje.
// ponytail: sin account_id — solo lo llama IMAP COPY, cuyos id/dest salen de los
// buzones de la propia sesión (findMailbox/ListMessagesByUID). Si algún día se
// expone por REST/JMAP, añadir el mismo scope que MoveMessage.
func (s *Store) CopyMessage(ctx context.Context, id, destMailbox uuid.UUID) (uint32, error) {
	var newUID uint32
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		src, err := scanMessageRow(tx.QueryRow(ctx, selectMessageCols+` AND id = $1 AND deleted_at IS NULL`, id))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: copy: leer origen: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			UPDATE mailboxes SET uid_next = uid_next + 1, updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $1 RETURNING uid_next - 1`, destMailbox).Scan(&newUID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: copy: reservar UID: %w", err)
		}
		normAddrs(&src)
		var newID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO messages (
				tenant, account_id, mailbox_id, thread_id, uid, rfc822_msg_id, in_reply_to,
				subject, from_addr, from_name, to_addrs, cc_addrs, bcc_addrs, reply_to,
				snippet, blob_key, size_bytes, flags, has_attach,
				spf_result, dkim_result, dmarc_result, received_at, sent_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
			RETURNING id`,
			tenantOf(ctx), src.AccountID, destMailbox, src.ThreadID, newUID, src.MessageID, src.InReplyTo,
			src.Subject, src.FromAddr, src.FromName, src.To, src.Cc, src.Bcc, src.ReplyTo,
			src.Snippet, src.BlobKey, src.SizeBytes, src.Flags, src.HasAttach,
			string(src.SPFResult), string(src.DKIMResult), string(src.DMARCResult), nz(src.ReceivedAt), nzPtr(src.SentAt)).Scan(&newID)
		if err != nil {
			return fmt.Errorf("store: copy: insertar: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE accounts SET used_bytes = used_bytes + $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			src.AccountID, src.SizeBytes); err != nil {
			return fmt.Errorf("store: copy: actualizar uso: %w", err)
		}
		return nil
	})
	return newUID, err
}

// RenameMailbox cambia el nombre de un buzón de usuario (no de sistema). Devuelve
// ErrSpecialMailbox si el buzón tiene rol especial (Inbox/Sent/...). Owner-scoped
// por account_id (como batch.go): un buzón ajeno del mismo tenant → ErrNotFound.
func (s *Store) RenameMailbox(ctx context.Context, accountID, mailboxID uuid.UUID, newName string) error {
	if !domain.ValidMailboxName(newName) {
		return domain.ErrInvalidAddress
	}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var role string
		if err := tx.QueryRow(ctx, `SELECT role FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2`, mailboxID, accountID).Scan(&role); err != nil {
			return err
		}
		if role != "" {
			return domain.ErrSpecialMailbox
		}
		tag, err := tx.Exec(ctx, `
			UPDATE mailboxes SET name = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`, mailboxID, newName)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if isUniqueViolation(err) {
		return domain.ErrMailboxExists
	}
	return err
}

// DeleteMailbox borra un buzón de usuario vacío (no de sistema). Devuelve
// ErrSpecialMailbox si tiene rol especial y ErrMailboxNotEmpty si contiene
// mensajes no borrados. Owner-scoped por account_id: buzón ajeno → ErrNotFound.
func (s *Store) DeleteMailbox(ctx context.Context, accountID, mailboxID uuid.UUID) error {
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var role string
		if err := tx.QueryRow(ctx, `SELECT role FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2`, mailboxID, accountID).Scan(&role); err != nil {
			return err
		}
		if role != "" {
			return domain.ErrSpecialMailbox
		}
		// El borrado del buzón cascadea (ON DELETE CASCADE) a sus mensajes, lo que
		// borraría FÍSICAMENTE incluso los ya soft-deleted. Si la cuenta está bajo
		// legal hold contamos TODOS los mensajes (incl. preservados): borrar la
		// carpeta perdería correo retenido, así que se niega hasta liberar el hold.
		held, err := isAccountHeldTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		countQ := `SELECT count(*) FROM messages WHERE tenant = current_setting('app.tenant', true) AND mailbox_id = $1 AND deleted_at IS NULL`
		if held {
			countQ = `SELECT count(*) FROM messages WHERE tenant = current_setting('app.tenant', true) AND mailbox_id = $1`
		}
		var n int
		if err := tx.QueryRow(ctx, countQ, mailboxID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return domain.ErrMailboxNotEmpty
		}
		tag, err := tx.Exec(ctx, `DELETE FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND id = $1`, mailboxID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
