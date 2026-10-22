// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// Asiento es un apunte doble tal como sale a la API.
type Asiento struct {
	ID       string    `json:"id"`
	Diario   string    `json:"diario"`
	Ref      string    `json:"ref"`
	Fecha    time.Time `json:"fecha"`
	Concepto string    `json:"concepto"`
	Moneda   string    `json:"moneda"`
	Estado   string    `json:"estado"`
	OrigenID string    `json:"origen_id,omitempty"`
	Apuntes  []Apunte  `json:"apuntes,omitempty"`
}

// Apunte es una línea del asiento. Importes en céntimos.
type Apunte struct {
	Cuenta   string `json:"cuenta"`
	Concepto string `json:"concepto,omitempty"`
	Debe     int64  `json:"debe"`
	Haber    int64  `json:"haber"`
	Tercero  string `json:"tercero,omitempty"`
}

// Asentar lleva un documento al libro diario: reserva su número de asiento,
// escribe el apunte doble y deja el documento confirmado. Todo en UNA
// transacción, así que o está el asiento entero y el documento confirmado, o no
// está ninguna de las dos cosas.
//
// El cuadre lo comprueba Postgres al confirmar (trigger `move_lines_cuadre`) y
// el «una sola vez» lo impide el índice único por documento: los dos controles
// que de verdad importan no dependen de este código.
func (s *Store) Asentar(ctx context.Context, m *catalogo.Modelo, id, ownerSub string) (Asiento, error) {
	if m.Contable == nil {
		return Asiento{}, fmt.Errorf("el modelo %q no lleva libro: %w", m.Clave, domain.ErrDatos)
	}
	var out Asiento
	err := s.escritura(ctx, func(tx pgx.Tx) error { return s.asentarEn(ctx, tx, m, id, ownerSub, &out) })
	return out, err
}

// asentarEn permite que el TPV reutilice todos los cerrojos del libro en su
// transacción: período abierto, aprobación, cuadre y emisión VERI*FACTU.
func (s *Store) asentarEn(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, id, ownerSub string, out *Asiento) error {
	var r Registro
	// FOR UPDATE: dos peticiones simultáneas sobre la misma factura se
	// serializan aquí; la segunda se encuentra el asiento ya hecho.
	if err := escanearRegistro(tx.QueryRow(ctx, `
		SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
		  FROM records
		 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL
		 FOR UPDATE`, id, m.Clave), &r); err != nil {
		return err
	}

	var ya int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM moves
		 WHERE tenant = current_setting('app.tenant', true) AND origen_id = $1::uuid AND estado <> 'anulado'`, id).Scan(&ya); err != nil {
		return err
	}
	if ya > 0 {
		return ErrYaAsentado
	}

	etiqueta := r.Etiqueta
	if etiqueta == "" {
		etiqueta = r.Ref
	}
	a, err := domain.AsientoDe(m, etiqueta, r.Data)
	if err != nil {
		return err
	}

	// La FECHA del asiento tiene que caer en período abierto. Va aquí, dentro
	// de la transacción que asienta: entre leerlo fuera y escribir cabe el
	// cierre de otra persona.
	if err := exigirPeriodoAbierto(ctx, tx, a.Fecha); err != nil {
		return err
	}
	// Y el IMPORTE tiene que estar por debajo del umbral del módulo, o traer
	// la aprobación de otra persona. Mismo sitio y por lo mismo.
	if err := exigirAprobacion(ctx, tx, m, id, ImporteDelAsiento(a), a.Moneda, huellaDe(r.Data)); err != nil {
		return err
	}

	ref, err := reservarRef(ctx, tx, "asiento:"+a.Diario, prefijoDiario(a.Diario),
		fmt.Sprintf("%04d", a.Fecha.Year()))
	if err != nil {
		return err
	}

	moveID, err := asentarDocumento(ctx, tx, a, ref, id, ownerSub, out)
	if err != nil {
		return err
	}

	// El documento pasa a confirmado si su flujo lo contempla. Si no, se queda
	// donde estaba: el libro no inventa estados que el modelo no declara. Una
	// factura que se emite así deja antes su registro de VERI*FACTU.
	if domain.EstadoValido(m, "confirmado") {
		if err := s.emitirVerifactu(ctx, tx, m.Clave, []string{id}, "confirmado", ownerSub); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE records SET estado = 'confirmado', updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id); err != nil {
			return err
		}
	}

	out.ID, out.Diario, out.Ref = moveID, a.Diario, ref
	out.Fecha, out.Concepto, out.Moneda = a.Fecha, a.Concepto, a.Moneda
	out.Estado, out.OrigenID = "asentado", id
	return nil
}

// asentarDocumento abre el asiento en borrador, escribe sus apuntes y lo cierra.
//
// EN BORRADOR PRIMERO, y no es una preferencia: el trigger
// `move_lines_inmutables` rechaza añadir apuntes a un asiento ya asentado, así
// que se abre, se llena y se cierra. Es como funciona un libro contable y ahora
// es como funciona la base. El cuadre lo comprueba Postgres al confirmar la
// transacción, no este código.
func asentarDocumento(ctx context.Context, tx pgx.Tx, a domain.Asiento, ref, origenID, ownerSub string, out *Asiento) (string, error) {
	var moveID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO moves (tenant, diario, ref, fecha, concepto, moneda, origen_id,
		                   estado, owner_sub)
		VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6::uuid,
		        'borrador', $7)
		RETURNING id::text`,
		a.Diario, ref, a.Fecha, a.Concepto, a.Moneda, origenID, ownerSub).Scan(&moveID); err != nil {
		return "", err
	}
	for _, ap := range a.Apuntes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO move_lines (move_id, tenant, cuenta, concepto, debe, haber, tercero_id)
			VALUES ($1::uuid, current_setting('app.tenant', true), $2, $3, $4, $5, $6::uuid)`,
			moveID, ap.Cuenta, ap.Concepto, ap.Debe, ap.Haber, nuloSiVacio(ap.Tercero)); err != nil {
			return "", err
		}
		out.Apuntes = append(out.Apuntes, Apunte{
			Cuenta: ap.Cuenta, Concepto: ap.Concepto,
			Debe: ap.Debe, Haber: ap.Haber, Tercero: ap.Tercero,
		})
	}
	if _, err := tx.Exec(ctx, `
		UPDATE moves SET estado = 'asentado', posted_at = now()
		 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, moveID); err != nil {
		return "", err
	}
	return moveID, nil
}

