// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiferencias(t *testing.T) {
	base := `{"sheets":[{"id":"h1","name":"H","cells":{"A1":"1"},"w":1.50}],"slides":[{"id":"s1","boxes":[{"id":"b1","html":"a"}]}]}`
	casos := []struct {
		nombre, mio  string
		quiero       []Cambio
		inexpresable bool
	}{
		{"igual", base, nil, false},
		{
			"celda nueva", `{"sheets":[{"id":"h1","name":"H","cells":{"A1":"1","B2":"x"},"w":1.50}],"slides":[{"id":"s1","boxes":[{"id":"b1","html":"a"}]}]}`,
			[]Cambio{{Path: []parcheTramo{"sheets", map[string]string{"id": "h1"}, "cells", "B2"}, Value: "x"}},
			false,
		},
		{
			"borra celda", `{"sheets":[{"id":"h1","name":"H","cells":{},"w":1.50}],"slides":[{"id":"s1","boxes":[{"id":"b1","html":"a"}]}]}`,
			[]Cambio{{Path: []parcheTramo{"sheets", map[string]string{"id": "h1"}, "cells", "A1"}, Del: true}},
			false,
		},
		{
			"caja editada", `{"sheets":[{"id":"h1","name":"H","cells":{"A1":"1"},"w":1.50}],"slides":[{"id":"s1","boxes":[{"id":"b1","html":"A"}]}]}`,
			[]Cambio{{Path: []parcheTramo{"slides", map[string]string{"id": "s1"}, "boxes", map[string]string{"id": "b1"}, "html"}, Value: "A"}},
			false,
		},
		{
			"hoja nueva al final", `{"sheets":[{"id":"h1","name":"H","cells":{"A1":"1"},"w":1.50},{"id":"h2"}],"slides":[{"id":"s1","boxes":[{"id":"b1","html":"a"}]}]}`,
			[]Cambio{{Path: []parcheTramo{"sheets", map[string]string{"id": "h2"}}, Value: map[string]any{"id": "h2"}}},
			false,
		},
		{"diapositiva en medio", `{"sheets":[{"id":"h1","name":"H","cells":{"A1":"1"},"w":1.50}],"slides":[{"id":"n"},{"id":"s1","boxes":[{"id":"b1","html":"a"}]}]}`, nil, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := Diferencias(base, c.mio)
			if c.inexpresable {
				if !errors.Is(err, errNoExpresable) {
					t.Fatalf("err = %v, quiero errNoExpresable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// comparar por JSON: Value llega como json.Number en el decodificado
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(c.quiero)
			if !bytes.Equal(a, b) && (len(got) != 0 || len(c.quiero) != 0) {
				t.Fatalf("got %s want %s", a, b)
			}
		})
	}
	// hojas por nombre (sin id): no es un parche
	if _, err := Diferencias(`{"sheets":[{"name":"A"}]}`, `{"sheets":[{"name":"A"},{"name":"B"}]}`); !errors.Is(err, errNoExpresable) {
		t.Fatalf("hojas por nombre: %v", err)
	}
}

func TestSaveCheckedPorParche(t *testing.T) {
	var cuerpo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /api/v1/documents/d1/ops" {
			t.Errorf("petición inesperada (un parche no lee el registro): %s %s", r.Method, r.URL.Path)
			return
		}
		b, _ := io.ReadAll(r.Body)
		cuerpo = string(b)
		var in struct {
			Ops []struct {
				OpID string `json:"op_id"`
			} `json:"ops"`
		}
		_ = json.Unmarshal(b, &in)
		_, _ = io.WriteString(w, `{"seq":9,"ops":[{"seq":9,"op":{"op_id":"`+in.Ops[0].OpID+`","value":"{\"sheets\":[]}","dot":{"a":"patch:x","c":1},"lamport":30}}]}`)
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, nil)
	base := DocContent{Value: `{"sheets":[{"id":"h1","cells":{}}]}`, Dot: DocDot{"web", 1}, Lamport: 5, Seq: 7}
	nuevo := `{"sheets":[{"id":"h1","cells":{"A1":"x"}}]}`
	res, err := d.SaveChecked(context.Background(), "d1", "doc:sheet", nuevo, base, DocDot{"gio", 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != nuevo || res.Seq != 9 || res.Lamport != 30 || res.Dot.Actor != "patch:x" {
		t.Errorf("res = %+v", res)
	}
	for _, quiero := range []string{`"kind":"patch"`, `"base":7`, `"path":["sheets",{"id":"h1"},"cells","A1"]`} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("el cuerpo no lleva %s: %s", quiero, cuerpo)
		}
	}
	if strings.Contains(cuerpo, "base_seq") {
		t.Errorf("un parche no manda base_seq: %s", cuerpo)
	}
}
