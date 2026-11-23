// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migcheck_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/migcheck"
)

// ficheroDeParametros es la única lista de parámetros app.* que los roles de
// servicio pueden fijar en sus funciones: la conceden el despliegue del piloto,
// el compose de dev y el arnés de integración leyendo este fichero. Vive en
// deploy/compose para que llegue a la suite pública; el piloto la lee por el enlace
// deploy/helm/sql/conceder-parametros.sql, que no es una copia.
const ficheroDeParametros = "deploy/compose/postgres/conceder-parametros.sql"

func TestParametrosFijados(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		sql    string
		quiere []migcheck.ParametroFijado
	}{
		{
			nombre: "cláusula SET de CREATE FUNCTION, con search_path delante",
			sql: "CREATE OR REPLACE FUNCTION f(a text)\n" +
				"RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,public SET app.maintenance='on' AS $$\n" +
				"BEGIN NULL; END $$;",
			quiere: []migcheck.ParametroFijado{{Nombre: "app.maintenance", Linea: 2}},
		},
		{
			nombre: "detrás del cuerpo, con TO y con FROM CURRENT",
			sql: "CREATE FUNCTION f() RETURNS int AS $cuerpo$ SELECT 1 $cuerpo$\n" +
				"LANGUAGE sql SET app.uno TO 'on' SET \"app.dos\" FROM CURRENT;",
			quiere: []migcheck.ParametroFijado{{Nombre: "app.uno", Linea: 2}, {Nombre: "app.dos", Linea: 2}},
		},
		{
			nombre: "ALTER FUNCTION … SET y … RESET, y ALTER ROLE",
			sql: "ALTER FUNCTION docs_resolve_share(text) SET app.docs_definer = 'on';\n" +
				"ALTER FUNCTION docs_anfitrion(uuid, text) RESET app.docs_definer;\n" +
				"ALTER ROLE r IN DATABASE d SET app.Tenant = 'x';",
			quiere: []migcheck.ParametroFijado{
				{Nombre: "app.docs_definer", Linea: 1},
				{Nombre: "app.docs_definer", Linea: 2},
				{Nombre: "app.tenant", Linea: 3},
			},
		},
		{
			nombre: "un CREATE FUNCTION que crea un DO, en un cuerpo con dólares anidado",
			sql: "DO $$\nBEGIN\n  EXECUTE $q$\n    CREATE FUNCTION g() RETURNS int LANGUAGE sql\n" +
				"    SET app.dentro = 'on' AS 'SELECT 1'$q$;\nEND $$;",
			quiere: []migcheck.ParametroFijado{{Nombre: "app.dentro", Linea: 5}},
		},
		{
			// Ninguna de estas deja nada escrito en el catálogo: un SET de sesión o
			// de transacción lo puede hacer cualquier rol.
			nombre: "SET de sesión, SET LOCAL, set_config, comentarios y literales no cuentan",
			sql: "SET app.merge = 'on';\nSET LOCAL app.tenant = 'acme';\nRESET app.merge;\n" +
				"-- CREATE FUNCTION f() SET app.comentario = 'on'\n" +
				"/* ALTER FUNCTION f() SET app.bloque = 'on'; /* anidado */ */\n" +
				"SELECT set_config('app.maintenance', 'on', true);\n" +
				"INSERT INTO t VALUES ('CREATE FUNCTION f() SET app.literal = ''on''');\n" +
				"CREATE FUNCTION h() RETURNS SETOF app.tipo LANGUAGE plpgsql AS $$\n" +
				"BEGIN SET app.cuerpo = 'on'; UPDATE t SET c = 1; RETURN; END $$;",
		},
		{
			nombre: "parámetros del propio PostgreSQL, sin punto",
			sql:    "CREATE FUNCTION f() RETURNS int LANGUAGE sql SET search_path = public SET row_security = off AS 'SELECT 1';",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got := migcheck.ParametrosFijados(c.sql)
			if !slices.Equal(got, c.quiere) {
				t.Errorf("ParametrosFijados = %+v, quiere %+v", got, c.quiere)
			}
		})
	}
}

