// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sqlorg es el guardián de «la organización va EN el SQL»: recorre los
// literales SQL del código Go de un servicio y falla si una sentencia lee o
// modifica una tabla con columna de organización sin nombrarla en su predicado.
//
// Por qué existe: el aislamiento entre organizaciones descansaba en la política
// RLS de cada tabla, y PostgreSQL no evalúa ninguna política para un superusuario
// ni para un rol con BYPASSRLS. Con ese rol en la conexión, un `WHERE id = $1`
// devuelve la fila de cualquiera. La política sigue siendo la segunda capa; la
// primera tiene que estar escrita en la consulta, donde se ve y donde no depende
// de con qué rol se conecte el despliegue.
//
// Cada servicio lo llama desde un test suyo, con sus migraciones, sus
// directorios y su lista de excepciones (lo que cruza organizaciones a
// propósito):
//
//	sqlorg.Exigir(t, sqlorg.Opciones{
//		Migraciones: db.Migrations, Dir: db.MigrationsDir,
//		Paquetes:    []string{"."},
//		Excepciones: map[string]string{"compartido.go:TenantsConPerfil": "el avisador recorre todas"},
//	})
//
// Es un análisis de texto, no un intérprete de SQL: mira qué tablas nombra cada
// sentencia tras FROM, JOIN, UPDATE, DELETE FROM e INSERT INTO, y exige una
// igualdad sobre la columna de organización de cada una (con su alias si lo
// lleva). Lo que no sabe leer —un nombre de tabla que llega por variable— no lo
// vigila.
package sqlorg

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Opciones dice qué servicio se revisa.
type Opciones struct {
	// Migraciones y Dir son el FS embebido del servicio y su subcarpeta: de sus
	// `CREATE TABLE` sale qué tablas llevan la columna de organización.
	Migraciones fs.FS
	Dir         string
	// Columna es el nombre de esa columna. Vacío es "tenant".
	Columna string
	// Paquetes son los directorios con el SQL, relativos al test que llama.
	Paquetes []string
	// Excepciones son las funciones que cruzan organizaciones a propósito, como
	// "fichero.go:Funcion", cada una con su porqué. Se comparan por IGUALDAD: una
	// excepción que ya no hace falta también falla.
	Excepciones map[string]string
}

// Hallazgo es una sentencia que toca una tabla con organización sin filtrarla.
type Hallazgo struct {
	Fichero string
	Linea   int
	Funcion string
	Falta   string // p. ej. "FROM records" o "UPDATE records (r)"
}

// Clave es cómo se nombra el hallazgo en Excepciones.
func (h Hallazgo) Clave() string { return h.Fichero + ":" + h.Funcion }

func (h Hallazgo) String() string {
	return fmt.Sprintf("%s:%d %s: %s sin la organización en el predicado", h.Fichero, h.Linea, h.Funcion, h.Falta)
}

// Exigir falla el test si hay sentencias sin filtrar fuera de las excepciones, o
// excepciones que ya no se usan.
func Exigir(t *testing.T, o Opciones) {
	t.Helper()
	hallazgos, err := Revisar(o)
	if err != nil {
		t.Fatalf("sqlorg: %v", err)
	}
	for _, queja := range quejas(hallazgos, o.Excepciones) {
		t.Error(queja)
	}
}

