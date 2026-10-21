// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Package pgtest es el HARNESS de tests de integracion contra un PostgreSQL
// real y efimero (CONVENTIONS §0.8, §15: paridad dev/prod). Levanta un
// contenedor PostgreSQL 18 con testcontainers-go, aplica las migraciones
// versionadas de un servicio (via platform/pkg/pg.MigrateUp sobre su embed.FS)
// y devuelve un *pg.Pool listo, mas su teardown.
//
// Todo el paquete vive detras del build-tag `integration` para no requerir
// Docker en el build/CI por defecto (los tests unitarios de plataforma siguen
// verdes sin este tag). testcontainers-go es dependencia SOLO de test (MIT).
//
// Uso tipico:
//
//	//go:build integration
//	func TestFoo(t *testing.T) {
//	    h := pgtest.New(t, db.Migrations, db.MigrationsDir)
//	    ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
//	    // ... usar h.Pool con el store del servicio ...
//	}
//
// # QUE GARANTIZA ESTE HARNESS (y que no)
//
// SI garantiza que la RLS se EJERCITA: Pool conecta con un rol de APLICACION
// (appUser) creado NOSUPERUSER + NOBYPASSRLS y con privilegios minimos (DML +
// secuencias + EXECUTE). PostgreSQL NO aplica politicas RLS a un superusuario
// ni a un rol con BYPASSRLS, asi que sin esto NINGUNA politica se evaluaba y un
// test verde no demostraba nada sobre el aislamiento. Con el rol de aplicacion,
// un test que lee filas de otro tenant falla de verdad, y quitar una politica
// de una migracion rompe los tests que dependen de ella.
//
// NO garantiza:
//   - Que las migraciones corran con privilegio minimo: se aplican con el rol
//     DUENO/superusuario del contenedor (h.DSN). Es lo realista (un DBA migra),
//     pero implica que este harness no detecta que falte un GRANT en prod.
//   - Que el rol de aplicacion sea BIT A BIT el de produccion: aqui tiene
//     TRUNCATE (lo necesita Reset) y es miembro de los roles *_app NOLOGIN que
//     crean las migraciones de docs/integrations, para que su `SET LOCAL ROLE`
//     siga funcionando. Ver appRoleGrants.
//   - Aislamiento que NO viva en la base: si una fuga se evita solo con un
//     predicado Go (WHERE tenant = $1), este harness no puede distinguir entre
//     "lo para la RLS" y "lo para el codigo". Para demostrar que lo para la RLS
//     hay que escribir el test contra h.Pool con SQL crudo, sin el store (ver
//     pgtest_integration_test.go).
//   - Nada sobre autorizacion de aplicacion (roles, ACL, membresias): la RLS
//     solo cubre el ambito tenant/team_drive/canal que declaran las politicas.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

const (
	// postgresImage fija la version de PostgreSQL del datastore unico de la
	// suite (CONVENTIONS §3: PostgreSQL 18.x, ultima estable GA). Se pinnea por
	// tag Alpine para arranque rapido en CI; el motor es identico al de prod.
	postgresImage = "postgres:18-alpine"

	// testUser es el rol DUENO del contenedor: superusuario, con BYPASSRLS. Solo
	// se usa para aplicar migraciones y aprovisionar el rol de aplicacion.
	testDB       = "sytue_test"
	testUser     = "sytue_test"
	testPassword = "sytue_test_pwd" //nolint:gosec // contraseña del PostgreSQL efímero de las pruebas

	// appUser es el rol de APLICACION con el que conecta el Pool de los tests:
	// NOSUPERUSER + NOBYPASSRLS, de modo que las politicas RLS SI se evaluan.
	appUser     = "sytue_app"
	appPassword = "sytue_app_pwd" //nolint:gosec // contraseña del rol de aplicación del PostgreSQL efímero

	// startupTimeout acota el arranque del contenedor + primer ping.
	startupTimeout = 90 * time.Second
)

