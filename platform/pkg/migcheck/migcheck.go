// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package migcheck analiza los ficheros `.up.sql` de las migraciones y rechaza
// las operaciones que rompen la compatibilidad con la versión ANTERIOR del
// servicio si no llevan una marca explícita.
//
// Por qué existe: cada servicio embebe su carpeta db/migrations y aplica
// pg.MigrateUp al arrancar. `helm upgrade --atomic` revierte los pods cuando el
// rollout falla, pero NO deshace una migración ya aplicada; y durante cualquier
// rollout conviven pods viejos y nuevos. Una migración destructiva deja al
// binario N-1 hablando con un esquema que ya no conoce: caída silenciosa que el
// rollback no arregla. El único camino seguro es expand → contract (una versión
// añade, la siguiente retira), y hasta ahora eso era una nota en prosa que nadie
// comprobaba.
//
// La marca de escape es una línea de comentario en el propio fichero:
//
//	-- sytue:destructive-ok la columna quedó sin uso en v1.4.0 (expand en 0018)
//
// Es por fichero —la unidad que se revisa en el PR— y exige una razón escrita:
// el objetivo no es prohibir el paso de contract, es obligar a que alguien lo
// declare y a que el revisor lo vea.
package migcheck

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// Marker es el prefijo que exime a un fichero de migración del gate.
const Marker = "sytue:destructive-ok"

// Finding describe una operación destructiva encontrada en una migración.
type Finding struct {
	Line   int    // línea (1-based) dentro del fichero
	Op     string // operación detectada, p. ej. "DROP COLUMN"
	Source string // texto de la línea, recortado
}

// destructivas son las operaciones que rompen al binario N-1 o pierden datos de
// forma irreversible. Deliberadamente NO están DROP INDEX ni DROP DEFAULT ni
// DROP NOT NULL: ninguna rompe al código anterior (a lo sumo lo hace más lento o
// admite valores que solo escribe el código nuevo), y marcarlas convertiría la
// marca en un trámite que se firma sin leer.
var destructivas = []struct {
	op string
	re *regexp.Regexp
}{
	{"DROP TABLE", regexp.MustCompile(`(?is)\bDROP\s+TABLE\b`)},
	{"DROP COLUMN", regexp.MustCompile(`(?is)\bDROP\s+COLUMN\b`)},
	{"DROP SCHEMA", regexp.MustCompile(`(?is)\bDROP\s+SCHEMA\b`)},
	{"DROP SEQUENCE", regexp.MustCompile(`(?is)\bDROP\s+SEQUENCE\b`)},
	{"DROP TYPE", regexp.MustCompile(`(?is)\bDROP\s+TYPE\b`)},
	{"TRUNCATE", regexp.MustCompile(`(?is)\bTRUNCATE\b`)},
	// Cualquier RENAME (tabla, columna o constraint) deja al binario anterior
	// consultando un nombre inexistente.
	{"RENAME", regexp.MustCompile(`(?is)\bRENAME\b`)},
	// Cambio de tipo de una columna. Ampliar un enum con ALTER TYPE ... ADD VALUE
	// no encaja en el patrón: solo añade valores y el binario anterior sigue leyendo.
	{"ALTER COLUMN TYPE", regexp.MustCompile(`(?is)\bALTER\s+(?:COLUMN\s+)?"?\w+"?\s+(?:SET\s+DATA\s+)?TYPE\b`)},
	// Un NOT NULL nuevo rechaza los INSERT del binario anterior, que no envía la columna.
	{"SET NOT NULL", regexp.MustCompile(`(?is)\bSET\s+NOT\s+NULL\b`)},
}

