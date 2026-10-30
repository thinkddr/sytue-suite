// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// MaxSubidasPendientes es cuántas URL de subida directa puede tener una
// organización sin finalizar ni caducar a la vez (FILE-08, migración 0030).
const MaxSubidasPendientes = 200

// ErrDemasiadasSubidas es pasar de MaxSubidasPendientes.
var ErrDemasiadasSubidas = errors.New("store: demasiadas subidas directas pendientes")

// ErrSubidaYaFinalizada indica que la reserva de subida directa ya se consumió.
var ErrSubidaYaFinalizada = errors.New("store: subida directa ya finalizada")

// ReservarSubida anota la URL de subida que se va a firmar. Falla con
// ErrQuotaExceeded si lo usado más lo pendiente más esta no cabe (`conCuota`
// falso: el almacén es del tenant y no gasta cuota de Sytue) y con
// ErrDemasiadasSubidas si hay demasiadas pendientes.
func (s *Store) ReservarSubida(ctx context.Context, clave string, tamano int64, caduca time.Time, conCuota bool) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Una reserva a la vez por organización: dos URL pedidas a la vez no se
		// cuelan las dos en el último hueco de la cuota.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('drive-subidas:' || $1))`, t.Slug); err != nil {
			return err
		}
		var pendientes, bytes int64
		if err := tx.QueryRow(ctx, `
			SELECT count(*), coalesce(sum(size_bytes) FILTER (WHERE con_cuota), 0) FROM subidas_pendientes
			 WHERE tenant = $1 AND NOT finalizada AND caduca_en > now()`, t.Slug).Scan(&pendientes, &bytes); err != nil {
			return err
		}
		if pendientes >= MaxSubidasPendientes {
			return ErrDemasiadasSubidas
		}
		if conCuota {
			q, err := scanQuota(tx.QueryRow(ctx, `SELECT `+quotaCols+` FROM quotas q WHERE q.tenant = $1`, t.Slug))
			if err != nil {
				return err
			}
			if q.UsedBytes+bytes+tamano > q.LimitBytes {
				return domain.ErrQuotaExceeded
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO subidas_pendientes (tenant, storage_key, size_bytes, caduca_en, con_cuota)
			VALUES ($1, $2, $3, $4, $5)`, t.Slug, clave, tamano, caduca, conCuota)
		return err
	})
}

// SubidaFinalizada saca la reserva de lo pendiente: el fichero ya cuenta en su
// cuota. La fila queda hasta que caduque la URL, para que el recolector borre lo
// que se escriba después en la clave. Devuelve ErrSubidaYaFinalizada si la
// reserva ya se había consumido antes (evita reutilizar el mismo handle tras
// deduplicar el blob original).
func (s *Store) SubidaFinalizada(ctx context.Context, clave string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE subidas_pendientes SET finalizada = true
			  WHERE tenant = $1 AND storage_key = $2 AND NOT finalizada`,
			t.Slug, clave)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
		var finalizada bool
		err = tx.QueryRow(ctx,
			`SELECT finalizada FROM subidas_pendientes WHERE tenant = $1 AND storage_key = $2`,
			t.Slug, clave).Scan(&finalizada)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if finalizada {
			return ErrSubidaYaFinalizada
		}
		return nil
	})
}

// BlobReferenciado indica si algún fichero (vivo o en papelera), versión
// archivada o blob cifrado de la organización sigue referenciando storageKey,
// en cualquier ámbito (unidad personal, unidad de equipo o canal).
func (s *Store) BlobReferenciado(ctx context.Context, tenant, storageKey string) (bool, error) {
	var ref bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var e error
		ref, e = blobReferenced(ctx, tx, tenant, storageKey)
		return e
	})
	return ref, err
}

// SubidaCaducada es una URL de subida que ya no vale.
type SubidaCaducada struct {
	Tenant     string
	StorageKey string
}

// ReclamarSubidasCaducadas saca, de todas las organizaciones, hasta `limite`
// reservas cuya URL caducó antes de `antes`, y las borra: quien llama borra el
// objeto de cada clave. SKIP LOCKED reparte entre réplicas.
func (s *Store) ReclamarSubidasCaducadas(ctx context.Context, antes time.Time, limite int) ([]SubidaCaducada, error) {
	var out []SubidaCaducada
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			DELETE FROM subidas_pendientes
			 WHERE (tenant, storage_key) IN (
			       SELECT tenant, storage_key FROM subidas_pendientes
			        WHERE caduca_en < $1
			        ORDER BY caduca_en
			        LIMIT $2
			        FOR UPDATE SKIP LOCKED)
			RETURNING tenant, storage_key`, antes, limite)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(filas, pgx.RowToStructByPos[SubidaCaducada])
		return err
	})
	return out, err
}