// TestParametrosDeFuncionConcedidos es el gate de la incidencia del 2026-11-16:
// toda migración (y todo fragmento SQL de platform/pkg que se expande dentro de
// ellas) que deje escrito un parámetro propio en una función tiene que tenerlo en
// la lista que concede el despliegue. Si no, el servicio migra en CI —con un
// superusuario— y se queda con la migración sucia en el piloto, donde migra su rol.
//
// Verlo en rojo: quita 'app.maintenance' de la lista del fichero de parámetros.
func TestParametrosDeFuncionConcedidos(t *testing.T) {
	t.Parallel()

	raiz := filepath.Join("..", "..", "..")
	fuente, err := os.ReadFile(filepath.Join(raiz, filepath.FromSlash(ficheroDeParametros)))
	if err != nil {
		t.Fatalf("leer %s: %v", ficheroDeParametros, err)
	}
	concedidos, err := migcheck.ParametrosConcedidos(string(fuente))
	if err != nil {
		t.Fatal(err)
	}
	valido := regexp.MustCompile(`^[a-z_]+\.[a-z_][a-z0-9_]*$`)
	for i, p := range concedidos {
		if !valido.MatchString(p) {
			t.Errorf("%s: «%s» no es un nombre de parámetro propio", ficheroDeParametros, p)
		}
		if i > 0 && concedidos[i-1] >= p {
			t.Errorf("%s: la lista va en orden alfabético y sin repetidos («%s» detrás de «%s»)", ficheroDeParametros, p, concedidos[i-1])
		}
	}

	// Todo .sql de services/ y platform/: las migraciones de cada servicio
	// (subida y bajada: una vuelta atrás también la aplica su rol) y los
	// fragmentos que platform/pkg/pg inserta en ellas.
	var ficheros []string
	for _, dir := range []string{"services", "platform"} {
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(ruta, ".sql") {
				ficheros = append(ficheros, ruta)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("recorrer %s: %v", dir, err)
		}
	}
	// Sin esto, mover las carpetas apagaría el gate en silencio.
	if len(ficheros) < 500 {
		t.Fatalf("solo %d ficheros .sql: ¿han cambiado de sitio? el gate estaría desactivado", len(ficheros))
	}

	vistos := 0
	for _, f := range ficheros {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		for _, p := range migcheck.ParametrosFijados(string(src)) {
			vistos++
			if slices.Contains(concedidos, p.Nombre) {
				continue
			}
			rel, _ := filepath.Rel(raiz, f)
			t.Errorf("%s:%d: deja «%s» escrito en una función, y no está en %s.\n\t"+
				"En el piloto cada servicio migra con su rol, sin SUPERUSER, y desde PostgreSQL 15 eso\n\t"+
				"exige GRANT SET ON PARAMETER: la migración fallaría y el servicio no arrancaría\n\t"+
				"(incidencia del 2026-11-16). Añade '%s' a la lista `parametros` de ese fichero.",
				filepath.ToSlash(rel), p.Linea, p.Nombre, ficheroDeParametros, p.Nombre)
		}
	}
	// Hoy hay decenas (almacenamiento privado, admisión de ámbitos, Docs). Si el
	// analizador deja de ver ninguna, está roto, no limpio.
	if vistos == 0 {
		t.Fatal("ninguna migración fija un parámetro en una función: el analizador no ve las que hay")
	}
}

// TestLaListaDeParametrosNoSeCopia: el permiso se concede SOLO desde el fichero de
// parámetros. Una segunda lista en un guion o un manifiesto se queda atrás el día
// que se añade un parámetro, que es justo el fallo que esto previene.
func TestLaListaDeParametrosNoSeCopia(t *testing.T) {
	t.Parallel()

	raiz := filepath.Join("..", "..", "..")
	grant := regexp.MustCompile(`(?i)GRANT\s+SET\s+ON\s+PARAMETER`)
	var copias []string
	for _, dir := range []string{"deploy", "platform", "services", "scripts", "infra"} {
		// La distribución pública (sytue-suite) no lleva infra/: lo que no está no
		// puede copiar la lista, así que se salta en vez de fallar.
		if _, err := os.Stat(filepath.Join(raiz, dir)); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(raiz, ruta)
			rel = filepath.ToSlash(rel)
			// La fuente, los enlaces a ella (no son copias) y los tests que comprueban
			// que se usa la fuente.
			if rel == ficheroDeParametros || d.Type()&fs.ModeSymlink != 0 ||
				strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, "-test.sh") {
				return nil
			}
			switch filepath.Ext(rel) {
			case ".sh", ".sql", ".go", ".yaml", ".yml", ".tpl", ".tf", ".py":
			default:
				return nil
			}
			src, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			// Los comentarios pueden nombrarlo (para explicar por qué hace falta).
			for _, linea := range strings.Split(string(src), "\n") {
				linea = strings.TrimSpace(linea)
				if strings.HasPrefix(linea, "#") || strings.HasPrefix(linea, "//") ||
					strings.HasPrefix(linea, "--") || strings.HasPrefix(linea, "*") {
					continue
				}
				if grant.MatchString(linea) {
					copias = append(copias, rel)
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("recorrer %s: %v", dir, err)
		}
	}
	for _, c := range copias {
		t.Errorf("%s concede SET sobre parámetros por su cuenta: hazlo ejecutando %s, que es la única lista", c, ficheroDeParametros)
	}
}
