// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// La papelera de Docs: los documentos con `deleted_at` (SoftDeleteDocument), que
// hasta ahora nadie podía ver ni recuperar. Solo los de su dueño: los demás con
// acceso los pierden de su lista al borrarse y no tienen nada que restaurar.

const colsDocumento = `id, tenant, kind, title, owner_sub, seq, snapshot_id,
	starred_by, created_at, updated_at, deleted_at, coalesce(source_ref, '')`

// CursorPapelera es el último documento visto de la papelera: la página siguiente
// empieza después de él. Así vaciar avanza aunque algo no se haya podido borrar.
type CursorPapelera struct {
	BorradoEn time.Time
	ID        string
}

// ListarPapelera devuelve los documentos de `ownerSub` en la papelera, el último
// borrado primero, a partir de `despues` (nil = desde el principio); con `kind`,
// solo de ese tipo (Calcus ve sus hojas). `limit` se acota a 1..200.
func (s *Store) ListarPapelera(ctx context.Context, ownerSub string, kind domain.Kind, despues *CursorPapelera, limit int) ([]domain.Document, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	antes, antesID := time.Now().Add(time.Hour), "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if despues != nil {
		antes, antesID = despues.BorradoEn, despues.ID
	}
	var out []domain.Document
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+colsDocumento+` FROM documents
			WHERE owner_sub = $1 AND deleted_at IS NOT NULL AND ($3 = '' OR kind = $3::doc_kind)
			  AND (deleted_at, id) < ($4::timestamptz, $5::uuid)
			ORDER BY deleted_at DESC, id DESC LIMIT $2`, ownerSub, limit, string(kind), antes, antesID)
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
		return nil, fmt.Errorf("store: listar la papelera: %w", err)
	}
	return out, nil
}

// DocumentoEnPapelera carga un documento de `ownerSub` que esté en la papelera.
// Uno ajeno, vivo o inexistente es domain.ErrNotFound: la papelera de otra persona
// no se enseña ni para decir que existe.
func (s *Store) DocumentoEnPapelera(ctx context.Context, id, ownerSub string) (*domain.Document, error) {
	var d domain.Document
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanDocument(tx.QueryRow(ctx, `SELECT `+colsDocumento+` FROM documents
			WHERE id = $1 AND owner_sub = $2 AND deleted_at IS NOT NULL`, id, ownerSub), &d)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: documento en la papelera: %w", err)
	}
	return &d, nil
}

// RestaurarDocumento saca un documento de la papelera. Si entretanto la persona
// volvió a abrir el mismo fichero de Drive y ya tiene otro documento vivo con ese
// origen, el índice único lo impide: domain.ErrConflict.
func (s *Store) RestaurarDocumento(ctx context.Context, id string) error {
	err := s.execAffecting(ctx, "restaurar documento",
		`UPDATE documents SET deleted_at = NULL, updated_at = now()
		 WHERE id = $1 AND deleted_at IS NOT NULL`, id)
	if esUnica(err) {
		return domain.ErrConflict
	}
	return err
}

// PurgarDocumento borra DEFINITIVAMENTE un documento de la papelera; sus
// operaciones, versiones, comentarios y permisos caen por ON DELETE CASCADE. Los
// ficheros de S3 (docs/<id>/) los borra quien llama.
func (s *Store) PurgarDocumento(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "purgar documento",
		`DELETE FROM documents WHERE id = $1 AND deleted_at IS NOT NULL`, id)
}

// Estados de un fichero de Drive, tal como los entrega su cola (mig 0027 de Drive).
const (
	DriveVivo     = "live"
	DrivePapelera = "trashed"
	DrivePurgado  = "gone"
)

// SincronizarConDrive lleva al documento lo que le pasó a su fichero en Drive. Solo
// cuenta el documento de `ownerSub` nacido de ese fichero con ese tipo: un
// documento abierto desde el fichero de otra persona no se toca.
//
//   - papelera o purgado: el documento vivo va a la papelera de Docs. NUNCA se borra
//     para siempre desde aquí: en «Mi unidad» cualquiera de la organización puede
//     purgar un fichero, y el contenido del documento es de su dueño, que es quien
//     decide vaciarlo (revisión de seguridad, 2026-09-28).
//   - vivo: vuelve el último que se tiró, si no hay ya otro vivo con ese origen.
//
// Todo es idempotente: aplicar dos veces el mismo estado no cambia nada.
func (s *Store) SincronizarConDrive(ctx context.Context, fileID, ownerSub string, kind domain.Kind, estado string) error {
	origen := "drive:" + fileID
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		switch estado {
		case DrivePapelera, DrivePurgado:
			_, err := tx.Exec(ctx, `UPDATE documents SET deleted_at = now(), updated_at = now()
				WHERE source_ref = $1 AND owner_sub = $2 AND kind = $3 AND deleted_at IS NULL`,
				origen, ownerSub, string(kind))
			return err
		case DriveVivo:
			_, err := tx.Exec(ctx, `UPDATE documents d SET deleted_at = NULL, updated_at = now()
				WHERE d.id = (
					SELECT id FROM documents
					 WHERE source_ref = $1 AND owner_sub = $2 AND kind = $3 AND deleted_at IS NOT NULL
					 ORDER BY deleted_at DESC, id DESC LIMIT 1)
				  AND NOT EXISTS (
					SELECT 1 FROM documents v
					 WHERE v.source_ref = $1 AND v.owner_sub = $2 AND v.deleted_at IS NULL)`,
				origen, ownerSub, string(kind))
			return err
		}
		return fmt.Errorf("store: estado de Drive desconocido %q", estado)
	})
	if err != nil {
		return fmt.Errorf("store: sincronizar con Drive: %w", err)
	}
	return nil
}

