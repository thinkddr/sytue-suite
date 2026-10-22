// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Organizacion es el perfil fiscal de la organización: lo que decide su
// calendario de obligaciones y lo que sale como emisor en una factura.
type Organizacion struct {
	Nombre    string `json:"nombre"`
	NIF       string `json:"nif"`
	Direccion string `json:"direccion"`
	CP        string `json:"cp"`
	Ciudad    string `json:"ciudad"`
	Pais      string `json:"pais"`
	Forma     string `json:"forma"`
	// Condiciones son los interruptores del perfil (iva, empleados…). Solo se
	// guardan las encendidas: una apagada y una ausente significan lo mismo.
	Condiciones   map[string]bool `json:"condiciones"`
	ActualizadoEn time.Time       `json:"actualizado_en"`
}

// Organizacion devuelve el perfil, o ErrNoEncontrado si aún no se ha hecho el
// asistente de arranque: es lo que le dice a la interfaz que lo enseñe.
func (s *Store) Organizacion(ctx context.Context) (Organizacion, error) {
	var o Organizacion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var cond []byte
		err := tx.QueryRow(ctx, `
			SELECT nombre, nif, direccion, cp, ciudad, pais, forma, condiciones, updated_at
			  FROM organizacion
			 WHERE tenant = current_setting('app.tenant', true)`).
			Scan(&o.Nombre, &o.NIF, &o.Direccion, &o.CP, &o.Ciudad, &o.Pais, &o.Forma, &cond, &o.ActualizadoEn)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(cond, &o.Condiciones)
	})
	if o.Condiciones == nil {
		o.Condiciones = map[string]bool{}
	}
	return o, err
}

// GuardarOrganizacion crea o sustituye el perfil. Es un upsert por tenant: el
// asistente se puede repetir y siempre deja UN perfil.
func (s *Store) GuardarOrganizacion(ctx context.Context, o Organizacion, sub string) (Organizacion, error) {
	cond := map[string]bool{}
	for k, v := range o.Condiciones {
		if v {
			cond[k] = true
		}
	}
	crudo, err := json.Marshal(cond)
	if err != nil {
		return Organizacion{}, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Con VERI*FACTU activado el NIF no cambia: es el del obligado de toda la
		// cadena de registros. Escrito de otra forma (con guiones o espacios), sí.
		err := tx.QueryRow(ctx, `
			INSERT INTO organizacion (tenant, nombre, nif, direccion, cp, ciudad, pais, forma, condiciones, actualizado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (tenant) DO UPDATE
			   SET nombre = EXCLUDED.nombre, nif = EXCLUDED.nif, direccion = EXCLUDED.direccion,
			       cp = EXCLUDED.cp, ciudad = EXCLUDED.ciudad, pais = EXCLUDED.pais,
			       forma = EXCLUDED.forma, condiciones = EXCLUDED.condiciones,
			       verifactu_cert_pem = CASE WHEN upper(regexp_replace(organizacion.nif, '[^A-Za-z0-9]', '', 'g')) = upper(regexp_replace(EXCLUDED.nif, '[^A-Za-z0-9]', '', 'g')) THEN organizacion.verifactu_cert_pem ELSE '' END,
			       verifactu_key_enc = CASE WHEN upper(regexp_replace(organizacion.nif, '[^A-Za-z0-9]', '', 'g')) = upper(regexp_replace(EXCLUDED.nif, '[^A-Za-z0-9]', '', 'g')) THEN organizacion.verifactu_key_enc ELSE '' END,
			       verifactu_cert_nif = CASE WHEN upper(regexp_replace(organizacion.nif, '[^A-Za-z0-9]', '', 'g')) = upper(regexp_replace(EXCLUDED.nif, '[^A-Za-z0-9]', '', 'g')) THEN organizacion.verifactu_cert_nif ELSE '' END,
			       verifactu_sello = CASE WHEN upper(regexp_replace(organizacion.nif, '[^A-Za-z0-9]', '', 'g')) = upper(regexp_replace(EXCLUDED.nif, '[^A-Za-z0-9]', '', 'g')) THEN organizacion.verifactu_sello ELSE false END,
			       actualizado_por = EXCLUDED.actualizado_por, updated_at = now()
			 WHERE organizacion.verifactu_desde IS NULL
			    OR upper(regexp_replace(organizacion.nif, '[^A-Za-z0-9]', '', 'g'))
			     = upper(regexp_replace(EXCLUDED.nif, '[^A-Za-z0-9]', '', 'g'))
			RETURNING updated_at`,
			o.Nombre, o.NIF, o.Direccion, o.CP, o.Ciudad, o.Pais, o.Forma, crudo, sub).Scan(&o.ActualizadoEn)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNIFConVerifactu
		}
		return err
	})
	o.Condiciones = cond
	return o, err
}

// BalanceEntre es el sumas y saldos acotado a un periodo: lo que necesita una
// obligación que se calcula (el IVA del trimestre, el resultado del año). Solo
// lo asentado, como `Balance`.
func (s *Store) BalanceEntre(ctx context.Context, desde, hasta time.Time) ([]SaldoCuenta, error) {
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
			   AND m.fecha >= $2::date AND m.fecha <= $3::date
			 GROUP BY l.cuenta
			 ORDER BY l.cuenta`, tenant, desde, hasta)
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
