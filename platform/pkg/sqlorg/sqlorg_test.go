// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sqlorg

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// Tres tablas: una con la columna desde que nace, otra que la gana con un ALTER y
// una de catálogo que no la lleva.
var migraciones = fstest.MapFS{
	"m/0001_inicio.up.sql": {Data: []byte(`
CREATE TABLE records (
    id     uuid PRIMARY KEY,
    tenant text NOT NULL,
    estado text
);
CREATE TABLE lineas (
    id uuid PRIMARY KEY
);
CREATE TABLE catalogo (
    clave text PRIMARY KEY
);
`)},
	"m/0002_lineas.up.sql": {Data: []byte("ALTER TABLE lineas ADD COLUMN tenant text;\n")},
}

// faltas revisa un paquete de un solo fichero y devuelve lo que le falta a cada
// sentencia.
func faltas(t *testing.T, cuerpo string) []string {
	t.Helper()
	dir := t.TempDir()
	fuente := "package x\n\nconst columnas = `id, estado`\n\nfunc f() {\n" + cuerpo + "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(fuente), 0o600); err != nil {
		t.Fatal(err)
	}
	hs, err := Revisar(Opciones{Migraciones: migraciones, Dir: "m", Paquetes: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		if h.Clave() != "x.go:f" {
			t.Errorf("clave %q, quiero x.go:f", h.Clave())
		}
		out = append(out, h.Falta)
	}
	return out
}

func TestSentenciasSinLaOrganizacion(t *testing.T) {
	casos := []struct {
		nombre, cuerpo string
		quiero         []string
	}{
		{"lectura por id", "q(`SELECT estado FROM records WHERE id = $1`)", []string{"FROM records"}},
		{"con parámetro", "q(`SELECT estado FROM records WHERE tenant = $2 AND id = $1`)", nil},
		{"con la variable de sesión", "q(`SELECT 1 FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1`)", nil},
		{"la variable de sesión sola no es un filtro", "q(`SELECT current_setting('app.tenant', true) FROM records WHERE id = $1`)", []string{"FROM records"}},
		{"alias sin filtrar", "q(`SELECT r.estado FROM records r WHERE r.id = $1`)", []string{"FROM records (r)"}},
		{"alias filtrado", "q(`SELECT r.estado FROM records r WHERE r.tenant = $2 AND r.id = $1`)", nil},
		{"el filtro de otra tabla no vale", "q(`SELECT 1 FROM records r JOIN lineas l ON l.id = r.id WHERE r.tenant = $1`)", []string{"JOIN lineas (l)"}},
		{"unión por la organización", "q(`SELECT 1 FROM records r JOIN lineas l ON l.tenant = r.tenant AND l.id = r.id WHERE r.tenant = $1`)", nil},
		{"update", "q(`UPDATE records SET estado = $2 WHERE id = $1`)", []string{"UPDATE records"}},
		{"delete", "q(`DELETE FROM records WHERE id = $1`)", []string{"DELETE FROM records"}},
		{"sin where", "q(`SELECT id FROM records ORDER BY id`)", []string{"FROM records"}},
		{"partida con una constante", "q(`SELECT ` + columnas + ` FROM records WHERE id = $1`)", []string{"FROM records"}},
		{"partida y filtrada", "q(`SELECT ` + columnas + ` FROM records WHERE tenant = $2 AND id = $1`)", nil},
		{"insert sin la columna", "q(`INSERT INTO records (id, estado) VALUES ($1, $2)`)", []string{"INSERT INTO records"}},
		{"insert con la columna", "q(`INSERT INTO records (id, tenant) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET estado = 'x'`)", nil},
		{"insert con alias y la columna", "q(`INSERT INTO records AS r (id, tenant) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET estado = 'x'`)", nil},
		{"insert con alias sin la columna", "q(`INSERT INTO records AS r (id, estado) VALUES ($1, $2)`)", []string{"INSERT INTO records"}},
		{"for update no es una tabla", "q(`SELECT 1 FROM records WHERE tenant = $1 FOR UPDATE`)", nil},
		{"tabla sin la columna", "q(`SELECT clave FROM catalogo WHERE clave = $1`)", nil},
		{"columna añadida después", "q(`SELECT id FROM lineas WHERE id = $1`)", []string{"FROM lineas"}},
		{"una igualdad no vale para dos tablas", "q(`SELECT 1 FROM records WHERE tenant = $1 AND EXISTS (SELECT 1 FROM lineas WHERE id = $2)`)", []string{"FROM lineas"}},
		{"sql dentro de una llamada sumada", "q(`x` + g(`SELECT 1 FROM records WHERE id = $1`))", []string{"FROM records"}},
		{"texto que no es sql", "q(`el registro no existe`)", nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if tengo := faltas(t, c.cuerpo); !slices.Equal(tengo, c.quiero) {
				t.Errorf("faltas = %q, quiero %q", tengo, c.quiero)
			}
		})
	}
}

func TestOtraColumna(t *testing.T) {
	dir := t.TempDir()
	fuente := "package x\n\nfunc f() {\n\tq(`SELECT 1 FROM cosas WHERE id = $1`)\n\tq(`SELECT 1 FROM cosas WHERE org_id = $2 AND id = $1`)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(fuente), 0o600); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"m/1.up.sql": {Data: []byte("CREATE TABLE cosas (\n    id uuid,\n    org_id text NOT NULL\n);\n")}}
	hs, err := Revisar(Opciones{Migraciones: fsys, Dir: "m", Columna: "org_id", Paquetes: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].Linea != 4 {
		t.Fatalf("hallazgos = %v, quiero solo el de la línea 4", hs)
	}
}

// Un servicio sin ninguna tabla con la columna no puede salir en verde: lo más
// probable es que la columna se llame de otra forma.
func TestSinTablasEsUnError(t *testing.T) {
	if _, err := Revisar(Opciones{Migraciones: migraciones, Dir: "m", Columna: "org_id", Paquetes: []string{t.TempDir()}}); err == nil {
		t.Fatal("sin tablas con la columna debería fallar")
	}
}

func TestLasExcepcionesSeComparanPorIgualdad(t *testing.T) {
	h := []Hallazgo{{Fichero: "a.go", Linea: 3, Funcion: "Todas", Falta: "FROM records"}}

	if q := quejas(h, map[string]string{"a.go:Todas": "el avisador recorre todas"}); len(q) != 0 {
		t.Errorf("con su excepción no debería quejarse: %q", q)
	}
	if q := quejas(h, nil); len(q) != 1 || !strings.Contains(q[0], "a.go:3 Todas") {
		t.Errorf("sin excepción: %q", q)
	}
	if q := quejas(nil, map[string]string{"a.go:Todas": "ya no cruza"}); len(q) != 1 || !strings.Contains(q[0], "ya no hace falta") {
		t.Errorf("una excepción sin usar tiene que cantar: %q", q)
	}
	if q := quejas(h, map[string]string{"a.go:Todas": " "}); len(q) != 1 || !strings.Contains(q[0], "no dice por qué") {
		t.Errorf("una excepción sin motivo tiene que cantar: %q", q)
	}
}
