// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// CreateDocument inserta un documento nuevo y devuelve su forma completa.
//
// Con SourceRef (un fichero de fuera: «drive:<uuid>») la operación es idempotente:
// si esa persona ya abrió ese fichero, devuelve SU documento y `creado=false`. Se
// resuelve en la propia sentencia y no consultando antes, porque abrir el mismo
// fichero en dos pestañas a la vez son dos peticiones simultáneas y el «mira si
// existe y si no créalo» dejaría un duplicado o un error de índice único.
//
// `creado` importa a quien llama: solo un documento recién creado debe sembrarse
// con el contenido del fichero. Sembrar uno existente pisaría lo que ya escribió
// su dueño encima.
func (s *Store) CreateDocument(ctx context.Context, p domain.CreateDocumentParams) (*domain.Document, bool, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range p.Seed {
		if err := p.Seed[i].Validate(); err != nil {
			return nil, false, err
		}
	}
	var (
		doc    domain.Document
		creado bool
	)
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		// El DO UPDATE es un no-op deliberado (se reasigna el propio título): con
		// DO NOTHING el RETURNING no devuelve nada cuando la fila ya está, que es
		// justo el caso que hay que resolver. `xmax = 0` distingue el INSERT.
		const q = `
			INSERT INTO documents (tenant, kind, title, owner_sub, source_ref)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''))
			ON CONFLICT (tenant, owner_sub, source_ref)
			    WHERE source_ref IS NOT NULL AND deleted_at IS NULL
			    DO UPDATE SET title = documents.title
			RETURNING id, tenant, kind, title, owner_sub, seq, snapshot_id,
			          starred_by, created_at, updated_at, deleted_at,
			          coalesce(source_ref, ''), (xmax = 0)`
		row := tx.QueryRow(ctx, q, t.Slug, string(p.Kind), p.Title, p.OwnerSub, p.SourceRef)
		if err := row.Scan(&doc.ID, &doc.Tenant, &doc.Kind, &doc.Title, &doc.OwnerSub, &doc.Seq,
			&doc.SnapshotID, &doc.StarredBy, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
			&doc.SourceRef, &creado); err != nil {
			return err
		}
		// El contenido inicial, en la misma transacción: si no se puede escribir,
		// tampoco queda el documento (vacío y con el nombre de la plantilla).
		if !creado || len(p.Seed) == 0 {
			return nil
		}
		_, seq, err := appendOpsTx(ctx, tx, t.Slug, doc.ID, p.OwnerSub, p.Seed)
		doc.Seq = seq
		return err
	})
	if err != nil {
		return nil, false, fmt.Errorf("store: crear documento: %w", err)
	}
	return &doc, creado, nil
}

// GetDocument carga un documento por id (no borrado). Devuelve domain.ErrNotFound.
func (s *Store) GetDocument(ctx context.Context, id string) (*domain.Document, error) {
	var doc domain.Document
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanDocument(tx.QueryRow(ctx, selectDocByID, id), &doc)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener documento: %w", err)
	}
	return &doc, nil
}

const selectDocByID = `
	SELECT id, tenant, kind, title, owner_sub, seq, snapshot_id,
	       starred_by, created_at, updated_at, deleted_at, coalesce(source_ref, '')
	FROM documents
	WHERE id = $1 AND deleted_at IS NULL`

// ListDocuments lista documentos del tenant accesibles por el subject. Combina
// los que posee y aquellos en los que tiene una entrada de ACL (usuario, grupo
// o tenant). El filtrado fino por rol se hace en dominio sobre cada documento.
//
// Escala: la ACL va en un EXISTS correlacionado (lookup por la PK de
// doc_permissions; sin DISTINCT ni join que multiplique filas) y la paginación
// es keyset por (updated_at, id) DESC sobre documents_updated_idx — la
// comparación de fila (updated_at, id) < (cursor) es sargable (PostgreSQL la
// trunca a updated_at <= cursor como condición de índice y rechequea el resto).
func (s *Store) ListDocuments(ctx context.Context, subj domain.Subject, f domain.ListFilter) ([]domain.Document, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	beforeAt, beforeID := cursorDe(f)
	out := make([]domain.Document, 0, f.Limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		// ponytail: el filtro de destacados ($7) no tiene índice GIN sobre starred_by.
		// Se aplica DESPUÉS de la verja de permisos (owner/permiso/tenant), que ya deja
		// un puñado de filas por persona; si un tenant llega a decenas de miles de
		// documentos visibles por usuario, el arreglo es
		// CREATE INDEX ... ON documents USING gin (starred_by).
		const q = `
			SELECT d.id, d.tenant, d.kind, d.title, d.owner_sub, d.seq,
			       d.snapshot_id, d.starred_by, d.created_at, d.updated_at, d.deleted_at,
			       coalesce(d.source_ref, '')
			FROM documents d
			WHERE d.deleted_at IS NULL
			  AND (
			        d.owner_sub = $1
			     OR EXISTS (
			          SELECT 1 FROM doc_permissions p
			          WHERE p.document_id = d.id
			            -- Lo canjeado de un enlace revocado o caducado, y lo bloqueado, no
			            -- cuenta (ADR 0052).
			            AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)
			            AND (   (p.principal_kind = 'user'   AND p.principal_id = $1)
			                 OR (p.principal_kind = 'group'  AND p.principal_id = ANY($2))
			                 OR (p.principal_kind = 'tenant' AND p.principal_id = $3))
			        )
			  )
			  AND ($4 = '' OR d.kind = $4::doc_kind)
			  AND ($5 = '' OR d.owner_sub = $5)
			  AND ($6 = '' OR plegar(d.title) LIKE '%' || plegar($6) || '%')
			  AND (NOT $7::bool OR d.starred_by @> ARRAY[$1]::text[])
			  AND ($11 = '' OR d.source_ref = $11)
			  AND (d.updated_at, d.id) < ($8::timestamptz, $9::uuid)
			ORDER BY d.updated_at DESC, d.id DESC
			LIMIT $10`
		rows, err := tx.Query(ctx, q,
			subj.Sub, subj.Groups, subj.Tenant,
			string(f.Kind), f.OwnerSub, f.Query, f.StarredOnly, beforeAt, beforeID, f.Limit,
			f.SourceRef)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Document
			if err := scanDocumentRows(rows, &d); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar documentos: %w", err)
	}
	return out, nil
}

