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

// Buzones compartidos + delegación (ver migración 0008). Un buzón compartido es
// una `accounts` con kind=shared; la delegación es un ACL sobre la cuenta personal
// de otro. El acceso lo concede `mailbox_acl` (grantee = sub de Sytue ID + nivel).
//
// SEGURIDAD — deny-by-default: sin fila de ACL vigente no hay acceso. Todas las
// consultas van bajo InTenantTx (RLS por app.tenant) y filtran ADEMÁS por tenant
// explícito (defensa en profundidad). Buzón compartido y delegación NUNCA cruzan
// tenant: grantees, cuenta y ACL comparten tenant.

// ListAccessibleMailboxes lista las cuentas (compartidas o delegadas) a las que el
// grantee `subject` tiene acceso por ACL, con su permiso. UNA consulta indexada
// por (tenant, grantee_subject) — sin N+1 aunque el usuario acceda a muchos
// buzones. Alimenta el sidebar y el selector «De».
func (s *Store) ListAccessibleMailboxes(ctx context.Context, subject string) ([]domain.SharedMailbox, error) {
	var out []domain.SharedMailbox
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.address, a.display_name, a.kind, acl.permission,
			       EXISTS (SELECT 1 FROM mailbox_peticiones_edicion p WHERE p.tenant = a.tenant AND p.account_id = a.id AND p.sujeto = $1)
			FROM mailbox_acl acl
			JOIN accounts a ON a.id = acl.account_id AND a.tenant = acl.tenant
			WHERE acl.grantee_subject = $1 AND acl.tenant = $2
			ORDER BY a.kind, lower(a.address)`, subject, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list accessible mailboxes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.SharedMailbox
			var perm string
			if err := rows.Scan(&m.AccountID, &m.Address, &m.DisplayName, &m.Kind, &perm, &m.EditRequested); err != nil {
				return err
			}
			m.Permission = domain.Permission(perm)
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// GranteePermission devuelve el permiso del grantee `subject` sobre `accountID`
// (ErrNotFound si no hay ACL vigente = sin acceso). Es la COMPROBACIÓN DE ACL que
// el servicio hace en cada acceso a un buzón que no es el propio del usuario.
func (s *Store) GranteePermission(ctx context.Context, subject string, accountID uuid.UUID) (domain.Permission, error) {
	var perm string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT permission FROM mailbox_acl
			WHERE account_id = $1 AND grantee_subject = $2 AND tenant = $3`,
			accountID, subject, tenantOf(ctx)).Scan(&perm)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: grantee permission: %w", err)
	}
	return domain.Permission(perm), nil
}

// GrantedIdentityByAddress resuelve la dirección «De» `address` a la cuenta a la
// que pertenece, SI el grantee `subject` tiene ACL sobre ella, devolviendo el
// permiso y el kind (shared ⇒ enviar-como; user ⇒ enviar-en-nombre-de). Base de
// la autorización server-side del envío. ErrNotFound si no hay ACL (deny).
func (s *Store) GrantedIdentityByAddress(ctx context.Context, subject, address string) (domain.GrantedIdentity, error) {
	var gi domain.GrantedIdentity
	var perm string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT a.id, a.address, a.kind, acl.permission
			FROM accounts a
			JOIN mailbox_acl acl ON acl.account_id = a.id AND acl.tenant = a.tenant
			WHERE acl.grantee_subject = $1 AND acl.tenant = $2 AND lower(a.address) = lower($3)
			LIMIT 1`, subject, tenantOf(ctx), address).Scan(&gi.AccountID, &gi.Address, &gi.Kind, &perm)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GrantedIdentity{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.GrantedIdentity{}, fmt.Errorf("store: granted identity: %w", err)
	}
	gi.Permission = domain.Permission(perm)
	return gi, nil
}

// MailboxInAccount indica si el buzón `mailboxID` pertenece a `accountID` (mismo
// tenant). Se usa tras la comprobación de ACL para impedir que un grantee de una
// cuenta liste mensajes de un buzón de OTRA cuenta pasando un mailbox_id ajeno.
func (s *Store) MailboxInAccount(ctx context.Context, mailboxID, accountID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM mailboxes WHERE id = $1 AND account_id = $2 AND tenant = $3)`,
			mailboxID, accountID, tenantOf(ctx)).Scan(&ok)
	})
	if err != nil {
		return false, fmt.Errorf("store: mailbox in account: %w", err)
	}
	return ok, nil
}

