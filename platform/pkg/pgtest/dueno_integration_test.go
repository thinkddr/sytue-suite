// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Las migraciones de cada servicio, aplicadas como las aplica el piloto con los
// roles separados (INF-01): por el DUEÑO de la base, sin SUPERUSER ni BYPASSRLS,
// con lo que le da el despliegue (deploy/helm/sql) y nada más.
//
// El resto de la batería migra con el superusuario del contenedor, que lo puede
// todo; por eso el 2026-11-16 la v3.20.3 pasó el CI en verde y en el piloto doce
// servicios se quedaron con la migración sucia: sus funciones nuevas llevan
// `SET app.maintenance = 'on'` y su rol no tenía `GRANT SET ON PARAMETER`.
package pgtest_test

import (
	"io/fs"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	analyticsdb "git.sytue.eu/sytue/one/services/analytics/db"
	basesdb "git.sytue.eu/sytue/one/services/bases/db"
	calendardb "git.sytue.eu/sytue/one/services/calendar/db"
	castdb "git.sytue.eu/sytue/one/services/cast/db"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	contactsdb "git.sytue.eu/sytue/one/services/contacts/db"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	maildb "git.sytue.eu/sytue/one/services/mail/db"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	pagesdb "git.sytue.eu/sytue/one/services/pages/db"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
	syerpdb "git.sytue.eu/sytue/one/services/syerp/db"
	syphotosdb "git.sytue.eu/sytue/one/services/syphotos/db"
)

const (
	sqlTraspaso   = "../../../deploy/helm/sql/traspasar-propiedad.sql"
	sqlParametros = "../../../deploy/helm/sql/conceder-parametros.sql"
)

// TestMigracionesConDuenoSinSuperusuario: todos los servicios con base propia. Un
// servicio nuevo entra aquí el día que nace su carpeta db/migrations.
func TestMigracionesConDuenoSinSuperusuario(t *testing.T) {
	servicios := []struct {
		nombre string
		fsys   fs.FS
		dir    string
		// superusuarioHasta: lo que el dueño no puede hacer por diseño y en el
		// piloto hizo el superusuario antes de traspasar (ver pgtest.Dueno).
		superusuarioHasta uint
	}{
		{"analytics", analyticsdb.Migrations, analyticsdb.MigrationsDir, 0},
		{"bases", basesdb.Migrations, basesdb.MigrationsDir, 0},
		{"calendar", calendardb.Migrations, calendardb.MigrationsDir, 0},
		{"cast", castdb.Migrations, castdb.MigrationsDir, 0},
		{"cloud", clouddb.Migrations, clouddb.MigrationsDir, 0},
		{"contacts", contactsdb.Migrations, contactsdb.MigrationsDir, 0},
		// 0001 crea el rol docs_app y 0005 vuelve a concederse su pertenencia:
		// las dos exigen CREATEROLE o ADMIN sobre el rol.
		{"docs", docsdb.Migrations, docsdb.MigrationsDir, 5},
		{"drive", drivedb.Migrations, drivedb.MigrationsDir, 0},
		{"hub", hubdb.Migrations, hubdb.MigrationsDir, 0},
		{"id", iddb.Migrations, iddb.MigrationsDir, 0},
		// Lo mismo con integrations_app: 0001 lo crea y 0011 rehace la pertenencia.
		{"integrations", integrationsdb.Migrations, integrationsdb.MigrationsDir, 11},
		{"mail", maildb.Migrations, maildb.MigrationsDir, 0},
		{"notes", notesdb.Migrations, notesdb.MigrationsDir, 0},
		{"pages", pagesdb.Migrations, pagesdb.MigrationsDir, 0},
		{"rooms", roomsdb.Migrations, roomsdb.MigrationsDir, 0},
		{"syerp", syerpdb.Migrations, syerpdb.MigrationsDir, 0},
		{"syphotos", syphotosdb.Migrations, syphotosdb.MigrationsDir, 0},
	}
	for _, s := range servicios {
		t.Run(s.nombre, func(t *testing.T) {
			t.Parallel()
			err := pgtest.MigrarComoDueno(t, s.fsys, s.dir, pgtest.Dueno{
				TraspasoSQL:       sqlTraspaso,
				ParametrosSQL:     sqlParametros,
				SuperusuarioHasta: s.superusuarioHasta,
			})
			if err != nil {
				t.Fatalf("las migraciones de %s no entran con su dueño sin SUPERUSER: %v\n\t"+
					"En el piloto el servicio se quedaría con la migración sucia y en CrashLoopBackOff.\n\t"+
					"Si es «permission denied to set parameter», añade el parámetro a %s;\n\t"+
					"si es otra cosa (CREATE ROLE, una extensión que no es de confianza…), la migración\n\t"+
					"necesita un superusuario y hay que reescribirla o aplicarla a mano antes de desplegar.",
					s.nombre, err, strings.TrimPrefix(sqlParametros, "../../../"))
			}
		})
	}
}

// TestSinElPermisoFallaComoEnElPiloto: Cast con su historia hasta la 0012 aplicada
// por el superusuario, traspasada, y la 0013 (almacenamiento privado) con el dueño
// SIN el permiso sobre los parámetros. Tiene que fallar como falló el 2026-11-16;
// si deja de fallar, la prueba de arriba ya no demuestra nada.
func TestSinElPermisoFallaComoEnElPiloto(t *testing.T) {
	err := pgtest.MigrarComoDueno(t, castdb.Migrations, castdb.MigrationsDir, pgtest.Dueno{
		TraspasoSQL:       sqlTraspaso,
		SuperusuarioHasta: 12,
	})
	if err == nil || !strings.Contains(err.Error(), `permission denied to set parameter "app.maintenance"`) {
		t.Fatalf(`sin el permiso, la 0013 de Cast tenía que fallar con «permission denied to set parameter "app.maintenance"»; dio: %v`, err)
	}
}
