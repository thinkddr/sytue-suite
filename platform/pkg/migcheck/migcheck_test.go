// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migcheck_test

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"git.sytue.eu/sytue/one/platform/pkg/migcheck"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		sql    string
		quiere []string // operaciones esperadas, en orden
	}{
		{"añadir columna", `ALTER TABLE users ADD COLUMN nickname text;`, nil},
		{"crear tabla e índice", "CREATE TABLE t (id uuid PRIMARY KEY);\nCREATE INDEX t_idx ON t (id);", nil},
		{"drop index no es destructivo", `DROP INDEX outbound_claim_idx;`, nil},
		{"drop not null amplía", `ALTER TABLE t ALTER COLUMN c DROP NOT NULL;`, nil},

		{"drop column", `ALTER TABLE t DROP COLUMN c;`, []string{"DROP COLUMN"}},
		{"drop table", `DROP TABLE t;`, []string{"DROP TABLE"}},
		{"rename column", `ALTER TABLE t RENAME COLUMN a TO b;`, []string{"RENAME"}},
		{"cambio de tipo", `ALTER TABLE t ALTER COLUMN c TYPE bigint;`, []string{"ALTER COLUMN TYPE"}},
		{"cambio de tipo sin COLUMN", `ALTER TABLE t ALTER c SET DATA TYPE bigint;`, []string{"ALTER COLUMN TYPE"}},
		{"not null nuevo", `ALTER TABLE t ALTER COLUMN c SET NOT NULL;`, []string{"SET NOT NULL"}},
		{"truncate", `TRUNCATE outbox;`, []string{"TRUNCATE"}},

		// ALTER TYPE ... ADD VALUE solo amplía un enum: compatible con el binario N-1.
		{"ampliar enum", `ALTER TYPE estado ADD VALUE 'pausado';`, nil},

		// Patrón de redefinición: retirar y volver a crear el mismo nombre en el
		// mismo fichero no retira nada.
		{"check ampliado", "ALTER TABLE t DROP CONSTRAINT IF EXISTS t_kind_check;\nALTER TABLE t ADD CONSTRAINT t_kind_check CHECK (kind IN ('a','b'));", nil},
		{"política reescrita", "DROP POLICY t_tenant ON t;\nCREATE POLICY t_tenant ON t USING (true);", nil},
		{"función reescrita", "DROP FUNCTION IF EXISTS f(text);\nCREATE OR REPLACE FUNCTION f(text) RETURNS int AS $$ SELECT 1 $$ LANGUAGE sql;", nil},
		{"constraint retirada de verdad", `ALTER TABLE t DROP CONSTRAINT t_kind_check;`, []string{"DROP CONSTRAINT"}},
		{"política retirada de verdad", `DROP POLICY t_tenant ON t;`, []string{"DROP POLICY"}},
		{"función retirada de verdad", `DROP FUNCTION IF EXISTS f(text);`, []string{"DROP FUNCTION"}},

		// El texto que no se ejecuta no cuenta.
		{"comentario de línea", "-- antes esto hacía DROP COLUMN c\nALTER TABLE t ADD COLUMN c text;", nil},
		{"comentario de bloque anidado", "/* DROP TABLE t; /* anidado DROP COLUMN c */ */\nSELECT 1;", nil},
		{"literal de texto", `INSERT INTO audit (msg) VALUES ('DROP TABLE users');`, nil},
		{"literal con comilla escapada", `INSERT INTO t (a, b) VALUES ('it''s DROP TABLE x', 'ok');`, nil},

		// Dentro de un bloque DO sí se ejecuta.
		{"destructivo dentro de DO", "DO $$ BEGIN\n  EXECUTE 'noop';\n  ALTER TABLE t DROP COLUMN c;\nEND $$;", []string{"DROP COLUMN"}},

		// La marca exime el fichero, pero solo si trae razón.
		{"marca con razón", "-- sytue:destructive-ok columna sin uso desde v1.4.0 (expand en 0018)\nALTER TABLE t DROP COLUMN c;", nil},
		{"marca sin razón", "-- sytue:destructive-ok\nALTER TABLE t DROP COLUMN c;", []string{"marca sin razón"}},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			hallazgos := migcheck.Check(c.sql)
			ops := make([]string, 0, len(hallazgos))
			for _, f := range hallazgos {
				ops = append(ops, f.Op)
			}
			if strings.Join(ops, ",") != strings.Join(c.quiere, ",") {
				t.Fatalf("Check() = %v, quiere %v", ops, c.quiere)
			}
		})
	}
}

