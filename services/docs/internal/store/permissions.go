// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// ListPermissions devuelve la ACL de un documento.
func (s *Store) ListPermissions(ctx context.Context, docID string) ([]domain.Permission, error) {
	out := make([]domain.Permission, 0, 8)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = listPermissionsInTx(ctx, tx, docID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar permisos: %w", err)
	}
	return out, nil
}

// Una sola consulta de ACL vigente para lectura y autorización transaccional.
func listPermissionsInTx(ctx context.Context, tx pgx.Tx, docID string) ([]domain.Permission, error) {
	out := make([]domain.Permission, 0, 8)
	rows, err := tx.Query(ctx, `
			SELECT p.document_id, p.principal_kind, p.principal_id, p.role, p.granted_by, p.created_at
			FROM doc_permissions p
			WHERE p.document_id = $1
			  -- Lo canjeado de un enlace revocado o caducado, y lo bloqueado, no cuenta
			  -- (ADR 0052). La fila queda; la borra borrar el enlace.
			  AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)
			ORDER BY p.created_at ASC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.DocumentID, &p.PrincipalKind, &p.PrincipalID,
			&p.Role, &p.GrantedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertPermission concede o actualiza un permiso (idempotente por principal).
func (s *Store) UpsertPermission(ctx context.Context, docID string, p domain.Permission) error {
	err := s.documentTx(ctx, docID, func(tx pgx.Tx) error {
		t, _ := tenancy.From(ctx)
		_, err := tx.Exec(ctx, `
			INSERT INTO doc_permissions
			    (document_id, tenant, principal_kind, principal_id, role, granted_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (document_id, principal_kind, principal_id)
			DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by,
			              -- Concedido a mano deja de depender de un enlace canjeado (ADR 0052):
			              -- revocar el enlace no se lo lleva, y uno ya revocado no lo esconde.
			              via_link = NULL, bloqueado_desde = NULL`,
			docID, t.Slug, string(p.PrincipalKind), p.PrincipalID, string(p.Role), p.GrantedBy)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: conceder permiso: %w", err)
	}
	return nil
}

// RevokePermission quita un permiso por principal. Uno que venía de canjear un enlace
// no se borra: se BLOQUEA desde ahora, y no se le vuelve a canjear ningún enlace que
// ya existiera (ADR 0052); uno nuevo del dueño, sí. Sin
// esto, quitar a alguien duraba lo que tardara en pulsar otra vez el botón.
func (s *Store) RevokePermission(ctx context.Context, docID string, kind domain.PrincipalKind, principalID string) error {
	return s.execAffectingDocument(ctx, docID, "revocar permiso", `
		WITH bloqueado AS (
		    UPDATE doc_permissions SET bloqueado_desde = now()
		    WHERE document_id = $1 AND principal_kind = $2 AND principal_id = $3
		      AND via_link IS NOT NULL AND bloqueado_desde IS NULL
		    RETURNING 1
		), borrado AS (
		    DELETE FROM doc_permissions
		    WHERE document_id = $1 AND principal_kind = $2 AND principal_id = $3
		      AND via_link IS NULL
		    RETURNING 1
		)
		SELECT 1 FROM bloqueado UNION ALL SELECT 1 FROM borrado`,
		docID, string(kind), principalID)
}

// CreateShareLink crea un enlace de compartición; recibe el hash del token
// (nunca el token en claro). El token en claro lo genera y devuelve la capa http.
//
// El techo de caducidad de la organización se vuelve a comprobar AQUÍ, en la misma
// transacción y bajo el cerrojo que comparte con el cambio de política: quien crea un
// enlace mientras alguien baja el techo no puede dejar uno que lo incumpla. Sin techo
// (0) vale cualquier caducidad, también ninguna.
// `passwordHash` vacío = sin contraseña (se guarda NULL).
func (s *Store) CreateShareLink(ctx context.Context, docID, tokenHash, passwordHash, createdBy string, role domain.Role, expiresAt *time.Time) (*domain.ShareLink, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var link domain.ShareLink
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtext($1))`, cerrojoPoliticaEnlaces+t.Slug); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			WITH techo AS (
			    SELECT coalesce((SELECT max_dias FROM doc_org_politica_enlaces), $7) AS dias
			)
			INSERT INTO doc_share_links
			    (document_id, tenant, token_hash, role, created_by, expires_at, password_hash)
			SELECT $1, $2, $3, $4, $5, $6, nullif($8, '')
			FROM techo
			WHERE techo.dias = 0
			   OR ($6::timestamptz IS NOT NULL
			       AND $6::timestamptz <= now() + make_interval(days => techo.dias) + interval '5 minutes')
			RETURNING id, document_id, role, created_by, expires_at, revoked_at, created_at`,
			docID, t.Slug, tokenHash, string(role), createdBy, expiresAt, domain.TechoEnlacesPorDefecto, passwordHash).
			Scan(&link.ID, &link.DocumentID, &link.Role, &link.CreatedBy,
				&link.ExpiresAt, &link.RevokedAt, &link.CreatedAt)
		if isNoRows(err) {
			return domain.ErrCaducidadExcedida
		}
		return err
	})
	if errors.Is(err, domain.ErrCaducidadExcedida) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("store: crear enlace: %w", err)
	}
	link.HasPassword = passwordHash != ""
	return &link, nil
}

// ResolveShareToken resuelve un enlace de compartición ACTIVO a partir del hash
// de su token. Usa la función SECURITY DEFINER `docs_resolve_share`, que es la
// ÚNICA superficie cross-tenant permitida (devuelve solo document_id/tenant/role
// de enlaces no revocados ni caducados sobre documentos vivos). Evita conceder
// lecturas cross-tenant directas a la aplicación.
func (s *Store) ResolveShareToken(ctx context.Context, tokenHash string) (*domain.ShareResolution, error) {
	var r domain.ShareResolution
	err := s.pool.QueryRow(ctx,
		`SELECT document_id, tenant, role FROM docs_resolve_share($1)`, tokenHash).
		Scan(&r.DocumentID, &r.Tenant, &r.Role)
	if isNoRows(err) {
		return nil, domain.ErrShareExpired
	}
	if err != nil {
		return nil, fmt.Errorf("store: resolver enlace: %w", err)
	}
	return &r, nil
}

// ListShareLinks devuelve los enlaces de un documento (sin el token en claro).
func (s *Store) ListShareLinks(ctx context.Context, docID string) ([]domain.ShareLink, error) {
	out := make([]domain.ShareLink, 0, 4)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, role, created_by, expires_at, revoked_at, created_at,
			       ajustado_por_politica_at, password_hash IS NOT NULL
			FROM doc_share_links WHERE document_id = $1 ORDER BY created_at DESC`, docID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l domain.ShareLink
			if err := rows.Scan(&l.ID, &l.DocumentID, &l.Role, &l.CreatedBy,
				&l.ExpiresAt, &l.RevokedAt, &l.CreatedAt, &l.AdjustedAt, &l.HasPassword); err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar enlaces: %w", err)
	}
	return out, nil
}