// appRoleCreate crea el rol de aplicacion ANTES de migrar y deja preparados los
// privilegios POR DEFECTO, para que cualquier objeto que creen las migraciones
// nazca ya accesible al rol (y tambien los que cree un test que rehaga el
// esquema, como el ciclo down/up de services/cloud).
//
// Lo que NO lleva es deliberado: NOSUPERUSER y NOBYPASSRLS son la razon de ser
// del rol; sin CREATEDB/CREATEROLE/REPLICATION porque el servicio en produccion
// tampoco los tiene.
var appRoleCreate = []string{
	// El CREATE ROLE va dentro de un bloque que se traga `duplicate_object`
	// porque los roles son de ámbito de CLÚSTER, no de base de datos. Con un
	// contenedor por harness eso daba igual —cada uno tenía su servidor—, pero
	// con PGTEST_DSN todos comparten servidor y el segundo en arrancar se
	// encontraba el rol ya creado y moría con «role sytue_app already exists».
	//
	// Y se captura la excepción en vez de preguntar antes si existe: dos harness
	// que arranquen a la vez pueden ver los dos que no existe y intentar crearlo
	// los dos. Esto es atómico; un IF NOT EXISTS no lo sería.
	//
	// Compartir el rol entre harness es inocuo: los privilegios se conceden por
	// base de datos y cada harness tiene la suya.
	`DO $$ BEGIN
	     CREATE ROLE ` + appUser + ` LOGIN PASSWORD '` + appPassword + `'
	         NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;
	 EXCEPTION WHEN duplicate_object THEN NULL;
	 END $$`,
	`GRANT USAGE ON SCHEMA public TO ` + appUser,
	// ponytail: TRUNCATE no lo tiene el rol de produccion; aqui hace falta para
	// Harness.Reset. La RLS NO se aplica a TRUNCATE, asi que si algun dia el
	// codigo de un servicio truncara una tabla multi-tenant este harness no lo
	// detectaria — hoy ningun servicio lo hace (solo Reset).
	`ALTER DEFAULT PRIVILEGES IN SCHEMA public
	    GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON TABLES TO ` + appUser,
	`ALTER DEFAULT PRIVILEGES IN SCHEMA public
	    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO ` + appUser,
	`ALTER DEFAULT PRIVILEGES IN SCHEMA public
	    GRANT EXECUTE ON FUNCTIONS TO ` + appUser,
}

// appRoleGrants completa los privilegios DESPUES de migrar: los ALTER DEFAULT
// PRIVILEGES de arriba solo alcanzan a los objetos creados por el rol que los
// declaro, y las extensiones (pg_trgm, pgcrypto) crean funciones a su nombre.
//
// El bucle concede al rol de aplicacion la MEMBRESIA en los roles *_app NOLOGIN
// que crean las migraciones de docs/integrations, porque sus stores hacen
// `SET LOCAL ROLE docs_app` en cada transaccion y eso exige ser miembro. Se
// filtra por NOT rolsuper AND NOT rolbypassrls: nunca se hereda un rol que
// pudiera reintroducir el bypass de RLS.
var appRoleGrants = []string{
	`GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA public TO ` + appUser,
	`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO ` + appUser,
	`GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ` + appUser,
	`DO $$
	 DECLARE r record;
	 BEGIN
	     FOR r IN SELECT rolname FROM pg_roles
	              WHERE NOT rolcanlogin AND NOT rolsuper AND NOT rolbypassrls
	                AND rolname NOT LIKE 'pg\_%'
	     LOOP
	         EXECUTE format('GRANT %I TO ` + appUser + `', r.rolname);
	     END LOOP;
	 END $$`,
}

// startExterno usa un PostgreSQL ya en marcha (PGTEST_DSN) en vez de arrancar un
// contenedor. Crea una base propia para este harness y la borra al terminar.
func startExterno(ctx context.Context, fsys fs.FS, dir, dsnBase string) (*Harness, error) {
	// El nombre lo da crypto/rand y no el reloj ni el PID: los tests corren en
	// paralelo dentro del mismo proceso y con el mismo servidor, y dos harness
	// que nacieran en el mismo milisegundo compartirían base y se pisarían las
	// migraciones. El fallo saldría como un error de esquema, en otro test, y
	// solo a veces.
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("pgtest: nombre de base: %w", err)
	}
	base := "pgtest_" + hex.EncodeToString(b[:])

	admin, err := pgx.Connect(ctx, dsnBase)
	if err != nil {
		return nil, fmt.Errorf("pgtest: conectar a PGTEST_DSN: %w", err)
	}
	// El identificador se compone a mano porque PostgreSQL no admite parámetros
	// en CREATE DATABASE. Es seguro: `base` son 16 dígitos hexadecimales
	// generados aquí mismo, no llega de fuera.
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+base+`"`); err != nil {
		_ = admin.Close(ctx)
		return nil, fmt.Errorf("pgtest: crear base %s: %w", base, err)
	}
	_ = admin.Close(ctx)

	dsn, err := conBaseDeDatos(dsnBase, base)
	if err != nil {
		return nil, err
	}

	terminate := func() { //nolint:contextcheck // el teardown corre cuando el contexto del test ya terminó
		termCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := pgx.Connect(termCtx, dsnBase)
		if err != nil {
			return
		}
		defer func() { _ = a.Close(termCtx) }()
		// WITH (FORCE) cierra las conexiones que queden: sin él, un pool que
		// tarde un instante en soltar deja la base colgada y el servidor se va
		// llenando de bases huérfanas a lo largo de una tanda de tests.
		_, _ = a.Exec(termCtx, `DROP DATABASE IF EXISTS "`+base+`" WITH (FORCE)`)
	}

	return terminarDeMontar(ctx, fsys, dir, dsn, nil, terminate)
}

