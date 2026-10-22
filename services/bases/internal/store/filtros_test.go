// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// El SQL de una vista solo contiene constantes y placeholders: lo que escribe el
// cliente (valores, ids de campo, operadores, dirección) viaja como parámetro o
// no entra. Y cada parámetro está referenciado: uno suelto es un error de Postgres.
func TestConstruirConsultaNoConcatenaEntrada(t *testing.T) {
	veneno := `'; DROP TABLE bases; --`
	campos := []domain.Campo{
		{ID: "f-texto", Kind: domain.TipoTexto},
		{ID: "f-num", Kind: domain.TipoNumero},
		{ID: "f-fecha", Kind: domain.TipoFecha},
		{ID: "f-check", Kind: domain.TipoCasilla},
		{ID: "f-sel", Kind: domain.TipoSeleccion},
		{ID: "f-mul", Kind: domain.TipoMultiple},
		{ID: "f-link", Kind: domain.TipoRelacion},
	}
	v := func(x any) json.RawMessage { b, _ := json.Marshal(x); return b }
	cfg := domain.ConfigVista{
		Filters: []domain.Filtro{
			{Field: "f-texto", Op: "contains", Value: v(veneno)},
			{Field: "f-texto", Op: "eq", Value: v(veneno)},
			{Field: "f-texto", Op: "neq", Value: v("x")},
			{Field: "f-texto", Op: "empty"},
			{Field: "f-num", Op: "gte", Value: v(3.5)},
			{Field: "f-num", Op: "eq", Value: v(1)},
			{Field: "f-fecha", Op: "lt", Value: v("2026-01-01")},
			{Field: "f-check", Op: "eq", Value: v(true)},
			{Field: "f-sel", Op: "neq", Value: v(veneno)},
			{Field: "f-mul", Op: "contains", Value: v(veneno)},
			{Field: "f-link", Op: "notempty"},
			{Field: "f-fantasma", Op: "eq", Value: v("x")}, // campo borrado: se ignora
			{Field: "f-texto", Op: "eq; DROP", Value: v("x")},
		},
		Sort: []domain.Orden{{Field: "f-num", Dir: "desc"}, {Field: "f-texto", Dir: "asc; --"}, {Field: "f-fantasma", Dir: "asc"}},
	}
	q := construirConsulta(campos, cfg)
	sql := q.where + " " + q.order
	for _, prohibido := range []string{"DROP", "--", "f-texto", "f-num"} {
		if strings.Contains(sql, prohibido) {
			t.Errorf("el SQL contiene entrada del cliente (%q): %s", prohibido, sql)
		}
	}
	for i := range q.args {
		ph := "$" + strconv.Itoa(i+3) + "::text"
		if !strings.Contains(sql, ph) {
			t.Errorf("el parámetro %d no se usa: %s", i+3, sql)
		}
	}
	if strings.Contains(sql, "$"+strconv.Itoa(len(q.args)+3)) {
		t.Error("referencia a un parámetro que no existe")
	}
	if strings.Contains(sql, " DESC") == false || strings.Count(sql, " ASC") < 1 {
		t.Errorf("las direcciones salen de constantes: %s", sql)
	}
	// El orden «asc; --» no es una dirección: cae a ASC, nunca se escribe tal cual.
	if strings.Contains(sql, "asc;") {
		t.Error("dirección del cliente en el SQL")
	}
}

func TestConstruirConsultaPorDefecto(t *testing.T) {
	q := construirConsulta(nil, domain.ConfigVista{})
	if q.where != "" || q.order != "r.created_at, r.id" || len(q.args) != 0 {
		t.Errorf("%+v", q)
	}
}
