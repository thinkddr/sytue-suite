// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Alias de correo: direcciones adicionales que entregan a la misma cuenta. La
// tabla `aliases` existe desde 0001 (la usa LookupAddress en la entrega entrante);
// aquí el CRUD del dueño. Owner-scoped por account_id (además de RLS por tenant),
// como mail_filters / mail_labels. La dirección es única en TODO el espacio de
// direcciones (accounts.address + aliases.address), global entre tenants: una
// dirección entrega a una sola cuenta.

func scanAlias(s scanner) (domain.Alias, error) {
	var a domain.Alias
	err := s.Scan(&a.ID, &a.Tenant, &a.AccountID, &a.Address, &a.CreatedAt)
	return a, err
}

// ListAliases devuelve los alias de una cuenta (los más recientes primero).
func (s *Store) ListAliases(ctx context.Context, accountID uuid.UUID) ([]domain.Alias, error) {
	var out []domain.Alias
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, account_id, address, created_at
			FROM aliases WHERE account_id = $1 AND tenant = $2
			ORDER BY created_at DESC, address ASC`, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list aliases: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAlias(rows)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// CreateAlias registra un alias para la cuenta. `address` ya debe venir validado
// como dirección de un dominio servido (lo hace el handler, que conoce cfg). El
// account_id es la cuenta autenticada del llamante (confiable): el alias se sella
// con su tenant y su cuenta, sin poder apuntar a la cuenta de otro.
//
// Seguridad — dirección única global (sin secuestrar la de otro): la comprobación
// de colisión lee el espacio de direcciones CROSS-TENANT (accounts + aliases) bajo
// mantenimiento, igual que LookupAddress, porque ese espacio es global (una
// dirección entrega a una sola cuenta, la vea RLS o no). El INSERT va bajo el
// tenant del llamante (RLS normal); el índice único aliases_address_idx —global,
// ajeno a RLS— respalda la carrera alias-vs-alias devolviendo ErrAliasExists.
func (s *Store) CreateAlias(ctx context.Context, accountID uuid.UUID, address string) (domain.Alias, error) {
	addr, err := domain.ParseAddress(address)
	if err != nil {
		return domain.Alias{}, domain.ErrInvalidAddress
	}
	canon := addr.String() // local + '@' + dominio en minúsculas

	taken, err := s.addressTaken(ctx, canon, uuid.Nil)
	if err != nil {
		return domain.Alias{}, err
	}
	if taken {
		return domain.Alias{}, domain.ErrAliasExists
	}

	var al domain.Alias
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO aliases (tenant, account_id, address)
			VALUES ($1, $2, $3)
			RETURNING id, tenant, account_id, address, created_at`,
			tenantOf(ctx), accountID, canon)
		var e error
		al, e = scanAlias(row)
		return e
	})
	if isUniqueViolation(err) {
		return domain.Alias{}, domain.ErrAliasExists
	}
	if err != nil {
		return domain.Alias{}, fmt.Errorf("store: create alias: %w", err)
	}
	return al, nil
}