// conBaseDeDatos devuelve el mismo DSN apuntando a otra base de datos.
func conBaseDeDatos(dsn, base string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("pgtest: PGTEST_DSN no parseable: %w", err)
	}
	u.Path = "/" + base
	return u.String(), nil
}

// execPrivileged abre una conexion con el rol dueno y ejecuta las sentencias en
// orden. El aprovisionamiento del rol de aplicacion (DDL de roles y GRANT) no
// puede hacerlo el propio rol de aplicacion, que es justo el punto.
func execPrivileged(ctx context.Context, dsn string, stmts []string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("pgtest: conectar con el rol privilegiado: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("pgtest: aprovisionar rol de aplicacion (%.48s...): %w", stmt, err)
		}
	}
	return nil
}

// withAppRole reescribe el usuario/clave del DSN al rol de aplicacion,
// conservando host, puerto, base y parametros.
func withAppRole(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("pgtest: DSN no parseable: %w", err)
	}
	u.User = url.UserPassword(appUser, appPassword)
	return u.String(), nil
}

// traspasoSQLEnv hace que el Pool conecte como DUEÑO de la base en vez de como
// sytue_app: tras migrar con el rol privilegiado, se crea un rol sin SUPERUSER ni
// BYPASSRLS, se le traspasa la propiedad de todo con el SQL de la ruta dada
// (deploy/helm/sql/traspasar-propiedad.sql) y el Pool entra con él.
//
// PARA QUÉ. Es el ensayo de separar los roles en producción (INF-01, paso 1b):
// allí cada servicio pasa a entrar como dueño de su base, sujeto a la RLS forzada.
// El modo normal del arnés no lo cubre por una diferencia que importa: aquí las
// funciones SECURITY DEFINER son del rol privilegiado y se saltan la RLS, y tras
// el traspaso son del dueño y quedan sujetas a ella. Una batería en verde con
// esta variable dice que el servicio no dependía de ese salto, y además prueba el
// mismo fichero SQL que luego se ejecuta en el piloto.
const traspasoSQLEnv = "PGTEST_TRASPASO_SQL"

