// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// DailyNotebook mantiene un diario por persona, organización y día. La apertura
// y el traslado de pendientes son UNA transacción. Abrirlo otra vez no duplica
// notas ni reescribe el cuaderno. El bloqueo por persona también serializa dos
// dispositivos que abren días distintos al mismo tiempo.
func (s *Store) DailyNotebook(ctx context.Context, sub, date, title string, carry bool) (domain.Notebook, error) {
	t := tenancy.MustFrom(ctx)
	var nb domain.Notebook
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		if nb, err = abrirDiario(ctx, tx, t.Slug, sub, date, title); err != nil || !carry {
			return err
		}
		_, err = trasladar(ctx, tx, t.Slug, sub, nb, date, nil)
		return err
	})
	return nb, err
}

// abrirDiario devuelve el diario de ese día, creándolo si no existe, con el
// bloqueo por persona tomado hasta el final de la transacción.
func abrirDiario(ctx context.Context, tx pgx.Tx, slug, sub, date, title string) (domain.Notebook, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		slug+":daily:"+sub); err != nil {
		return domain.Notebook{}, err
	}
	nb, err := scanNotebook(tx.QueryRow(ctx, `SELECT `+nbColsBase+`
        FROM notebooks WHERE tenant=$1 AND owner_sub=$2 AND daily_date=$3 AND deleted_at IS NULL`,
		slug, sub, date), slug, sub)
	if isNoRows(err) {
		nb, err = scanNotebook(tx.QueryRow(ctx, `INSERT INTO notebooks
            (tenant, owner_sub, title, emoji, color, layout, daily_date, position)
            VALUES ($1,$2,$3,'','#86198f','list',$4,
                (SELECT COALESCE(max(position),0)+1 FROM notebooks WHERE tenant=$1 AND owner_sub=$2))
            RETURNING `+nbColsBase, slug, sub, title, date), slug, sub)
	}
	return nb, err
}

// DailyRange devuelve los días con notas de `[from, to]`, en orden, con sus cuentas y,
// si se piden, sus notas. Es UNA transacción para toda la semana o el tramo de días.
func (s *Store) DailyRange(ctx context.Context, sub, from, to string, notas bool) ([]domain.DailyDay, error) {
	return s.diasDelDiario(ctx, sub, `nb.daily_date BETWEEN $3 AND $4`, "ASC", notas, from, to)
}

// ThisDay es «Este día»: el mismo día y mes de años anteriores, del más reciente al
// más antiguo, con sus notas.
func (s *Store) ThisDay(ctx context.Context, sub, date string) ([]domain.DailyDay, error) {
	return s.diasDelDiario(ctx, sub, `substr(nb.daily_date, 6) = substr($3, 6) AND nb.daily_date < $3`, "DESC", true, date)
}

// diasDelDiario: `filtro` y `orden` son constantes de este fichero, nunca entrada.
// Solo los diarios de quien pregunta y, dentro, lo que ve de ellos: las notas privadas
// de otra persona no cuentan ni viajan. Los diarios cifrados dan cuentas pero no notas,
// que aquí serían criptogramas sin la clave.
func (s *Store) diasDelDiario(ctx context.Context, sub, filtro, orden string, notas bool, args ...any) ([]domain.DailyDay, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.DailyDay
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT nb.id, nb.daily_date, nb.encrypted, count(n.id),
			       count(n.id) FILTER (WHERE n.kind = 'todo' AND NOT n.checked AND n.cancelled_at IS NULL)
			  FROM notebooks nb
			  JOIN notes n ON n.notebook_id = nb.id AND n.tenant = nb.tenant AND n.deleted_at IS NULL
			       AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)
			 WHERE nb.tenant = $1 AND nb.owner_sub = $2 AND nb.daily_date <> '' AND nb.deleted_at IS NULL
			   AND `+filtro+`
			 GROUP BY nb.id ORDER BY nb.daily_date `+orden+` LIMIT `+strconv.Itoa(domain.MaxDiasCuentas),
			append([]any{t.Slug, sub}, args...)...)
		if err != nil {
			return err
		}
		var conNotas []uuid.UUID
		porCuaderno := map[uuid.UUID]int{}
		for rows.Next() {
			var d domain.DailyDay
			var cifrado bool
			if err := rows.Scan(&d.NotebookID, &d.Date, &cifrado, &d.Total, &d.Pending); err != nil {
				rows.Close()
				return err
			}
			if notas && !cifrado {
				conNotas = append(conNotas, d.NotebookID)
				porCuaderno[d.NotebookID] = len(out)
			}
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil || len(conNotas) == 0 {
			return err
		}
		// ponytail: tope de 2000 notas por respuesta; más que eso en 31 días es un
		// volcado, no un diario, y pediría paginar por día.
		nrows, err := tx.Query(ctx, `
			SELECT `+noteCols+`
			  FROM notes n
			 WHERE n.tenant = $1 AND n.notebook_id = ANY($2) AND n.deleted_at IS NULL
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $3)
			 ORDER BY n.pinned DESC, n.col, n.position, n.created_at LIMIT 2000`, t.Slug, conNotas, sub)
		if err != nil {
			return err
		}
		var todas []domain.Note
		for nrows.Next() {
			n, err := scanNote(nrows, t.Slug)
			if err != nil {
				nrows.Close()
				return err
			}
			todas = append(todas, n)
		}
		nrows.Close()
		if err := nrows.Err(); err != nil {
			return err
		}
		if err := s.adjuntarEnPaquete(ctx, tx, t.Slug, todas); err != nil {
			return err
		}
		for _, n := range todas {
			d := &out[porCuaderno[n.NotebookID]]
			d.Notes = append(d.Notes, n)
		}
		return nil
	})
	return out, err
}