// addressTaken indica si una dirección ya existe en el espacio de direcciones
// (dirección primaria de alguna cuenta o algún alias), CROSS-TENANT. Corre en
// transacción de mantenimiento (app.maintenance='on') para ver todo el espacio
// global sin un rol BYPASSRLS — mismo criterio que LookupAddress.
//
// `except` deja fuera una cuenta: quien muda su propia dirección principal se
// encuentra a sí mismo en este espacio y eso no es un choque (uuid.Nil ⇒ nadie
// queda fuera, que es lo que quiere un alias nuevo).
func (s *Store) addressTaken(ctx context.Context, address string, except uuid.UUID) (bool, error) {
	var taken bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM accounts WHERE lower(address) = lower($1) AND id <> $2
				UNION ALL
				SELECT 1 FROM aliases  WHERE lower(address) = lower($1) AND account_id <> $2
			)`, address, except).Scan(&taken)
	})
	if err != nil {
		return false, fmt.Errorf("store: address taken: %w", err)
	}
	return taken, nil
}

// MoveAccountAddress muda la dirección principal de una cuenta a la que Sytue ID
// le asigna ahora, y CONSERVA la anterior como alias.
//
// PARA QUÉ: cuando una organización conecta su dominio propio, Sytue ID muda a
// toda la plantilla (`PromoteOrgMailDomain`) y encola las direcciones nuevas. El
// reconciliador las empujaba abriendo la sesión JMAP del dueño, pero eso solo
// CREA la cuenta si no existe: la de quien ya la tenía se quedaba con la
// dirección vieja, así que la nueva no existía en Sytue Mail ni como principal
// ni como alias y el MX contestaba 550 a todo el correo dirigido al dominio que
// el cliente acababa de conectar. El fallo era mudo por partida doble: la
// consola de Sytue ID enseñaba la dirección nueva como asignada y la fila
// quedaba marcada como creada.
//
// La anterior pasa a ALIAS y no se borra, igual que hace Sytue ID con la suya:
// quien tenga guardada la dirección vieja —o cualquier cosa de la suite que la
// tuviera apuntada— sigue entregando. Mudarse no es perder el correo anterior.
//
// Solo la mueve el claim `mailbox`, que es el que firma Sytue ID: ver account().
func (s *Store) MoveAccountAddress(ctx context.Context, acc domain.Account, nueva string) (domain.Account, error) {
	addr, err := domain.ParseAddress(nueva)
	if err != nil {
		return acc, domain.ErrInvalidAddress
	}
	canon := addr.String()
	if strings.EqualFold(canon, acc.Address) {
		return acc, nil
	}
	// Que no sea de otro. El índice único respalda la carrera, pero comprobarlo
	// antes distingue «esta dirección es de otra persona» —que hay que mirar— de
	// un fallo cualquiera de base.
	ajena, err := s.addressTaken(ctx, canon, acc.ID)
	if err != nil {
		return acc, err
	}
	if ajena {
		return acc, domain.ErrAliasExists
	}
	var out domain.Account
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		t := tenantOf(ctx)
		if acc.Address != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO aliases (tenant, account_id, address)
				VALUES ($1, $2, lower($3))
				ON CONFLICT DO NOTHING`, t, acc.ID, acc.Address); err != nil {
				return fmt.Errorf("store: conservar la dirección anterior: %w", err)
			}
		}
		// Si la nueva ya era alias suyo, deja de serlo: pasa a ser su principal, y
		// tenerla en las dos tablas dejaría una fila que «borrar» desde la pantalla
		// de alias sin que la dirección dejara de entregar.
		if _, err := tx.Exec(ctx, `
			DELETE FROM aliases
			WHERE account_id = $1 AND tenant = $2 AND lower(address) = lower($3)`,
			acc.ID, t, canon); err != nil {
			return fmt.Errorf("store: promover el alias: %w", err)
		}
		row := tx.QueryRow(ctx, `
			UPDATE accounts SET address = $1, updated_at = now()
			WHERE id = $2 AND tenant = $3
			RETURNING id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at`,
			canon, acc.ID, t)
		var e error
		out, e = scanAccount(row)
		return e
	})
	if isUniqueViolation(err) {
		return acc, domain.ErrAliasExists
	}
	if err != nil {
		return acc, fmt.Errorf("store: mudar la dirección principal: %w", err)
	}
	return out, nil
}

// DeleteAlias borra un alias de la cuenta. Owner-scoped: filtra por account_id y
// tenant, de modo que un usuario nunca borra el alias de otro. ErrNotFound si no
// existe o no es suyo.
func (s *Store) DeleteAlias(ctx context.Context, accountID, id uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM aliases WHERE id = $1 AND account_id = $2 AND tenant = $3`,
			id, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: delete alias: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// AliasBelongsTo indica si `address` es un alias VERIFICADO de la cuenta (existe en
// aliases con ese account_id). Base del «enviar-como»: solo se permite un From que
// sea la dirección primaria (lo comprueba el handler) o un alias propio. Owner y
// tenant scoped: no revela ni acepta alias de otra cuenta.
func (s *Store) AliasBelongsTo(ctx context.Context, accountID uuid.UUID, address string) (bool, error) {
	var ok bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM aliases
				WHERE account_id = $1 AND tenant = $2 AND lower(address) = lower($3)
			)`, accountID, tenantOf(ctx), address).Scan(&ok)
	})
	if err != nil {
		return false, fmt.Errorf("store: alias belongs to: %w", err)
	}
	return ok, nil
}
