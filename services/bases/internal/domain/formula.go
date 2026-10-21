// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Campos de FÓRMULA (ADR 0068, bases-2). Un campo de fórmula se calcula en el SERVIDOR
// con el motor de fórmulas de la suite (apps/nucleo/formula, el mismo de Calcus) y su
// resultado se guarda en el registro como cualquier otro valor: así filtros, orden,
// exportación y conector ven lo mismo sin saber que es una fórmula. Nadie lo escribe
// desde fuera (ValidarParcial lo rechaza).
//
// El lenguaje es el de una hoja de cálculo, RECORTADO:
//   - Las únicas referencias son a campos, entre llaves y por id: `{campo}` (un campo
//     del mismo registro; si es un enlace, vale el número de registros enlazados) y
//     `{enlace.campo}` (los valores de ese campo en los registros enlazados, para SUM,
//     COUNT, MIN, MAX…). No hay celdas, rangos, nombres ni matrices: se rechazan.
//   - Solo las funciones de `funcionesFormula`: puras, sin reloj ni azar (nada volátil),
//     sin red, sin bucles ni matrices, y ninguna que fabrique textos enormes (REPT).
//   - Coste acotado por construcción: longitud, nodos, fórmulas por tabla, y un agregado
//     recorre como mucho MaxEnlaces valores.

// TipoFormula es el tipo de campo calculado.
const TipoFormula Tipo = "formula"

// Límites de las fórmulas.
const (
	MaxLongFormula      = 1000
	MaxNodosFormula     = 200
	MaxFormulasPorTabla = 20
	MaxLongResultado    = 1000
)

// funcionesFormula es la lista CERRADA de funciones que admite un campo de fórmula.
// Añadir una exige mirar que sea pura y de coste acotado por sus argumentos.
var funcionesFormula = []string{
	"ABS", "AND", "AVERAGE", "CONCAT", "COUNT", "COUNTA", "DATE", "DAY", "IF", "IFERROR", "IFS", "INT",
	"ISBLANK", "ISNUMBER", "LEFT", "LEN", "LOWER", "MAX", "MEDIAN", "MID", "MIN", "MOD", "MONTH", "NOT",
	"OR", "POWER", "RIGHT", "ROUND", "ROUNDDOWN", "ROUNDUP", "SQRT", "SUM", "SWITCH", "TRIM", "UPPER",
	"VALUE", "YEAR",
}

// RefFormula es una referencia de una fórmula: un campo del mismo registro (Enlace
// vacío) o un campo de los registros enlazados por el campo de enlace `Enlace`.
type RefFormula struct{ Enlace, Campo string }

// Formula es una expresión ya troceada: el texto entre referencias y las referencias.
type Formula struct {
	trozos []string // len(trozos) == len(Refs)+1
	Refs   []RefFormula
}

var reRefFormula = regexp.MustCompile(`^([0-9a-f-]{36})(?:\.([0-9a-f-]{36}))?$`)

// AnalizarFormula trocea y valida la FORMA de una expresión: tamaño, referencias bien
// escritas y nada fuera del lenguaje admitido. Que los campos existan y sean del tipo
// que toca lo comprueba Comprobar, con el esquema delante.
func AnalizarFormula(expr string) (Formula, error) {
	malo := func(motivo string) (Formula, error) {
		return Formula{}, fmt.Errorf("%w: fórmula: %s", ErrInvalido, motivo)
	}
	expr = strings.TrimPrefix(strings.TrimSpace(expr), "=")
	if expr == "" || len(expr) > MaxLongFormula || !utf8.ValidString(expr) || tieneControl(expr) {
		return malo("vacía, demasiado larga o con caracteres de control")
	}
	var f Formula
	var trozo strings.Builder
	enTexto := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case c == '"':
			enTexto = !enTexto // «""» dentro de un texto abre y cierra: no cambia nada
		case enTexto:
		case c == '}':
			return malo("llave suelta")
		case c == '{':
			fin := strings.IndexByte(expr[i:], '}')
			if fin < 0 {
				return malo("referencia sin cerrar")
			}
			m := reRefFormula.FindStringSubmatch(expr[i+1 : i+fin])
			if m == nil {
				return malo("referencia que no es un campo")
			}
			ref := RefFormula{Campo: m[1]}
			if m[2] != "" {
				ref = RefFormula{Enlace: m[1], Campo: m[2]}
			}
			f.Refs = append(f.Refs, ref)
			f.trozos = append(f.trozos, trozo.String())
			trozo.Reset()
			i += fin
			continue
		}
		trozo.WriteByte(c)
	}
	if enTexto {
		return malo("texto sin cerrar")
	}
	f.trozos = append(f.trozos, trozo.String())
	// Con un literal en el sitio de cada referencia, lo que quede NO puede referenciar
	// nada: una celda, un rango o un nombre escritos a mano no son un campo. Entre
	// paréntesis aquí y al evaluar, para que lo de alrededor no se pegue a la referencia
	// («{campo}0» no es la celda de diez filas más abajo: no se entiende, y ya).
	uso, err := formula.Inspeccionar(f.texto(func(RefFormula) string { return "(0)" }))
	if err != nil {
		return malo("no se entiende")
	}
	if uso.Referencias > 0 || uso.Matrices > 0 {
		return malo("solo se pueden referenciar campos, entre llaves")
	}
	if uso.Nodos > MaxNodosFormula {
		return malo("demasiado compleja")
	}
	for _, fn := range uso.Funciones {
		if !slices.Contains(funcionesFormula, fn) {
			return malo("función no admitida: " + fn)
		}
	}
	return f, nil
}

