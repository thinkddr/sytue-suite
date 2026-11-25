// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"io/fs"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/purgatenant"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	analyticsdb "git.sytue.eu/sytue/one/services/analytics/db"
	basesdb "git.sytue.eu/sytue/one/services/bases/db"
	calendardb "git.sytue.eu/sytue/one/services/calendar/db"
	castdb "git.sytue.eu/sytue/one/services/cast/db"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	contactsdb "git.sytue.eu/sytue/one/services/contacts/db"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	maildb "git.sytue.eu/sytue/one/services/mail/db"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	pagesdb "git.sytue.eu/sytue/one/services/pages/db"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
	syerpdb "git.sytue.eu/sytue/one/services/syerp/db"
	syphotosdb "git.sytue.eu/sytue/one/services/syphotos/db"
)

// esquemaPurga es UN servicio de la suite visto por la purga: sus migraciones
// reales, una siembra mínima de su contenido (el `$1` es el tenant) y lo que se
// espera que quede retenido.
type esquemaPurga struct {
	fsys    fs.FS
	dir     string
	siembra []string
	// retenidas son las tablas que un guardián de integridad no deja vaciar.
	retenidas []string
	// objetos: el servicio guarda objetos y la purga espera a su trabajador.
	objetos bool
	// claves que el gancho de la base debe dejar en la orden de objetos.
	claves []string
}

