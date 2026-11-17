// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migcheck

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
)

// ParametroFijado es un parámetro propio (con punto, como `app.maintenance`) que una
// sentencia deja escrito en la definición de una función, un procedimiento, un rol
// o una base: `CREATE FUNCTION … SET app.x = 'on'`, `ALTER FUNCTION … SET` o
// `RESET`, `ALTER ROLE … SET`.
//
// Por qué importa: desde PostgreSQL 15 eso exige ser superusuario o tener
// `GRANT SET ON PARAMETER` sobre el parámetro, y en el piloto cada servicio migra
// con su rol, que no es superusuario. Un `SET app.x` suelto en una sesión o dentro
// del cuerpo de una función no lo exige y no cuenta.
type ParametroFijado struct {
	Nombre string // en minúsculas, p. ej. "app.maintenance"
	Linea  int    // línea (1-based) dentro del fichero
}

var (
	// Sentencias cuyas cláusulas SET/RESET quedan escritas en el catálogo.
	sentenciaPersistente = regexp.MustCompile(`(?is)\b(?:CREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)|ALTER\s+(?:FUNCTION|PROCEDURE|ROUTINE|DATABASE|ROLE|USER))\b`)
	// `SET nombre = …`, `SET nombre TO …`, `SET nombre FROM CURRENT` y `RESET nombre`,
	// con el nombre entre comillas dobles o sin ellas. Solo nombres con punto: los
	// del propio PostgreSQL (search_path, row_security…) no piden nada.
	clausulaSet = regexp.MustCompile(`(?is)\b(?:SET\s+"?([a-z_][a-z0-9_$]*\.[a-z_][a-z0-9_$]*)"?\s*(?:=|\bTO\b|\bFROM\s+CURRENT\b)|RESET\s+"?([a-z_][a-z0-9_$]*\.[a-z_][a-z0-9_$]*)"?)`)
	// Etiqueta de un literal con dólares: `$$` o `$etiqueta$` (no `$1`).
	etiquetaDolar = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)
)

// ParametrosFijados devuelve los parámetros propios que el SQL deja escritos en
// la definición de una función, un rol o una base, con su línea.
//
// No es un analizador de SQL, pero no se deja engañar por lo habitual: ignora los
// comentarios y los literales entre comillas simples (blank, como el resto del
// paquete), y los cuerpos con dólares ($$ … $$) se miran aparte, como SQL propio,
// para ver también lo que crea un DO. Lo que NO ve es SQL que se monte con
// format() dentro de un literal con comillas simples.
func ParametrosFijados(sql string) []ParametroFijado {
	var hallados []ParametroFijado
	analizarParametros(sql, 0, &hallados)
	return hallados
}

func analizarParametros(sql string, lineaBase int, hallados *[]ParametroFijado) {
	esqueleto, cuerpos := esqueletoSQL(sql)
	inicio := 0
	for _, sentencia := range strings.SplitAfter(esqueleto, ";") {
		if loc := sentenciaPersistente.FindStringIndex(sentencia); loc != nil {
			for _, m := range clausulaSet.FindAllStringSubmatchIndex(sentencia[loc[1]:], -1) {
				nombre := m[2:4]
				if nombre[0] < 0 {
					nombre = m[4:6]
				}
				pos := inicio + loc[1] + nombre[0]
				*hallados = append(*hallados, ParametroFijado{
					Nombre: strings.ToLower(esqueleto[pos : inicio+loc[1]+nombre[1]]),
					Linea:  lineaBase + strings.Count(sql[:pos], "\n") + 1,
				})
			}
		}
		inicio += len(sentencia)
	}
	for _, c := range cuerpos {
		analizarParametros(sql[c[0]:c[1]], lineaBase+strings.Count(sql[:c[0]], "\n"), hallados)
	}
}

// esqueletoSQL es blank —sin comentarios ni literales, con la misma longitud y
// los mismos saltos de línea— con los cuerpos con dólares también en blanco, para
// que un `;` o un `SET` de dentro no cuenten como de la sentencia que los
// contiene. Devuelve también dónde empieza y acaba cada cuerpo.
func esqueletoSQL(sql string) (string, [][2]int) {
	esq := []byte(blank(sql))
	var cuerpos [][2]int
	for i := 0; i < len(esq); i++ {
		if esq[i] != '$' || (i > 0 && esDeIdentificador(esq[i-1])) {
			continue
		}
		etiqueta := etiquetaDolar.Find(esq[i:])
		if etiqueta == nil {
			continue // `$1`: un parámetro, no un cuerpo
		}
		desde := i + len(etiqueta)
		hasta := bytes.Index(esq[desde:], etiqueta)
		if hasta < 0 {
			hasta = len(esq) - desde
		}
		cuerpos = append(cuerpos, [2]int{desde, desde + hasta})
		fin := min(desde+hasta+len(etiqueta), len(esq))
		for k := i; k < fin; k++ {
			if esq[k] != '\n' {
				esq[k] = ' '
			}
		}
		i = fin - 1
	}
	return string(esq), cuerpos
}

func esDeIdentificador(c byte) bool {
	return c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// listaDeParametros es la declaración de la lista en
// deploy/helm/sql/conceder-parametros.sql.
var listaDeParametros = regexp.MustCompile(`(?s)\bparametros\s+CONSTANT\s+text\[\]\s*:=\s*ARRAY\[(.*?)\]`)

// ParametrosConcedidos lee la lista de parámetros de
// deploy/helm/sql/conceder-parametros.sql, la única que hay: la
// misma que conceden el despliegue, dev y pgtest.
func ParametrosConcedidos(sql string) ([]string, error) {
	m := listaDeParametros.FindStringSubmatch(sql)
	if m == nil {
		return nil, errors.New("migcheck: no encuentro «parametros CONSTANT text[] := ARRAY[…]» en el fichero de los parámetros")
	}
	var lista []string
	for _, literal := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(m[1], -1) {
		lista = append(lista, literal[1])
	}
	if len(lista) == 0 {
		return nil, errors.New("migcheck: la lista de parámetros está vacía")
	}
	return lista, nil
}