// texto recompone la expresión con cada referencia sustituida.
func (f Formula) texto(ref func(RefFormula) string) string {
	var b strings.Builder
	for i, t := range f.trozos {
		b.WriteString(t)
		if i < len(f.Refs) {
			b.WriteString(ref(f.Refs[i]))
		}
	}
	return b.String()
}

// Comprobar valida las referencias contra el esquema: un campo propio ha de ser de la
// tabla de la fórmula; un agregado, de la tabla a la que apunta un campo de enlace de
// ESTA tabla, y de un tipo que se pueda sumar o comparar. `deTabla` da los campos de
// una tabla de la misma base.
func (f Formula) Comprobar(propios []Campo, deTabla func(tabla string) []Campo) error {
	buscar := func(campos []Campo, id string) (Campo, bool) {
		i := slices.IndexFunc(campos, func(c Campo) bool { return c.ID == id })
		if i < 0 {
			return Campo{}, false
		}
		return campos[i], true
	}
	for _, r := range f.Refs {
		if r.Enlace == "" {
			if _, ok := buscar(propios, r.Campo); !ok {
				return fmt.Errorf("%w: fórmula: campo que no es de esta tabla", ErrInvalido)
			}
			continue
		}
		enlace, ok := buscar(propios, r.Enlace)
		if !ok || enlace.Kind != TipoRelacion {
			return fmt.Errorf("%w: fórmula: el agregado no parte de un enlace de esta tabla", ErrInvalido)
		}
		destino, ok := buscar(deTabla(enlace.LinkTableID), r.Campo)
		if !ok {
			return fmt.Errorf("%w: fórmula: campo que no es de la tabla enlazada", ErrInvalido)
		}
		if k := destino.Efectivo().Kind; k != TipoNumero && k != TipoCasilla && k != TipoFecha {
			return fmt.Errorf("%w: fórmula: solo se agregan números, fechas y casillas", ErrInvalido)
		}
	}
	return nil
}

// Efectivo es el campo tal como lo ven filtros, orden, exportación y conector: un campo
// de fórmula se comporta como un campo del tipo de su resultado.
func (c Campo) Efectivo() Campo {
	if c.Kind == TipoFormula {
		c.Kind = c.Result
	}
	return c
}

// Efectivos aplica Efectivo a una lista de campos.
func Efectivos(campos []Campo) []Campo {
	out := make([]Campo, len(campos))
	for i, c := range campos {
		out[i] = c.Efectivo()
	}
	return out
}

const hojaRegistro = "R"

