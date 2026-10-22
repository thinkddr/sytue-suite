// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Canjear un enlace (ADR 0052): el enlace de editor o de comentarista pasa a ser un
// permiso de quien lo abre con su sesión, también desde otra organización.

// EnlaceCanjeable es un enlace ACTIVO con su id, que es de lo que cuelga lo canjeado.
type EnlaceCanjeable struct {
	LinkID string
	domain.ShareResolution
}

// EnlaceParaCanjear resuelve el token por `docs_canjear_enlace` (SECURITY DEFINER,
// la misma superficie que `docs_resolve_share` más el id del enlace).
func (s *Store) EnlaceParaCanjear(ctx context.Context, tokenHash string) (*EnlaceCanjeable, error) {
	var e EnlaceCanjeable
	err := s.pool.QueryRow(ctx,
		`SELECT link_id, document_id, tenant, role FROM docs_canjear_enlace($1)`, tokenHash).
		Scan(&e.LinkID, &e.DocumentID, &e.Tenant, &e.Role)
	if isNoRows(err) {
		return nil, domain.ErrShareExpired
	}
	if err != nil {
		return nil, fmt.Errorf("store: resolver enlace para canjear: %w", err)
	}
	return &e, nil
}

// Canjear deja el permiso de `sub` colgado del enlace. Va con el tenant del
// documento en el contexto (el de la organización anfitriona).
//
// Un permiso DIRECTO que ya tuviera no se toca: ni se rebaja ni pasa a depender del
// enlace (si no, revocar el enlace le quitaría lo que le dieron a mano). Uno que ya
// venía de un enlace lo sustituye el último canjeado, salvo que el dueño se lo
// quitara: entonces solo vale un enlace creado DESPUÉS (bloqueado_desde).
//
// Devuelve si escribió: con un permiso directo delante, no.
func (s *Store) Canjear(ctx context.Context, e *EnlaceCanjeable, sub string, externo bool) (bool, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return false, err
	}
	var escrito bool
	err = s.documentTx(ctx, e.DocumentID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO doc_permissions
			    (document_id, tenant, principal_kind, principal_id, role, granted_by, via_link, externo)
			VALUES ($1, $2, 'user', $3, $4, $5, $6, $7)
			ON CONFLICT (document_id, principal_kind, principal_id)
			DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by,
			              via_link = EXCLUDED.via_link, externo = EXCLUDED.externo, bloqueado_desde = NULL
			WHERE doc_permissions.via_link IS NOT NULL
			  AND (doc_permissions.bloqueado_desde IS NULL
			       OR (SELECT l.created_at FROM doc_share_links l WHERE l.id = EXCLUDED.via_link)
			          > doc_permissions.bloqueado_desde)`,
			e.DocumentID, t.Slug, sub, string(e.Role), "link:"+e.LinkID, e.LinkID, externo)
		escrito = err == nil && tag.RowsAffected() > 0
		return err
	})
	if err != nil {
		return false, fmt.Errorf("store: canjear enlace: %w", err)
	}
	return escrito, nil
}

// Anfitrion es la organización de un documento en el que `sub` es externo con un
// permiso vivo, o "" si no lo es. Es `docs_anfitrion` (SECURITY DEFINER), la única
// lectura entre organizaciones de lo canjeado.
func (s *Store) Anfitrion(ctx context.Context, docID, sub string) (string, error) {
	var t *string
	if err := s.pool.QueryRow(ctx, `SELECT docs_anfitrion($1::uuid, $2)`, docID, sub).Scan(&t); err != nil {
		return "", fmt.Errorf("store: organización anfitriona: %w", err)
	}
	if t == nil {
		return "", nil
	}
	return *t, nil
}

// CanjeadosDe son los `sub` que canjearon ese enlace: al revocarlo, a los que tengan el
// documento abierto se les corta el canal en vivo.
func (s *Store) CanjeadosDe(ctx context.Context, docID, linkID string) ([]string, error) {
	var out []string
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT principal_id FROM doc_permissions
			WHERE document_id = $1 AND via_link = $2 AND principal_kind = 'user'`, docID, linkID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err != nil {
				return err
			}
			out = append(out, sub)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: canjeados de un enlace: %w", err)
	}
	return out, nil
}

// DeFuera son los documentos de OTRAS organizaciones en los que `sub` es externo con
// un permiso vivo, con los filtros y el cursor de la lista (`docs_de_fuera`, SECURITY
// DEFINER). Van a la lista de su organización.
func (s *Store) DeFuera(ctx context.Context, sub string, f domain.ListFilter) ([]domain.Document, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	beforeAt, beforeID := cursorDe(f)
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant, kind, title, owner_sub, seq, snapshot_id, starred_by,
		       created_at, updated_at, source_ref
		FROM docs_de_fuera($1, $2, $3, $4, $5, $6, $7, $8::uuid, $9)`,
		sub, string(f.Kind), f.OwnerSub, f.Query, f.StarredOnly, f.SourceRef, beforeAt, beforeID, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("store: documentos de fuera: %w", err)
	}
	defer rows.Close()
	var out []domain.Document
	for rows.Next() {
		var d domain.Document
		if err := rows.Scan(&d.ID, &d.Tenant, &d.Kind, &d.Title, &d.OwnerSub, &d.Seq, &d.SnapshotID,
			&d.StarredBy, &d.CreatedAt, &d.UpdatedAt, &d.SourceRef); err != nil {
			return nil, err
		}
		d.Starred = slices.Contains(d.StarredBy, sub)
		out = append(out, d)
	}
	return out, rows.Err()
}
