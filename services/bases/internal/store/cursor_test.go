// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func consultaDePrueba() consulta {
	campos := []domain.Campo{{ID: "f-texto", Kind: domain.TipoTexto}, {ID: "f-num", Kind: domain.TipoNumero}, {ID: "f-check", Kind: domain.TipoCasilla}}
	return construirConsulta(campos, domain.ConfigVista{Sort: []domain.Orden{{Field: "f-num", Dir: "desc"}, {Field: "f-texto", Dir: "asc"}, {Field: "f-check", Dir: "asc"}}})
}

func cursorCon(q consulta, k []*string, t, i string) string {
	b, _ := json.Marshal(cursorPagina{O: q.firmaOrden(), K: k, T: t, I: i})
	return base64.RawURLEncoding.EncodeToString(b)
}

func p(s string) *string { return &s }

const (
	idBueno = "0b0f6d0e-6f0a-4f0e-9d59-0d7b6d3f2a11"
	tBueno  = "2026-10-04T10:00:00.123456Z"
)

// El cursor es entrada del cliente: nada de lo que trae llega al SQL como texto, y lo
// que no tiene la forma exacta se rechaza ANTES de la base de datos (422, no 500).
func TestCursorHostil(t *testing.T) {
	veneno := `'; DROP TABLE bases; --`
	q := consultaDePrueba()
	hostiles := map[string]string{
		"no es base64": "%%%",
		"no es JSON":   base64.RawURLEncoding.EncodeToString([]byte("{")),
		// Válido salvo por el tamaño (JSON admite espacios detrás): lo para el techo, no el parseo.
		"gigante":             base64.RawURLEncoding.EncodeToString([]byte(`{"o":"` + q.firmaOrden() + `","k":[null,null,null],"t":"` + tBueno + `","i":"` + idBueno + `"}` + strings.Repeat(" ", domain.MaxCursor))),
		"de otro orden":       base64.RawURLEncoding.EncodeToString([]byte(`{"o":"0000000000000000","k":[null,null,null],"t":"` + tBueno + `","i":"` + idBueno + `"}`)),
		"claves de menos":     cursorCon(q, []*string{p("1")}, tBueno, idBueno),
		"número que no lo es": cursorCon(q, []*string{p(veneno), nil, nil}, tBueno, idBueno),
		"número infinito":     cursorCon(q, []*string{p("1e999"), nil, nil}, tBueno, idBueno),
		"NaN":                 cursorCon(q, []*string{p("NaN"), nil, nil}, tBueno, idBueno),
		"casilla rara":        cursorCon(q, []*string{nil, nil, p("t")}, tBueno, idBueno),
		"texto demasiado":     cursorCon(q, []*string{nil, p(strings.Repeat("x", domain.MaxClaveOrden+1)), nil}, tBueno, idBueno),
		"texto con NUL":       cursorCon(q, []*string{nil, p("a\x00b"), nil}, tBueno, idBueno),
		"fecha rara":          cursorCon(q, []*string{nil, nil, nil}, veneno, idBueno),
		"id que no es UUID":   cursorCon(q, []*string{nil, nil, nil}, tBueno, veneno),
	}
	for nombre, cur := range hostiles {
		q := consultaDePrueba()
		if err := q.despuesDe(cur); !errors.Is(err, domain.ErrInvalido) {
			t.Errorf("%s: %v, esperado ErrInvalido", nombre, err)
		}
	}
	// Uno válido con veneno en el texto: entra como parámetro, nunca en el SQL.
	q = consultaDePrueba()
	if err := q.despuesDe(cursorCon(q, []*string{p("3.5"), p(veneno), nil}, tBueno, idBueno)); err != nil {
		t.Fatal(err)
	}
	sql := q.where + " " + q.order
	for _, prohibido := range []string{"DROP", "--", "f-texto", idBueno, "2026"} {
		if strings.Contains(sql, prohibido) {
			t.Errorf("el SQL contiene entrada del cliente (%q): %s", prohibido, sql)
		}
	}
	for i := range q.args {
		if !strings.Contains(sql, "$"+strconv.Itoa(i+3)+"::text") {
			t.Errorf("el parámetro %d no se usa: %s", i+3, sql)
		}
	}
	if strings.Contains(sql, "$"+strconv.Itoa(len(q.args)+3)) {
		t.Error("referencia a un parámetro que no existe")
	}
	if strings.Contains(sql, "OFFSET") {
		t.Error("la página no usa OFFSET")
	}
}

// El cursor que sale de un registro vuelve a entrar, y recorta el texto como lo hace
// la clave del orden (left(…, MaxClaveOrden)): si no, el empate no empataría.
func TestCursorIdaYVuelta(t *testing.T) {
	q := consultaDePrueba()
	largo := strings.Repeat("ñ", domain.MaxClaveOrden+50)
	r := domain.Registro{ID: idBueno, CreatedAt: time.Date(2026, 10, 4, 10, 0, 0, 123456000, time.UTC), Data: map[string]json.RawMessage{
		"f-num": json.RawMessage(`-12.50`), "f-texto": json.RawMessage(strconv.Quote(largo)), "f-check": json.RawMessage(`null`),
	}}
	cur := q.cursorDe(r)
	if len(cur) > domain.MaxCursor {
		t.Fatalf("el cursor no cabe: %d", len(cur))
	}
	if err := q.despuesDe(cur); err != nil {
		t.Fatal(err)
	}
	// num, texto recortado, fecha e id; la casilla vacía no lleva parámetro.
	got := q.args[len(q.args)-4:]
	if got[0] != tBueno || got[1] != idBueno || got[2] != strings.Repeat("ñ", domain.MaxClaveOrden) || got[3] != "-12.50" {
		t.Errorf("parámetros del cursor: %v", got)
	}
}