// RevokeShareLink marca un enlace como revocado.
func (s *Store) RevokeShareLink(ctx context.Context, docID, linkID string) error {
	return s.execAffectingDocument(ctx, docID, "revocar enlace",
		`UPDATE doc_share_links SET revoked_at = now()
		 WHERE id = $1 AND document_id = $2 AND revoked_at IS NULL`, linkID, docID)
}

// ClaveDeEnlace devuelve el id y el hash de la contraseña de un enlace VIVO por el hash
// de su token ("" = no pide contraseña). ErrShareExpired si el enlace no existe o ya no
// vale. Usa `docs_enlace_clave` (SECURITY DEFINER): sin sesión y entre organizaciones.
func (s *Store) ClaveDeEnlace(ctx context.Context, tokenHash string) (linkID, hash string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT link_id::text, password_hash FROM docs_enlace_clave($1)`, tokenHash).Scan(&linkID, &hash)
	if isNoRows(err) {
		return "", "", domain.ErrShareExpired
	}
	if err != nil {
		return "", "", fmt.Errorf("store: contraseña del enlace: %w", err)
	}
	return linkID, hash, nil
}

// EstablecerClaveEnlace pone, cambia o quita ("" = quitar) la contraseña de un enlace no
// revocado. Recibe el hash, nunca la contraseña.
func (s *Store) EstablecerClaveEnlace(ctx context.Context, docID, linkID, passwordHash string) error {
	return s.execAffectingDocument(ctx, docID, "contraseña del enlace",
		`UPDATE doc_share_links SET password_hash = nullif($3, '')
		 WHERE id = $1 AND document_id = $2 AND revoked_at IS NULL`, linkID, docID, passwordHash)
}
