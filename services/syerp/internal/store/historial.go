// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// El historial de cada registro (migración 0011) lo escribe un trigger de la base;
// aquí solo se dice quién actúa, se lee y se añaden notas.

// escritura abre la transacción de la organización para cambiar registros y
// apunta en `app.sub` quién lo hace: es lo que firma cada entrada del historial.
// Sin sesión (un proceso del propio servicio) la entrada queda sin autor.
func (s *Store) escritura(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if c, ok := authn.FromContext(ctx); ok && c.Subject != "" {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.sub', $1, true)`, c.Subject); err != nil {
				return err
			}
		}
		return fn(tx)
	})
}

// Cambio es el valor de un campo antes y después.
type Cambio struct {
	Antes   any `json:"antes"`
	Despues any `json:"despues"`
}

// EntradaHistorial es una línea del historial de un registro.
type EntradaHistorial struct {
	ID       string            `json:"id"`
	Tipo     string            `json:"tipo"`
	Sub      string            `json:"sub,omitempty"`
	Nombre   string            `json:"nombre,omitempty"`
	Cambios  map[string]Cambio `json:"cambios,omitempty"`
	Texto    string            `json:"texto,omitempty"`
	CreadoEn time.Time         `json:"creado_en"`
}

// ErrNotaVacia: una nota sin texto no se guarda.
var ErrNotaVacia = errors.New("la nota está vacía")

const columnasHistorial = `h.id::text, h.tipo, h.sub, COALESCE(NULLIF(u.nombre, ''), u.email, ''), h.cambios, h.texto, h.creado_en`

// Historial devuelve lo que ha pasado con un registro, lo último primero. El
// nombre de quien actuó sale de quienes han entrado en el ERP.
func (s *Store) Historial(ctx context.Context, modelo, registro string, limite int) ([]EntradaHistorial, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	if limite <= 0 || limite > 200 {
		limite = 100
	}
	out := []EntradaHistorial{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT `+columnasHistorial+`
			  FROM historial h
			  LEFT JOIN usuarios u ON u.tenant = h.tenant AND u.sub = h.sub
			 WHERE h.tenant = $1 AND h.modelo = $2 AND h.registro = $3::uuid
			 ORDER BY h.creado_en DESC, h.id DESC
			 LIMIT $4`, tenant, modelo, registro, limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			e, err := escanearEntrada(filas)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return filas.Err()
	})
	return out, err
}

// Anotar deja una nota en el historial de un registro que existe.
func (s *Store) Anotar(ctx context.Context, modelo, registro, sub, texto string) (EntradaHistorial, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return EntradaHistorial{}, ErrNotaVacia
	}
	var e EntradaHistorial
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		fila := tx.QueryRow(ctx, `
			WITH nueva AS (
				INSERT INTO historial (tenant, modelo, registro, sub, tipo, texto)
				SELECT r.tenant, r.model, r.id, $3, 'nota', $4
				  FROM records r
				 WHERE r.tenant = current_setting('app.tenant', true)
				   AND r.id = $2::uuid AND r.model = $1
				RETURNING *
			)
			SELECT `+columnasHistorial+`
			  FROM nueva h
			  LEFT JOIN usuarios u ON u.tenant = h.tenant AND u.sub = h.sub`,
			modelo, registro, sub, truncarTexto(texto, 5000))
		var err error
		e, err = escanearEntrada(fila)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEncontrado
		}
		return err
	})
	return e, err
}

func escanearEntrada(fila pgx.Row) (EntradaHistorial, error) {
	var e EntradaHistorial
	var cambios []byte
	if err := fila.Scan(&e.ID, &e.Tipo, &e.Sub, &e.Nombre, &cambios, &e.Texto, &e.CreadoEn); err != nil {
		return e, err
	}
	if len(cambios) > 2 {
		if err := json.Unmarshal(cambios, &e.Cambios); err != nil {
			return e, err
		}
	}
	return e, nil
}