// esquemasPurga cubre CADA base de merge.Services. TestPurgaPorServicio falla si
// aparece una nueva sin su entrada: un servicio que la purga no ha visto nunca
// vaciarse es uno del que no se sabe si se vacía.
var esquemasPurga = map[string]esquemaPurga{
	"mail": {
		fsys: maildb.Migrations, dir: maildb.MigrationsDir, objetos: true,
		claves: []string{"shared/mensajes/solo-suyo.eml"},
		siembra: []string{
			`WITH a AS (INSERT INTO accounts (tenant, subject, address) VALUES ($1, 'ana', 'ana@' || $1 || '.test') RETURNING id),
			      m AS (INSERT INTO mailboxes (tenant, account_id, name) SELECT $1, id, 'INBOX' FROM a RETURNING id, account_id)
			 INSERT INTO messages (tenant, account_id, mailbox_id, uid, blob_key)
			 SELECT $1, account_id, id, n, k FROM m, (VALUES
			     (1, 'shared/mensajes/' || $1 || '/propio.eml'),
			     (2, 'shared/mensajes/legado-comun.eml'),
			     (3, CASE WHEN $1 = 'p-se-va' THEN 'shared/mensajes/solo-suyo.eml' ELSE 'shared/mensajes/de-otra.eml' END)) v(n, k)`,
			`INSERT INTO blob_refs (hash, key, size_bytes, refs, listo) VALUES
			     ($1 || '/propio', 'shared/mensajes/' || $1 || '/propio.eml', 1, 1, true)`,
		},
	},
	"drive": {
		fsys: drivedb.Migrations, dir: drivedb.MigrationsDir, objetos: true,
		siembra: []string{
			`WITH f AS (INSERT INTO folders (tenant, name, is_root) VALUES ($1, '', true) RETURNING id),
			      x AS (INSERT INTO files (tenant, folder_id, name, storage_key)
			            SELECT $1, id, 'informe.pdf', 'shared/drive/legado-comun' FROM f RETURNING id)
			 INSERT INTO file_versions (tenant, file_id, version, storage_key) SELECT $1, id, 1, 'shared/drive/' || $1 || '/v1' FROM x`,
		},
	},
	"docs": {
		fsys: docsdb.Migrations, dir: docsdb.MigrationsDir, objetos: true,
		siembra: []string{`INSERT INTO documents (tenant, kind, owner_sub) VALUES ($1, 'document', 'ana')`},
	},
	"calendar": {
		fsys: calendardb.Migrations, dir: calendardb.MigrationsDir,
		siembra: []string{`INSERT INTO calendars (tenant, owner_sub, name) VALUES ($1, 'ana', 'Personal')`},
	},
	"contacts": {
		fsys: contactsdb.Migrations, dir: contactsdb.MigrationsDir,
		siembra: []string{`INSERT INTO address_books (tenant, owner_sub, name) VALUES ($1, 'ana', 'Contactos')`},
	},
	"rooms": {
		fsys: roomsdb.Migrations, dir: roomsdb.MigrationsDir, objetos: true,
		siembra: []string{
			`WITH t AS (INSERT INTO teams (tenant, slug, name) VALUES ($1, 'equipo', 'Equipo') RETURNING id),
			      c AS (INSERT INTO channels (tenant, team_id, slug, name) SELECT $1, id, 'general', 'General' FROM t RETURNING id)
			 INSERT INTO messages (tenant, channel_id, author_sub, body) SELECT $1, id, 'ana', 'hola' FROM c`,
			// Cada una confía en la otra (mig 0083): la que se queda deja de confiar
			// en la purgada (gancho de la mig 0084), no en las demás.
			`INSERT INTO politica_contacto_externo (tenant, modo, confianza)
			 VALUES ($1::text, 'confianza', ARRAY[CASE WHEN $1::text = 'p-se-va' THEN 'otra-organizacion' ELSE 'p-se-va' END, 'tercera'])`,
		},
	},
	"hub": {
		fsys: hubdb.Migrations, dir: hubdb.MigrationsDir,
		siembra: []string{`INSERT INTO notifications (tenant, subject, source, type, title) VALUES ($1, 'ana', 'mail', 'x', 'Aviso')`},
	},
	"integrations": {
		fsys: integrationsdb.Migrations, dir: integrationsdb.MigrationsDir,
		siembra: []string{
			`INSERT INTO events_outbox (id, channel, type, tenant) VALUES (gen_random_uuid(), 'c', 't', $1)`,
			`INSERT INTO policies (org_id, name) VALUES ($1, 'Política')`,
		},
	},
	"cloud": {
		fsys: clouddb.Migrations, dir: clouddb.MigrationsDir, objetos: true,
		siembra: []string{`INSERT INTO projects (tenant, slug, name) VALUES ($1, 'w-' || $1, 'Web')`},
	},
	"analytics": {
		fsys: analyticsdb.Migrations, dir: analyticsdb.MigrationsDir,
		siembra: []string{`INSERT INTO analytics_sites (tenant, host) VALUES ($1, $1 || '.example.test')`},
	},
	"notes": {
		fsys: notesdb.Migrations, dir: notesdb.MigrationsDir,
		// Con el contrato de escritura de Notepads, que la purga también adopta.
		siembra: []string{`WITH c AS (SELECT set_config('app.notes_write_version', '2', true))
			INSERT INTO notebooks (tenant, owner_sub, title) SELECT $1, 'ana', 'Cuaderno' FROM c`},
	},
	"syphotos": {
		fsys: syphotosdb.Migrations, dir: syphotosdb.MigrationsDir, objetos: true,
		siembra: []string{`INSERT INTO albums (tenant, owner_sub, title) VALUES ($1, 'ana', 'Verano')`},
	},
	// El libro diario: un asiento asentado no se borra (syerp_asiento_inmutable) y
	// la purga lo deja retenido; el borrador, sí.
	"syerp": {
		fsys: syerpdb.Migrations, dir: syerpdb.MigrationsDir, retenidas: []string{"moves"},
		siembra: []string{
			`INSERT INTO moves (tenant, diario, ref, fecha, owner_sub) VALUES ($1, 'VEN', 'B-1', current_date, 'ana')`,
			`INSERT INTO moves (tenant, diario, ref, fecha, owner_sub, estado, posted_at) VALUES ($1, 'VEN', 'A-1', current_date, 'ana', 'asentado', now())`,
		},
	},
	"cast": {
		fsys: castdb.Migrations, dir: castdb.MigrationsDir, objetos: true,
		siembra: []string{`INSERT INTO videos (tenant, owner_sub, kind, status, title) VALUES ($1, 'ana', 'upload', 'ready', 'Charla')`},
	},
	"pages": {
		fsys: pagesdb.Migrations, dir: pagesdb.MigrationsDir, objetos: true,
		siembra: []string{`INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web-' || $1, 'Web', '{}', 'ana')`},
	},
	"bases": {
		fsys: basesdb.Migrations, dir: basesdb.MigrationsDir, objetos: true,
		siembra: []string{
			`WITH b AS (INSERT INTO bases (tenant, name, owner_sub) VALUES ($1, 'Clientes', 'ana') RETURNING id)
			 INSERT INTO base_tables (tenant, base_id, name) SELECT $1, id, 'Tabla' FROM b`,
		},
	},
}

