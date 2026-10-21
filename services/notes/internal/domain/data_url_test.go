// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// `data.url` acaba en un `href` que pulsa OTRA persona: la verja tiene que ser la
// misma que la de `links[]`, y antes no había ninguna.
func TestNormalizeExigeQueDataURLSeaWeb(t *testing.T) {
	casos := []struct {
		url string
		ok  bool
	}{
		{"https://sytue.eu/x", true},
		{"http://intranet/x", true},
		{"javascript:alert(1)", false},
		{"data:text/html,hola", false},
		{"ftp://h/x", false},
		{"https://", false},
	}
	for _, c := range casos {
		in := NoteInput{Kind: "link", Data: json.RawMessage(`{"url":` + strconv.Quote(c.url) + `}`)}
		if err := in.Normalize(false); (err == nil) != c.ok {
			t.Errorf("url %q: err=%v; quiero ok=%v", c.url, err, c.ok)
		}
	}
	// Sin `url`, o con un `data` que no es un objeto, no hay nada que validar.
	for _, d := range []string{`{"nombre":"x"}`, `{}`, `[1,2]`, `"texto"`} {
		in := NoteInput{Kind: "text", Data: json.RawMessage(d)}
		if err := in.Normalize(false); err != nil {
			t.Errorf("data %s: %v", d, err)
		}
	}
}

// `data.client_id` es una cadena corta o no está. Y un `client_id` raro no puede abrir
// paso a un `url` que no sea web: se validan por separado.
func TestNormalizeClientID(t *testing.T) {
	casos := []struct {
		data string
		ok   bool
	}{
		{`{"client_id":"c-1"}`, true},
		{`{"client_id":""}`, false},
		{`{"client_id":` + strconv.Quote(strings.Repeat("x", MaxClientID+1)) + `}`, false},
		{`{"client_id":7}`, false},
		{`{"client_id":7,"url":"javascript:alert(1)"}`, false},
		{`{"client_id":"c-2","url":"javascript:alert(1)"}`, false},
	}
	for _, c := range casos {
		in := NoteInput{Kind: "link", Data: json.RawMessage(c.data)}
		if err := in.Normalize(false); (err == nil) != c.ok {
			t.Errorf("data %s: err=%v; quiero ok=%v", c.data, err, c.ok)
		}
	}
	in := NoteInput{Data: json.RawMessage(`{"client_id":"c-1","url":"https://x.eu"}`)}
	if in.ClientID() != "c-1" {
		t.Errorf("ClientID = %q", in.ClientID())
	}
}