// Anular marca un asiento como anulado. No borra ni reescribe nada: corregir un
// asiento asentado se hace con otro que lo revierte, y eso lo impone el trigger
// `moves_inmutables` — aquí solo se ejerce la única transición que permite.
func (s *Store) Anular(ctx context.Context, asientoID, motivo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Anular es tocar el libro: si su período está cerrado, no se toca. Que
		// la anulación sea «la única transición permitida» no la saca del
		// ejercicio ya declarado — es justo el agujero por el que un balance
		// cerrado cambiaba solo.
		var fecha time.Time
		if err := tx.QueryRow(ctx, `SELECT fecha FROM moves WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, asientoID).Scan(&fecha); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		if err := exigirPeriodoAbierto(ctx, tx, fecha); err != nil {
			return err
		}
		etiq, err := tx.Exec(ctx, `
			UPDATE moves
			   SET estado = 'anulado',
			       concepto = concepto || ' · ' || $2
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND estado = 'asentado'`, asientoID, motivo)
		if err != nil {
			return err
		}
		if etiq.RowsAffected() == 0 {
			return ErrNoEncontrado
		}
		return nil
	})
}

// Libro devuelve los asientos de un rango de fechas, con sus apuntes.
func (s *Store) Libro(ctx context.Context, desde, hasta time.Time, limite int) ([]Asiento, error) {
	if limite <= 0 || limite > limiteMax {
		limite = 50
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []Asiento
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// `m.tenant = $1` por lo mismo que en `Listar`: es la primera columna de
		// `moves_libro` y sin ella el planificador no puede recorrer el índice en
		// orden. Ver el comentario de `tenantDe`.
		filas, err := tx.Query(ctx, `
			SELECT m.id::text AS mid, m.diario, m.ref, m.fecha, m.concepto, m.moneda, m.estado,
			       COALESCE(m.origen_id::text, '')
			  FROM moves m
			 WHERE m.tenant = $1
			   AND ($2::date IS NULL OR m.fecha >= $2::date)
			   AND ($3::date IS NULL OR m.fecha <= $3::date)
			 ORDER BY m.fecha DESC, m.id DESC
			 LIMIT $4`, tenant, nuloSiCero(desde), nuloSiCero(hasta), limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		porID := map[string]int{}
		for filas.Next() {
			var a Asiento
			if err := filas.Scan(&a.ID, &a.Diario, &a.Ref, &a.Fecha, &a.Concepto,
				&a.Moneda, &a.Estado, &a.OrigenID); err != nil {
				return err
			}
			porID[a.ID] = len(out)
			out = append(out, a)
		}
		if err := filas.Err(); err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}

		// Los apuntes de TODOS los asientos de la página en una consulta: un
		// libro que hace una consulta por asiento no se puede paginar.
		ids := make([]string, 0, len(out))
		for _, a := range out {
			ids = append(ids, a.ID)
		}
		lineas, err := tx.Query(ctx, `
			SELECT move_id::text AS mid, cuenta, concepto, debe, haber,
			       COALESCE(tercero_id::text, '')
			  FROM move_lines
			 WHERE tenant = $1 AND move_id = ANY($2::uuid[])
			 ORDER BY id`, tenant, ids)
		if err != nil {
			return err
		}
		defer lineas.Close()
		for lineas.Next() {
			var moveID string
			var ap Apunte
			if err := lineas.Scan(&moveID, &ap.Cuenta, &ap.Concepto, &ap.Debe,
				&ap.Haber, &ap.Tercero); err != nil {
				return err
			}
			if i, ok := porID[moveID]; ok {
				out[i].Apuntes = append(out[i].Apuntes, ap)
			}
		}
		return lineas.Err()
	})
	return out, err
}

// SaldoCuenta es el saldo acumulado de una cuenta del plan.
type SaldoCuenta struct {
	Cuenta string `json:"cuenta"`
	Debe   int64  `json:"debe"`
	Haber  int64  `json:"haber"`
	Saldo  int64  `json:"saldo"`
}

// Balance devuelve el sumas y saldos: una fila por cuenta con movimiento.
// Solo cuenta lo ASENTADO; los borradores y lo anulado no son contabilidad.
func (s *Store) Balance(ctx context.Context, hasta time.Time) ([]SaldoCuenta, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []SaldoCuenta
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT l.cuenta, sum(l.debe), sum(l.haber), sum(l.debe) - sum(l.haber)
			  FROM move_lines l
			  JOIN moves m ON m.id = l.move_id AND m.tenant = l.tenant
			 WHERE l.tenant = $1
			   AND m.estado = 'asentado'
			   AND ($2::date IS NULL OR m.fecha <= $2::date)
			 GROUP BY l.cuenta
			 ORDER BY l.cuenta`, tenant, nuloSiCero(hasta))
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var c SaldoCuenta
			if err := filas.Scan(&c.Cuenta, &c.Debe, &c.Haber, &c.Saldo); err != nil {
				return err
			}
			out = append(out, c)
		}
		return filas.Err()
	})
	return out, err
}