const (
	sePurga  = "p-se-va"
	seQueda  = "otra-organizacion"
	sinPurga = "p-sin-nada"
)

// abrirUna devuelve un abridor que siempre entrega la misma base.
func abrirUna(db pg.Database) abridorDeBases {
	return func(context.Context, string) (pg.Database, func(), error) { return db, func() {}, nil }
}

func TestPurgaPorServicio(t *testing.T) {
	for _, svc := range merge.Services {
		if _, ok := esquemasPurga[svc]; !ok {
			t.Errorf("%s está en merge.Services y no en esquemasPurga: nadie ha visto su purga", svc)
		}
	}
	for _, svc := range merge.Services {
		e, ok := esquemasPurga[svc]
		if !ok {
			continue
		}
		t.Run(svc, func(t *testing.T) {
			ctx := context.Background()
			h := pgtest.New(t, e.fsys, e.dir)
			admin := h.PoolSinRLS(t)
			for _, slug := range []string{sePurga, seQueda} {
				for _, q := range e.siembra {
					if _, err := admin.Exec(ctx, q, slug); err != nil {
						t.Fatalf("sembrar %s: %v\n%s", slug, err, q)
					}
				}
			}
			// La traza de acceso es solo de añadir (audit_log_v1.sql), pero la purga
			// de una organización que desaparece se la lleva igual.
			var conTraza bool
			if err := admin.QueryRow(ctx, `SELECT to_regclass('audit_log') IS NOT NULL`).Scan(&conTraza); err != nil {
				t.Fatal(err)
			}
			if conTraza {
				for _, slug := range []string{sePurga, seQueda} {
					if _, err := admin.Exec(ctx, `INSERT INTO audit_log (tenant, actor, action, target) VALUES ($1, 'ana', 'acceso.export', '')`, slug); err != nil {
						t.Fatal(err)
					}
				}
			}
			if svc == "mail" {
				// Un .eml de antes de la deduplicación por organización, usado por las
				// dos, y otro que solo usa la que se va.
				if _, err := admin.Exec(ctx, `INSERT INTO blob_refs (hash, key, size_bytes, refs, listo) VALUES
					('legado-comun', 'shared/mensajes/legado-comun.eml', 1, 2, true),
					('solo-suyo', 'shared/mensajes/solo-suyo.eml', 1, 1, true),
					('de-otra', 'shared/mensajes/de-otra.eml', 1, 1, true)`); err != nil {
					t.Fatal(err)
				}
			}
			antes, err := merge.LeftoversIn(ctx, h.Pool, svc, seQueda)
			if err != nil || len(antes) == 0 {
				t.Fatalf("la siembra de la otra organización no se ve: %v, %v", antes, err)
			}

			e1 := purgarEnServicio(ctx, sePurga, svc, store.ProgresoServicio{}, abrirUna(h.Pool))
			if len(e.retenidas) > 0 {
				if e1.Retenido == "" || e1.Hecho || e1.BaseVacia {
					t.Fatalf("lo retenido por un guardián no se informa: %+v", e1)
				}
			} else if e1.Error != "" || e1.Retenido != "" || !e1.BaseVacia || e1.Filas == 0 {
				t.Fatalf("purga: %+v", e1)
			}
			restos, err := merge.LeftoversIn(ctx, h.Pool, svc, sePurga)
			if err != nil {
				t.Fatal(err)
			}
			var tablas []string
			for _, r := range restos {
				tablas = append(tablas, r.Table)
			}
			if !slices.Equal(tablas, e.retenidas) {
				t.Fatalf("quedan filas del tenant purgado en %v; solo se esperaban en %v", restos, e.retenidas)
			}
			despues, err := merge.LeftoversIn(ctx, h.Pool, svc, seQueda)
			if err != nil || !slices.Equal(antes, despues) {
				t.Fatalf("la purga tocó otra organización:\nantes   %v\ndespués %v (%v)", antes, despues, err)
			}
			// LeftoversIn solo mira `tenant`; las tablas por `org_id` (Integrations),
			// aparte.
			if n := filasPorOrgID(t, admin, sePurga); n != 0 {
				t.Fatalf("quedan %d filas por org_id del tenant purgado", n)
			}
			if svc == "integrations" && filasPorOrgID(t, admin, seQueda) != 1 {
				t.Fatal("la purga tocó las filas por org_id de otra organización")
			}
			if len(e.retenidas) > 0 {
				return
			}

			// Los objetos: sin cola, terminado; con cola, espera al trabajador.
			if !e.objetos {
				if !e1.Hecho || e1.EsperaObjetos {
					t.Fatalf("un servicio sin objetos termina con la base: %+v", e1)
				}
				return
			}
			if e1.Hecho || !e1.EsperaObjetos {
				t.Fatalf("con objetos hay que esperar al servicio: %+v", e1)
			}
			alm := &almacenAnotado{}
			w := purgatenant.Trabajador{DB: h.Pool, Almacen: alm}
			if n, err := w.Pasada(ctx); err != nil || n != 1 {
				t.Fatalf("trabajador: %d, %v", n, err)
			}
			if !slices.Equal(alm.tenants, []string{sePurga}) {
				t.Fatalf("el trabajador barrió %v", alm.tenants)
			}
			if !slices.Equal(alm.claves, e.claves) {
				t.Fatalf("claves de la orden: %v; quiero %v", alm.claves, e.claves)
			}
			e2 := purgarEnServicio(ctx, sePurga, svc, e1, abrirUna(h.Pool))
			if !e2.Hecho || e2.EsperaObjetos || e2.Error != "" {
				t.Fatalf("tras el trabajador: %+v", e2)
			}
			// Repetir no cambia nada: la base sigue vacía y la orden, cerrada.
			if e3 := purgarEnServicio(ctx, sePurga, svc, store.ProgresoServicio{}, abrirUna(h.Pool)); !e3.Hecho || e3.Filas != 0 {
				t.Fatalf("repetir la purga: %+v", e3)
			}
			if svc == "mail" {
				comprobarContabilidadDeMail(t, admin)
			}
			if svc == "drive" {
				var n int
				_ = admin.QueryRow(ctx, `SELECT count(*) FROM blobs_candidatos WHERE storage_key = 'shared/drive/legado-comun'`).Scan(&n)
				if n != 1 {
					t.Fatal("el blob compartido entre organizaciones no quedó como candidato al barrido")
				}
			}
			if svc == "rooms" {
				var confianza []string
				if err := admin.QueryRow(ctx, `SELECT confianza FROM politica_contacto_externo WHERE tenant = $1`, seQueda).Scan(&confianza); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(confianza, []string{"tercera"}) {
					t.Fatalf("la confianza en la organización purgada sigue en otra: %v", confianza)
				}
			}
		})
	}
}

