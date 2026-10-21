// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La etiqueta es OBLIGATORIA aquí: `Harness` vive en pgtest.go, que sí la lleva.
// Sin ella este fichero se compila siempre y el resto del paquete no, así que
// `go build ./...` fallaba con «undefined: Harness» —y con él el gate de
// pre-push y el CI— para cualquiera que tocara el repositorio.

package pgtest

import (
	"context"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// ExigirRLSForzada comprueba que TODA tabla multi-tenant tenga la RLS habilitada Y
// FORZADA, salvo las que se declaren excepción explícita.
//
// «Multi-tenant» es «lleva columna `tenant` O `org_id`». Son los DOS nombres que usa la
// casa para lo mismo y hay que mirar los dos: mirando solo `tenant`, de las quince tablas
// de `integrations` este test veía UNA —su `events_outbox`, que además es la excepción— y
// las once por organización se quedaban sin comprobar. Un invariante que solo cubre la
// mitad del esquema es peor que ninguno: sale verde y se lee como cobertura.
//
// ponytail: dos nombres cableados, no un parámetro. El día que aparezca un tercero, se
// añade aquí y lo ven los diez servicios de golpe; un parámetro por llamante garantiza
// que el servicio nuevo se olvide de pasarlo.
//
// Por qué la distinción importa, y por qué no basta con ENABLE:
//
//	ENABLE aplica la política a todos MENOS al PROPIETARIO de la tabla. Y en esta suite
//	el propietario es el rol de la APLICACIÓN: las migraciones se ejecutan con la misma
//	`DatabaseURL` que sirve las peticiones (cada `main.go` llama a `pg.MigrateUp` con
//	ella), así que el rol que crea las tablas es el mismo que luego consulta. Sobre una
//	tabla con solo ENABLE, la política de aislamiento por tenant NO se evalúa: un SELECT
//	con `app.tenant='acme'` devuelve también las filas de las demás organizaciones.
//
// El mecanismo se comprobó en un PostgreSQL real reproduciendo el montaje —rol de
// aplicación que crea su propia tabla—: con solo ENABLE se veían las filas de otra
// organización; con FORCE, cada una las suyas.
//
// El esquema actual está BIEN: todas las tablas por organización llevan ya ENABLE+FORCE.
// Este test no arregla nada, evita una regresión, y existe por cómo estuvo a punto de
// colarse una: auditando el esquema con un `grep` de `FORCE ROW LEVEL SECURITY` salieron
// diez tablas «sin proteger» que en realidad lo estaban — la declaración llevaba DOS
// espacios («FORCE  ROW») y el patrón no casaba. La conclusión contraria (creer que falta
// cuando está) cuesta una migración inútil; la de verdad peligrosa es la simétrica: dar
// por protegida una tabla porque el grep encontró la línea en OTRO sitio del fichero.
//
// Por eso se comprueba contra el CATÁLOGO de PostgreSQL y no leyendo el SQL: el catálogo
// dice lo que la base hace, no lo que el fichero parece decir. Una tabla nueva por
// organización falla este test el día que se crea, que es cuando sale barato.
//
// Entran las tablas ordinarias ('r') y también las PARTICIONADAS ('p'): `analytics_events`
// —la tabla más caliente de la plataforma— es partitioned y con el filtro anterior el
// catálogo no la devolvía, así que quedaba fuera del invariante justo la que más importa.
// Sus particiones sí se excluyen (`relispartition`): la RLS que se evalúa es la de la
// relación NOMBRADA en la consulta, y el servicio siempre consulta por la padre.
func ExigirRLSForzada(t *testing.T, h *Harness, excepciones map[string]string) {
	t.Helper()
	ctx := context.Background()

	rows, err := h.Pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind IN ('r', 'p')
		   AND NOT c.relispartition
		   AND n.nspname = 'public'
		   AND EXISTS (
		         SELECT 1 FROM pg_attribute a
		          WHERE a.attrelid = c.oid AND a.attname IN ('tenant', 'org_id')
		            AND a.attnum > 0 AND NOT a.attisdropped)
		 ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("pgtest: leer el catálogo de tablas: %v", err)
	}
	defer rows.Close()

	var fallos []string
	var vistas int
	for rows.Next() {
		var nombre string
		var enable, force bool
		if err := rows.Scan(&nombre, &enable, &force); err != nil {
			t.Fatalf("pgtest: leer fila del catálogo: %v", err)
		}
		vistas++
		if motivo, exenta := excepciones[nombre]; exenta {
			if motivo == "" {
				t.Errorf("la excepción de %q no explica por qué; una exención sin motivo se hereda sin mirar", nombre)
			}
			continue
		}
		if !enable || !force {
			fallos = append(fallos,
				fmt.Sprintf("%s (enable=%v force=%v)", nombre, enable, force))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pgtest: recorrer el catálogo: %v", err)
	}
	if vistas == 0 {
		t.Fatal("pgtest: ninguna tabla con columna `tenant` ni `org_id`; ¿se aplicaron las migraciones?")
	}
	for _, f := range fallos {
		t.Errorf("tabla por organización sin RLS ENABLE+FORCE: %s", f)
	}
}

// ExigirPoliticaDeFusion comprueba que TODA tabla con columna `tenant` tenga una
// política de fusión que el preflight de Sytue ID reconozca. Usa la MISMA consulta que
// ese preflight (pg.ConsultaTablasConTenant): una tabla que no la pase aborta TODAS
// las fusiones de organizaciones de la suite, no solo las del servicio que la creó,
// y eso se descubre meses después en la consola de administración.
//
// No admite excepciones, a diferencia de ExigirRLSForzada: una tabla con `tenant` que
// la fusión no mueve deja las filas de la organización absorbida con un tenant que ya
// no existe. `events_outbox` también la necesita, aunque no lleve RLS forzada.
func ExigirPoliticaDeFusion(t *testing.T, h *Harness) {
	t.Helper()
	tablas, sinPolitica, err := pg.ConsultaTablasConTenant(context.Background(), h.Pool)
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	if len(tablas) == 0 {
		t.Fatal("pgtest: ninguna tabla con columna `tenant`; ¿se aplicaron las migraciones?")
	}
	for _, tabla := range sinPolitica {
		t.Errorf("tabla con `tenant` sin política de fusión: %s (el preflight de la fusión "+
			"abortaría todas las de la suite; añade %s_platform_merge con app.merge)", tabla, tabla)
	}
}
