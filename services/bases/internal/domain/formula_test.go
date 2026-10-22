// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

const (
	fPrecio = "11111111-1111-4111-8111-111111111111"
	fCant   = "22222222-2222-4222-8222-222222222222"
	fTexto  = "33333333-3333-4333-8333-333333333333"
	fEnlace = "44444444-4444-4444-8444-444444444444"
	fImp    = "55555555-5555-4555-8555-555555555555" // campo de la tabla enlazada
	fA      = "66666666-6666-4666-8666-666666666666"
	fB      = "77777777-7777-4777-8777-777777777777"
)

// Lo que NO es el lenguaje de un campo de fórmula se rechaza al guardarla: referencias
// escritas a mano (a las hojas internas del evaluador incluidas), funciones volátiles,
// de red o de coste no acotado, matrices y expresiones enormes.
func TestFormulaHostil(t *testing.T) {
	hostiles := map[string]string{
		"vacía":                    "  ",
		"celda":                    "A1",
		"celda de la hoja interna": "R!A1",
		"rango de hoja interna":    "SUM(E0!A1:A100)",
		"hoja entre comillas":      "'R'!A1",
		"columna entera":           "SUM(A:A)",
		"fila entera":              "SUM(1:1)",
		"nombre":                   "tasa * 2",
		"referencia estructurada":  "SUM(Tabla1[Importe])",
		"matriz":                   "SUM({1,2,3})",
		"reloj":                    "NOW()",
		"hoy":                      "TODAY() - 1",
		"azar":                     "RAND()",
		"indirecta":                `INDIRECT("R!A1")`,
		"desplazamiento":           "OFFSET(1, 1, 1)",
		"texto enorme":             `REPT("x", 1000000000)`,
		"secuencia":                "SUM(SEQUENCE(1000000))",
		"lambda":                   "LAMBDA(x, x)(1)",
		"let":                      "LET(x, 1, x)",
		"red":                      `WEBSERVICE("http://169.254.169.254/")`,
		"enlace":                   `HYPERLINK("http://x")`,
		"función desconocida":      "NOEXISTE(1)",
		"sintaxis":                 "1 +",
		"texto sin cerrar":         `"abc`,
		"llave suelta":             "1 } 2",
		"referencia sin cerrar":    "{" + fPrecio,
		"referencia que no es id":  "{Precio}",
		"referencia con inyección": "{" + fPrecio + ".R!A1}",
		"pegada a un número":       "{" + fPrecio + "}0",
		"pegada a un nombre":       "E{" + fPrecio + "}",
		"demasiado larga":          strings.Repeat("1+", MaxLongFormula/2) + "1",
		"demasiados nodos":         strings.Repeat("1+", MaxNodosFormula) + "1",
		"control":                  "1 +\x00 2",
	}
	for nombre, expr := range hostiles {
		if _, err := AnalizarFormula(expr); !errors.Is(err, ErrInvalido) {
			t.Errorf("%s (%q): %v, esperado ErrInvalido", nombre, expr, err)
		}
	}
	// Una referencia DENTRO de un texto es texto: no cuenta como dependencia.
	f, err := AnalizarFormula(`"{` + fPrecio + `}" & {` + fTexto + `}`)
	if err != nil || len(f.Refs) != 1 || f.Refs[0].Campo != fTexto {
		t.Errorf("referencia dentro de un texto: %+v %v", f.Refs, err)
	}
	// La lista de funciones admitidas existe en el motor (una errata no abre nada, pero
	// tampoco serviría) y no tiene ninguna volátil.
	todas := formula.FunctionNames()
	for _, fn := range funcionesFormula {
		if !slices.Contains(todas, fn) || formula.EsVolatil(fn) {
			t.Errorf("función admitida que el motor no tiene, o volátil: %s", fn)
		}
	}
}

func camposDePrueba(formulas ...Campo) []Campo {
	return append([]Campo{
		{ID: fPrecio, Kind: TipoNumero},
		{ID: fCant, Kind: TipoNumero},
		{ID: fTexto, Kind: TipoTexto},
		{ID: fEnlace, Kind: TipoRelacion, LinkTableID: "t2"},
	}, formulas...)
}