// EnsureSharedMailbox crea (idempotente por dirección) un buzón COMPARTIDO: una
// `accounts` con kind=shared y subject sintético, sus buzones de sistema, y una
// ACL `manage` para el creador (ownerSubject) — que podrá gestionar el resto de
// accesos. La dirección debe ser única en el espacio global (como un alias).
func (s *Store) EnsureSharedMailbox(ctx context.Context, address, displayName string, quotaBytes int64, ownerSubject string) (domain.Account, error) {
	addr, err := domain.ParseAddress(address)
	if err != nil {
		return domain.Account{}, domain.ErrInvalidAddress
	}
	canon := addr.String()

	taken, err := s.addressTaken(ctx, canon, uuid.Nil)
	if err != nil {
		return domain.Account{}, err
	}
	if taken {
		return domain.Account{}, domain.ErrAliasExists
	}

	syntheticSubject := "shared:" + canon
	var acc domain.Account
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		t := tenantOf(ctx)
		row := tx.QueryRow(ctx, `
			INSERT INTO accounts (tenant, subject, address, display_name, quota_bytes, kind)
			VALUES ($1, $2, $3, $4, $5, 'shared')
			ON CONFLICT (tenant, subject) DO UPDATE SET display_name = EXCLUDED.display_name, updated_at = now()
			RETURNING id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at`,
			t, syntheticSubject, canon, displayName, quotaBytes)
		acc, err = scanAccount(row)
		if err != nil {
			return fmt.Errorf("store: ensure shared mailbox: %w", err)
		}
		if err := ensureDefaultMailboxes(ctx, tx, t, acc.ID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO mailbox_acl (tenant, account_id, grantee_subject, permission)
			VALUES ($1, $2, $3, 'manage')
			ON CONFLICT (account_id, grantee_subject) DO UPDATE SET permission = 'manage', updated_at = now()`,
			t, acc.ID, ownerSubject)
		return err
	})
	if isUniqueViolation(err) {
		return domain.Account{}, domain.ErrAliasExists
	}
	return acc, err
}

// ContarBuzonesCompartidos dice cuántos buzones compartidos tiene la organización
// del contexto. Es el dato del tope al crear uno.
// ponytail: se cuenta y luego se crea, sin cerrojo; dos altas a la vez pueden pasarse
// del tope en uno. Si importara, contar dentro de la transacción de EnsureSharedMailbox.
func (s *Store) ContarBuzonesCompartidos(ctx context.Context) (int, error) {
	var n int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM accounts WHERE tenant = $1 AND kind = 'shared'`, tenantOf(ctx)).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("store: contar buzones compartidos: %w", err)
	}
	return n, nil
}

// ListGrants lista el ACL de una cuenta (grantees + permiso), resolviendo el
// grantee a su dirección legible (LEFT JOIN por sub en el tenant). Owner/tenant
// scoped; el handler ya exige `manage`.
func (s *Store) ListGrants(ctx context.Context, accountID uuid.UUID) ([]domain.MailboxGrant, error) {
	var out []domain.MailboxGrant
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT acl.grantee_subject, COALESCE(a.address, ''), acl.permission, acl.created_at
			FROM mailbox_acl acl
			LEFT JOIN accounts a ON a.subject = acl.grantee_subject AND a.tenant = acl.tenant
			WHERE acl.account_id = $1 AND acl.tenant = $2
			ORDER BY acl.created_at`, accountID, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list grants: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var g domain.MailboxGrant
			var perm string
			if err := rows.Scan(&g.GranteeSubject, &g.GranteeEmail, &perm, &g.CreatedAt); err != nil {
				return err
			}
			g.Permission = domain.Permission(perm)
			out = append(out, g)
		}
		return rows.Err()
	})
	return out, err
}

// GrantAccess concede (o re-concede, actualizando el nivel) acceso `perm` sobre
// `accountID` al usuario cuya dirección es `granteeEmail`. Resuelve la dirección a
// su sub DENTRO DEL TENANT (así el grantee siempre es del mismo tenant: no se puede
// delegar cross-tenant). ErrNotFound si la dirección no es de una cuenta del tenant.
// El handler ya exige `manage` sobre accountID.
func (s *Store) GrantAccess(ctx context.Context, accountID uuid.UUID, granteeEmail string, perm domain.Permission) error {
	if !domain.ValidPermission(string(perm)) {
		return domain.ErrInvalidAddress
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		t := tenantOf(ctx)
		var granteeSubject string
		err := tx.QueryRow(ctx, `
			SELECT subject FROM accounts WHERE lower(address) = lower($1) AND tenant = $2`,
			granteeEmail, t).Scan(&granteeSubject)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: resolver grantee: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO mailbox_acl (tenant, account_id, grantee_subject, permission)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (account_id, grantee_subject)
			DO UPDATE SET permission = EXCLUDED.permission, updated_at = now()`,
			t, accountID, granteeSubject, string(perm))
		if err != nil {
			return fmt.Errorf("store: grant access: %w", err)
		}
		return nil
	})
}

// RevokeAccess retira el acceso de `granteeEmail` sobre `accountID`. Idempotente
// (revocar lo inexistente no es error). El handler ya exige `manage`.
func (s *Store) RevokeAccess(ctx context.Context, accountID uuid.UUID, granteeEmail string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		t := tenantOf(ctx)
		var granteeSubject string
		err := tx.QueryRow(ctx, `
			SELECT subject FROM accounts WHERE lower(address) = lower($1) AND tenant = $2`,
			granteeEmail, t).Scan(&granteeSubject)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // nada que revocar
		}
		if err != nil {
			return fmt.Errorf("store: resolver grantee: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM mailbox_acl WHERE account_id = $1 AND grantee_subject = $2 AND tenant = $3`,
			accountID, granteeSubject, t); err != nil {
			return fmt.Errorf("store: revoke access: %w", err)
		}
		return nil
	})
}
