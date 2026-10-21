// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// TestElRellenoComoLaWeb recorre los vectores que grabó la web.
func TestElRellenoComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/relleno.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Peticiones []struct {
			Nombre    string     `json:"nombre"`
			Cabeceras []string   `json:"cabeceras"`
			Filas     [][]string `json:"filas"`
			Primera   int        `json:"primera"`
			Objetivo  int        `json:"objetivo"`
			Pista     string     `json:"pista"`
			Esperado  *struct {
				Instruction string `json:"instruction"`
				Context     string `json:"context"`
				Pending     []int  `json:"pending"`
			} `json:"esperado"`
		} `json:"peticiones"`
		Lecturas []struct {
			Nombre     string            `json:"nombre"`
			Texto      string            `json:"texto"`
			Pendientes []int             `json:"pendientes"`
			Esperado   map[string]string `json:"esperado"`
		} `json:"lecturas"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Peticiones {
		p, ok := PedirRelleno(c.Cabeceras, c.Filas, c.Primera, c.Objetivo, c.Pista)
		if c.Esperado == nil {
			if ok {
				t.Errorf("%s: pide algo y la web no", c.Nombre)
			}
			continue
		}
		e := c.Esperado
		if !ok || p.Instruccion != e.Instruction || p.Contexto != e.Context || !reflect.DeepEqual(p.Pendientes, e.Pending) {
			t.Errorf("%s: pide\n%q\n%q\n%v\ny la web\n%q\n%q\n%v", c.Nombre, p.Instruccion, p.Contexto, p.Pendientes, e.Instruction, e.Context, e.Pending)
		}
	}
	for _, c := range v.Lecturas {
		got := map[string]string{}
		for k, x := range LeerRelleno(c.Texto, c.Pendientes) {
			got[strconv.Itoa(k)] = x
		}
		if !reflect.DeepEqual(got, c.Esperado) {
			t.Errorf("%s: lee %v, y la web %v", c.Nombre, got, c.Esperado)
		}
	}
}
