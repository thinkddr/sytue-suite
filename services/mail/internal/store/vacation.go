// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Autorespuesta / vacaciones. Config por cuenta (mail_vacation, upsert) + bitácora
// de dedupe por remitente (mail_vacation_log) para no responder más de una vez por
// ventana (RFC 3834). Owner-scoped por account_id además de RLS por tenant. La
// entrega entrante ya corre bajo el tenant del destinatario, así que estas
// consultas usan la transacción de tenant normal (sin mantenimiento).

// GetVacation devuelve la configuración de vacaciones de la cuenta. Si no hay fila,
// devuelve una config desactivada (cero-valor), no un error: «sin configurar» =
// «desactivada».
func (s *Store) GetVacation(ctx context.Context, accountID uuid.UUID) (domain.Vacation, error) {
	var v domain.Vacation
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT enabled, subject, body, starts_at, ends_at, updated_at
			FROM mail_vacation WHERE account_id = $1 AND tenant = $2`,
			accountID, tenantOf(ctx)).Scan(&v.Enabled, &v.Subject, &v.Body, &v.StartsAt, &v.EndsAt, &v.UpdatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Vacation{}, nil
	}
	if err != nil {
		return domain.Vacation{}, fmt.Errorf("store: get vacation: %w", err)
	}
	return v, nil
}

// SetVacation guarda (upsert) la configuración de vacaciones de la cuenta.
func (s *Store) SetVacation(ctx context.Context, accountID uuid.UUID, v domain.Vacation) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO mail_vacation (account_id, tenant, enabled, subject, body, starts_at, ends_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			ON CONFLICT (account_id) DO UPDATE SET
				enabled    = EXCLUDED.enabled,
				subject    = EXCLUDED.subject,
				body       = EXCLUDED.body,
				starts_at  = EXCLUDED.starts_at,
				ends_at    = EXCLUDED.ends_at,
				updated_at = now()`,
			accountID, tenantOf(ctx), v.Enabled, v.Subject, v.Body, v.StartsAt, v.EndsAt)
		if err != nil {
			return fmt.Errorf("store: set vacation: %w", err)
		}
		return nil
	})
}

// GetSignature devuelve la firma de la cuenta (columna accounts.signature).
// Cuenta inexistente ⇒ ErrNotFound; cuenta sin firma ⇒ cadena vacía, que es un
// valor legítimo («sin firma»), no un error.
func (s *Store) GetSignature(ctx context.Context, accountID uuid.UUID) (string, error) {
	var sig string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT signature FROM accounts WHERE tenant = current_setting('app.tenant', true) AND id = $1`, accountID).Scan(&sig)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: get signature: %w", err)
	}
	return sig, nil
}

// SetSignature reemplaza la firma de la cuenta. RLS por tenant + el id de cuenta
// que resuelve el token; si no toca ninguna fila es que la cuenta no es de este
// tenant (o no existe), y eso es ErrNotFound, no un éxito silencioso.
func (s *Store) SetSignature(ctx context.Context, accountID uuid.UUID, signature string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE accounts SET signature = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			accountID, signature)
		if err != nil {
			return fmt.Errorf("store: set signature: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ClaimAutoreply intenta reservar el derecho a autorresponder a `sender` para la
// cuenta, respetando la ventana de dedupe. Devuelve true si se debe enviar (primera
// vez con este remitente, o ya pasó la ventana desde la última) y false si NO
// (ya se respondió dentro de la ventana). Atómico y sin carrera: el upsert
// condicional toma el lock de la fila (account_id, sender); dos entregas
// concurrentes del mismo remitente solo dejan que UNA reclame (RFC 3834: exactly
// once por ventana, sin tormenta de respuestas). `sender` se normaliza a minúsculas.
func (s *Store) ClaimAutoreply(ctx context.Context, accountID uuid.UUID, sender string, window time.Duration) (bool, error) {
	windowSecs := int64(window / time.Second)
	var claimed bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// RETURNING sale con fila si INSERTa (primera vez) o si el UPDATE casa el
		// predicado de ventana; sin fila (ErrNoRows) = respuesta reciente, no enviar.
		err := tx.QueryRow(ctx, `
			INSERT INTO mail_vacation_log (tenant, account_id, sender, last_sent_at)
			VALUES ($1, $2, lower($3), now())
			ON CONFLICT (account_id, sender) DO UPDATE SET last_sent_at = now()
			WHERE mail_vacation_log.last_sent_at < now() - (interval '1 second' * $4)
			RETURNING account_id`,
			tenantOf(ctx), accountID, sender, windowSecs).Scan(new(uuid.UUID))
		if errors.Is(err, pgx.ErrNoRows) {
			claimed = false
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: claim autoreply: %w", err)
		}
		claimed = true
		return nil
	})
	return claimed, err
}