func TestCheckLinea(t *testing.T) {
	t.Parallel()

	src := "-- comentario\n\nALTER TABLE t ADD COLUMN c text;\nALTER TABLE t DROP COLUMN d;\n"
	f := migcheck.Check(src)
	if len(f) != 1 {
		t.Fatalf("Check() = %d hallazgos, quiere 1", len(f))
	}
	if f[0].Line != 4 {
		t.Errorf("línea = %d, quiere 4", f[0].Line)
	}
	if f[0].Source != "ALTER TABLE t DROP COLUMN d;" {
		t.Errorf("fuente = %q", f[0].Source)
	}
}

// TestMigracionesDelRepo es el gate: ninguna migración `.up.sql` del monorepo
// puede llevar operaciones destructivas sin la marca. Vive como test para que lo
// ejecute el `go test ./...` que ya corre en ci-go.yml, sin un job aparte.
func TestMigracionesDelRepo(t *testing.T) {
	t.Parallel()

	raiz := filepath.Join("..", "..", "..")
	var ficheros []string
	for _, patron := range []string{
		filepath.Join(raiz, "services", "*", "db", "migrations", "*.up.sql"),
		filepath.Join(raiz, "platform", "pkg", "*", "migrations", "*.up.sql"),
	} {
		m, err := filepath.Glob(patron)
		if err != nil {
			t.Fatalf("glob %s: %v", patron, err)
		}
		ficheros = append(ficheros, m...)
	}
	// Sin esto, mover el paquete o renombrar las carpetas apagaría el gate en
	// silencio y el test seguiría en verde.
	if len(ficheros) < 100 {
		t.Fatalf("solo %d migraciones encontradas: ¿han cambiado de sitio? el gate estaría desactivado", len(ficheros))
	}

	for _, f := range ficheros {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		for _, h := range migcheck.Check(string(src)) {
			rel, _ := filepath.Rel(raiz, f)
			t.Errorf("%s:%d: %s — %s\n\t"+
				"rompe a la versión anterior del servicio, que sigue viva durante el rollout y tras un rollback de Helm.\n\t"+
				"Divide el cambio en expand → contract, o si el expand ya se desplegó añade al fichero:\n\t"+
				"-- %s <razón>",
				rel, h.Line, h.Op, h.Source, migcheck.Marker)
		}
	}
}

