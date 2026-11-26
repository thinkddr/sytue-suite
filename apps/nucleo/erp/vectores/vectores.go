// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package vectores son los VECTORES DORADOS del ERP: las mismas entradas tienen que dar
// la misma numeración, los mismos asientos, el mismo desglose y los mismos saldos en el
// almacén de Postgres de services/syerp y en el de SQLite de Sytue Office
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.13, X-08). Cada almacén tiene su prueba que
// recorre los pasos; este paquete solo los describe, embebidos.
package vectores

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.json
var ficheros embed.FS

// Escenario es una historia de pasos con lo que tiene que salir.
type Escenario struct {
	Nombre      string  `json:"nombre"`
	Descripcion string  `json:"descripcion"`
	Pasos       []Paso  `json:"pasos"`
	Balance     []Saldo `json:"balance"`
}

// Paso es UNA de estas acciones: crear un registro o asentar uno ya creado.
type Paso struct {
	// Crear es el modelo del registro que se crea; Como, el nombre con el que lo citan
	// los pasos siguientes («$cli»); Ref, la referencia que tiene que recibir.
	Crear string         `json:"crear,omitempty"`
	Como  string         `json:"como,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
	Ref   string         `json:"ref,omitempty"`
	// Asentar es el nombre del documento que se lleva al libro. Error es el que tiene
	// que dar («ya_asentado» o «datos»); sin él, el asiento que sale (Diario, Ref,
	// Apuntes) y el Estado en el que queda el documento.
	Asentar string   `json:"asentar,omitempty"`
	Error   string   `json:"error,omitempty"`
	Diario  string   `json:"diario,omitempty"`
	Estado  string   `json:"estado,omitempty"`
	Apuntes []Apunte `json:"apuntes,omitempty"`
}

// Apunte es una línea esperada del asiento; Tercero puede ser «$nombre».
type Apunte struct {
	Cuenta  string `json:"cuenta"`
	Debe    int64  `json:"debe,omitempty"`
	Haber   int64  `json:"haber,omitempty"`
	Tercero string `json:"tercero,omitempty"`
}

// Saldo es una fila esperada del sumas y saldos.
type Saldo struct {
	Cuenta string `json:"cuenta"`
	Debe   int64  `json:"debe"`
	Haber  int64  `json:"haber"`
	Saldo  int64  `json:"saldo"`
}

// Cargar lee todos los escenarios, por nombre.
func Cargar() ([]Escenario, error) {
	entradas, err := ficheros.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []Escenario
	for _, e := range entradas {
		b, err := ficheros.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		var esc Escenario
		if err := json.Unmarshal(b, &esc); err != nil {
			return nil, fmt.Errorf("vectores: %s: %w", e.Name(), err)
		}
		out = append(out, esc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nombre < out[j].Nombre })
	return out, nil
}

// Resolver cambia cada «$nombre» de los datos por el id que recibió ese registro.
func Resolver(data map[string]any, ids map[string]string) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "$") {
			v = ids[s[1:]]
		}
		out[k] = v
	}
	return out
}

// Tercero es el id esperado en un apunte.
func (a Apunte) TerceroID(ids map[string]string) string {
	if strings.HasPrefix(a.Tercero, "$") {
		return ids[a.Tercero[1:]]
	}
	return a.Tercero
}