// quejas contrasta lo encontrado con las excepciones, por igualdad.
func quejas(hallazgos []Hallazgo, excepciones map[string]string) []string {
	var out []string
	usadas := map[string]bool{}
	for _, h := range hallazgos {
		if _, ok := excepciones[h.Clave()]; ok {
			usadas[h.Clave()] = true
			continue
		}
		out = append(out, h.String())
	}
	claves := make([]string, 0, len(excepciones))
	for clave := range excepciones {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	for _, clave := range claves {
		if strings.TrimSpace(excepciones[clave]) == "" {
			out = append(out, "la excepción "+clave+" no dice por qué cruza organizaciones")
		}
		if !usadas[clave] {
			out = append(out, "la excepción "+clave+" ya no hace falta: bórrala")
		}
	}
	return out
}

// Revisar devuelve todas las sentencias sin filtrar, con o sin excepción.
func Revisar(o Opciones) ([]Hallazgo, error) {
	columna := o.Columna
	if columna == "" {
		columna = "tenant"
	}
	tablas, err := tablasConColumna(o.Migraciones, o.Dir, columna)
	if err != nil {
		return nil, err
	}
	if len(tablas) == 0 {
		return nil, fmt.Errorf("ninguna tabla de %s lleva la columna %q: un verde aquí no diría nada", o.Dir, columna)
	}
	a := analizador{tablas: tablas, columna: columna}
	a.compilar()
	var out []Hallazgo
	for _, dir := range o.Paquetes {
		hs, err := a.paquete(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, hs...)
	}
	return out, nil
}

var (
	reCrear   = regexp.MustCompile(`(?s)CREATE TABLE (?:IF NOT EXISTS )?(\w+)\s*\((.*?)\n\)`)
	reAnadir  = regexp.MustCompile(`ALTER TABLE (?:IF EXISTS )?(\w+)\s+ADD COLUMN (?:IF NOT EXISTS )?(\w+)`)
	reTablaEn = regexp.MustCompile(`\b(DELETE FROM|INSERT INTO|FROM|JOIN|UPDATE)\s+(\w+)(?:\s+(?:AS\s+)?(\w+))?`)
)

// palabras que siguen a una tabla sin alias y no son un alias.
var noEsAlias = map[string]bool{
	"WHERE": true, "SET": true, "ON": true, "ORDER": true, "GROUP": true, "LIMIT": true, "JOIN": true,
	"LEFT": true, "RIGHT": true, "INNER": true, "FULL": true, "CROSS": true, "NATURAL": true, "USING": true,
	"RETURNING": true, "FOR": true, "AS": true, "VALUES": true, "UNION": true, "SELECT": true,
	"HAVING": true, "OFFSET": true, "DO": true,
}

func tablasConColumna(fsys fs.FS, dir, columna string) (map[string]bool, error) {
	if fsys == nil {
		return nil, errors.New("faltan las migraciones")
	}
	reCol := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(columna) + `\s`)
	tablas := map[string]bool{}
	ficheros, err := fs.Glob(fsys, filepath.ToSlash(filepath.Join(dir, "*.up.sql")))
	if err != nil {
		return nil, err
	}
	sort.Strings(ficheros)
	for _, f := range ficheros {
		crudo, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		sql := string(crudo)
		for _, m := range reCrear.FindAllStringSubmatch(sql, -1) {
			if reCol.MatchString(m[2]) {
				tablas[m[1]] = true
			}
		}
		for _, m := range reAnadir.FindAllStringSubmatch(sql, -1) {
			if m[2] == columna {
				tablas[m[1]] = true
			}
		}
	}
	return tablas, nil
}

type analizador struct {
	tablas  map[string]bool
	columna string
	// igualdades sobre la columna: `[alias.]columna =`, `[alias.]columna IN` y
	// `= [alias.]columna`. El primer grupo es el alias.
	reIzq, reDer *regexp.Regexp
	// la columna como palabra, para la lista de un INSERT.
	reCol *regexp.Regexp
}

func (a *analizador) compilar() {
	c := regexp.QuoteMeta(a.columna)
	a.reIzq = regexp.MustCompile(`(?:(\w+)\.)?\b` + c + `\s*(?:=|IN\b)`)
	a.reDer = regexp.MustCompile(`=\s*(?:(\w+)\.)?` + c + `\b`)
	a.reCol = regexp.MustCompile(`\b` + c + `\b`)
}

// sentencia devuelve lo que le falta a un texto SQL, o nada.
func (a *analizador) sentencia(sql string) []string {
	refs := reTablaEn.FindAllStringSubmatchIndex(sql, -1)
	if len(refs) == 0 {
		return nil
	}
	filtrados := map[string]int{} // alias ("" = sin alias) → cuántas igualdades
	for _, re := range []*regexp.Regexp{a.reIzq, a.reDer} {
		for _, m := range re.FindAllStringSubmatch(sql, -1) {
			filtrados[m[1]]++
		}
	}
	var faltas []string
	sinAlias := 0
	for _, r := range refs {
		verbo, tabla := sql[r[2]:r[3]], sql[r[4]:r[5]]
		if !a.tablas[tabla] {
			continue
		}
		alias := ""
		if r[6] >= 0 && !noEsAlias[strings.ToUpper(sql[r[6]:r[7]])] {
			alias = sql[r[6]:r[7]]
		}
		if verbo == "INSERT INTO" {
			// Un INSERT escribe la columna: basta con que esté en su lista.
			// Con alias (`INSERT INTO quotas AS q (tenant, …)`), la lista va tras él.
			desde := r[5]
			if alias != "" {
				desde = r[7]
			}
			resto := strings.TrimLeft(sql[desde:], " \t\n")
			fin := strings.Index(resto, ")")
			if !strings.HasPrefix(resto, "(") || fin < 0 || !a.reCol.MatchString(resto[:fin]) {
				faltas = append(faltas, "INSERT INTO "+tabla)
			}
			continue
		}
		if alias != "" {
			if filtrados[alias] == 0 {
				faltas = append(faltas, verbo+" "+tabla+" ("+alias+")")
			}
			continue
		}
		// Sin alias: cada tabla necesita SU igualdad, sin calificar o con el nombre
		// de la tabla. Una sola no vale para dos tablas.
		sinAlias++
		if filtrados[""]+filtrados[tabla] < sinAlias {
			faltas = append(faltas, verbo+" "+tabla)
		}
	}
	return faltas
}

