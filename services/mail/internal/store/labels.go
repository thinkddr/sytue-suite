// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Etiquetas (labels) de usuario. Ver domain.Label y la migración 0005: son
// KEYWORDS JMAP/IMAP (el token en messages.flags es el id de la etiqueta en
// texto); esta tabla solo guarda nombre/color/dueño. CRUD owner-scoped por
// account_id (además de RLS por tenant), como mail_filters.

func scanLabel(s scanner) (domain.Label, error) {
	var l domain.Label
	err := s.Scan(&l.ID, &l.Tenant, &l.AccountID, &l.Name, &l.Color, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

// ListLabels devuelve las etiquetas de una cuenta con contadores total/no-leído,
// calculados en UNA sola consulta (sin N+1): un LEFT JOIN por membresía del
// keyword (=id) en messages.flags, que el índice GIN messages_flags_gin_idx
// acelera. Defensa en profundidad: filtra por account_id y tenant además de RLS.
func (s *Store) ListLabels(ctx context.Context, accountID uuid.UUID) ([]domain.Label, error) {
	var out []domain.Label
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT l.id, l.tenant, l.account_id, l.name, l.color, l.created_at, l.updated_at,
			       COUNT(m.id) FILTER (WHERE m.deleted_at IS NULL) AS total,
			       COUNT(m.id) FILTER (WHERE m.deleted_at IS NULL AND NOT (m.flags @> ARRAY['\Seen'])) AS unread
			FROM mail_labels l
			LEFT JOIN messages m
			  ON m.tenant = l.tenant AND m.account_id = l.account_id AND m.flags @> ARRAY[l.id::text]
			WHERE l.account_id = $1 AND l.tenant = $2
			GROUP BY l.id
			ORDER BY lower(l.name)`, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list labels: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var l domain.Label
			if err := rows.Scan(&l.ID, &l.Tenant, &l.AccountID, &l.Name, &l.Color,
				&l.CreatedAt, &l.UpdatedAt, &l.TotalCount, &l.UnreadCount); err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, err
}

// CreateLabel crea una etiqueta de la cuenta. Valida nombre/color en el dominio;
// el nombre duplicado (case-insensitive) da ErrLabelExists.
func (s *Store) CreateLabel(ctx context.Context, accountID uuid.UUID, name, color string) (domain.Label, error) {
	name = strings.TrimSpace(name)
	if !domain.ValidLabelName(name) || !domain.ValidLabelColor(color) {
		return domain.Label{}, domain.ErrInvalidAddress
	}
	var out domain.Label
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO mail_labels (tenant, account_id, name, color)
			VALUES ($1, $2, $3, $4)
			RETURNING id, tenant, account_id, name, color, created_at, updated_at`,
			tenantOf(ctx), accountID, name, color)
		var err error
		out, err = scanLabel(row)
		return err
	})
	if isUniqueViolation(err) {
		return domain.Label{}, domain.ErrLabelExists
	}
	return out, err
}

// UpdateLabel renombra y/o recolorea una etiqueta de la cuenta. Como el keyword
// es el id (estable), esto NO toca ningún mensaje. ErrNotFound si no es suya.
func (s *Store) UpdateLabel(ctx context.Context, accountID, id uuid.UUID, name, color string) error {
	name = strings.TrimSpace(name)
	if !domain.ValidLabelName(name) || !domain.ValidLabelColor(color) {
		return domain.ErrInvalidAddress
	}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE mail_labels SET name = $3, color = $4, updated_at = now()
			WHERE id = $1 AND account_id = $2 AND tenant = $5`,
			id, accountID, name, color, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: update label: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if isUniqueViolation(err) {
		return domain.ErrLabelExists
	}
	return err
}

// DeleteLabel borra la etiqueta y RETIRA su keyword de todos los mensajes de la
// cuenta (no borra el mensaje: solo lo desetiqueta). Owner-scoped por account_id.
func (s *Store) DeleteLabel(ctx context.Context, accountID, id uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM mail_labels WHERE id = $1 AND account_id = $2 AND tenant = $3`,
			id, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: delete label: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `
			UPDATE messages SET flags = array_remove(flags, $1)
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND flags @> ARRAY[$1]`, id.String(), accountID); err != nil {
			return fmt.Errorf("store: desetiquetar mensajes: %w", err)
		}
		return nil
	})
}

// ApplyLabel aplica (add=true) o quita (add=false) una etiqueta a un LOTE de
// mensajes en un solo UPDATE (sin N round-trips). Seguridad: verifica que la
// etiqueta es de la cuenta y filtra los mensajes por account_id, de modo que un
// cliente no puede etiquetar mensajes ajenos ni con una etiqueta ajena (además de
// RLS por tenant). Añadir es idempotente (array_remove + append deduplica).
func (s *Store) ApplyLabel(ctx context.Context, accountID, id uuid.UUID, messageIDs []uuid.UUID, add bool) error {
	if len(messageIDs) == 0 {
		return nil
	}
	kw := id.String()
	q := `UPDATE messages SET flags = array_append(array_remove(flags, $1), $1), updated_at = now()
	      WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND id = ANY($3) AND deleted_at IS NULL`
	if !add {
		q = `UPDATE messages SET flags = array_remove(flags, $1), updated_at = now()
		     WHERE tenant = current_setting('app.tenant', true) AND account_id = $2 AND id = ANY($3) AND deleted_at IS NULL`
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM mail_labels WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2)`,
			id, accountID).Scan(&owned); err != nil {
			return fmt.Errorf("store: verificar etiqueta: %w", err)
		}
		if !owned {
			return domain.ErrNotFound
		}
		if _, err := tx.Exec(ctx, q, kw, accountID, messageIDs); err != nil {
			return fmt.Errorf("store: aplicar etiqueta: %w", err)
		}
		return nil
	})
}

// ListMessagesByLabel pagina por KEYSET los mensajes con una etiqueta (across
// carpetas, como la vista de etiqueta de Gmail), más recientes primero. Filtra
// por account_id (owner) + membresía del keyword en flags (índice GIN). El cursor
// (before, beforeID) es el (received_at, id) del último visto; cero = primera
// página. Mismo contrato que ListMessages.
//
// ponytail: el orden received_at DESC lo resuelve un sort sobre las filas que casa
// el GIN (no un índice ordenado), coste ~proporcional al tamaño de la etiqueta por
// página, no O(1) como el listado de carpeta. Suficiente a escala de etiqueta
// (subconjunto del buzón); si una sola etiqueta debe keyset sobre millones,
// promover a una tabla message_labels(label_id, received_at DESC, id DESC) btree.
func (s *Store) ListMessagesByLabel(ctx context.Context, accountID, id uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	kw := id.String()
	q := selectMessageCols + `
		  AND account_id = $1 AND flags @> ARRAY[$2] AND deleted_at IS NULL
		ORDER BY received_at DESC, id DESC LIMIT $3`
	args := []any{accountID, kw, limit}
	if !before.IsZero() {
		q = selectMessageCols + `
			  AND account_id = $1 AND flags @> ARRAY[$2] AND deleted_at IS NULL
			  AND received_at <= $4 AND (received_at, id) < ($4, $5)
			ORDER BY received_at DESC, id DESC LIMIT $3`
		args = append(args, before, beforeID)
	}
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("store: list by label: %w", err)
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
