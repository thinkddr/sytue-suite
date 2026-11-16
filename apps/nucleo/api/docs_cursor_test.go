// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

// vectoresCursor son las posiciones y los pares de coedicion/testdata/cursores.json, que
// recorre también `editor/state/cursores-compartidos.test.ts` con `parseCursorPos` y
// `parsePeerCursor`: lo que la web descarta, Fyne lo descarta, y lo que lee, lo lee igual.
type vectoresCursor struct {
	Posiciones []struct {
		Nombre   string          `json:"nombre"`
		Crudo    json.RawMessage `json:"crudo"`
		Esperado json.RawMessage `json:"esperado"`
	} `json:"posiciones"`
	Pares []struct {
		Nombre   string          `json:"nombre"`
		Crudo    json.RawMessage `json:"crudo"`
		Esperado *struct {
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Color string          `json:"color"`
			Pos   json.RawMessage `json:"pos"`
		} `json:"esperado"`
	} `json:"pares"`
}

func leerVectoresCursor(t *testing.T) vectoresCursor {
	t.Helper()
	crudo, err := os.ReadFile("../coedicion/testdata/cursores.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresCursor
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Posiciones) < 10 || len(v.Pares) < 4 {
		t.Fatalf("pocos vectores: %d y %d", len(v.Posiciones), len(v.Pares))
	}
	return v
}

// mismoJSON compara dos JSON por su valor, no por el orden de las claves.
func mismoJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// TestLeerCursorComoLaWeb: cada posición se lee (y se vuelve a escribir) como la deja
// `parseCursorPos`, y las que la web descarta se descartan.
func TestLeerCursorComoLaWeb(t *testing.T) {
	for _, c := range leerVectoresCursor(t).Posiciones {
		pos, ok := LeerCursor(c.Crudo)
		if string(c.Esperado) == "null" {
			if ok {
				t.Errorf("%s: leído %+v y la web lo descarta", c.Nombre, pos)
			}
			continue
		}
		escrito, err := json.Marshal(pos)
		if !ok || err != nil || !mismoJSON(t, escrito, c.Esperado) {
			t.Errorf("%s: %s (%v), quiero %s", c.Nombre, escrito, ok, c.Esperado)
		}
	}
}

// TestElCursorDeUnParComoLaWeb: el par se queda con su sub, su nombre, el color de su sub
// y la posición; sin sub o sin una posición que encaje, no hay cursor que pintar.
func TestElCursorDeUnParComoLaWeb(t *testing.T) {
	for _, c := range leerVectoresCursor(t).Pares {
		var p *DocPresencia
		if err := json.Unmarshal(c.Crudo, &p); err != nil {
			t.Fatalf("%s: %v", c.Nombre, err)
		}
		p = cursorDePar(p)
		if c.Esperado == nil {
			if p != nil {
				t.Errorf("%s: se queda %+v y la web lo descarta", c.Nombre, p)
			}
			continue
		}
		if p == nil {
			t.Errorf("%s: descartado", c.Nombre)
			continue
		}
		pos, _ := json.Marshal(p.Cursor)
		if p.Sub != c.Esperado.ID || p.Name != c.Esperado.Name || p.Color != c.Esperado.Color || !mismoJSON(t, pos, c.Esperado.Pos) {
			t.Errorf("%s: %+v %s, quiero %+v %s", c.Nombre, p, pos, c.Esperado, c.Esperado.Pos)
		}
	}
}

// TestElCanalMandaElCursorYEntregaElDeLosDemas: el cursor propio sale por la misma
// conexión como `{"type":"cursor","cursor":…}`, y del servicio llega el roster y el cursor
// de cada par ya leído; el de una posición que no encaja no llega.
func TestElCanalMandaElCursorYEntregaElDeLosDemas(t *testing.T) {
	recibido := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		var hola, cursor map[string]any
		if wsjson.Read(r.Context(), c, &hola) != nil || wsjson.Read(r.Context(), c, &cursor) != nil {
			return
		}
		recibido <- cursor
		for _, m := range []string{
			`{"type":"presence","presence":[{"sub":"yo"},{"sub":"u-1"}]}`,
			`{"type":"presence","cursor":{"sub":"u-1","name":"Ada","cursor":{"kind":"doc","anchor":"x","focus":0}}}`,
			`{"type":"presence","cursor":{"sub":"u-1","name":"Ada","cursor":{"kind":"doc","anchor":4,"focus":9,"anchorDot":{"a":"ada","c":3},"focusDot":{"a":"ada","c":8}}}}`,
		} {
			if c.Write(r.Context(), websocket.MessageText, []byte(m)) != nil {
				return
			}
		}
		_ = c.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, srv.Client())
	var llegados []DocMensaje
	err := d.CoeditarConCursor(context.Background(), "d1", 0, func(canal *CanalDoc) {
		mio := Cursor{Kind: "doc", Anchor: 2, Focus: 5, Puntos: &coedicion.Puntos{FocusDot: coedicion.Punto{A: "yo", C: 7}}}
		if err := canal.Enviar(context.Background(), mio); err != nil {
			t.Errorf("enviar: %v", err)
		}
	}, func(m DocMensaje) { llegados = append(llegados, m) })
	if err == nil {
		t.Fatal("el canal se cerró sin decirlo")
	}
	enviado, _ := json.Marshal(<-recibido)
	quiero := `{"type":"cursor","cursor":{"kind":"doc","anchor":2,"focus":5,"anchorDot":{"a":"","c":0},"focusDot":{"a":"yo","c":7}}}`
	if !mismoJSON(t, enviado, []byte(quiero)) {
		t.Errorf("enviado %s, quiero %s", enviado, quiero)
	}
	if len(llegados) != 3 {
		t.Fatalf("llegaron %d mensajes", len(llegados))
	}
	if r := llegados[0].Presence; len(r) != 2 || r[1].Sub != "u-1" || r[1].Cursor != nil {
		t.Errorf("roster %+v", r)
	}
	if llegados[1].Cursor != nil {
		t.Errorf("una posición inválida llegó: %+v", llegados[1].Cursor)
	}
	p := llegados[2].Cursor
	if p == nil || p.Sub != "u-1" || p.Name != "Ada" || p.Color == "" || p.Cursor == nil {
		t.Fatalf("cursor del par: %+v", p)
	}
	if c := p.Cursor; c.Kind != "doc" || c.Anchor != 4 || c.Focus != 9 || c.Puntos == nil || c.Puntos.FocusDot != (coedicion.Punto{A: "ada", C: 8}) {
		t.Errorf("posición %+v", c)
	}
}
