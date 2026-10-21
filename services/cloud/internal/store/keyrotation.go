// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Reseal descifra un blob con el anillo de claves y lo vuelve a cifrar con la KEK
// primaria (lo implementa crypto.Sealer.Reseal). Se pasa como función para que el
// store no dependa del paquete de cifrado: aquí solo se sabe mover bytes.
type Reseal func([]byte) ([]byte, error)

// RotationBatch son las filas que se re-sellan por transacción. Lotes pequeños a
// propósito: la rotación compite con el tráfico normal por las mismas filas, y una
// transacción larga bloquearía escrituras de clientes durante todo el barrido.
const RotationBatch = 200

// RotationStats cuenta las filas re-selladas en una pasada, por tabla.
type RotationStats struct {
	EnvVars  int64 // env_vars.value_enc
	Services int64 // managed_services.creds_enc
	Projects int64 // projects.webhook_secret_enc + repo_creds_enc
}

// Total suma las filas re-selladas.
func (r RotationStats) Total() int64 { return r.EnvVars + r.Services + r.Projects }

// SQL de la rotación. Notas que no se ven en el texto:
//
//   - El predicado es `key_version < $1`, NO `<> $1`: una fila marcada con una
//     versión POSTERIOR a la primaria de este proceso (binario recién revertido a
//     una configuración anterior) está cifrada con una clave más nueva y volver a
//     sellarla con la vieja sería degradarla. Ese caso se deja en paz.
//   - No se toca updated_at. Es metadato que el usuario ve y, en projects, la
//     ventana por la que el controlador decide qué producción reaplicar: re-cifrar
//     un secreto en reposo no es un cambio de configuración y no debe provocar
//     despliegues ni «modificado hace un momento» en la interfaz.
//   - FOR UPDATE SKIP LOCKED: varias réplicas pueden rotar a la vez sin pisarse, y
//     una fila que un cliente esté editando se salta en vez de bloquearla (esa
//     escritura ya usa la KEK primaria, así que se resuelve sola).
//
// ponytail: el barrido es un seq scan acotado por LIMIT; con la primaria en 1 (el
// caso normal, sin rotación en curso) no encuentra nada y recorre la tabla entera.
// A escala de millones de filas, si pesa: índice btree en key_version — el
// predicado `<` es de rango, indexable tal cual, sin cambiar este SQL.
const (
	selectEnvValues = `
		SELECT id, value_enc FROM env_vars
		WHERE value_enc IS NOT NULL AND key_version < $1
		ORDER BY id LIMIT $2
		FOR UPDATE SKIP LOCKED`
	updateEnvValues = `UPDATE env_vars SET value_enc = $3, key_version = $2 WHERE id = $1`

	selectServiceValues = `
		SELECT id, creds_enc FROM managed_services
		WHERE creds_enc IS NOT NULL AND key_version < $1
		ORDER BY id LIMIT $2
		FOR UPDATE SKIP LOCKED`
	updateServiceValues = `UPDATE managed_services SET creds_enc = $3, key_version = $2 WHERE id = $1`

	// projects lleva DOS blobs en la misma fila y se re-sellan JUNTOS, en el mismo
	// UPDATE que sube key_version: si se subiera la versión habiendo re-cifrado solo
	// uno, el otro quedaría marcado como al día con la clave vieja y se perdería al
	// retirarla.
	selectProjectValues = `
		SELECT id, webhook_secret_enc, repo_creds_enc FROM projects
		WHERE key_version < $1
		  AND (webhook_secret_enc IS NOT NULL OR repo_creds_enc IS NOT NULL)
		ORDER BY id LIMIT $2
		FOR UPDATE SKIP LOCKED`
	updateProjectValues = `
		UPDATE projects SET webhook_secret_enc = $3, repo_creds_enc = $4, key_version = $2
		WHERE id = $1`
)

// RotateSecretKeys re-cifra con la KEK primaria los secretos que sigan sellados con
// una versión anterior del anillo, en lotes y sin parar el servicio: las lecturas
// funcionan durante todo el proceso porque el anillo conserva la clave vieja
// (crypto.Sealer.Open la prueba). Es incremental y reanudable — cada lote va en su
// propia transacción, así que cortarla a medias no deshace lo ya hecho—, e
// idempotente: repetirla cuando no queda nada por rotar no escribe nada.
//
// Cuando termina sin dejar filas por debajo de `version` (ver SecretKeyVersions),
// la clave vieja ya no descifra nada y se puede quitar de SECRET_ENC_KEY.
func (s *Store) RotateSecretKeys(ctx context.Context, version int, reseal Reseal) (RotationStats, error) {
	var st RotationStats
	var err error
	if st.EnvVars, err = s.rotateBlobs(ctx, selectEnvValues, updateEnvValues, 1, version, reseal); err != nil {
		return st, err
	}
	if st.Services, err = s.rotateBlobs(ctx, selectServiceValues, updateServiceValues, 1, version, reseal); err != nil {
		return st, err
	}
	st.Projects, err = s.rotateBlobs(ctx, selectProjectValues, updateProjectValues, 2, version, reseal)
	return st, err
}

