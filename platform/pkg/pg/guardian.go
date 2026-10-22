// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"fmt"
)

// AtributosDelRol son los dos atributos de un rol de PostgreSQL que apagan la
// seguridad por filas para quien lo usa.
type AtributosDelRol struct {
	Nombre       string
	Superusuario bool
	BypassRLS    bool
}

// EludeRLS dice si con este rol no se evalúa NINGUNA política: PostgreSQL no las
// aplica a un superusuario ni a un rol con BYPASSRLS, ni siquiera con FORCE ROW
// LEVEL SECURITY, que solo sujeta al dueño de la tabla.
func (a AtributosDelRol) EludeRLS() bool { return a.Superusuario || a.BypassRLS }

// RolActual lee del catálogo los atributos del rol EFECTIVO de q: el de la
// conexión, o el asumido si q es una transacción que ya hizo SET LOCAL ROLE.
func RolActual(ctx context.Context, q Querier) (AtributosDelRol, error) {
	var a AtributosDelRol
	err := q.QueryRow(ctx,
		`SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&a.Nombre, &a.Superusuario, &a.BypassRLS)
	return a, err
}

// verificarNoEludeRLS es el guardián de arranque: lo llama Connect, que es por
// donde pasan todos los servicios.
//
// EXISTE PORQUE NADIE LO MIRABA. El piloto entregó a todos los servicios el
// usuario de arranque de la imagen de PostgreSQL, que es superusuario, y las
// políticas de catorce servicios dejaron de evaluarse sin un error ni una línea
// de registro (auditoría del 2026-10-06, INF-01). Solo Docs e Integrations lo
// comprobaban, dentro de AppRole.Resolve.
//
// Con los roles separados por servicio (2026-10-21), ningún servicio se conecta
// con superusuario ni con BYPASSRLS: si alguno recibe por error una credencial
// que elude la RLS, Connect falla en cerrado y el proceso no arranca.
func verificarNoEludeRLS(ctx context.Context, q Querier) error {
	a, err := RolActual(ctx, q)
	if err != nil {
		return fmt.Errorf("pg: comprobar si el rol de conexión elude la RLS: %w", err)
	}
	if a.EludeRLS() {
		return fmt.Errorf(
			"pg: el rol de conexión %q elude la seguridad por filas (superusuario=%t bypassrls=%t): "+
				"ninguna política RLS se evalúa con él",
			a.Nombre, a.Superusuario, a.BypassRLS,
		)
	}
	return nil
}