// AsientosDe devuelve los asientos generados por un documento.
//
// El documento tiene que ser de `modelo`: la ruta es /{modelo}/{id}/asientos y el
// permiso se mira sobre el modelo de la URL. Sin atarlo, quien leía contactos
// pedía /contacto/<id de una nómina>/asientos y leía la nómina (NEG-17).
func (s *Store) AsientosDe(ctx context.Context, modelo, documentoID string) ([]Asiento, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []Asiento
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text AS mid, diario, ref, fecha, concepto, moneda, estado
			  FROM moves
			 WHERE tenant = $1 AND origen_id = $2::uuid
			   AND EXISTS (SELECT 1 FROM records r WHERE r.tenant = $1 AND r.id = $2::uuid AND r.model = $3)
			 ORDER BY fecha, id`, tenant, documentoID, modelo)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var a Asiento
			if err := filas.Scan(&a.ID, &a.Diario, &a.Ref, &a.Fecha, &a.Concepto,
				&a.Moneda, &a.Estado); err != nil {
				return err
			}
			a.OrigenID = documentoID
			out = append(out, a)
		}
		return filas.Err()
	})
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	return out, err
}

// prefijoDiario convierte el nombre del diario en el prefijo de sus asientos
// (ventas → VENTAS-2026-00001). Seis letras como mucho, que es lo que admite la
// forma de la referencia.
func prefijoDiario(diario string) string {
	p := ""
	for _, r := range diario {
		if r >= 'a' && r <= 'z' {
			p += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			p += string(r)
		}
		if len(p) == 6 {
			break
		}
	}
	if len(p) < 2 {
		return "DIARIO"
	}
	return p
}
