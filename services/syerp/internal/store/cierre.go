// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PERÍODOS CONTABLES (migración 0094).
//
// El libro ya era inmutable apunte a apunte, pero no tenía fecha de corte: se
// podía asentar en un ejercicio ya declarado y el balance cambiaba solo. Cada
// asiento nuevo es, por separado, válido; lo que no es válido es la FECHA.

// ErrPeriodoCerrado es intentar tocar el libro en un período ya cerrado.
var ErrPeriodoCerrado = errors.New("el período contable está cerrado")

// Cierre es un movimiento del libro de cierres: un cierre o una reapertura.
type Cierre struct {
	Hasta  *time.Time `json:"hasta,omitempty"`
	Accion string     `json:"accion"`
	Motivo string     `json:"motivo,omitempty"`
	Por    string     `json:"por,omitempty"`
	En     time.Time  `json:"en"`
}

// CierreVigente dice hasta cuándo está cerrado el libro. `cerrado` es falso
// cuando está abierto entero, que es un estado normal y no la ausencia de una
// respuesta: por eso va como bandera y no como un puntero nulo, que quien lo
// recibe confundiría con «no se pudo leer».
func (s *Store) CierreVigente(ctx context.Context) (hasta time.Time, cerrado bool, err error) {
	var p *time.Time
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return cierreVigenteTx(ctx, tx, &p)
	})
	if err != nil {
		return time.Time{}, false, err
	}
	if p == nil {
		return time.Time{}, false, nil
	}
	return *p, true, nil
}

func cierreVigenteTx(ctx context.Context, tx pgx.Tx, hasta **time.Time) error {
	err := tx.QueryRow(ctx, `
		SELECT hasta FROM cierre_contable
		 WHERE tenant = current_setting('app.tenant', true)
		 ORDER BY en DESC, id DESC LIMIT 1`).Scan(hasta)
	if errors.Is(err, pgx.ErrNoRows) {
		*hasta = nil
		return nil
	}
	return err
}

// exigirPeriodoAbierto falla si `fecha` cae en el período cerrado.
//
// Va DENTRO de la transacción que asienta, y no en una comprobación previa del
// handler, por la misma razón que el cuadre: entre leer y escribir cabe un
// cierre de otra persona, y la fecha de corte es justamente el dato que no
// puede leerse fuera de la transacción que la respeta.
func exigirPeriodoAbierto(ctx context.Context, tx pgx.Tx, fecha time.Time) error {
	var hasta *time.Time
	if err := cierreVigenteTx(ctx, tx, &hasta); err != nil {
		return err
	}
	if hasta == nil {
		return nil
	}
	// Comparación por día: `hasta` es la última fecha CERRADA, así que el día
	// siguiente ya se puede asentar.
	if !fecha.Truncate(24 * time.Hour).After(hasta.Truncate(24 * time.Hour)) {
		return ErrPeriodoCerrado
	}
	return nil
}

// Cerrar mueve la fecha de corte hacia adelante.
//
// No deja retroceder: para eso está Reabrir, que es excepcional, pide motivo y
// se ve en el historial. Sin esta asimetría, «cerrar» con una fecha anterior
// sería una reapertura silenciosa con otro nombre.
func (s *Store) Cerrar(ctx context.Context, hasta time.Time, motivo, por string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var actual *time.Time
		if err := cierreVigenteTx(ctx, tx, &actual); err != nil {
			return err
		}
		if actual != nil && !hasta.After(*actual) {
			return ErrPeriodoCerrado
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO cierre_contable (tenant, hasta, accion, motivo, por)
			VALUES (current_setting('app.tenant', true), $1, 'cierre', $2, $3)`,
			hasta, motivo, por)
		return err
	})
}

// Reabrir retrocede la fecha de corte, o la quita del todo (hasta = nil).
// Siempre con motivo: es la operación que deja volver a tocar un ejercicio
// declarado, y la única defensa que tiene es que se vea quién y por qué.
func (s *Store) ReabrirPeriodo(ctx context.Context, hasta *time.Time, motivo, por string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO cierre_contable (tenant, hasta, accion, motivo, por)
			VALUES (current_setting('app.tenant', true), $1, 'reapertura', $2, $3)`,
			hasta, motivo, por)
		return err
	})
}

// HistorialDeCierres son los cierres y reaperturas, del más reciente al más
// antiguo. Tope de 200: son unos pocos al año.
func (s *Store) HistorialDeCierres(ctx context.Context) ([]Cierre, error) {
	out := make([]Cierre, 0, 16)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT hasta, accion, motivo, por, en FROM cierre_contable
			 WHERE tenant = current_setting('app.tenant', true)
			 ORDER BY en DESC, id DESC LIMIT 200`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var c Cierre
			if err := filas.Scan(&c.Hasta, &c.Accion, &c.Motivo, &c.Por, &c.En); err != nil {
				return err
			}
			out = append(out, c)
		}
		return filas.Err()
	})
	return out, err
}
