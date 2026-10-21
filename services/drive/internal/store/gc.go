// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PurgedBlob es un blob liberado por la auto-purga de papelera: su clave S3, el
// tenant dueño (los objetos van namespaced por tenant, así que hace falta para
// localizarlo) y si quedó HUÉRFANO —ninguna copia/versión viva referencia ya su
// storage_key— y por tanto puede borrarse de S3 sin pérdida de datos. Un blob que
// otra copia server-side o una versión archivada aún comparte (Orphaned=false) NO
// debe borrarse: es la misma guarda que PurgeFile/PurgeFolder.
type PurgedBlob struct {
	Tenant     string
	StorageKey string
	Orphaned   bool
}

// claim identifica una fila de nivel superior reclamada para purgar.
type claim struct {
	tenant string
	id     uuid.UUID
}

// ClaimAndPurgeExpiredTrash reclama y purga, EN LA MISMA TRANSACCIÓN, hasta `limit`
// elementos de NIVEL SUPERIOR de la papelera cuyo deleted_at es anterior a `cutoff`
// (retención cumplida), CROSS-TENANT. Devuelve cuántos elementos de nivel superior
// purgó (para que el worker sepa si drenó el lote) y los blobs liberados.
//
// CROSS-RÉPLICA SEGURO: reclama con FOR UPDATE SKIP LOCKED, así dos réplicas del
// worker nunca toman la misma fila a la vez; y como la purga es TERMINAL (DELETE)
// dentro de la MISMA tx que reclama, una fila purgada desaparece al hacer commit y
// no puede re-reclamarse (idempotente, sin ventana de doble trabajo). Solo se
// reclaman elementos de nivel superior (carpeta cuyo padre NO está en papelera,
// fichero cuya carpeta NO lo está): purgar una carpeta arrastra su subárbol por
// cascada, así que sus descendientes no son «nivel superior» y no se reclaman
// aparte —imposible purgar dos veces el mismo blob del subárbol—.
//
// RETENCIÓN por índice: la selección `deleted_at < cutoff` la sirve el índice
// parcial (deleted_at) de la mig 0009 (solo filas en papelera), ordenada por
// antigüedad. GUARDA DE BLOB COMPARTIDO y CUOTA: reusa los núcleos purgeFolderTx /
// purgeFileTx (idénticos a la purga manual), con el tenant EXPLÍCITO de cada fila
// reclamada. Cross-tenant vía `app.maintenance` (política PERMISSIVE de la mig
// 0009), sin BYPASSRLS; el predicado `tenant=$1` de cada purga mantiene el
// aislamiento (una purga solo toca datos de su tenant).
func (s *Store) ClaimAndPurgeExpiredTrash(ctx context.Context, cutoff time.Time, limit int) (int, []PurgedBlob, error) {
	if limit <= 0 {
		limit = 200
	}
	var claimed int
	var blobs []PurgedBlob
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		// 1) Carpetas de nivel superior caducadas (padre NO en papelera; nunca la raíz).
		//    FOR UPDATE OF f bloquea SOLO las filas de folders del SELECT externo (no
		//    las del subquery p), y SKIP LOCKED salta las que otra réplica ya reclamó.
		// held_tainted: carpetas que guardan un fichero de un sujeto bajo hold activo
		// (directamente) MÁS todos sus ancestros. Una carpeta de nivel superior en
		// held_tainted tiene un fichero retenido en su subárbol, así que su cascada de
		// purga lo borraría; se EXCLUYE del claim (legal hold NO destructivo). El JOIN
		// por h.tenant = fi.tenant mantiene el aislamiento cross-tenant del worker.
		// Cuando no hay holds activos, el ancla no produce filas y el CTE es trivial.
		// ponytail: el CTE se recomputa por claim; barato con legal_holds vacío (caso
		// común). Materializar «carpetas con hold» si el volumen de holds lo exige.
		folders, err := claimRows(ctx, tx, `
			WITH RECURSIVE held_tainted AS (
				SELECT fi.tenant, fi.folder_id AS id
				  FROM files fi
				  JOIN legal_holds h ON h.tenant = fi.tenant AND h.released_at IS NULL
				       AND (h.subject IS NULL OR h.subject = fi.owner_sub)
				UNION
				SELECT c.tenant, c.parent_id AS id
				  FROM folders c
				  JOIN held_tainted t ON c.tenant = t.tenant AND c.id = t.id
				 WHERE c.parent_id IS NOT NULL
			)
			SELECT f.tenant, f.id FROM folders f
			 WHERE f.deleted_at IS NOT NULL AND f.deleted_at < $1 AND NOT f.is_root
			   AND NOT EXISTS (
			         SELECT 1 FROM folders p
			          WHERE p.tenant = f.tenant AND p.id = f.parent_id AND p.deleted_at IS NOT NULL)
			   AND NOT EXISTS (
			         SELECT 1 FROM held_tainted t WHERE t.tenant = f.tenant AND t.id = f.id)
			 ORDER BY f.deleted_at
			 FOR UPDATE OF f SKIP LOCKED
			 LIMIT $2`, cutoff.UTC(), limit)
		if err != nil {
			return fmt.Errorf("store: reclamar carpetas caducadas: %w", err)
		}
		// 2) Ficheros de nivel superior caducados (carpeta NO en papelera), hasta
		//    completar el lote.
		var files []claim
		if remaining := limit - len(folders); remaining > 0 {
			files, err = claimRows(ctx, tx, `
				SELECT fi.tenant, fi.id FROM files fi
				 WHERE fi.deleted_at IS NOT NULL AND fi.deleted_at < $1
				   AND NOT EXISTS (
				         SELECT 1 FROM folders p
				          WHERE p.tenant = fi.tenant AND p.id = fi.folder_id AND p.deleted_at IS NOT NULL)
				   AND NOT EXISTS (
				         SELECT 1 FROM legal_holds h
				          WHERE h.tenant = fi.tenant AND h.released_at IS NULL
				            AND (h.subject IS NULL OR h.subject = fi.owner_sub))
				 ORDER BY fi.deleted_at
				 FOR UPDATE OF fi SKIP LOCKED
				 LIMIT $2`, cutoff.UTC(), remaining)
			if err != nil {
				return fmt.Errorf("store: reclamar ficheros caducados: %w", err)
			}
		}

		// 3) Purga en el acto (misma tx, filas ya bloqueadas). Reusa los núcleos.
		for _, c := range folders {
			deleted, err := purgeFolderTx(ctx, tx, c.tenant, c.id)
			if err != nil {
				return err
			}
			for _, df := range deleted {
				blobs = append(blobs, PurgedBlob{Tenant: c.tenant, StorageKey: df.StorageKey, Orphaned: df.BlobOrphaned})
			}
		}
		for _, c := range files {
			df, err := purgeFileTx(ctx, tx, c.tenant, c.id)
			if err != nil {
				return err
			}
			blobs = append(blobs, PurgedBlob{Tenant: c.tenant, StorageKey: df.StorageKey, Orphaned: df.BlobOrphaned})
		}
		claimed = len(folders) + len(files)
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return claimed, blobs, nil
}

// claimRows ejecuta una consulta de reclamo (SELECT tenant, id ... FOR UPDATE SKIP
// LOCKED) y materializa el resultado, cerrando el cursor ANTES de que el llamante
// dispare las purgas (pgx: una consulta activa por tx a la vez).
func claimRows(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]claim, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	var out []claim
	for rows.Next() {
		var c claim
		if err := rows.Scan(&c.tenant, &c.id); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	return out, rows.Err()
}
