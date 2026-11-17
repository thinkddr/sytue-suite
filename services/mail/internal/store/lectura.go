// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Preferencias de lectura (columnas de accounts, migración 0023) y remitentes de
// confianza (tabla mail_trusted_senders, misma migración). Mismo patrón que la
// ventana de deshacer y las plantillas: RLS por tenant más el id de cuenta que
// resuelve el token en cada consulta.

// GetReadingPrefs devuelve las preferencias de lectura de la cuenta.
// Cuenta inexistente ⇒ ErrNotFound.
func (s *Store) GetReadingPrefs(ctx context.Context, accountID uuid.UUID) (domain.ReadingPrefs, error) {
	var p domain.ReadingPrefs
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT auto_advance, swipe_right, swipe_left FROM accounts
			WHERE tenant = current_setting('app.tenant', true) AND id = $1`, accountID).
			Scan(&p.AutoAdvance, &p.SwipeRight, &p.SwipeLeft)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReadingPrefs{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ReadingPrefs{}, fmt.Errorf("store: get reading prefs: %w", err)
	}
	return p, nil
}

// SetReadingPrefs guarda las preferencias de lectura. Se validan aquí también: los
// CHECK de la tabla son la red de abajo, no el primer filtro. Ninguna fila tocada
// ⇒ la cuenta no es de este tenant ⇒ ErrNotFound.
func (s *Store) SetReadingPrefs(ctx context.Context, accountID uuid.UUID, p domain.ReadingPrefs) error {
	if err := p.Validar(); err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE accounts SET auto_advance = $2, swipe_right = $3, swipe_left = $4, updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			accountID, p.AutoAdvance, p.SwipeRight, p.SwipeLeft)
		if err != nil {
			return fmt.Errorf("store: set reading prefs: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ListTrustedSenders devuelve los remitentes de confianza de la cuenta, en orden
// alfabético (así los enseña Ajustes).
func (s *Store) ListTrustedSenders(ctx context.Context, accountID uuid.UUID) ([]string, error) {
	out := []string{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT address FROM mail_trusted_senders
			WHERE account_id = $1 AND tenant = $2
			ORDER BY address`, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list trusted senders: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var a string
			if err := rows.Scan(&a); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// AddTrustedSender marca un remitente como de confianza. Idempotente: volver a
// marcar uno que ya lo es no es un error (dos pestañas pueden pulsarlo a la vez).
// Devuelve la dirección normalizada, que es la que se guardó.
func (s *Store) AddTrustedSender(ctx context.Context, accountID uuid.UUID, address string) (string, error) {
	addr, err := domain.NormalizarRemitente(address)
	if err != nil {
		return "", err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Se bloquea la fila de la CUENTA para contar, como en CreateTemplate: sin el
		// bloqueo dos altas a la vez saltarían el tope. Confirma además que la cuenta
		// es de esta organización antes de escribir.
		var bloqueada uuid.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id FROM accounts WHERE id = $1 AND tenant = $2 FOR UPDATE`,
			accountID, tenantOf(ctx)).Scan(&bloqueada); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: add trusted sender: %w", err)
		}
		var n int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM mail_trusted_senders WHERE account_id = $1 AND tenant = $2`,
			accountID, tenantOf(ctx)).Scan(&n); err != nil {
			return fmt.Errorf("store: add trusted sender: %w", err)
		}
		if n >= domain.RemitentesDeConfianzaPorCuenta {
			var ya bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM mail_trusted_senders WHERE account_id = $1 AND tenant = $2 AND address = $3)`,
				accountID, tenantOf(ctx), addr).Scan(&ya); err != nil {
				return fmt.Errorf("store: add trusted sender: %w", err)
			}
			if !ya {
				return domain.ErrTrustedSenderLimite
			}
			return nil
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mail_trusted_senders (tenant, account_id, address)
			VALUES ($1, $2, $3)
			ON CONFLICT (account_id, address) DO NOTHING`,
			tenantOf(ctx), accountID, addr); err != nil {
			return fmt.Errorf("store: add trusted sender: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return addr, nil
}

// RemoveTrustedSender deja de confiar en un remitente. Idempotente: quitar uno que
// no estaba no es un error (el resultado es el pedido: no está).
func (s *Store) RemoveTrustedSender(ctx context.Context, accountID uuid.UUID, address string) error {
	addr, err := domain.NormalizarRemitente(address)
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM mail_trusted_senders WHERE account_id = $1 AND tenant = $2 AND address = $3`,
			accountID, tenantOf(ctx), addr); err != nil {
			return fmt.Errorf("store: remove trusted sender: %w", err)
		}
		return nil
	})
}