// filasPorOrgID cuenta, como superusuario, las filas de `slug` en todas las
// tablas con `org_id` de texto.
func filasPorOrgID(t *testing.T, admin *pg.Pool, slug string) int {
	t.Helper()
	ctx := context.Background()
	rows, err := admin.Query(ctx, `SELECT c.relname FROM pg_class c
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'org_id' AND NOT a.attisdropped
		WHERE c.relkind = 'r' AND c.relnamespace = 'public'::regnamespace AND a.atttypid = 'text'::regtype`)
	if err != nil {
		t.Fatal(err)
	}
	var tablas []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tablas = append(tablas, n)
	}
	rows.Close()
	total := 0
	for _, tabla := range tablas {
		var n int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{tabla}.Sanitize()+` WHERE org_id = $1`, slug).Scan(&n); err != nil {
			t.Fatal(err)
		}
		total += n
	}
	return total
}

// comprobarContabilidadDeMail: el .eml compartido con otra organización baja una
// referencia y se queda; el que solo usaba la que se va se cierra; el suyo por
// organización desaparece de la contabilidad (lo barre su prefijo).
func comprobarContabilidadDeMail(t *testing.T, admin *pg.Pool) {
	t.Helper()
	ctx := context.Background()
	refs := map[string]int{}
	rows, err := admin.Query(ctx, `SELECT key, refs FROM blob_refs`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatal(err)
		}
		refs[k] = n
	}
	if refs["shared/mensajes/legado-comun.eml"] != 1 {
		t.Errorf("el compartido con otra organización: %d referencias, quiero 1", refs["shared/mensajes/legado-comun.eml"])
	}
	if _, ok := refs["shared/mensajes/solo-suyo.eml"]; ok {
		t.Error("el .eml que solo usaba la que se va sigue contabilizado")
	}
	if _, ok := refs["shared/mensajes/"+sePurga+"/propio.eml"]; ok {
		t.Error("la contabilidad de los .eml de la organización purgada sigue ahí")
	}
	if refs["shared/mensajes/"+seQueda+"/propio.eml"] != 1 || refs["shared/mensajes/de-otra.eml"] != 1 {
		t.Errorf("se tocó la contabilidad de otra organización: %v", refs)
	}
}

