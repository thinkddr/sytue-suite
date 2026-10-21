// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # La validación de datos
//
// Es `validations` de SheetData y lo que `sheets/format.ts` hace con ellas: una lista de
// valores permitidos (con su desplegable en la celda), o un número, una fecha o una
// longitud de texto comparados con uno o dos operandos, o una fórmula que tiene que
// dar VERDADERO. La estricta rechaza lo que no cumple; la otra solo lo marca en rojo.
// Si dos cubren la misma celda, gana la última.

// Validacion es `DataValidation`. `type` vacío es la lista de siempre.
type Validacion struct {
	Rango   string   `json:"range"`
	Tipo    string   `json:"type,omitempty"`
	Error   string   `json:"error,omitempty"`
	Estrict bool     `json:"strict,omitempty"`
	Valores []string `json:"values,omitempty"`
	Formula string   `json:"formula,omitempty"`
	Op      string   `json:"op,omitempty"`
	A       string   `json:"a,omitempty"`
	B       string   `json:"b,omitempty"`
	Entero  bool     `json:"integer,omitempty"`
}

var (
	tiposValidacion = []string{"list", "number", "date", "textLength", "formula"}
	opsValidacion   = []string{"between", "notBetween", "eq", "ne", "gt", "lt", "ge", "le"}
	reSepValores    = regexp.MustCompile(`[\n,]`)
	reNoNumEstricto = regexp.MustCompile(`[^\d\s.,+-]`)
	reNumSuelto     = regexp.MustCompile(`^[-+]?\d+(?:[.,]\d+)?$`)
	reFechaES       = regexp.MustCompile(`^(\d{1,4})[/.-](\d{1,2})[/.-](\d{1,4})$`)
)

// validacionCruda es una DataValidation como llega.
type validacionCruda struct {
	Rango   any `json:"range"`
	Tipo    any `json:"type"`
	Error   any `json:"error"`
	Estrict any `json:"strict"`
	Valores any `json:"values"`
	Formula any `json:"formula"`
	Op      any `json:"op"`
	A       any `json:"a"`
	B       any `json:"b"`
	Entero  any `json:"integer"`
}