// paquete revisa los ficheros Go (sin los de test) de un directorio.
func (a *analizador) paquete(dir string) ([]Hallazgo, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	l := lector{a: a, fset: token.NewFileSet(), consts: map[string]ast.Expr{}}
	var ficheros []*ast.File
	var nombres []string
	for _, e := range entradas {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(l.fset, filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		ficheros, nombres = append(ficheros, f), append(nombres, n)
		l.apuntarConstantes(f)
	}
	for i, f := range ficheros {
		l.fichero = nombres[i]
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				l.revisar(x, x.Name.Name)
			case *ast.GenDecl:
				for _, s := range x.Specs {
					if v, ok := s.(*ast.ValueSpec); ok && len(v.Names) > 0 {
						l.revisar(v, v.Names[0].Name)
					}
				}
			}
		}
	}
	slices.SortFunc(l.out, func(x, y Hallazgo) int {
		if c := strings.Compare(x.Fichero, y.Fichero); c != 0 {
			return c
		}
		return x.Linea - y.Linea
	})
	return l.out, nil
}

// lector recorre un paquete acumulando lo que encuentra.
type lector struct {
	a    *analizador
	fset *token.FileSet
	// consts son las constantes y variables de texto del paquete: con ellas
	// `SELECT `+columnas+` FROM …` se lee como una sola sentencia.
	consts  map[string]ast.Expr
	fichero string
	out     []Hallazgo
}

func (l *lector) apuntarConstantes(f *ast.File) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || (g.Tok != token.CONST && g.Tok != token.VAR) {
			continue
		}
		for _, s := range g.Specs {
			v, ok := s.(*ast.ValueSpec)
			if !ok || len(v.Names) != len(v.Values) {
				continue
			}
			for i, nombre := range v.Names {
				l.consts[nombre.Name] = v.Values[i]
			}
		}
	}
}

// revisar analiza cada texto de `nodo`: un literal suelto o una cadena de sumas
// entera, una sola vez.
func (l *lector) revisar(nodo ast.Node, funcion string) {
	var visitar func(ast.Node) bool
	visitar = func(n ast.Node) bool {
		e, ok := n.(ast.Expr)
		if !ok || !esTexto(e) {
			return true
		}
		for _, falta := range l.a.sentencia(texto(e, l.consts, 0)) {
			l.out = append(l.out, Hallazgo{
				Fichero: l.fichero, Linea: l.fset.Position(e.Pos()).Line, Funcion: funcion, Falta: falta,
			})
		}
		// La cadena de sumas ya se leyó entera; lo que cuelga de ella y no es
		// texto (una llamada con su propio SQL dentro) se sigue recorriendo.
		for _, h := range hojas(e) {
			ast.Inspect(h, visitar)
		}
		return false
	}
	ast.Inspect(nodo, visitar)
}

// esTexto dice si la expresión es un literal de texto o una suma.
func esTexto(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BinaryExpr:
		return x.Op == token.ADD
	case *ast.BasicLit:
		return x.Kind == token.STRING
	}
	return false
}

// hojas son los operandos de una cadena de sumas que no son ni literal ni nombre.
func hojas(e ast.Expr) []ast.Expr {
	switch x := e.(type) {
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			return append(hojas(x.X), hojas(x.Y)...)
		}
	case *ast.ParenExpr:
		return hojas(x.X)
	case *ast.BasicLit, *ast.Ident:
		return nil
	}
	return []ast.Expr{e}
}

// texto reconstruye una expresión de texto: literales, sumas y constantes del
// paquete. Lo que no se conoce al leer el código (un argumento, una llamada)
// queda como un hueco que no nombra ninguna tabla.
func texto(e ast.Expr, consts map[string]ast.Expr, hondo int) string {
	const hueco = " ? "
	if hondo > 20 {
		return hueco
	}
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return hueco
		}
		s, err := strconv.Unquote(x.Value)
		if err != nil {
			return hueco
		}
		return s
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return hueco
		}
		return texto(x.X, consts, hondo+1) + texto(x.Y, consts, hondo+1)
	case *ast.ParenExpr:
		return texto(x.X, consts, hondo+1)
	case *ast.Ident:
		if v, ok := consts[x.Name]; ok {
			return texto(v, consts, hondo+1)
		}
	}
	return hueco
}