// EvaluarFormulas calcula los campos de fórmula de UN registro. `data` son sus valores
// (los de fórmula que traiga se ignoran: se recalculan), `enlaces` el número de
// registros enlazados por cada campo de enlace y `agregados` los valores de los
// registros enlazados para cada referencia `{enlace.campo}`. Devuelve, por campo de
// fórmula, su valor (nil = vacío: sin resultado o con error).
//
// Cada valor entra en el motor como LITERAL tipado, nunca como texto de celda: un texto
// que empieza por «=» es un texto, no una fórmula dentro de la fórmula.
func EvaluarFormulas(campos []Campo, data map[string]json.RawMessage, enlaces map[string]int, agregados map[RefFormula][]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	col := make(map[string]int, len(campos))
	hayFormulas := false
	for i, c := range campos {
		col[c.ID] = i
		hayFormulas = hayFormulas || c.Kind == TipoFormula
	}
	if !hayFormulas {
		return out
	}
	h := hojaDeRegistro{e: formula.New(formula.Options{Sheets: []string{hojaRegistro}}), col: col, agregados: agregados, hojas: map[RefFormula]string{}}
	for i, c := range campos {
		switch c.Kind { //nolint:exhaustive // el resto son valores del registro
		case TipoFormula:
			// Una fórmula guardada que ya no se admite no se evalúa: queda vacía.
			if f, err := AnalizarFormula(c.Formula); err == nil {
				h.e.SetCellAt(hojaRegistro, i, 0, "="+f.texto(h.celda))
			}
		case TipoRelacion:
			h.e.SetCellAt(hojaRegistro, i, 0, strconv.Itoa(enlaces[c.ID]))
		default:
			h.e.SetCellAt(hojaRegistro, i, 0, literalDeFormula(data[c.ID]))
		}
	}
	for i, c := range campos {
		if c.Kind == TipoFormula {
			out[c.ID] = resultadoDeFormula(h.e.ValueAt(hojaRegistro, i, 0), c.Result)
		}
	}
	return out
}

// hojaDeRegistro es el registro puesto en el motor: la hoja «R» con un campo por columna
// en la fila 1, y una hoja «E<n>» por agregado con los valores de los enlazados en su
// columna A. Las referencias de celda las escribe ESTE código, nunca quien define la
// fórmula (AnalizarFormula rechaza cualquiera que venga escrita).
type hojaDeRegistro struct {
	e         *formula.Engine
	col       map[string]int
	agregados map[RefFormula][]json.RawMessage
	hojas     map[RefFormula]string
}

// celda es la referencia de motor de una referencia de fórmula.
func (h *hojaDeRegistro) celda(r RefFormula) string {
	if r.Enlace == "" {
		if j, ok := h.col[r.Campo]; ok {
			return "(" + hojaRegistro + "!" + formula.NumToCol(j) + "1)"
		}
		return "(" + hojaRegistro + "!A2)" // el campo ya no existe: una celda vacía
	}
	hoja, ok := h.hojas[r]
	if !ok {
		hoja = "E" + strconv.Itoa(len(h.hojas))
		h.hojas[r] = hoja
		for j, v := range h.agregados[r] {
			if j < MaxEnlaces {
				h.e.SetCellAt(hoja, 0, j, literalDeFormula(v))
			}
		}
	}
	return "(" + hoja + "!A1:A" + strconv.Itoa(MaxEnlaces) + ")"
}

// literalDeFormula pasa un valor guardado al texto de celda que el motor lee como ESE
// valor: un número, TRUE/FALSE o un texto entre comillas («="…"», con las comillas
// dobladas). "" = celda vacía.
func literalDeFormula(raw json.RawMessage) string {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return ""
	}
	texto := func(s string) string { return `="` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strings.ToUpper(strconv.FormatBool(x))
	case string:
		return texto(x)
	case []any: // selección múltiple: sus opciones, unidas
		partes := make([]string, 0, len(x))
		for _, p := range x {
			partes = append(partes, fmt.Sprint(p))
		}
		return texto(strings.Join(partes, ", "))
	}
	return ""
}

// resultadoDeFormula convierte lo que da el motor al valor que se guarda, del tipo
// declarado del campo. Un error del motor (#DIV/0!, #VALUE!, un ciclo) o un resultado
// de otro tipo quedan VACÍOS.
//
// ponytail: el error no se guarda (un «#DIV/0!» en un campo numérico rompería el
// `::numeric` de filtros y orden); si hace falta enseñarlo, columna aparte.
func resultadoDeFormula(v formula.Value, tipo Tipo) json.RawMessage {
	if formula.IsError(v) {
		return nil
	}
	if tipo == TipoNumero {
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e15 {
			return nil
		}
		b, _ := json.Marshal(n)
		return b
	}
	s := formula.Display(v)
	if s == "" || !utf8.ValidString(s) {
		return nil
	}
	if rs := []rune(s); len(rs) > MaxLongResultado {
		s = string(rs[:MaxLongResultado])
	}
	b, _ := json.Marshal(s)
	return b
}