// redefinibles son objetos con nombre que las migraciones de este repo retiran y
// vuelven a crear en el mismo fichero para redefinirlos (ampliar un CHECK,
// reescribir una política RLS, cambiar el cuerpo de una función). Eso NO es una
// retirada, así que solo se marca el DROP cuyo nombre no reaparece en un CREATE
// del mismo fichero. Sin esta excepción el gate gritaría en el patrón más común
// del repo y dejaría de significar nada.
//
// El emparejamiento es por nombre, sin mirar la tabla: aquí las políticas y las
// constraints van prefijadas con el nombre de su tabla, así que basta. Si algún
// día dos tablas comparten nombre de política, este check las daría por
// redefinidas.
var redefinibles = []struct {
	op     string
	drop   *regexp.Regexp // grupo 1: nombre retirado
	create *regexp.Regexp // grupo 1: nombre creado
}{
	{
		"DROP POLICY",
		regexp.MustCompile(`(?is)\bDROP\s+POLICY\s+(?:IF\s+EXISTS\s+)?"?([\w.]+)"?`),
		regexp.MustCompile(`(?is)\bCREATE\s+POLICY\s+"?([\w.]+)"?`),
	},
	{
		"DROP CONSTRAINT",
		regexp.MustCompile(`(?is)\bDROP\s+CONSTRAINT\s+(?:IF\s+EXISTS\s+)?"?([\w.]+)"?`),
		regexp.MustCompile(`(?is)\bADD\s+CONSTRAINT\s+"?([\w.]+)"?`),
	},
	{
		"DROP FUNCTION",
		regexp.MustCompile(`(?is)\bDROP\s+FUNCTION\s+(?:IF\s+EXISTS\s+)?"?([\w.]+)"?`),
		regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+"?([\w.]+)"?`),
	},
	{
		"DROP TRIGGER",
		regexp.MustCompile(`(?is)\bDROP\s+TRIGGER\s+(?:IF\s+EXISTS\s+)?"?([\w.]+)"?`),
		regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+"?([\w.]+)"?`),
	},
	{
		"DROP VIEW",
		regexp.MustCompile(`(?is)\bDROP\s+(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+EXISTS\s+)?"?([\w.]+)"?`),
		regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?(?:MATERIALIZED\s+)?VIEW\s+"?([\w.]+)"?`),
	},
}

var reMarker = regexp.MustCompile(`(?i)--\s*` + regexp.QuoteMeta(Marker) + `\s*(.*)$`)

// Check devuelve las operaciones destructivas sin justificar de una migración
// `.up.sql`. Lista vacía = la migración puede desplegarse con rollback seguro.
func Check(src string) []Finding {
	if line, reason, ok := findMarker(src); ok {
		if reason == "" {
			// Una marca sin razón es peor que ninguna: exime el fichero entero sin
			// dejar rastro de por qué era seguro.
			return []Finding{{Line: line, Op: "marca sin razón", Source: lineAt(src, line)}}
		}
		return nil
	}

	sql := blank(src)
	var out []Finding
	for _, d := range destructivas {
		for _, loc := range d.re.FindAllStringIndex(sql, -1) {
			out = append(out, finding(src, loc[0], d.op))
		}
	}
	for _, r := range redefinibles {
		creados := map[string]bool{}
		for _, m := range r.create.FindAllStringSubmatch(sql, -1) {
			creados[strings.ToLower(m[1])] = true
		}
		for _, loc := range r.drop.FindAllStringSubmatchIndex(sql, -1) {
			if creados[strings.ToLower(sql[loc[2]:loc[3]])] {
				continue
			}
			out = append(out, finding(src, loc[0], r.op))
		}
	}
	slices.SortStableFunc(out, func(a, b Finding) int { return cmp.Compare(a.Line, b.Line) })
	return out
}

// reCreateTable captura el nombre de la tabla y deja el índice tras el paréntesis
// que abre la lista de columnas.
var reCreateTable = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNLOGGED\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?(\w+)"?\s*\(`)

// reColumnaTenant reconoce la DECLARACIÓN de la columna —nombre seguido de tipo—
// y no sus menciones en `PRIMARY KEY (tenant, …)` o `REFERENCES t (tenant)`.
var reColumnaTenant = regexp.MustCompile(`(?im)^\s*"?tenant"?\s+[a-z]`)

// reAddColumnTenant cubre la otra forma de que una tabla pase a tener tenant.
var reAddColumnTenant = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:ONLY\s+)?"?(\w+)"?[^;]*?\bADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?"?tenant"?\b`)

// reCreatePolicy captura una sentencia CREATE POLICY entera y el nombre de su
// tabla, para poder mirar si esa sentencia menciona `app.merge`.
var reCreatePolicy = regexp.MustCompile(`(?is)\bCREATE\s+POLICY\b[^;]*?\bON\s+(?:ONLY\s+)?"?(\w+)"?[^;]*;`)

// Table es una tabla nombrada dentro de una migración, con la línea donde
// aparece para poder señalarla.
type Table struct {
	Name string
	Line int
}

// TenantTables devuelve las tablas que ESTRENAN columna `tenant` en esta
// migración, por crearse con ella o por añadírsela.
//
// Existe para un gate que se comprueba sobre la carpeta entera de un servicio
// (ver TestMigracionesNuevasTraenPoliticaDeFusion): toda tabla con columna
// `tenant` necesita su política de fusión de organizaciones, y sin ella la
// fusión NO falla donde está la causa. Falla meses después, en la consola de
// administración, con el preflight abortando una fusión entera por una tabla que
// nadie relaciona con la migración que la creó. Ya ha pasado dos veces
// —smime_authorities y smime_identities en mail, slash_commands en rooms—, que
// es la definición de algo que se comprueba en vez de recordarse.
//
// No mira las menciones de `tenant` en `PRIMARY KEY (...)` ni en `REFERENCES t
// (tenant)`: solo la declaración de la columna.
func TenantTables(src string) []Table {
	sql := blank(src)
	var out []Table
	add := func(off int, nombre string) {
		out = append(out, Table{Name: strings.ToLower(nombre), Line: 1 + strings.Count(src[:off], "\n")})
	}
	for _, loc := range reCreateTable.FindAllStringSubmatchIndex(sql, -1) {
		if reColumnaTenant.MatchString(sql[loc[1]:finParentesis(sql, loc[1])]) {
			add(loc[0], sql[loc[2]:loc[3]])
		}
	}
	for _, loc := range reAddColumnTenant.FindAllStringSubmatchIndex(sql, -1) {
		add(loc[0], sql[loc[2]:loc[3]])
	}
	slices.SortStableFunc(out, func(a, b Table) int { return cmp.Compare(a.Line, b.Line) })
	return out
}

