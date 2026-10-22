// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestLeerCSVInfiereTipos(t *testing.T) {
	csv := "Nombre,Importe,Alta,Activo\nAna,12.5,2026-01-02,sí\nBea,,2026-02-03,no\n"
	tb, err := LeerCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	want := []Tipo{TipoTexto, TipoNumero, TipoFecha, TipoCasilla}
	for i, w := range want {
		if tb.Campos[i].Kind != w {
			t.Errorf("columna %s: %s, esperado %s", tb.Campos[i].Name, tb.Campos[i].Kind, w)
		}
	}
	if len(tb.Filas) != 2 || tb.Filas[1][1] != nil {
		t.Errorf("filas: %v", tb.Filas)
	}
}

func TestLeerCSVHostil(t *testing.T) {
	filas := strings.Repeat("a\n", MaxFilasCSV+1)
	columnas := strings.Repeat("c,", MaxColumnasCSV) + "c\n"
	casos := map[string]string{
		"vacío":               "",
		"NUL":                 "a,b\n1,\x002\n",
		"UTF-8 inválido":      "a,b\n\xff\xfe,2\n",
		"demasiadas filas":    "a\n" + filas,
		"demasiadas columnas": columnas,
		"celda enorme":        "a\n" + strings.Repeat("x", MaxLongTexto+1) + "\n",
		"comillas rotas":      "a,b\n\"sin cerrar,2\n",
		"fila más ancha":      "a\n1,2\n",
	}
	for n, c := range casos {
		if _, err := LeerCSV(strings.NewReader(c)); err == nil {
			t.Errorf("%s: debía fallar", n)
		}
	}
	grande := "a\n" + strings.Repeat("x", MaxBytesCSV)
	if _, err := LeerCSV(strings.NewReader(grande)); err == nil {
		t.Error("más de 5 MiB debe fallar")
	}
}

func TestLeerCSVNoEjecutaFormulasNiRompeNombres(t *testing.T) {
	csv := "\xef\xbb\xbfA;A;;" + strings.Repeat("l", 300) + "\n=1+1;x;y;z\n"
	tb, err := LeerCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	nombres := map[string]bool{}
	for _, c := range tb.Campos {
		if nombres[strings.ToLower(c.Name)] || c.Name == "" {
			t.Errorf("nombre repetido o vacío: %q", c.Name)
		}
		nombres[strings.ToLower(c.Name)] = true
		if _, err := ValidarEntradaCampo(c); err != nil {
			t.Errorf("campo inferido no válido %q: %v", c.Name, err)
		}
	}
	if string(tb.Filas[0][0]) != `"=1+1"` {
		t.Errorf("la fórmula es texto: %s", tb.Filas[0][0])
	}
}

func TestInferirTipoNoConfundeNumerosRaros(t *testing.T) {
	for _, col := range [][]string{{"0x10"}, {"1_000"}, {"+34600"}, {"NaN"}, {"Inf"}, {"1e999"}} {
		if InferirTipo(col) != TipoTexto {
			t.Errorf("%v debe ser texto", col)
		}
	}
	if InferirTipo([]string{"-3.5", "2"}) != TipoNumero {
		t.Error("números")
	}
}

func TestCeldaCSVNeutralizaFormulas(t *testing.T) {
	for _, s := range []string{"=cmd|' /C calc'!A0", "+1", "-1", "@SUM(A1)", "\tx", "\rx"} {
		if got := CeldaCSV(s); !strings.HasPrefix(got, "'") {
			t.Errorf("%q no neutralizada: %q", s, got)
		}
	}
	if CeldaCSV("normal") != "normal" || CeldaCSV("") != "" {
		t.Error("lo normal no se toca")
	}
}
