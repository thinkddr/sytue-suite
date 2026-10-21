// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Recuerdo son las fotos de un mismo día de un año anterior.
type Recuerdo struct {
	Anyo  int
	Fotos []domain.Activo
}

// MaxPorRecuerdo acota lo que se enseña de cada año. Un recuerdo es una
// invitación a mirar, no la carpeta entera: con doce fotos hay tira de sobra y
// quien quiera más tiene la línea de tiempo.
const MaxPorRecuerdo = 12

// Recuerdos devuelve las fotos que esa persona hizo un día como hoy en años
// anteriores, agrupadas por año y de más reciente a más antiguo.
//
// SE COMPARA MES Y DÍA, no «hace 365 días». Un año no son 365 días y los
// bisiestos desplazarían el recuerdo un día cada cuatro años, que es justo la
// clase de error que nadie reporta y todo el mundo nota.
//
// SOLO CUENTA LA FECHA DE CAPTURA. Una foto colocada por su fecha de SUBIDA no
// se hizo ese día: sacarla como recuerdo sería inventarse un aniversario a
// partir del día en que alguien vació una tarjeta.
//
// ponytail: filtra con `extract()` sobre la línea de una persona, así que no
// usa `assets_timeline` y recorre sus filas. Para una biblioteca personal son
// milisegundos; si alguna llega a cientos de miles, el sitio es un índice sobre
// `(tenant, owner_sub, extract(month …), extract(day …))`.
func (s *Store) Recuerdos(ctx context.Context, duenyo string, hoy time.Time) ([]Recuerdo, error) {
	var out []Recuerdo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+`
			  FROM assets a
			 WHERE a.tenant = $1 AND a.owner_sub = $2 AND a.deleted_at IS NULL
			   AND a.taken_at IS NOT NULL
			   AND extract(month from a.taken_at) = $3
			   AND extract(day   from a.taken_at) = $4
			   AND extract(year  from a.taken_at) < $5
			 ORDER BY a.taken_at DESC, a.id DESC
			 LIMIT 500`, tenant, duenyo, int(hoy.Month()), hoy.Day(), hoy.Year())
		if err != nil {
			return err
		}
		defer rows.Close()

		porAnyo := map[int]*Recuerdo{}
		var orden []int
		for rows.Next() {
			a, err := scanActivo(rows, tenant)
			if err != nil {
				return err
			}
			anyo := a.TomadaEn.Year()
			r, ya := porAnyo[anyo]
			if !ya {
				r = &Recuerdo{Anyo: anyo}
				porAnyo[anyo] = r
				orden = append(orden, anyo)
			}
			if len(r.Fotos) < MaxPorRecuerdo {
				r.Fotos = append(r.Fotos, a)
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// `orden` sale de recorrer las filas, que ya vienen de más reciente a
		// más antigua: no hace falta ordenar otra vez.
		for _, anyo := range orden {
			out = append(out, *porAnyo[anyo])
		}
		return nil
	})
	return out, err
}