// pendingRow es una fila reclamada para re-sellar: su id y sus blobs cifrados.
type pendingRow struct {
	id  uuid.UUID
	enc [][]byte
}

// rotateBlobs re-sella por lotes las filas de una tabla. selSQL recibe ($1=versión
// primaria, $2=tamaño del lote) y devuelve (id, blob…); updSQL recibe ($1=id,
// $2=versión, $3…=blobs re-sellados) en el mismo orden.
func (s *Store) rotateBlobs(ctx context.Context, selSQL, updSQL string, blobs, version int, reseal Reseal) (int64, error) {
	var total int64
	for {
		var done int64
		err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			batch, err := claimPending(ctx, tx, selSQL, blobs, version)
			if err != nil {
				return err
			}
			for _, row := range batch {
				args := make([]any, 0, blobs+2)
				args = append(args, row.id, version)
				for _, enc := range row.enc {
					// NULL o vacío se dejan tal cual: no hay nada que descifrar y
					// convertirlos en un blob sellado inventaría un secreto vacío.
					if len(enc) == 0 {
						args = append(args, enc)
						continue
					}
					next, err := reseal(enc)
					if err != nil {
						return fmtErr("re-sellar secreto", err)
					}
					args = append(args, next)
				}
				if _, err := tx.Exec(ctx, updSQL, args...); err != nil {
					return fmtErr("guardar secreto re-sellado", err)
				}
			}
			done = int64(len(batch))
			return nil
		})
		if err != nil {
			return total, err
		}
		total += done
		// Un lote vacío significa que no queda nada reclamable: o está todo al día,
		// o lo que falta lo tiene bloqueado otra transacción y lo cogerá la
		// siguiente pasada. En ambos casos, esta termina.
		if done == 0 {
			return total, nil
		}
	}
}

// claimPending reclama (FOR UPDATE SKIP LOCKED) un lote de filas por rotar. Lee
// TODO el lote antes de volver para no intercalar el UPDATE con el cursor abierto:
// pgx no admite dos consultas a la vez sobre la misma conexión.
func claimPending(ctx context.Context, tx pgx.Tx, selSQL string, blobs, version int) ([]pendingRow, error) {
	rows, err := tx.Query(ctx, selSQL, version, RotationBatch)
	if err != nil {
		return nil, fmtErr("reclamar secretos por rotar", err)
	}
	defer rows.Close()
	var batch []pendingRow
	for rows.Next() {
		row := pendingRow{enc: make([][]byte, blobs)}
		dst := make([]any, 0, blobs+1)
		dst = append(dst, &row.id)
		for i := range row.enc {
			dst = append(dst, &row.enc[i])
		}
		if err := rows.Scan(dst...); err != nil {
			return nil, fmtErr("leer secreto por rotar", err)
		}
		batch = append(batch, row)
	}
	return batch, rows.Err()
}

// SecretKeyVersions cuenta los BLOBS cifrados que hay en cada versión de KEK
// (una fila de projects con sus dos secretos cuenta dos). Es el semáforo de la
// rotación: mientras aparezca una versión distinta de la primaria, su clave sigue
// haciendo falta en SECRET_ENC_KEY y retirarla dejaría datos ilegibles.
//
// Al ser key_version una cota inferior (ver migración 0025), el recuento puede
// señalar como pendiente algo que ya está al día — nunca al revés. Por eso se lee
// DESPUÉS de una pasada de RotateSecretKeys, que es cuando dice la verdad.
func (s *Store) SecretKeyVersions(ctx context.Context) (map[int]int64, error) {
	out := map[int]int64{}
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT v, sum(n)::bigint FROM (
			    SELECT key_version::int AS v, count(*) AS n FROM env_vars
			        WHERE value_enc IS NOT NULL GROUP BY 1
			    UNION ALL
			    SELECT key_version::int, count(*) FROM managed_services
			        WHERE creds_enc IS NOT NULL GROUP BY 1
			    UNION ALL
			    SELECT key_version::int, count(*) FROM projects
			        WHERE webhook_secret_enc IS NOT NULL GROUP BY 1
			    UNION ALL
			    SELECT key_version::int, count(*) FROM projects
			        WHERE repo_creds_enc IS NOT NULL GROUP BY 1
			) t GROUP BY v`)
		if err != nil {
			return fmtErr("censar versiones de clave", err)
		}
		defer rows.Close()
		for rows.Next() {
			var version int
			var n int64
			if err := rows.Scan(&version, &n); err != nil {
				return err
			}
			out[version] = n
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