type almacenAnotado struct{ tenants, prefijos, claves []string }

func (a *almacenAnotado) BorrarTenant(_ context.Context, t tenancy.Tenant) (int, error) {
	a.tenants = append(a.tenants, t.Slug)
	return 0, nil
}

func (a *almacenAnotado) BorrarPrefijo(_ context.Context, p string) (int, error) {
	a.prefijos = append(a.prefijos, p)
	return 0, nil
}

func (a *almacenAnotado) BorrarClave(_ context.Context, k string) error {
	a.claves = append(a.claves, k)
	return nil
}

// TestPurgarTenantRecorreTodasLasBasesYSeCierra: un intento con una base que no
// existe en el despliegue, una que falla y una que termina; el siguiente
// intento solo toca lo pendiente.
func TestPurgarTenantRecorreTodasLasBasesYSeCierra(t *testing.T) {
	ctx := context.Background()
	h := pgtest.New(t, esquemasPurga["hub"].fsys, esquemasPurga["hub"].dir)
	caida := true
	abrir := func(_ context.Context, svc string) (pg.Database, func(), error) {
		switch svc {
		case "bases":
			return nil, nil, &pgconn.PgError{Code: "3D000"}
		case "notes":
			if caida {
				return nil, nil, context.DeadlineExceeded
			}
		}
		return h.Pool, func() {}, nil
	}
	p := store.PurgaDeTenant{OrgID: "o", Slug: sinPurga, Progreso: map[string]store.ProgresoServicio{}}
	prog, completa, r := purgarTenant(ctx, p, abrir, []string{"hub", "bases", "notes"}, nil)
	if completa || r.fallos != 1 || !prog["bases"].SinBase || !prog["hub"].Hecho || prog["notes"].Error == "" {
		t.Fatalf("primer intento: %+v %+v", prog, r)
	}
	caida = false
	p.Progreso = prog
	prog, completa, r = purgarTenant(ctx, p, abrir, []string{"hub", "bases", "notes"}, nil)
	if !completa || r.fallos != 0 || !prog["notes"].Hecho {
		t.Fatalf("segundo intento: %+v %+v", prog, r)
	}
	if prog["hub"].Actualizado == nil || prog["hub"].Actualizado.After(time.Now()) {
		t.Fatal("el progreso no lleva su marca de tiempo")
	}
	if esperaPurga(1) != BarridoCuentasBorradas || esperaPurga(3) != 4*BarridoCuentasBorradas || esperaPurga(50) != esperaMaxPurga {
		t.Fatal("la espera creciente no crece como se dice")
	}
}
