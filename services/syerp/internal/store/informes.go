// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Tablero es un informe de Sytue AI guardado. Se guarda la CONSULTA y no los
// datos: al abrirlo se vuelve a ejecutar, así que un tablero de «este mes» enseña
// siempre el mes en curso.
type Tablero struct {
	ID        string          `json:"id"`
	Titulo    string          `json:"titulo"`
	Informe   json.RawMessage `json:"informe"`
	CreadoPor string          `json:"creado_por"`
	CreadoEn  time.Time       `json:"creado_en"`
}

// maxTableros son los que se listan.
//
// ponytail: los cien más recientes y sin paginar; paginar cuando una
// organización guarde más que eso.
const maxTableros = 100

// Tableros lista los tableros de la organización, del más reciente al más viejo.
func (s *Store) Tableros(ctx context.Context) ([]Tablero, error) {
	out := []Tablero{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text, titulo, informe, creado_por, created_at
			  FROM tableros
			 WHERE tenant = current_setting('app.tenant', true)
			 ORDER BY created_at DESC
			 LIMIT $1`, maxTableros)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var t Tablero
			if err := filas.Scan(&t.ID, &t.Titulo, &t.Informe, &t.CreadoPor, &t.CreadoEn); err != nil {
				return err
			}
			out = append(out, t)
		}
		return filas.Err()
	})
	return out, err
}

// GuardarTablero guarda un informe ya comprobado.
func (s *Store) GuardarTablero(ctx context.Context, titulo string, informe json.RawMessage, sub string) (Tablero, error) {
	t := Tablero{Titulo: titulo, Informe: informe, CreadoPor: sub}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO tableros (tenant, titulo, informe, creado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3)
			RETURNING id::text, created_at`, titulo, informe, sub).Scan(&t.ID, &t.CreadoEn)
	})
	return t, err
}

// BorrarTablero borra un tablero de quien lo creó; quien administra borra cualquiera.
// ErrNoEncontrado si no existe o no es suyo: a quien no puede borrarlo no se le
// confirma que exista.
func (s *Store) BorrarTablero(ctx context.Context, id, sub string, administra bool) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `
			DELETE FROM tableros
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND (creado_por = $2 OR $3)`,
			id, sub, administra)
		if err == nil && res.RowsAffected() == 0 {
			return ErrNoEncontrado
		}
		return err
	})
}

// LineaDeLibro es un apunte con los datos de su asiento, para los informes.
type LineaDeLibro struct {
	Fecha    time.Time
	Diario   string
	Ref      string
	Concepto string
	Cuenta   string
	Debe     int64
	Haber    int64
}

// LineasDeLibro devuelve los apuntes ASENTADOS más recientes, hasta `tope`.
func (s *Store) LineasDeLibro(ctx context.Context, tope int) ([]LineaDeLibro, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []LineaDeLibro
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT m.fecha, m.diario, m.ref, COALESCE(NULLIF(l.concepto, ''), m.concepto), l.cuenta, l.debe, l.haber
			  FROM moves m
			  JOIN move_lines l ON l.tenant = m.tenant AND l.move_id = m.id
			 WHERE m.tenant = $1 AND m.estado = 'asentado'
			 ORDER BY m.fecha DESC, m.id DESC
			 LIMIT $2`, tenant, tope)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var l LineaDeLibro
			if err := filas.Scan(&l.Fecha, &l.Diario, &l.Ref, &l.Concepto, &l.Cuenta, &l.Debe, &l.Haber); err != nil {
				return err
			}
			out = append(out, l)
		}
		return filas.Err()
	})
	return out, err
}
