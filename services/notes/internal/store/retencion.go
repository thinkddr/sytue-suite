// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

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
// La alternativa —iterar organizaciones— no existe aquí: la lista de tenants
// saldría de estas mismas tablas, y leerlas ya requiere ser cross-tenant.
//
// # Orden y alcance
//
// Primero las notas y después los cuadernos: un cuaderno en papelera arrastra sus
// notas, y borrarlo antes dejaría notas apuntando a un padre inexistente durante
// el hueco entre las dos consultas. Van en la MISMA transacción, así que ese
// hueco no lo ve nadie, pero el orden se mantiene porque la restricción de clave
// ajena sí lo mira.
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
	var total int64
	err := s.inMaintenanceWriteTx(ctx, func(tx pgx.Tx) error {
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
			 WHERE n.deleted_at IS NOT NULL AND n.deleted_at < $1
			   AND NOT EXISTS (
			         SELECT 1 FROM legal_holds h
			          WHERE h.tenant = n.tenant AND h.released_at IS NULL
			            AND (h.subject IS NULL OR h.subject = n.owner_sub))`, corte)
		if err != nil {
			return err
		}
		total = tag.RowsAffected()

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
				 WHERE EXISTS (
				         SELECT 1 FROM legal_holds h
				          WHERE h.tenant = nb.tenant AND h.released_at IS NULL
				            AND (h.subject IS NULL OR h.subject = nb.owner_sub))
				UNION
				SELECT n.tenant, n.notebook_id
				  FROM notes n
				 WHERE EXISTS (
				         SELECT 1 FROM legal_holds h
				          WHERE h.tenant = n.tenant AND h.released_at IS NULL
				            AND (h.subject IS NULL OR h.subject = n.owner_sub))
				UNION
				SELECT p.tenant, p.id
				  FROM protegido t
				  JOIN notebooks c ON c.tenant = t.tenant AND c.id = t.id
				  JOIN notebooks p ON p.tenant = c.tenant AND p.id = c.parent_id
			)
			DELETE FROM notebooks nb
			 WHERE nb.deleted_at IS NOT NULL AND nb.deleted_at < $1
			   AND NOT EXISTS (
			         SELECT 1 FROM protegido pr
			          WHERE pr.tenant = nb.tenant AND pr.id = nb.id)`, corte)
		if err != nil {
			return err
		}
		total += tag.RowsAffected()
		return nil
	})
	return total, err
}