// esUnica dice si el error es una violación de un índice único.
func esUnica(err error) bool {
	var pgErr *pgconn.PgError
	return err != nil && errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Caducado es un documento de la papelera cuya retención venció, de cualquier
// organización.
type Caducado struct {
	Tenant string
	Doc    domain.Document
}

// PapeleraCaducada devuelve hasta `limit` documentos que llevan en la papelera desde
// antes de `antes`, de TODAS las organizaciones, el más antiguo primero, a partir de
// `despues` (nil = desde el principio: lo que no se pudo borrar no atasca). Solo lee
// (política de la 0014): cada uno se borra luego en una transacción de su
// organización con PurgarCaducado.
func (s *Store) PapeleraCaducada(ctx context.Context, antes time.Time, despues *CursorPapelera, limit int) ([]Caducado, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	desde, desdeID := time.Time{}, "00000000-0000-0000-0000-000000000000"
	if despues != nil {
		desde, desdeID = despues.BorradoEn, despues.ID
	}
	var out []Caducado
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+colsDocumento+` FROM documents
			WHERE deleted_at IS NOT NULL AND deleted_at < $1
			  AND (deleted_at, id) > ($3::timestamptz, $4::uuid)
			ORDER BY deleted_at, id LIMIT $2`, antes, limit, desde, desdeID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Document
			if err := scanDocumentRows(rows, &d); err != nil {
				return err
			}
			out = append(out, Caducado{Tenant: d.Tenant, Doc: d})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: papelera caducada: %w", err)
	}
	return out, nil
}

// SigueCaducado dice si el documento sigue en la papelera desde antes de `antes`.
func (s *Store) SigueCaducado(ctx context.Context, id string, antes time.Time) (bool, error) {
	var ok bool
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM documents
			WHERE id = $1 AND deleted_at IS NOT NULL AND deleted_at < $2)`, id, antes).Scan(&ok)
	})
	if err != nil {
		return false, fmt.Errorf("store: sigue caducado: %w", err)
	}
	return ok, nil
}

// PurgarCaducado borra para siempre el documento SOLO si sigue en la papelera desde
// antes de `antes`: si alguien lo restauró mientras tanto, no se toca (ErrNotFound).
func (s *Store) PurgarCaducado(ctx context.Context, id string, antes time.Time) error {
	return s.execAffecting(ctx, "purgar documento caducado",
		`DELETE FROM documents WHERE id = $1 AND deleted_at IS NOT NULL AND deleted_at < $2`, id, antes)
}

// SinReconciliar son, de la organización del contexto, los documentos que están en
// la papelera y cuyo fichero de Drive nadie ha tocado: uno por (dueño, fichero,
// tipo), el último que se tiró. Quedan fuera los que tienen OTRO documento vivo del
// mismo fichero y dueño: ese fichero es el hogar del vivo y no va a ninguna papelera.
// Es la entrada de `docs reconciliar-papelera`.
func (s *Store) SinReconciliar(ctx context.Context) ([]domain.Document, error) {
	var out []domain.Document
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (d.owner_sub, d.source_ref, d.kind) `+colsDocumentoD+`
			FROM documents d
			WHERE d.deleted_at IS NOT NULL AND d.source_ref LIKE 'drive:%'
			  AND NOT EXISTS (
				SELECT 1 FROM documents v
				 WHERE v.owner_sub = d.owner_sub AND v.source_ref = d.source_ref AND v.deleted_at IS NULL)
			ORDER BY d.owner_sub, d.source_ref, d.kind, d.deleted_at DESC`)
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
		return nil, fmt.Errorf("store: documentos sin reconciliar: %w", err)
	}
	return out, nil
}

// colsDocumentoD es colsDocumento con el alias `d.`.
const colsDocumentoD = `d.id, d.tenant, d.kind, d.title, d.owner_sub, d.seq, d.snapshot_id,
	d.starred_by, d.created_at, d.updated_at, d.deleted_at, coalesce(d.source_ref, '')`