// valoresValidacion es `parseValidationValues`: una entrada por línea o por coma, sin
// vacíos ni repetidos, en su orden.
func valoresValidacion(texto string) []string {
	var out []string
	for _, p := range reSepValores.Split(texto, -1) {
		if v := strings.TrimSpace(p); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// sanear es `sanitizeDataValidation`: false si no vale. Cada criterio se queda con sus
// campos; el tipo «list» no se escribe.
func (c validacionCruda) sanear() (Validacion, bool) {
	rango, ok := c.Rango.(string)
	if !ok {
		return Validacion{}, false
	}
	if _, ok := formula.ParseRangeRef(rango); !ok {
		return Validacion{}, false
	}
	tipo, _ := c.Tipo.(string)
	if !contiene(tiposValidacion, tipo) {
		tipo = "list"
	}
	v := Validacion{Rango: rango}
	if tipo != "list" {
		v.Tipo = tipo
	}
	if e, ok := c.Error.(string); ok && strings.TrimSpace(e) != "" {
		v.Error = cortarUTF16(strings.TrimSpace(e), 200)
	}
	v.Estrict = c.Estrict == true
	switch tipo {
	case "list":
		lista, _ := c.Valores.([]any)
		var textos []string
		for _, x := range lista {
			if s, ok := x.(string); ok {
				textos = append(textos, s)
			}
		}
		if v.Valores = valoresValidacion(strings.Join(textos, "\n")); len(v.Valores) == 0 {
			return Validacion{}, false
		}
		return v, true
	case "formula":
		f, ok := c.Formula.(string)
		if !ok || strings.TrimSpace(f) == "" {
			return Validacion{}, false
		}
		v.Formula = strings.TrimSpace(f)
		return v, true
	}
	return c.sanearComparacion(tipo, v)
}

// sanearComparacion valida el operador y los operandos de un número, una fecha o una
// longitud de texto.
func (c validacionCruda) sanearComparacion(tipo string, v Validacion) (Validacion, bool) {
	op, okOp := c.Op.(string)
	a, okA := c.A.(string)
	if !okOp || !contiene(opsValidacion, op) || !okA {
		return Validacion{}, false
	}
	if _, ok := operando(tipo, a); !ok {
		return Validacion{}, false
	}
	v.Op, v.A = op, strings.TrimSpace(a)
	if op == "between" || op == "notBetween" {
		b, ok := c.B.(string)
		if !ok {
			return Validacion{}, false
		}
		if _, ok := operando(tipo, b); !ok {
			return Validacion{}, false
		}
		v.B = strings.TrimSpace(b)
	}
	v.Entero = tipo == "number" && c.Entero == true
	return v, true
}

// numeroEstricto es `parseEsNumberStrict`: como numeroTecleado, pero un texto con
// letras o símbolos («A1», «$50», «12 kg») no es un número.
func numeroEstricto(texto string) (float64, bool) {
	if reNoNumEstricto.MatchString(texto) {
		return 0, false
	}
	return numeroTecleado(texto)
}

// fechaTecleada es `parseEsDate`: «dd/mm/aaaa» (con «/», «-» o «.»), «aaaa-mm-dd» o un
// número suelto que ya es un serial; en el sistema de 1900 del motor.
func fechaTecleada(texto string) (float64, bool) {
	t := strings.TrimSpace(texto)
	if t == "" {
		return 0, false
	}
	if reNumSuelto.MatchString(t) {
		return numeroTecleado(t)
	}
	m := reFechaES.FindStringSubmatch(t)
	if m == nil {
		return 0, false
	}
	n := func(s string) int { x, _ := strconv.Atoi(s); return x }
	a, d := n(m[3]), n(m[1])
	if len(m[1]) == 4 {
		a, d = n(m[1]), n(m[3])
	}
	mes := n(m[2])
	if a < 1900 || a > 9999 || mes < 1 || mes > 12 || d < 1 || d > 31 {
		return 0, false
	}
	f := time.Date(a, time.Month(mes), d, 0, 0, 0, 0, time.UTC)
	if f.Year() != a || int(f.Month()) != mes || f.Day() != d {
		return 0, false // el 31 de febrero no existe
	}
	dias := int(math.Round(f.Sub(epocaFechas).Hours() / 24))
	if dias < 1 {
		return 0, false
	}
	if dias >= 60 {
		dias++ // se salta el 29-2-1900 fantasma (serial 60)
	}
	return float64(dias), true
}

// operando es `parseOperand`: una fecha en su serial; lo demás, número estricto.
func operando(tipo, texto string) (float64, bool) {
	if tipo == "date" {
		return fechaTecleada(texto)
	}
	return numeroEstricto(texto)
}

// comparar es `compareValidationOp`. Un operando ausente o ilegible no restringe.
func (v Validacion) comparar(x float64) bool {
	a, ok := operando(v.Tipo, v.A)
	if !ok {
		return true
	}
	switch v.Op {
	case "gt":
		return x > a
	case "lt":
		return x < a
	case "ge":
		return x >= a
	case "le":
		return x <= a
	case "eq":
		return x == a
	case "ne":
		return x != a
	case "between", "notBetween":
		b, ok := operando(v.Tipo, v.B)
		if !ok {
			return true
		}
		dentro := x >= min(a, b) && x <= max(a, b)
		return dentro == (v.Op == "between")
	}
	return true
}

// cumple es `validateValue`: la celda VACÍA siempre vale (borrar). `evalFormula` es la
// comprobación de una validación de fórmula; sin él, se da por buena.
func (v Validacion) cumple(texto string, evalFormula func() bool) bool {
	t := strings.TrimSpace(texto)
	if t == "" {
		return true
	}
	switch v.Tipo {
	case "formula":
		return v.Formula == "" || evalFormula == nil || evalFormula()
	case "number":
		n, ok := numeroEstricto(t)
		if !ok || (v.Entero && n != math.Trunc(n)) {
			return false
		}
		return v.comparar(n)
	case "date":
		n, ok := fechaTecleada(t)
		return ok && v.comparar(n)
	case "textLength":
		return v.comparar(float64(len(utf16.Encode([]rune(t)))))
	}
	return slices.Contains(v.Valores, t)
}

// mensaje es `validationMessage`: el error que puso quien la creó o el de por defecto.
// Van en español también en la web, sin catálogo.
func (v Validacion) mensaje(ref string) string {
	if strings.TrimSpace(v.Error) != "" {
		return v.Error
	}
	if v.Tipo == "" || v.Tipo == "list" {
		return "La celda " + ref + " solo admite: " + strings.Join(v.Valores, ", ") + "."
	}
	return "El valor de " + ref + " no cumple la validación de datos."
}

// ---- en el libro ----

// Validaciones son las de la hoja activa.
func (l *Libro) Validaciones() []Validacion { return l.h().validaciones }

// validacionEn es `validationAt`: la ÚLTIMA validación que cubre la celda, o nil.
func (l *Libro) validacionEn(c, f int) *Validacion {
	var hallada *Validacion
	for i, v := range l.h().validaciones {
		if r, ok := formula.ParseRangeRef(v.Rango); ok && c >= r.C1 && c <= r.C2 && f >= r.R1 && f <= r.R2 {
			hallada = &l.h().validaciones[i]
		}
	}
	return hallada
}

// AnadirValidacion es `validadd`. false si no vale.
func (l *Libro) AnadirValidacion(crudo validacionCruda) bool {
	v, ok := crudo.sanear()
	if !ok {
		return false
	}
	l.anotar()
	l.version++
	l.h().validaciones = append(slices.Clone(l.h().validaciones), v)
	return true
}

// QuitarValidacion es `validdel`.
func (l *Libro) QuitarValidacion(i int) {
	if i < 0 || i >= len(l.h().validaciones) {
		return
	}
	l.anotar()
	l.version++
	l.h().validaciones = slices.Delete(slices.Clone(l.h().validaciones), i, i+1)
}

// rechaza dice si escribir `texto` en (c, f) lo rechaza una validación ESTRICTA, y con
// qué mensaje. La de fórmula se evalúa con el candidato puesto en un motor aparte
// (`dvFormulaOk`), que no toca la hoja.
func (l *Libro) rechaza(c, f int, texto string) (string, bool) {
	v := l.validacionEn(c, f)
	if v == nil || !v.Estrict {
		return "", false
	}
	evaluar := func() bool {
		r, ok := formula.ParseRangeRef(v.Rango)
		if !ok {
			return true
		}
		ff := v.Formula
		if !strings.HasPrefix(ff, "=") {
			ff = "=" + ff
		}
		movida := formula.ShiftRefs(ff, c-r.C1, f-r.R1)
		if strings.HasPrefix(movida, "#") {
			return false
		}
		aparte := &Libro{}
		aparte.restaurar(l.copia())
		aparte.motor.SetCellAt(l.Hoja(), c, f, texto)
		_ = aparte.motor.SetCell(l.Hoja(), celdaApoyo, movida)
		res, _ := aparte.motor.GetValue(l.Hoja(), celdaApoyo)
		switch x := res.(type) {
		case bool:
			return x
		case float64:
			return x != 0
		}
		return false
	}
	if v.cumple(texto, evaluar) {
		return "", false
	}
	return v.mensaje(Ref(c, f)), true
}

// marcaInvalida dice si una celda con un literal que NO cumple una validación no
// estricta se marca en rojo. Las fórmulas no se miran, como en la web.
func (l *Libro) marcaInvalida(c, f int) bool {
	v := l.validacionEn(c, f)
	if v == nil || v.Estrict {
		return false
	}
	html := l.HTML(c, f)
	if html == "" || esFormula(html) {
		return false
	}
	return !v.cumple(textoPlano(html), nil)
}
