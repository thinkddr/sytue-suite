// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

// TestLaCoedicionHablaConElServicio: el estado trae la secuencia y la seq; el lote lleva
// las ops por carácter delante de la marca y del `set` (como `buildOpsBody`); el sondeo
// pide desde la seq; la presencia manda la conexión; y un lote que no cabe no sale.
func TestLaCoedicionHablaConElServicio(t *testing.T) {
	var lote string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/documents/d1/state":
			_, _ = io.WriteString(w, `{"seq":41,"state":{"lamport":7,"elements":[{"dot":{"a":"w","c":1},"after":{"a":"","c":0},"value":"a","deleted":false}],"registers":{"doc:html":{"value":"a","dot":{"a":"w","c":2},"lamport":7}}}}`)
		case "POST /api/v1/documents/d1/ops":
			lote = string(cuerpo)
			_, _ = io.WriteString(w, `{"seq":43}`)
		case "GET /api/v1/documents/d1/ops":
			if r.URL.Query().Get("since") != "41" {
				t.Errorf("since=%s", r.URL.Query().Get("since"))
			}
			_, _ = io.WriteString(w, `{"ops":[{"seq":42,"actor":"w","op":{"kind":"insert","dot":{"a":"w","c":3},"after":{"a":"w","c":1},"value":"b","lamport":8}}]}`)
		case "PUT /api/v1/documents/d1/presence":
			if string(cuerpo) != `{"conn":"yo"}` {
				t.Errorf("presencia: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"members":[{"conn":"yo","sub":"u1"},{"conn":"w","sub":"u2"}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()
	c, err := d.Content(ctx, "d1", "doc:html")
	if err != nil || c.Seq != 41 || len(coedicion.ElementosDeEstado(c.Elementos)) != 1 {
		t.Fatalf("estado: %+v, %v", c, err)
	}
	co := coedicion.Nueva("yo", coedicion.ElementosDeEstado(c.Elementos), c.Value)
	chars := co.OpsLocales("ab", 7, false)
	if seq, err := d.SaveCoedicion(ctx, "d1", "doc:html", "ab", DocDot{"yo", 3}, 9, false, chars); err != nil || seq != 43 {
		t.Fatalf("guardar: %d, %v", seq, err)
	}
	if !strings.HasPrefix(lote, `{"ops":[{"kind":"insert","dot":{"a":"yo","c":2},"after":{"a":"w","c":1},"value":"b","lamport":8},{"kind":"set",`) ||
		strings.Contains(lote, "doc:html:lww") {
		t.Errorf("lote: %s", lote)
	}
	ops, err := d.OpsDesde(ctx, "d1", 41)
	if err != nil || len(ops) != 1 || ops[0].Seq != 42 {
		t.Fatalf("sondeo: %+v, %v", ops, err)
	}
	if op, ok := coedicion.OpDeSecuencia(ops[0].Op); !ok || op.Value != "b" {
		t.Errorf("la op sondeada: %+v", op)
	}
	if m, err := d.Presencia(ctx, "d1", "yo"); err != nil || len(m) != 2 || m[1].Sub != "u2" {
		t.Errorf("presencia: %+v, %v", m, err)
	}
	if _, err := d.SaveCoedicion(ctx, "d1", "doc:html", strings.Repeat("x", coedicion.MaxCuerpo), DocDot{"yo", 3}, 10, true, nil); !errors.Is(err, ErrLoteGrande) {
		t.Errorf("un lote enorme no sale: %v", err)
	}
}
