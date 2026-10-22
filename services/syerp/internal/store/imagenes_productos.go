// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ImagenesProductos devuelve el último adjunto de imagen de cada producto vivo.
// Una consulta para toda la página, sin bajar contenidos ni consultar por tarjeta.
func (s *Store) ImagenesProductos(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT DISTINCT ON (a.registro) a.registro, a.id::text
			  FROM adjuntos a
			  JOIN records r ON r.tenant = a.tenant AND r.id::text = a.registro
			 WHERE a.tenant = $1 AND a.modelo = 'producto'
			   AND a.registro = ANY($2::text[])
			   AND a.tipo IN ('image/jpeg', 'image/png', 'image/webp')
			   AND r.model = 'producto' AND r.deleted_at IS NULL
			   AND r.id = ANY($2::uuid[])
			 ORDER BY a.registro, a.creado_en DESC, a.id DESC`, tenant, ids)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var registro, adjunto string
			if err := filas.Scan(&registro, &adjunto); err != nil {
				return err
			}
			out[registro] = adjunto
		}
		return filas.Err()
	})
	return out, err
}