// traspasarAUnDueno devuelve el DSN del dueño nuevo y la función que retira su
// rol (los roles son del servidor y sobreviven a la base).
func traspasarAUnDueno(ctx context.Context, dsn, ruta string) (string, func(), error) {
	sql, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la da quien lanza los tests.
	if err != nil {
		return "", nil, fmt.Errorf("pgtest: leer %s: %w", traspasoSQLEnv, err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", nil, fmt.Errorf("pgtest: conectar con el rol privilegiado: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var de, base string
	if err := conn.QueryRow(ctx, `SELECT current_user, current_database()`).Scan(&de, &base); err != nil {
		return "", nil, fmt.Errorf("pgtest: leer rol y base: %w", err)
	}
	// Un rol por base: los tests corren en paralelo contra el mismo servidor.
	rol := "dueno_" + base
	dueno := pgx.Identifier{rol}.Sanitize()
	if _, err := conn.Exec(ctx, `CREATE ROLE `+dueno+` LOGIN PASSWORD '`+appPassword+`'
	     NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION`); err != nil {
		return "", nil, fmt.Errorf("pgtest: crear el rol dueño: %w", err)
	}
	if _, err := conn.Exec(ctx,
		`SELECT set_config('sytue.de', $1, false), set_config('sytue.dueno', $2, false)`, de, rol); err != nil {
		return "", nil, fmt.Errorf("pgtest: fijar los parámetros del traspaso: %w", err)
	}
	if _, err := conn.Exec(ctx, string(sql)); err != nil {
		return "", nil, fmt.Errorf("pgtest: traspasar la propiedad: %w", err)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", nil, fmt.Errorf("pgtest: DSN no parseable: %w", err)
	}
	u.User = url.UserPassword(rol, appPassword)
	soltar := func() { //nolint:contextcheck // el teardown corre cuando el contexto del test ya terminó
		// Tras borrar la base (o el contenedor: entonces esto falla y da igual).
		c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := conBaseDeDatos(dsn, "postgres")
		if err != nil {
			return
		}
		a, err := pgx.Connect(c, admin)
		if err != nil {
			return
		}
		defer func() { _ = a.Close(c) }()
		_, _ = a.Exec(c, `DROP ROLE IF EXISTS `+dueno)
	}
	return u.String(), soltar, nil
}

// Harness agrupa el PostgreSQL efimero y el pool conectado para un test de
// integracion. Se obtiene con New y se limpia solo (t.Cleanup); no requiere
// teardown manual, pero se expone Teardown para usos fuera de *testing.T.
type Harness struct {
	// Pool es el pool pgx ya conectado, con las migraciones aplicadas y con el
	// rol de APLICACION (sin superusuario, sin BYPASSRLS): la RLS se aplica.
	Pool *pg.Pool
	// DSN es la cadena de conexion PRIVILEGIADA (rol dueno del contenedor,
	// superusuario). Aplica las migraciones y sirve de escotilla para lo que
	// necesite DDL o saltarse la RLS a proposito. Un pool creado con este DSN NO
	// ejercita las politicas RLS.
	DSN string
	// AppDSN es la cadena de conexion del rol de aplicacion, la misma que usa
	// Pool. Para tests que necesiten un segundo pool con la RLS aplicada.
	AppDSN string

	container *tcpostgres.PostgresContainer
	teardown  func()
}

// New arranca un PostgreSQL efimero, aplica las migraciones del FS embebido
// (fsys/dir, tipicamente db.MigrationsFS / db.MigrationsDir de un servicio) y
// devuelve un Harness con el pool conectado. El contenedor y el pool se cierran
// automaticamente al terminar el test (t.Cleanup). Falla el test (t.Fatal) ante
// cualquier error de infraestructura.
//
// El pool NO conecta como superusuario: usa un rol de aplicacion sin BYPASSRLS
// para que las politicas RLS (deny-by-default) se ejerciten de verdad — igual
// que el rol de aplicacion en produccion.
func New(t *testing.T, fsys fs.FS, dir string) *Harness {
	t.Helper()
	h, err := Start(context.Background(), fsys, dir)
	if err != nil {
		t.Fatalf("pgtest: arrancar harness: %v", err)
	}
	t.Cleanup(h.teardown)
	return h
}

// Start es la variante sin *testing.T (para TestMain o benchmarks): arranca el
// contenedor, aplica migraciones y devuelve el Harness. El llamante DEBE invocar
// Teardown para liberar recursos.
// Start levanta el harness. Por defecto arranca un contenedor con
// testcontainers, que es lo cómodo en un portátil: no hay que tener nada
// preparado.
//
// Con PGTEST_DSN apuntando a un PostgreSQL ya en marcha, usa ese en vez del
// contenedor. Existe por el runner de CI: no monta el socket de contenedores a
// propósito —ahí corre lo que diga un workflow, que puede venir de cualquier
// rama, y darle el motor de contenedores de la máquina es un permiso mayor y
// menos contenido que el resto de lo que hace—. Sin esta vía, la única forma de
// tener tests de integración en CI era relajar eso.
//
// Cada llamada crea su PROPIA base dentro de ese servidor y la borra al
// terminar: los tests corren en paralelo y comparten servidor, no datos.
func Start(ctx context.Context, fsys fs.FS, dir string) (*Harness, error) {
	if dsn := os.Getenv("PGTEST_DSN"); dsn != "" {
		return startExterno(ctx, fsys, dir, dsn)
	}
	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase(testDB),
		tcpostgres.WithUsername(testUser),
		tcpostgres.WithPassword(testPassword),
		// Espera a que el log confirme que acepta conexiones (dos veces: el
		// primer arranque reinicia el servidor tras inicializar el cluster) Y a
		// que el puerto publicado sea alcanzable DESDE EL HOST: con el reenvio
		// dinamico de puertos de colima/Docker Desktop el log llega antes que el
		// forward, y el harness fallaba con «connection refused».
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(startupTimeout),
				wait.ForListeningPort("5432/tcp").
					WithStartupTimeout(startupTimeout),
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("pgtest: run postgres container: %w", err)
	}

	// terminate se acumula en el teardown y se usa para limpiar si algo falla.
	terminate := func() { //nolint:contextcheck // el teardown corre cuando el contexto del test ya terminó
		termCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(termCtx)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		terminate()
		return nil, fmt.Errorf("pgtest: connection string: %w", err)
	}

	return terminarDeMontar(ctx, fsys, dir, dsn, container, terminate)
}

// terminarDeMontar es lo que ambos caminos —contenedor propio o servidor
// externo— hacen igual una vez hay un DSN con permisos de dueño: aprovisionar el
// rol de aplicación, migrar y dejar el pool listo.
func terminarDeMontar(
	ctx context.Context,
	fsys fs.FS,
	dir string,
	dsn string,
	container *tcpostgres.PostgresContainer,
	terminate func(),
) (*Harness, error) {
	// Rol de aplicacion ANTES de migrar: asi los privilegios por defecto cubren
	// los objetos que crean las migraciones.
	if err := execPrivileged(ctx, dsn, appRoleCreate); err != nil {
		terminate()
		return nil, err
	}

	// Migraciones: se aplican como el rol dueno (crea tablas/politicas). El DDL
	// de RLS (ENABLE/FORCE + policies) vive en las propias migraciones del
	// servicio, de modo que este harness es agnostico al esquema.
	if err := pg.MigrateUp(fsys, dir, dsn); err != nil {
		terminate()
		return nil, fmt.Errorf("pgtest: aplicar migraciones: %w", err)
	}

	if err := execPrivileged(ctx, dsn, appRoleGrants); err != nil {
		terminate()
		return nil, err
	}

	appConn, err := withAppRole(dsn)
	if err != nil {
		terminate()
		return nil, err
	}
	if ruta := os.Getenv(traspasoSQLEnv); ruta != "" {
		var soltar func()
		if appConn, soltar, err = traspasarAUnDueno(ctx, dsn, ruta); err != nil {
			terminate()
			return nil, err
		}
		limpiar := terminate
		terminate = func() { limpiar(); soltar() }
	}

	pool, err := pg.Connect(ctx, pg.Config{
		DSN:      appConn,
		MaxConns: 4,
		MinConns: 0,
	})
	if err != nil {
		terminate()
		return nil, fmt.Errorf("pgtest: conectar pool: %w", err)
	}

	h := &Harness{
		Pool:      pool,
		DSN:       dsn,
		AppDSN:    appConn,
		container: container,
	}
	h.teardown = func() {
		pool.Close()
		terminate()
	}
	return h, nil
}

// Teardown cierra el pool y termina el contenedor. Idempotente-seguro: llamarlo
// una sola vez. En tests con New se invoca automaticamente (t.Cleanup).
func (h *Harness) Teardown() {
	if h != nil && h.teardown != nil {
		h.teardown()
	}
}

// PoolSinRLS abre un *pg.Pool conectado con el rol privilegiado del arnés
// (h.DSN, superusuario) sin pasar por el guardián de pg.Connect, que rechaza
// cualquier rol que eluda la seguridad por filas.
//
// Se usa únicamente en tests que:
//   - siembran o cuentan filas saltándose las políticas desde fuera; o
//   - prueban la defensa en profundidad del SQL de un store cuando la RLS no
//     interviene (*_sin_rls_integration_test.go).
func (h *Harness) PoolSinRLS(t *testing.T) *pg.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(h.DSN)
	if err != nil {
		t.Fatalf("pgtest: parsear DSN sin RLS: %v", err)
	}
	cfg.MaxConns = 4
	raw, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgtest: abrir pool sin RLS: %v", err)
	}
	if err := raw.Ping(ctx); err != nil {
		raw.Close()
		t.Fatalf("pgtest: ping pool sin RLS: %v", err)
	}
	t.Cleanup(raw.Close)
	return &pg.Pool{Pool: raw}
}

// Reset trunca todas las tablas de datos del esquema public (CASCADE),
// preservando el esquema y las migraciones aplicadas. Util para aislar
// subtests que comparten el mismo contenedor sin re-arrancar PostgreSQL.
// No toca la tabla de versiones de migracion (schema_migrations).
func (h *Harness) Reset(ctx context.Context) error {
	rows, err := h.Pool.Query(ctx,
		`SELECT tablename FROM pg_tables
		 WHERE schemaname = 'public' AND tablename <> 'schema_migrations'`)
	if err != nil {
		return fmt.Errorf("pgtest: listar tablas: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, tbl := range tables {
		// Nombre proviene del catalogo de PostgreSQL; se cita con comillas
		// dobles para tratarlo como identificador literal.
		if _, err := h.Pool.Exec(ctx, `TRUNCATE TABLE "`+tbl+`" RESTART IDENTITY CASCADE`); err != nil {
			return fmt.Errorf("pgtest: truncar %s: %w", tbl, err)
		}
	}
	return nil
}
