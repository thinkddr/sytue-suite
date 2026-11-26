// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"encoding/json"
	"fmt"
	"time"
)

// VectorDeVista es un caso de filtro, búsqueda y orden con lo que ha de salir: los
// nombres (el campo de texto) de los registros, en orden.
type VectorDeVista struct {
	Nombre string
	Config ConfigVista
	Quiere []string
}

// JuegoDeVistas es una tabla de prueba y los casos que se leen de ella.
type JuegoDeVistas struct {
	Campos    []Campo
	Registros []Registro
	Casos     []VectorDeVista
	// Texto es el id del campo que nombra cada registro en Quiere.
	Texto string
}

// Ids fijos de los campos de los juegos: los dos lados (el SQL del servicio y el
// evaluador en memoria) leen los mismos.
const (
	vecTexto   = "00000000-0000-4000-8000-0000000000a1"
	vecNumero  = "00000000-0000-4000-8000-0000000000a2"
	vecEstado  = "00000000-0000-4000-8000-0000000000a3"
	vecCasilla = "00000000-0000-4000-8000-0000000000a4"
	vecMultip  = "00000000-0000-4000-8000-0000000000a5"
)

// VectoresDeVista son los casos de las pruebas del SQL del servicio
// (`services/bases/internal/store`: TestVistasYFiltros y TestBusquedaYOrdenAdHoc) más
// los bordes de NULL de `construirConsulta`, escritos una vez para los dos lados. El
// servicio puede correrlos contra Postgres creando los mismos campos y registros.
func VectoresDeVista() []JuegoDeVistas {
	j := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	reg := func(i int, datos map[string]any) Registro {
		d := map[string]json.RawMessage{}
		for k, v := range datos {
			d[k] = j(v)
		}
		return Registro{ID: fmt.Sprintf("00000000-0000-4000-9000-%012d", i), Data: d, CreatedAt: t0.Add(time.Duration(i) * time.Minute)}
	}
	campos := []Campo{
		{ID: vecTexto, Name: "Nombre", Kind: TipoTexto},
		{ID: vecNumero, Name: "Importe", Kind: TipoNumero},
		{ID: vecEstado, Name: "Estado", Kind: TipoSeleccion, Choices: []string{"nuevo", "hecho"}},
		{ID: vecCasilla, Name: "Hecho", Kind: TipoCasilla},
		{ID: vecMultip, Name: "Etiquetas", Kind: TipoMultiple, Choices: []string{"a", "b"}},
	}
	f := func(campo, op string, v any) Filtro {
		if v == nil {
			return Filtro{Field: campo, Op: op}
		}
		return Filtro{Field: campo, Op: op, Value: j(v)}
	}
	// TestVistasYFiltros, tal cual, y sus bordes.
	vistas := JuegoDeVistas{Campos: campos, Texto: vecTexto, Registros: []Registro{
		reg(1, map[string]any{vecTexto: "100% Ana", vecNumero: 5, vecEstado: "nuevo", vecCasilla: true, vecMultip: []string{"a"}}),
		reg(2, map[string]any{vecTexto: "Bea", vecNumero: 20, vecEstado: "hecho", vecCasilla: false, vecMultip: []string{"a", "b"}}),
		reg(3, map[string]any{vecTexto: "Cleo", vecNumero: 15}),
	}, Casos: []VectorDeVista{
		{"número > 10 desc", ConfigVista{Filters: []Filtro{f(vecNumero, "gt", 10)}, Sort: []Orden{{vecNumero, "desc"}}}, []string{"Bea", "Cleo"}},
		{"% no es comodín", ConfigVista{Filters: []Filtro{f(vecTexto, "contains", "%")}}, []string{"100% Ana"}},
		{"contains sin mayúsculas", ConfigVista{Filters: []Filtro{f(vecTexto, "contains", "BEA")}}, []string{"Bea"}},
		{"selección vacía", ConfigVista{Filters: []Filtro{f(vecEstado, "empty", nil)}}, []string{"Cleo"}},
		{"dos filtros", ConfigVista{Filters: []Filtro{f(vecEstado, "eq", "hecho"), f(vecNumero, "lte", 20)}}, []string{"Bea"}},
		{"neq deja pasar los NULL", ConfigVista{Filters: []Filtro{f(vecEstado, "neq", "hecho")}}, []string{"100% Ana", "Cleo"}},
		{"casilla falsa incluye la vacía", ConfigVista{Filters: []Filtro{f(vecCasilla, "eq", false)}}, []string{"Bea", "Cleo"}},
		{"múltiple contiene", ConfigVista{Filters: []Filtro{f(vecMultip, "contains", "b")}}, []string{"Bea"}},
		{"número igual exacto", ConfigVista{Filters: []Filtro{f(vecNumero, "eq", 15)}}, []string{"Cleo"}},
		{"campo borrado se ignora", ConfigVista{Filters: []Filtro{f("00000000-0000-4000-8000-0000000000ff", "eq", "x")}}, []string{"100% Ana", "Bea", "Cleo"}},
		{"NULL al final en asc", ConfigVista{Sort: []Orden{{vecEstado, "asc"}}}, []string{"Bea", "100% Ana", "Cleo"}},
		{"NULL al final en desc", ConfigVista{Sort: []Orden{{vecEstado, "desc"}}}, []string{"100% Ana", "Bea", "Cleo"}},
		{"búsqueda en cualquier campo", ConfigVista{Search: "hech"}, []string{"Bea"}},
		{"búsqueda por número", ConfigVista{Search: "15"}, []string{"Cleo"}},
	}}
	// TestBusquedaYOrdenAdHoc: treinta filas, tres con «AGUJA».
	filas := make([]Registro, 0, 30)
	for i := range 30 {
		palabra := "paja"
		if i%10 == 7 {
			palabra = "AGUJA"
		}
		filas = append(filas, reg(i, map[string]any{vecTexto: fmt.Sprintf("Fila %d %s", i, palabra), vecNumero: i}))
	}
	busqueda := JuegoDeVistas{Campos: campos[:2], Texto: vecTexto, Registros: filas, Casos: []VectorDeVista{
		{"búsqueda y orden de un momento", ConfigVista{Search: "aguja", Sort: []Orden{{vecNumero, "desc"}}}, []string{"Fila 27 AGUJA", "Fila 17 AGUJA", "Fila 7 AGUJA"}},
	}}
	return []JuegoDeVistas{vistas, busqueda}
}
