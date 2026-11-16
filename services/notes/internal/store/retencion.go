// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var errPurgeTreeBusy = errors.New("notes: tree is busy; retry purge")

// PurgeExpiredTrash destruye definitivamente lo que lleva en la papelera más de
// `retention`, en TODAS las organizaciones, y devuelve cuántas filas se fueron.
//
// # Por qué es cross-tenant
//
// No hay sesión detrás: lo llama un worker cada hora. Corre bajo
// `InMaintenanceTx`, que fija `app.maintenance='on'`, y la política de la
// migración 0005 es la que deja ver las filas de cualquier organización. Sin
// ella el DELETE afectaría a cero y el worker informaría de «0 purgadas» para
// siempre sin que nada fallara.
//
// El inventario de tenants también se lee en mantenimiento; cada operación de
// borrado posterior queda acotada a uno de ellos y a su cerrojo correspondiente.
//
// # Orden y alcance
//
// Primero las notas y después los cuadernos, en la MISMA transacción por tenant.
// Cada organización toma su cerrojo de árbol antes de tocar filas; no se mantienen
// cerrojos de todas las organizaciones hasta que termine la ronda. Si una falla,
// se devuelve el número ya confirmado y el error; otra ronda puede reintentarla.
// Las organizaciones que lleguen tras el inventario se atienden en la siguiente.
//
// `retention <= 0` no purga NADA y se sale en el acto: es el interruptor de
// apagado (`TRASH_RETENTION=0`) para instalaciones que deban conservarlo todo.
// Comprobarlo aquí y no solo en el worker evita que una llamada directa se salte
// la decisión.
func (s *Store) PurgeExpiredTrash(ctx context.Context, retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	corte := time.Now().Add(-retention)
	var tenants []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant FROM notes WHERE deleted_at IS NOT NULL AND deleted_at<$1
			UNION SELECT tenant FROM notebooks WHERE deleted_at IS NOT NULL AND deleted_at<$1 ORDER BY tenant`, corte)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tenant string
			if err := rows.Scan(&tenant); err != nil {
				return err
			}
			tenants = append(tenants, tenant)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	var total int64
	var failures []error
	for _, tenant := range tenants {
		var affected int64
		err = s.inMaintenanceWriteTx(ctx, func(tx pgx.Tx) error {
			var locked bool
			if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('sytue:notes:tree:' || $1,0))`, tenant).Scan(&locked); err != nil {
				return err
			}
			if !locked {
				return errPurgeTreeBusy
			}
			// Las notas de cuadernos que también caducan entran igual por su propio
			// `deleted_at`: `DeleteNotebook` marca la rama entera, no solo la raíz.
			// EL `NOT EXISTS` ES LA RETENCIÓN LEGAL (migración 0006), y se evalúa aquí
			// y no antes a propósito: el hold no sella cada nota, se comprueba en el
			// punto de borrado. Así, poner un hold protege también lo que YA estaba en
			// la papelera, y liberarlo devuelve todo al ciclo normal sin recorrer nada.
			//
			// `h.tenant = n.tenant` es lo que mantiene el aislamiento en una transacción
			// que NO fija `app.tenant`: sin ese predicado, el hold de una organización
			// preservaría notas de otra.
			tag, err := tx.Exec(ctx, `
			DELETE FROM notes n
			 WHERE n.tenant=$2 AND n.deleted_at IS NOT NULL AND n.deleted_at < $1
			   AND NOT EXISTS (SELECT 1 FROM notes foreign_child WHERE foreign_child.group_id=n.id AND foreign_child.tenant<>n.tenant)
 AND NOT sytue_private_storage_frozen('notes_private_storage_states'::regclass,n.tenant,n.owner_sub)
 AND NOT EXISTS(SELECT 1 FROM notebooks b WHERE b.tenant=n.tenant AND b.id=n.notebook_id AND sytue_private_storage_frozen('notes_private_storage_states'::regclass,b.tenant,b.owner_sub))
			   AND NOT EXISTS (
			         SELECT 1 FROM legal_holds h
			          WHERE h.tenant = n.tenant AND h.released_at IS NULL
			            AND (h.subject IS NULL OR h.subject = n.owner_sub))`, corte, tenant)
			if err != nil {
				return err
			}
			affected = tag.RowsAffected()

			// LA CASCADA SE SALTARÍA EL HOLD, y por eso este borrado no puede ser un
			// simple NOT EXISTS como el de arriba. `notes.notebook_id` y
			// `notebooks.parent_id` llevan ON DELETE CASCADE (migración 0001): borrar un
			// cuaderno caducado destruye sus notas y sus cuadernos hijos SIN pasar por
			// ninguna condición nuestra —la cascada la ejecuta Postgres, no esta
			// consulta—. Un cuaderno del que se soltó la mano hace un mes se llevaría
			// por delante las notas que un juzgado mandó conservar, en silencio.
			//
			// `protegido` sube por el árbol: parte de lo que está bajo hold —un cuaderno
			// por su dueño, o un cuaderno que CONTIENE una nota bajo hold— y añade a sus
			// ancestros, porque borrar a un ancestro es borrar al descendiente. Es el
			// mismo `held_tainted` recursivo que usa la purga de Drive, por el mismo
			// motivo y con la misma forma.
			tag, err = tx.Exec(ctx, `
			WITH RECURSIVE protegido AS (
				SELECT nb.tenant, nb.id
				  FROM notebooks nb
				 WHERE nb.tenant=$2 AND (sytue_private_storage_frozen('notes_private_storage_states'::regclass,nb.tenant,nb.owner_sub) OR EXISTS (
				         SELECT 1 FROM legal_holds h
				          WHERE h.tenant = nb.tenant AND h.released_at IS NULL
				            AND (h.subject IS NULL OR h.subject = nb.owner_sub)))
				UNION
				SELECT n.tenant, n.notebook_id
				  FROM notes n
				 WHERE n.tenant=$2 AND (sytue_private_storage_frozen('notes_private_storage_states'::regclass,n.tenant,n.owner_sub) OR EXISTS (
				         SELECT 1 FROM legal_holds h
				          WHERE h.tenant = n.tenant AND h.released_at IS NULL
				            AND (h.subject IS NULL OR h.subject = n.owner_sub)))
				UNION
				-- A legacy FK is keyed only by UUID, not (tenant,id). Refuse
				-- cascades that would modify another tenant without its fence.
				SELECT parent.tenant,parent.id FROM notebooks parent JOIN notebooks child ON child.parent_id=parent.id
				 WHERE parent.tenant=$2 AND child.tenant<>parent.tenant
				UNION
				SELECT parent.tenant,parent.id FROM notebooks parent JOIN notes child ON child.notebook_id=parent.id
				 WHERE parent.tenant=$2 AND child.tenant<>parent.tenant
				UNION
				SELECT parent.tenant,parent.notebook_id FROM notes parent JOIN notes child ON child.group_id=parent.id
				 WHERE parent.tenant=$2 AND child.tenant<>parent.tenant
				UNION
				SELECT p.tenant, p.id
				  FROM protegido t
				  JOIN notebooks c ON c.tenant = t.tenant AND c.id = t.id
				  JOIN notebooks p ON p.tenant = c.tenant AND p.id = c.parent_id
			)
			DELETE FROM notebooks nb
			 WHERE nb.tenant=$2 AND nb.deleted_at IS NOT NULL AND nb.deleted_at < $1
			   AND NOT EXISTS (
			         SELECT 1 FROM protegido pr
			          WHERE pr.tenant = nb.tenant AND pr.id = nb.id)`, corte, tenant)
			if err != nil {
				return err
			}
			affected += tag.RowsAffected()
			return nil
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("notes: purge tenant %q: %w", tenant, err))
			if ctx.Err() != nil {
				return total, errors.Join(failures...)
			}
			continue
		}
		total += affected
	}
	return total, errors.Join(failures...)
}