func TestEvaluarFormulas(t *testing.T) {
	ref := func(ids ...string) string { return "{" + strings.Join(ids, ".") + "}" }
	j := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	casos := []struct {
		nombre string
		expr   string
		tipo   Tipo
		quiero string // JSON del resultado; "" = vacío
	}{
		{"aritmética", ref(fPrecio) + " * " + ref(fCant), TipoNumero, `7.5`},
		{"condición", `IF(` + ref(fCant) + ` > 2, "muchos", "pocos")`, TipoTexto, `"muchos"`},
		{"redondeo", `ROUND(` + ref(fPrecio) + ` / 3, 2)`, TipoNumero, `0.83`},
		// El texto del registro es TEXTO aunque parezca una fórmula, un número o un booleano.
		{"texto que parece fórmula", ref(fTexto), TipoTexto, `"=1+1 \"x\" {y}"`},
		{"texto concatenado", ref(fTexto) + ` & "!"`, TipoTexto, `"=1+1 \"x\" {y}!"`},
		{"longitud del texto", `LEN(` + ref(fTexto) + `)`, TipoNumero, `12`},
		{"suma de enlazados", `SUM(` + ref(fEnlace, fImp) + `)`, TipoNumero, `60`},
		{"mínimo y máximo", `MAX(` + ref(fEnlace, fImp) + `) - MIN(` + ref(fEnlace, fImp) + `)`, TipoNumero, `20`},
		{"cuenta de valores", `COUNT(` + ref(fEnlace, fImp) + `)`, TipoNumero, `3`},
		{"cuenta de enlaces", ref(fEnlace), TipoNumero, `4`},
		{"división por cero", ref(fPrecio) + ` / 0`, TipoNumero, ``},
		{"desbordamiento", `POWER(10, 400)`, TipoNumero, ``},
		{"torre de potencias", `9^9^9^9`, TipoNumero, ``},
		{"texto donde va un número", `"hola"`, TipoNumero, ``},
		{"error como texto", `1 / 0`, TipoTexto, ``},
	}
	data := map[string]json.RawMessage{fPrecio: j(2.5), fCant: j(3), fTexto: j(`=1+1 "x" {y}`)}
	agregados := map[RefFormula][]json.RawMessage{{Enlace: fEnlace, Campo: fImp}: {j(10), j(20), nil, j(30)}}
	for _, c := range casos {
		campos := camposDePrueba(Campo{ID: fA, Kind: TipoFormula, Formula: c.expr, Result: c.tipo})
		if _, err := AnalizarFormula(c.expr); err != nil {
			t.Errorf("%s: no se admite: %v", c.nombre, err)
			continue
		}
		got := EvaluarFormulas(campos, data, map[string]int{fEnlace: 4}, agregados)[fA]
		if string(got) != c.quiero {
			t.Errorf("%s: %s, esperado %s", c.nombre, got, c.quiero)
		}
	}
}

// Una fórmula puede usar otra del mismo registro; dos que se usan la una a la otra
// quedan vacías (y el servidor rechaza guardarlas: store.comprobarFormula).
func TestFormulasEncadenadasYCiclo(t *testing.T) {
	data := map[string]json.RawMessage{fPrecio: json.RawMessage(`4`)}
	campos := camposDePrueba(
		Campo{ID: fB, Kind: TipoFormula, Formula: "{" + fA + "} + 1", Result: TipoNumero},
		Campo{ID: fA, Kind: TipoFormula, Formula: "{" + fPrecio + "} * 2", Result: TipoNumero},
	)
	got := EvaluarFormulas(campos, data, nil, nil)
	if string(got[fA]) != "8" || string(got[fB]) != "9" {
		t.Errorf("encadenadas: %s y %s", got[fA], got[fB])
	}
	campos[5].Formula = "{" + fB + "} * 2"
	got = EvaluarFormulas(campos, data, nil, nil)
	if got[fA] != nil || got[fB] != nil {
		t.Errorf("un ciclo no da valor: %s y %s", got[fA], got[fB])
	}
	// El valor que traiga el registro para un campo de fórmula no cuenta: se recalcula.
	data[fA] = json.RawMessage(`999`)
	campos[5].Formula = "{" + fPrecio + "} * 2"
	if got := EvaluarFormulas(campos, data, nil, nil); string(got[fA]) != "8" {
		t.Errorf("valor de fórmula heredado: %s", got[fA])
	}
}

// Nadie escribe un campo de fórmula desde fuera.
func TestFormulaNoSeEscribe(t *testing.T) {
	campos := camposDePrueba(Campo{ID: fA, Kind: TipoFormula, Formula: "1", Result: TipoNumero})
	if _, err := ValidarParcial(campos, map[string]json.RawMessage{fA: json.RawMessage(`5`)}); !errors.Is(err, ErrInvalido) {
		t.Errorf("escribir un campo de fórmula: %v", err)
	}
}

func TestComprobarFormula(t *testing.T) {
	propios := camposDePrueba()
	deTabla := func(t string) []Campo {
		if t == "t2" {
			return []Campo{{ID: fImp, Kind: TipoNumero}, {ID: fB, Kind: TipoTexto}}
		}
		return nil
	}
	ref := func(ids ...string) string { return "{" + strings.Join(ids, ".") + "}" }
	for expr, valida := range map[string]bool{
		ref(fPrecio):                      true,
		`SUM(` + ref(fEnlace, fImp) + `)`: true,
		ref(fImp):                         false, // campo de OTRA tabla, sin pasar por un enlace
		ref(fPrecio, fImp):                false, // el agregado no parte de un enlace
		ref(fEnlace, fPrecio):             false, // el campo no es de la tabla enlazada
		ref(fEnlace, fB):                  false, // no se agregan textos
		ref(fA):                           false, // no existe
	} {
		f, err := AnalizarFormula(expr)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if err := f.Comprobar(propios, deTabla); (err == nil) != valida {
			t.Errorf("%s: %v, válida=%v", expr, err, valida)
		}
	}
}