func TestTenantTablesYMergePolicyTables(t *testing.T) {
	t.Parallel()

	const politica = `CREATE POLICY t_platform_merge ON t
	    USING (current_setting('app.merge', true) = 'on')
	    WITH CHECK (current_setting('app.merge', true) = 'on');`

	casos := []struct {
		nombre    string
		sql       string
		tablas    []string // las que estrenan `tenant`
		politicas []string // las que reciben política de fusión
	}{
		{"tabla sin tenant", `CREATE TABLE t (id uuid PRIMARY KEY);`, nil, nil},
		{"tabla con tenant", `CREATE TABLE t (tenant text NOT NULL, id uuid);`, []string{"t"}, nil},
		{
			"tabla con tenant y política", "CREATE TABLE t (tenant text NOT NULL);\n" + politica,
			[]string{"t"},
			[]string{"t"},
		},
		{"añadir columna tenant", `ALTER TABLE t ADD COLUMN tenant text NOT NULL;`, []string{"t"}, nil},
		{"añadir columna tenant si no existe", `ALTER TABLE t ADD COLUMN IF NOT EXISTS tenant text;`, []string{"t"}, nil},
		// El nombre de la política es irrelevante: la hace válida mirar `app.merge`.
		// wopi_locks llamó a la suya `wopi_locks_merge` y era correcta.
		{
			"política con otro nombre", `CREATE POLICY t_merge ON t USING (current_setting('app.merge', true) = 'on');`,
			nil,
			[]string{"t"},
		},
		// Una política de aislamiento NO es una de fusión, por mucho que exista.
		{
			"política de aislamiento no cuenta", `CREATE POLICY t_iso ON t USING (tenant = current_setting('app.tenant', true));`,
			nil, nil,
		},
		// `tenant` en una clave o una referencia no declara la columna.
		{"tenant solo en clave ajena", "CREATE TABLE t (id uuid, otro text,\n" +
			"    CONSTRAINT fk FOREIGN KEY (otro) REFERENCES o (tenant));", nil, nil},
		{
			"tenant solo en clave primaria compuesta", "CREATE TABLE t (a text, b text,\n    PRIMARY KEY (tenant, a));",
			nil, nil,
		},
		// El comentario en castellano de la propia migración no es una política.
		{
			"app.merge solo en un comentario", "-- pendiente: app.merge\nCREATE TABLE t (tenant text NOT NULL);",
			[]string{"t"},
			nil,
		},
		// Un CHECK con paréntesis dentro no debe cortar el cuerpo antes de tiempo.
		{
			"tenant tras un check con paréntesis", "CREATE TABLE t (\n" +
				"    name text CHECK (name ~ '^[a-z](x)$'),\n    tenant text NOT NULL\n);",
			[]string{"t"},
			nil,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			var tablas []string
			for _, x := range migcheck.TenantTables(c.sql) {
				tablas = append(tablas, x.Name)
			}
			if !slices.Equal(tablas, c.tablas) {
				t.Errorf("tablas con tenant: got %v, want %v", tablas, c.tablas)
			}
			if got := migcheck.MergePolicyTables(c.sql); !slices.Equal(got, c.politicas) {
				t.Errorf("tablas con política de fusión: got %v, want %v", got, c.politicas)
			}
		})
	}
}