// MergePolicyTables devuelve las tablas para las que esta migración crea una
// política de fusión.
//
// El NOMBRE de la política no importa: el preflight la reconoce por lo que hace
// —mirar `app.merge`— y no por cómo se llame. Esa distinción no es teórica:
// `wopi_locks` llevaba desde el principio una política correcta llamada
// `wopi_locks_merge` y el preflight la daba por ausente, abortando la fusión por
// una convención de nombres.
//
// Las políticas se leen del fuente ORIGINAL dentro del rango de su sentencia,
// porque blank vacía los literales y `app.merge` es justo un literal; acotar al
// rango evita que un comentario suelto en el fichero cuente como política.
func MergePolicyTables(src string) []string {
	sql := blank(src)
	var out []string
	for _, loc := range reCreatePolicy.FindAllStringSubmatchIndex(sql, -1) {
		if strings.Contains(src[loc[0]:loc[1]], MergeSetting) {
			out = append(out, strings.ToLower(sql[loc[2]:loc[3]]))
		}
	}
	return out
}

// MergeSetting es la variable de sesión que habilita las políticas de fusión.
// Se repite aquí en vez de importarla de platform/pkg/pg para que este gate no
// arrastre una dependencia a un paquete que abre conexiones.
const MergeSetting = "app.merge"

// finParentesis devuelve el desplazamiento del paréntesis que cierra el abierto
// justo antes de i (la lista de columnas de un CREATE TABLE, que anida los suyos
// en CHECK y DEFAULT).
func finParentesis(sql string, i int) int {
	nivel := 1
	for j := i; j < len(sql); j++ {
		switch sql[j] {
		case '(':
			nivel++
		case ')':
			nivel--
			if nivel == 0 {
				return j
			}
		}
	}
	return len(sql)
}

func finding(src string, off int, op string) Finding {
	line := 1 + strings.Count(src[:off], "\n")
	return Finding{Line: line, Op: op, Source: lineAt(src, line)}
}

func lineAt(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

// findMarker busca la marca sobre el fuente original (los comentarios son
// justo lo que blank borra) y devuelve su línea y la razón escrita.
func findMarker(src string) (line int, reason string, ok bool) {
	for i, l := range strings.Split(src, "\n") {
		if m := reMarker.FindStringSubmatch(l); m != nil {
			return i + 1, strings.TrimSpace(m[1]), true
		}
	}
	return 0, "", false
}

// blank sustituye por espacios lo que no es SQL ejecutable —comentarios y
// literales de texto— conservando longitud y saltos de línea, para que las
// expresiones regulares no se disparen con los comentarios en castellano de las
// propias migraciones (que describen lo que hacen: «...DROP COLUMN...») y para
// que el desplazamiento del hallazgo siga apuntando a la línea real.
//
// Los identificadores entre comillas dobles se conservan: son nombres que las
// reglas necesitan leer. Las etiquetas de dólar ($$...$$) no necesitan trato
// especial: dentro el texto sigue siendo SQL con comillas equilibradas, de modo
// que un DROP dentro de un bloque DO también se detecta.
func blank(src string) string {
	out := []byte(src)
	borra := func(i, j int) {
		for k := i; k < j && k < len(out); k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "--"):
			j := finLinea(src, i)
			borra(i, j)
			i = j
		case strings.HasPrefix(src[i:], "/*"):
			j := finBloque(src, i)
			borra(i, j)
			i = j
		case src[i] == '\'' || src[i] == '"':
			j := finLiteral(src, i)
			if src[i] == '\'' {
				borra(i, j)
			}
			i = j
		default:
			i++
		}
	}
	return string(out)
}

func finLinea(src string, i int) int {
	if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(src)
}

// finBloque devuelve el desplazamiento tras el `*/` que cierra el comentario
// abierto en i. PostgreSQL los anida, a diferencia del SQL estándar.
func finBloque(src string, i int) int {
	nivel, j := 1, i+2
	for j < len(src) && nivel > 0 {
		switch {
		case strings.HasPrefix(src[j:], "/*"):
			nivel++
			j += 2
		case strings.HasPrefix(src[j:], "*/"):
			nivel--
			j += 2
		default:
			j++
		}
	}
	return j
}

// finLiteral devuelve el desplazamiento tras la comilla que cierra el literal (o
// el identificador) abierto en i. PostgreSQL escapa la comilla duplicándola.
func finLiteral(src string, i int) int {
	q := src[i]
	for j := i + 1; j < len(src); j++ {
		if src[j] != q {
			continue
		}
		if j+1 < len(src) && src[j+1] == q {
			j++
			continue
		}
		return j + 1
	}
	return len(src)
}
