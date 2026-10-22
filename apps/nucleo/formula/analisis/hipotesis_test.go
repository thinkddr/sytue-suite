// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

type vectoresHipotesis struct {
	Objetivo []struct {
		Nombre   string            `json:"nombre"`
		Celdas   map[string]string `json:"celdas"`
		Objetivo string            `json:"objetivo"`
		Variable string            `json:"variable"`
		Valor    float64           `json:"valor"`
		Esperado map[string]any    `json:"esperado"`
	} `json:"objetivo"`
	Solver []struct {
		Nombre    string            `json:"nombre"`
		Celdas    map[string]string `json:"celdas"`
		Objetivo  string            `json:"objetivo"`
		Variables []string          `json:"variables"`
		Meta      struct {
			Tipo  string  `json:"tipo"`
			Valor float64 `json:"valor"`
		} `json:"meta"`
		Restricciones []struct {
			Izq string `json:"lhs"`
			Rel string `json:"rel"`
			Der string `json:"rhs"`
		} `json:"restricciones"`
		Esperado map[string]any `json:"esperado"`
	} `json:"solver"`
	Tabla []struct {
		Nombre   string `json:"nombre"`
		Rect     [4]int `json:"rect"`
		Fila     bool   `json:"fila"`
		Col      bool   `json:"col"`
		Esperado any    `json:"esperado"`
	} `json:"tabla"`
}

func motorDe(t *testing.T, celdas map[string]string) *formula.Engine {
	t.Helper()
	e := formula.New(formula.Options{})
	for ref, raw := range celdas {
		if err := e.SetCell("Hoja1", ref, raw); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// numeroDe es el valor de una celda como número, o NaN si no lo es.
func numeroDe(e *formula.Engine, ref string) float64 {
	v, _ := e.GetValue("Hoja1", ref)
	if n, ok := v.(float64); ok && !math.IsInf(n, 0) {
		return n
	}
	return math.NaN()
}

// escribir es `String(x)`: el número más corto que vuelve a dar x.
func escribir(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }

func cercaDe(t *testing.T, nombre, campo string, got, want any) {
	t.Helper()
	switch w := want.(type) {
	case float64:
		g, ok := got.(float64)
		if !ok || math.Abs(g-w) > 1e-9*math.Max(1, math.Abs(w)) {
			t.Errorf("%s: %s = %v, y la web da %v", nombre, campo, got, want)
		}
	case []any:
		g, ok := got.([]float64)
		if !ok || len(g) != len(w) {
			t.Errorf("%s: %s = %v, y la web da %v", nombre, campo, got, want)
			return
		}
		for i := range w {
			cercaDe(t, nombre, campo, g[i], w[i])
		}
	case nil:
		if g, ok := got.(float64); !ok || !math.IsNaN(g) {
			t.Errorf("%s: %s = %v, y la web da null", nombre, campo, got)
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %s = %v, y la web da %v", nombre, campo, got, want)
		}
	}
}

// TestLaHipotesisComoLaWeb recorre los vectores que grabó la web.
func TestLaHipotesisComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/hipotesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresHipotesis
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Objetivo {
		e := motorDe(t, c.Celdas)
		x0 := numeroDe(e, c.Variable)
		if math.IsNaN(x0) {
			x0 = 0
		}
		r := BuscarObjetivo(func(x float64) (float64, bool) {
			_ = e.SetCell("Hoja1", c.Variable, escribir(x))
			y := numeroDe(e, c.Objetivo)
			return y, !math.IsNaN(y)
		}, c.Valor, OpcionesObjetivo{X0: x0})
		got := map[string]any{"resuelto": r.Resuelto, "valor": r.Valor, "iteraciones": float64(r.Iteraciones)}
		for k, w := range c.Esperado {
			cercaDe(t, c.Nombre, k, got[k], w)
		}
	}
	celda := regexp.MustCompile(`^[A-Z]+\d+$`)
	for _, c := range v.Solver {
		e := motorDe(t, c.Celdas)
		lado := func(s string) float64 {
			if celda.MatchString(s) {
				return numeroDe(e, s)
			}
			n, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return math.NaN()
			}
			return n
		}
		rels := make([]string, len(c.Restricciones))
		for i, r := range c.Restricciones {
			rels[i] = r.Rel
		}
		x0 := make([]float64, len(c.Variables))
		for i, ref := range c.Variables {
			if x0[i] = numeroDe(e, ref); math.IsNaN(x0[i]) {
				x0[i] = 0
			}
		}
		r := Resolver(func(x []float64) EvaluacionSolver {
			for i, ref := range c.Variables {
				_ = e.SetCell("Hoja1", ref, escribir(x[i]))
			}
			ev := EvaluacionSolver{Objetivo: numeroDe(e, c.Objetivo)}
			for _, rs := range c.Restricciones {
				ev.Restricciones = append(ev.Restricciones, [2]float64{lado(rs.Izq), lado(rs.Der)})
			}
			return ev
		}, x0, MetaSolver{Tipo: c.Meta.Tipo, Valor: c.Meta.Valor}, rels)
		got := map[string]any{"factible": r.Factible, "valores": r.Valores, "objetivo": r.Objetivo, "iteraciones": float64(r.Iteraciones)}
		for k, w := range c.Esperado {
			cercaDe(t, c.Nombre, k, got[k], w)
		}
	}
	for _, c := range v.Tabla {
		p, fallo := PlanearTabla(c.Rect[0], c.Rect[1], c.Rect[2], c.Rect[3], c.Fila, c.Col)
		var got any = map[string]any{"error": fallo}
		if fallo == "" {
			got = planComoLaWeb(p)
		}
		var a any
		b, _ := json.Marshal(got)
		_ = json.Unmarshal(b, &a)
		if !reflect.DeepEqual(a, c.Esperado) {
			w, _ := json.Marshal(c.Esperado)
			t.Errorf("%s: sale %s, y la web da %s", c.Nombre, b, w)
		}
	}
}

// planComoLaWeb es el plan con la forma de `DataTablePlan`.
func planComoLaWeb(p PlanTabla) map[string]any {
	cr := func(x [2]int) map[string]int { return map[string]int{"c": x[0], "r": x[1]} }
	celdas := make([]map[string]any, 0, len(p.Celdas))
	for _, c := range p.Celdas {
		m := map[string]any{"c": c.C, "r": c.F}
		if c.FilaDe != nil {
			m["rowFrom"] = cr(*c.FilaDe)
		}
		if c.ColDe != nil {
			m["colFrom"] = cr(*c.ColDe)
		}
		celdas = append(celdas, m)
	}
	return map[string]any{"orientation": p.Orientacion, "formula": cr(p.Formula), "cells": celdas}
}