// RenameDocument cambia el título y actualiza updated_at.
func (s *Store) RenameDocument(ctx context.Context, id, title string) error {
	return s.execAffecting(ctx, "renombrar documento",
		`UPDATE documents SET title = $2, updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id, title)
}

// SoftDeleteDocument mueve el documento a la papelera (deleted_at).
func (s *Store) SoftDeleteDocument(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "borrar documento",
		`UPDATE documents SET deleted_at = now(), updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id)
}

// SetStarred marca/desmarca el documento como favorito para un usuario.
func (s *Store) SetStarred(ctx context.Context, id, sub string, starred bool) error {
	op := `array_remove(starred_by, $2)`
	if starred {
		op = `(SELECT array_agg(DISTINCT e) FROM unnest(array_append(starred_by, $2)) AS e)`
	}
	q := fmt.Sprintf(`UPDATE documents SET starred_by = COALESCE(%s, '{}'), updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, op)
	return s.execAffecting(ctx, "favorito", q, id, sub)
}

// execAffecting ejecuta una sentencia que debe afectar exactamente una fila,
// devolviendo domain.ErrNotFound si no afectó ninguna (id inexistente/borrado).
func (s *Store) execAffecting(ctx context.Context, what, q string, args ...any) error {
	return s.execAffectingDocument(ctx, "", what, q, args...)
}

func (s *Store) execAffectingDocument(ctx context.Context, docID, what, q string, args ...any) error {
	err := s.documentTx(ctx, docID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("store: %s: %w", what, err)
	}
	return err
}

// documentTx comparte el cerrojo de AppendOps con las mutaciones de permisos.
// Vacío conserva el helper de mutaciones que no necesitan bloquear un documento.
func (s *Store) documentTx(ctx context.Context, docID string, action func(pgx.Tx) error) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		if docID != "" {
			var found int
			err := tx.QueryRow(ctx, `SELECT 1 FROM documents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, docID).Scan(&found)
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		return action(tx)
	})
}

// scanDocument escanea una fila de documento desde un pgx.Row.
func scanDocument(row pgx.Row, d *domain.Document) error {
	return row.Scan(&d.ID, &d.Tenant, &d.Kind, &d.Title, &d.OwnerSub, &d.Seq,
		&d.SnapshotID, &d.StarredBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt, &d.SourceRef)
}

// scanDocumentRows escanea desde pgx.Rows (mismo orden de columnas).
func scanDocumentRows(rows pgx.Rows, d *domain.Document) error {
	return rows.Scan(&d.ID, &d.Tenant, &d.Kind, &d.Title, &d.OwnerSub, &d.Seq,
		&d.SnapshotID, &d.StarredBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt, &d.SourceRef)
}

// querierFromTx asegura que el tx implementa pg.Querier (uso interno/eventos).
var _ pg.Querier = pgx.Tx(nil)

// cursorDe es el cursor keyset de una lista: primera página = (futuro, uuid máximo)
// —todo pasa y la consulta es una sola forma sargable, sin OR con parámetros—. Con
// cursor pero sin id (cliente antiguo), uuid cero => estrictamente updated_at <.
func cursorDe(f domain.ListFilter) (time.Time, string) {
	if f.BeforeUpdatedAt == nil {
		return time.Now().Add(time.Hour), "ffffffff-ffff-ffff-ffff-ffffffffffff"
	}
	if f.BeforeID != "" {
		return *f.BeforeUpdatedAt, f.BeforeID
	}
	return *f.BeforeUpdatedAt, "00000000-0000-0000-0000-000000000000"
}