// Toda tabla con columna `tenant` creada DESPUÉS del barrido de su servicio debe
// traer su política de fusión. Antes del barrido no aplica: de eso se encarga la
// propia migración de barrido, que las recorre todas.
//
// Se comprueba aquí, sobre los ficheros, y no en un test de integración con base
// de datos, porque este corre en el `go test ./...` de siempre. El fallo que
// previene tarda meses en aparecer y lo hace en un sitio que no se parece en
// nada a su causa: una fusión de organizaciones abortando en la consola de
// administración por una tabla que se creó en otro servicio y otro trimestre.
func TestMigracionesNuevasTraenPoliticaDeFusion(t *testing.T) {
	t.Parallel()

	raiz := filepath.Join("..", "..", "..")
	carpetas, err := filepath.Glob(filepath.Join(raiz, "services", "*", "db", "migrations"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(carpetas) < 5 {
		t.Fatalf("solo %d carpetas de migraciones: ¿han cambiado de sitio? el gate estaría desactivado", len(carpetas))
	}

	revisadas := 0
	for _, carpeta := range carpetas {
		barrido, err := filepath.Glob(filepath.Join(carpeta, "*_platform_merge_policy.up.sql"))
		if err != nil {
			t.Fatalf("glob %s: %v", carpeta, err)
		}
		// Un servicio sin barrido no participa en la fusión (id no se mueve por
		// tenant). Si algún día uno que sí participa lo pierde, lo dice el
		// preflight, que es quien conoce la lista de bases.
		if len(barrido) == 0 {
			continue
		}
		desde := filepath.Base(barrido[0])

		ficheros, err := filepath.Glob(filepath.Join(carpeta, "*.up.sql"))
		if err != nil {
			t.Fatalf("glob %s: %v", carpeta, err)
		}
		sort.Strings(ficheros)

		// El cruce es por CARPETA y no por fichero: las migraciones de un servicio
		// se aplican todas seguidas en el mismo arranque, así que la política puede
		// llegar en una posterior sin dejar ninguna ventana real. Exigirla en el
		// mismo fichero obligaría a reescribir migraciones ya aplicadas, que es
		// justamente lo que no se hace.
		type pendiente struct {
			tabla   string
			fichero string
			linea   int
		}
		var pendientes []pendiente
		conPolitica := map[string]bool{}
		for _, f := range ficheros {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("leer %s: %v", f, err)
			}
			for _, tabla := range migcheck.MergePolicyTables(string(src)) {
				conPolitica[tabla] = true
			}
			// Las anteriores al barrido ya las cubre el propio barrido, que las
			// recorre todas por catálogo.
			if filepath.Base(f) <= desde {
				continue
			}
			revisadas++
			for _, tabla := range migcheck.TenantTables(string(src)) {
				rel, _ := filepath.Rel(raiz, f)
				pendientes = append(pendientes, pendiente{tabla.Name, rel, tabla.Line})
			}
		}
		for _, p := range pendientes {
			if conPolitica[p.tabla] {
				continue
			}
			t.Errorf("%s:%d: la tabla %s tiene columna `tenant` y ninguna migración de este "+
				"servicio le da política de fusión.\n\t"+
				"La fusión de organizaciones abortará en el preflight, y hasta que alguien la "+
				"lance esta tabla se quedaría atrás sin que nada falle.\n\t"+
				"Añade una migración con:\n\t"+
				"CREATE POLICY %s_platform_merge ON %s\n\t"+
				"    USING (current_setting('%s', true) = 'on')\n\t"+
				"    WITH CHECK (current_setting('%s', true) = 'on');",
				p.fichero, p.linea, p.tabla, p.tabla, p.tabla,
				migcheck.MergeSetting, migcheck.MergeSetting)
		}
	}
	if revisadas == 0 {
		t.Fatal("ninguna migración posterior al barrido: el gate no está comprobando nada")
	}
}

// Dos migraciones con el MISMO número dejan al servicio sin arrancar: el índice
// de golang-migrate falla al construirse («duplicate migration file»), antes
// incluso de conectar a la base. No es hipotético — pasó al añadir a la vez el
// client OIDC de la forja y el de la administración de plataforma, ambos como
// 0032, y la única señal fue un pod que no levantaba.
//
// Es el fallo típico de dos sesiones trabajando en paralelo: cada una coge «el
// siguiente número» sin ver el del otro, y nada lo detecta hasta el despliegue.
func TestNoHayVersionesDeMigracionDuplicadas(t *testing.T) {
	t.Parallel()

	raiz := filepath.Join("..", "..", "..")
	carpetas, err := filepath.Glob(filepath.Join(raiz, "services", "*", "db", "migrations"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(carpetas) < 5 {
		t.Fatalf("solo %d carpetas de migraciones: ¿han cambiado de sitio?", len(carpetas))
	}
	for _, carpeta := range carpetas {
		ficheros, err := filepath.Glob(filepath.Join(carpeta, "*.up.sql"))
		if err != nil {
			t.Fatalf("glob %s: %v", carpeta, err)
		}
		porVersion := map[string][]string{}
		for _, f := range ficheros {
			base := filepath.Base(f)
			version, _, ok := strings.Cut(base, "_")
			if !ok {
				t.Errorf("%s no sigue el patrón <version>_<nombre>.up.sql", base)
				continue
			}
			porVersion[version] = append(porVersion[version], base)
		}
		for version, nombres := range porVersion {
			if len(nombres) > 1 {
				sort.Strings(nombres)
				rel, _ := filepath.Rel(raiz, carpeta)
				t.Errorf("%s: la versión %s está duplicada (%s).\n\t"+
					"El servicio NO arrancaría: el índice de migraciones falla al construirse.\n\t"+
					"Renumera la más reciente al siguiente hueco libre.",
					rel, version, strings.Join(nombres, ", "))
			}
		}
	}
}

// TestRedirectUrisDeSytueComCubrenElCompose comprueba que CADA superficie que el
// frontal sirve en `sytue.com` tiene su `redirect_uri` registrado en Sytue ID.
//
// POR QUÉ EXISTE. `sytue.com` corre un SEGUNDO JUEGO de las mismas imágenes Astro
// (ADR 0049), y cada contenedor anuncia su `SYTUE_OIDC_REDIRECT_URI` en
// `deploy/sytue-com/docker-compose.yml`. OIDC compara ese valor LITERAL contra la
// lista registrada, que vive en una migración. Las dos mitades están en ficheros
// distintos y nada las ataba.
//
// El fallo que produce no se parece a un fallo: el contenedor arranca, `/healthz`
// contesta, la portada se sirve, y lo único que no funciona es entrar. Y solo por
// ese dominio, así que `sytue.eu` va bien mientras `sytue.com` está muerto. Ya
// pasó con el host de organización: la migración 0054 se escribió DESPUÉS de que
// el chart empezara a anunciar esa dirección.
//
// Verlo en rojo: quita una fila de 0094, o añade un servicio al compose con su
// `SYTUE_OIDC_REDIRECT_URI` y sin su fila.
func TestRedirectUrisDeSytueComCubrenElCompose(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")

	compose, err := os.ReadFile(filepath.Join(raiz, "deploy", "sytue-com", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("leer el compose de sytue.com: %v", err)
	}
	// Del compose se leen los `redirect_uri` tal cual están escritos: es
	// EXACTAMENTE la cadena que el contenedor le manda al IdP.
	re := regexp.MustCompile(`SYTUE_OIDC_REDIRECT_URI:\s*(\S+)`)
	anunciados := re.FindAllStringSubmatch(string(compose), -1)
	if len(anunciados) == 0 {
		t.Fatal("el compose de sytue.com no anuncia ningún redirect_uri: el test no comprobaría nada")
	}

	// Todo el SQL de Sytue ID junto: da igual en qué migración esté registrada
	// una URL, lo que importa es que lo esté. Solo los `.up.sql`: los `.down.sql`
	// la nombran precisamente para quitarla.
	ficheros, err := filepath.Glob(filepath.Join(raiz, "services", "id", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob de las migraciones de Sytue ID: %v", err)
	}
	if len(ficheros) == 0 {
		t.Fatal("no se encontró ninguna migración de Sytue ID: el test no comprobaría nada")
	}
	var sql strings.Builder
	for _, f := range ficheros {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		sql.Write(b)
	}
	registradas := sql.String()

	vistos := make([]string, 0, len(anunciados))
	for _, m := range anunciados {
		uri := strings.Trim(m[1], `"'`)
		vistos = append(vistos, uri)
		if !strings.Contains(registradas, "'"+uri+"'") {
			t.Errorf("el frontal anuncia %s y Sytue ID no lo tiene registrado.\n\t"+
				"El contenedor arranca igual y su /healthz contesta, así que el despliegue\n\t"+
				"sale verde; lo que falla es ENTRAR, y solo por sytue.com.\n\t"+
				"Añade la fila a services/id/db/migrations/0094_redirect_uri_de_sytue_com.up.sql\n\t"+
				"(y a su .down.sql).", uri)
		}
	}
	slices.Sort(vistos)
	if len(slices.Compact(vistos)) != len(vistos) {
		t.Errorf("hay redirect_uri repetidos en el compose de sytue.com: %v.\n\t"+
			"Dos superficies con el mismo callback significa que una manda a la otra.", vistos)
	}
}

// TestRedirectUrisDeLosHostsDeSuiteCubrenElChart comprueba que CADA superficie
// con prefijo tiene registrado su `redirect_uri` en Sytue ID para CADA host que
// sirve la suite entera: el de organización (`orgHost.canonical`) y los dominios
// extra (`orgHost.extraHosts`, hoy `sytue.com`).
//
// POR QUÉ EXISTE. OIDC compara el `redirect_uri` LITERAL, y el chart lo compone
// solo: `https://<host>/<path>/auth/callback`. Las dos mitades viven en sitios
// distintos —el prefijo y el client_id en values.yaml, la URL en una migración—
// y nada las ataba. El fallo que produce no se parece a un fallo: el despliegue
// sale verde, los pods pasan sus sondas, la portada responde, y lo único que no
// funciona es entrar. Y solo por ese host, así que en `sytue.eu` todo va bien
// mientras `sytue.com` está muerto.
//
// Ya había pasado con el host de organización: la migración 0054 se escribió
// DESPUÉS de que el chart empezara a anunciar esa dirección. Esto lo detecta en
// el mismo commit que añade la app.
//
// Verlo en rojo: quita una fila de 0094 (o añade una app con `path` y
// `clientId` a values.yaml sin su fila) y este test la nombra.
func TestRedirectUrisDeLosHostsDeSuiteCubrenElChart(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")

	crudo, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "charts", "sytue-frontend", "values.yaml"))
	if err != nil {
		t.Fatalf("leer values.yaml del chart de frontends: %v", err)
	}
	var chart struct {
		Domain  string `yaml:"domain"`
		OrgHost struct {
			Canonical  string   `yaml:"canonical"`
			ExtraHosts []string `yaml:"extraHosts"` //nolint:tagliatelle // contrato de values.yaml de Helm.
		} `yaml:"orgHost"` //nolint:tagliatelle // contrato de values.yaml de Helm.
		Apps map[string]struct {
			Enabled    bool   `yaml:"enabled"`
			Path       string `yaml:"path"`
			Subdomain  string `yaml:"subdomain"`
			GlobalHost string `yaml:"globalHost"` //nolint:tagliatelle // contrato de values.yaml de Helm.
			ClientID   string `yaml:"clientId"`   //nolint:tagliatelle // contrato de values.yaml de Helm.
		} `yaml:"apps"`
	}
	if err := yaml.Unmarshal(crudo, &chart); err != nil {
		t.Fatalf("interpretar values.yaml: %v", err)
	}
	dominio := cmp.Or(chart.Domain, "sytue.eu")

	// Los hosts que sirven la suite ENTERA por prefijos. El comodín del Ingress
	// cubre cualquier organización, pero OIDC exige la URL exacta, así que se
	// comprueba el host de organización (su callback sigue valiendo durante la
	// transición) más los dominios extra.
	hosts := append([]string{chart.OrgHost.Canonical + "." + dominio}, chart.OrgHost.ExtraHosts...)

	// Todo el SQL de Sytue ID junto: da igual en qué migración esté registrada
	// una URL, lo que importa es que lo esté. Se leen los `.up.sql` y NO los
	// `.down.sql`, que la nombran precisamente para quitarla.
	ficheros, err := filepath.Glob(filepath.Join(raiz, "services", "id", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob de las migraciones de Sytue ID: %v", err)
	}
	if len(ficheros) == 0 {
		t.Fatal("no se encontró ninguna migración de Sytue ID: el test no comprobaría nada")
	}
	var sql strings.Builder
	for _, f := range ficheros {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		sql.Write(b)
	}
	registradas := sql.String()

	nombres := make([]string, 0, len(chart.Apps))
	for nombre := range chart.Apps {
		nombres = append(nombres, nombre)
	}
	slices.Sort(nombres)

	for _, nombre := range nombres {
		app := chart.Apps[nombre]
		// Sin prefijo (`www`, `platform`) la app se sirve en su propio host y no
		// entra por el Ingress de organización; sin `clientId` no tiene BFF.
		if !app.Enabled || app.Path == "" || app.ClientID == "" {
			continue
		}
		// La CANÓNICA desde el 2026-09-26 es la de PRODUCTO
		// (`<subdomain>.<dominio><path>`, migración 0099): es la que el chart
		// anuncia en `SYTUE_OIDC_REDIRECT_URI`, así que es la que NO puede faltar
		// — si falta, el login falla con invalid_request en el host de producto
		// y sin rastro en los pods. Excepción: con `globalHost` (Mi cuenta) lo
		// canónico es el host global (`id.<dominio>/account`, 0097), que ya
		// comprueba su propia migración.
		if app.GlobalHost == "" {
			producto := "https://" + app.Subdomain + "." + dominio + app.Path + "/auth/callback"
			if !strings.Contains(registradas, "'"+producto+"'") {
				t.Errorf("app %q (client %s): falta el redirect_uri CANÓNICO %s en services/id/db/migrations/.\n\t"+
					"El chart lo anuncia en SYTUE_OIDC_REDIRECT_URI, así que el despliegue saldrá verde\n\t"+
					"y el login de esa app fallará SOLO en %s, con invalid_request y sin rastro en los pods.\n\t"+
					"Añade la fila a la migración de ese host (0099 para los hosts de producto,\n\t"+
					"0054 para el host de organización, 0094 para los dominios extra).",
					nombre, app.ClientID, producto, producto)
			}
		}
		for _, host := range hosts {
			uri := "https://" + host + app.Path + "/auth/callback"
			if !strings.Contains(registradas, "'"+uri+"'") {
				t.Errorf("app %q (client %s): falta el redirect_uri %s en services/id/db/migrations/.\n\t"+
					"El chart lo anuncia igualmente, así que el despliegue saldrá verde y el login\n\t"+
					"de esa app fallará SOLO en %s, con invalid_request y sin rastro en los pods.\n\t"+
					"Añade la fila a la migración de ese host (0054 para el host de organización,\n\t"+
					"0094 para los dominios extra).",
					nombre, app.ClientID, uri, host)
			}
		}
	}
}

// TestCadaFrenteDeSytueComTieneSuPropioClientSecret comprueba que ningún
// servicio del compose de `sytue.com` se queda sin su `SYTUE_OIDC_CLIENT_SECRET`
// propio.
//
// POR QUÉ EXISTE. Los trece contenedores comparten `env_file: [.env,
// .secrets.env]`, que es UN fichero. La primera versión del compose no nombraba
// `SYTUE_OIDC_CLIENT_SECRET` en ningún sitio, así que los doce con login habrían
// recibido EL MISMO valor —el que estuviera en ese fichero— cuando cada cliente
// OIDC tiene el suyo. El README ya pedía `SYTUE_OIDC_CLIENT_SECRET_<APP>`, pero
// nada leía esa variable: la mitad que documenta y la que ejecuta no coincidían.
//
// El fallo, otra vez, no se parece a un fallo: once de los doce arrancan,
// contestan `/healthz` y sirven su portada; lo único que no funciona es entrar,
// con `invalid_client` del IdP.
//
// Verlo en rojo: quita una línea `SYTUE_OIDC_CLIENT_SECRET:` del compose, o
// ponle a dos servicios la misma variable.
func TestCadaFrenteDeSytueComTieneSuPropioClientSecret(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")
	b, err := os.ReadFile(filepath.Join(raiz, "deploy", "sytue-com", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("leer el compose de sytue.com: %v", err)
	}

	// Un servicio necesita secreto si y solo si anuncia un `client_id`: `www` es
	// el sitio público y no inicia sesión.
	reID := regexp.MustCompile(`SYTUE_OIDC_CLIENT_ID:\s*(\S+)`)
	reSec := regexp.MustCompile(`SYTUE_OIDC_CLIENT_SECRET:\s*\$\{(SYTUE_OIDC_CLIENT_SECRET_[A-Z0-9_]+)`)
	conID := reID.FindAllStringSubmatch(string(b), -1)
	conSec := reSec.FindAllStringSubmatch(string(b), -1)

	if len(conID) == 0 {
		t.Fatal("el compose de sytue.com no anuncia ningún client_id: el test no comprobaría nada")
	}
	if len(conSec) != len(conID) {
		t.Errorf("%d servicios anuncian client_id y solo %d tienen su client_secret propio.\n\t"+
			"Los que falten heredarán el valor suelto de .secrets.env, que es UN fichero para\n\t"+
			"los trece: arrancarán y contestarán /healthz, y fallará el login con invalid_client.",
			len(conID), len(conSec))
	}

	// Y que no se repita ninguna: dos servicios apuntando a la misma variable es
	// el mismo fallo escrito de otra forma.
	vistas := make(map[string]bool, len(conSec))
	for _, m := range conSec {
		if vistas[m[1]] {
			t.Errorf("dos servicios leen %s: cada cliente OIDC tiene su propio secreto", m[1])
		}
		vistas[m[1]] = true
	}
}
