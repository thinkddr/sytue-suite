// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
)

// «Pedir acceso de edición» de un módulo (0096), con el contrato común de
// platform/pkg/peticiones. Pide quien tiene `lectura`; concede la administración de la
// organización (owner o admin), que lo sube a `usuario`.
var tablaPeticiones = peticiones.Tabla{Nombre: "erp_peticiones_edicion", Recurso: "recurso", PorTenant: true}

// PedirEdicion deja la petición y devuelve la administración a la que avisar (los
// owner y admin que han entrado en el ERP). nueva=false si ya estaba pedida.
func (s *Store) PedirEdicion(ctx context.Context, modulo string, p peticiones.Peticion) (nueva bool, admins []string, err error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return false, nil, err
	}
	p.Recurso = modulo
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		if nueva, e = tablaPeticiones.Pedir(ctx, tx, tenant, p); e != nil || !nueva {
			return e
		}
		filas, e := tx.Query(ctx, `SELECT sub FROM usuarios WHERE tenant = $1 AND roles && ARRAY['owner', 'admin'] AND sub <> $2`, tenant, p.Sujeto)
		if e != nil {
			return e
		}
		admins, e = pgx.CollectRows(filas, pgx.RowTo[string])
		return e
	})
	return nueva, admins, err
}

// PeticionesEdicion son las pendientes de un módulo. La ruta ya exige owner o admin.
func (s *Store) PeticionesEdicion(ctx context.Context, modulo string) ([]peticiones.Peticion, error) {
	var out []peticiones.Peticion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = tablaPeticiones.Listar(ctx, tx, modulo)
		return e
	})
	return out, err
}

// ResolverPeticionEdicion borra la petición y, si se concede, pone a quien la pidió
// en `usuario` en ese módulo, en la MISMA transacción. Nunca baja un nivel: si ya
// tenía `usuario` o `administrador`, se queda. ErrSinPeticion si no la había.
func (s *Store) ResolverPeticionEdicion(ctx context.Context, modulo, sujeto string, concede bool, por string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var nombre, correo string
		e := tx.QueryRow(ctx, `SELECT nombre, correo FROM erp_peticiones_edicion WHERE tenant = current_setting('app.tenant', true) AND recurso = $1 AND sujeto = $2`,
			modulo, sujeto).Scan(&nombre, &correo)
		if e != nil {
			return e
		}
		if _, e := tablaPeticiones.Quitar(ctx, tx, modulo, sujeto); e != nil {
			return e
		}
		if !concede {
			return nil
		}
		_, e = tx.Exec(ctx, `
			INSERT INTO permisos (tenant, sub, modulo, nivel, email, nombre, actualizado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, 'usuario', $3, $4, $5)
			ON CONFLICT (tenant, sub, modulo) DO UPDATE
			   SET nivel = 'usuario', actualizado_por = EXCLUDED.actualizado_por, actualizado_en = now()
			 WHERE permisos.nivel IN ('ninguno', 'lectura')`,
			sujeto, modulo, correo, nombre, por)
		return e
	})
}

// ModulosPedidos son los módulos en los que `sub` tiene una petición pendiente.
func (s *Store) ModulosPedidos(ctx context.Context, sub string) ([]string, error) {
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, e := tx.Query(ctx, `SELECT recurso FROM erp_peticiones_edicion WHERE tenant = current_setting('app.tenant', true) AND sujeto = $1`, sub)
		if e != nil {
			return e
		}
		out, e = pgx.CollectRows(filas, pgx.RowTo[string])
		return e
	})
	return out, err
}
