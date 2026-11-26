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

// Los filtros de fecha relativos solo llevan constantes y parámetros: el día de quien
// mira viaja como `$n::text::date` y el número de días se escribe desde un entero ya
// acotado. Sin `today` vale `current_date`; un número de días roto no deja pasar nada.
func TestFiltrosDeFechaRelativos(t *testing.T) {
	campos := []domain.Campo{{ID: "f-fecha", Kind: domain.TipoFecha}}
	v := func(x any) json.RawMessage { b, _ := json.Marshal(x); return b }
	con := func(op string, valor json.RawMessage, hoy string) consulta {
		return construirConsulta(campos, domain.ConfigVista{Filters: []domain.Filtro{{Field: "f-fecha", Op: op, Value: valor}}, Today: hoy})
	}
	q := con(domain.OpUltimosDias, v(7), "2026-11-25")
	if !strings.Contains(q.where, "BETWEEN to_char($3::text::date - 6, 'YYYY-MM-DD') AND to_char($4::text::date, 'YYYY-MM-DD')") {
		t.Errorf("últimos 7 días (hoy incluido): %s", q.where)
	}
	if len(q.args) != 3 || q.args[1] != "2026-11-25" {
		t.Errorf("parámetros: %v", q.args)
	}
	if q = con(domain.OpProximosDia, v(1), ""); !strings.Contains(q.where, "BETWEEN to_char(current_date, 'YYYY-MM-DD') AND to_char(current_date + 0, 'YYYY-MM-DD')") {
		t.Errorf("próximo día sin today: %s", q.where)
	}
	if q = con(domain.OpHoy, nil, ""); !strings.Contains(q.where, "= to_char(current_date, 'YYYY-MM-DD')") {
		t.Errorf("hoy: %s", q.where)
	}
	if q = con(domain.OpEsteMes, nil, "2026-02-10"); !strings.Contains(q.where, "date_trunc('month', $6::text::date) + interval '1 month'") {
		t.Errorf("este mes: %s", q.where)
	}
	veneno := v("7; DELETE")
	for _, malo := range []json.RawMessage{veneno, v(0), v(domain.MaxDiasRelativos + 1), nil} {
		if q = con(domain.OpUltimosDias, malo, ""); !strings.HasSuffix(q.where, " AND false") {
			t.Errorf("días %s: %s", malo, q.where)
		}
	}
	if strings.Contains(con(domain.OpUltimosDias, veneno, "").where, "DELETE") {
		t.Error("el valor del cliente acaba en el SQL")
	}
}

// El resumen agrega cada columna con su id como parámetro, solo suma lo que es número
// en el JSON y no resume adjuntos. Un numérico lleva cinco columnas y el resto, una: es
// lo que `Scan` espera recibir.
func TestColumnasDeResumen(t *testing.T) {
	campos := []domain.Campo{
		{ID: "f-num", Kind: domain.TipoNumero},
		{ID: "f-mon", Kind: domain.TipoMoneda},
		{ID: "f-txt", Kind: domain.TipoTexto},
		{ID: "f-check", Kind: domain.TipoCasilla},
		{ID: "f-link", Kind: domain.TipoRelacion},
		{ID: "f-adj", Kind: domain.TipoAdjunto},
		{ID: "f-form", Kind: domain.TipoFormula, Result: domain.TipoNumero},
	}
	q := consulta{}
	sel, lectores := columnasDeResumen(campos, q.add)
	if len(lectores) != 6 {
		t.Fatalf("lectores: %d (el adjunto no se resume)", len(lectores))
	}
	destinos := 0
	for _, l := range lectores {
		destinos += len(l.destinos())
	}
	if destinos != 3*5+3 || strings.Count(sel, ", ") < destinos {
		t.Errorf("columnas: %d destinos para %q", destinos, sel)
	}
	for _, id := range []string{"f-num", "f-txt", "f-link"} {
		if strings.Contains(sel, id) {
			t.Errorf("el id %s va pegado en el SQL", id)
		}
	}
	if !strings.Contains(sel, "jsonb_typeof(r.data -> $3::text) = 'number'") {
		t.Errorf("la suma no se protege de una fórmula con error: %s", sel)
	}
	for i := range q.args {
		if !strings.Contains(sel, "$"+strconv.Itoa(i+3)+"::text") {
			t.Errorf("el parámetro %d no se usa", i+3)
		}
	}
}
