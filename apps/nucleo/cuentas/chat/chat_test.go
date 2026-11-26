// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func respuesta(estado int, cuerpo string) *http.Response {
	return &http.Response{StatusCode: estado, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(cuerpo))}
}

// tokenQueSeRenueva da «viejo» hasta que se invalida y luego «nuevo».
type tokenQueSeRenueva struct {
	mu          sync.Mutex
	invalidado  bool
	invalidados int
}

func (t *tokenQueSeRenueva) Acceso(context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.invalidado {
		return "nuevo", nil
	}
	return "viejo", nil
}

func (t *tokenQueSeRenueva) Invalidar() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.invalidado = true
	t.invalidados++
}

// Un 401 tira el token y repite UNA vez; el error no lleva el cuerpo del proveedor.
func TestUn401RenuevaYRepiteUnaVez(t *testing.T) {
	tok := &tokenQueSeRenueva{}
	var vistos []string
	g := &Graph{Base: "https://graph.ejemplo.eu/v1.0", Token: tok, HTTP: doerFunc(func(r *http.Request) (*http.Response, error) {
		vistos = append(vistos, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer nuevo" {
			return respuesta(401, `{"error":"secreto del proveedor"}`), nil
		}
		return respuesta(200, `{"id":"11111111-1111-1111-1111-111111111111","displayName":"Ana"}`), nil
	})}
	u, err := g.Yo(t.Context())
	if err != nil || u.Nombre != "Ana" || strings.Join(vistos, ",") != "Bearer viejo,Bearer nuevo" || tok.invalidados != 1 {
		t.Fatal(u, err, vistos)
	}
	g.HTTP = doerFunc(func(*http.Request) (*http.Response, error) { return respuesta(403, `secreto del proveedor`), nil })
	_, err = g.Yo(t.Context())
	if Estado(err) != 403 || strings.Contains(err.Error(), "secreto") || tok.invalidados != 1 {
		t.Fatal(err, tok.invalidados)
	}
}

// Lo que viene de fuera no se convierte en otra ruta ni en otro servidor.
func TestEntradasQueNoSonDelProveedor(t *testing.T) {
	for _, mal := range [][2]string{{"chat/../../me/drive", ""}, {"chat/19:a@thread.v2", "hilo"}, {"no-es-guid/19:c", ""}, {"00000000-0000-0000-0000-000000000001/19:c", "../x"}} {
		if _, err := RutaMensajesTeams(mal[0], mal[1]); !errors.Is(err, ErrEntrada) {
			t.Errorf("%v: %v", mal, err)
		}
	}
	if r, err := RutaMensajesTeams("00000000-0000-0000-0000-000000000001/19:c@thread.tacv2", "123"); err != nil || r != "/teams/00000000-0000-0000-0000-000000000001/channels/19:c@thread.tacv2/messages/123/replies" {
		t.Fatal(r, err)
	}
	if _, _, err := EnvioGoogle("spaces/AAA", "spaces/OTRO/threads/x", "hola"); !errors.Is(err, ErrEntrada) {
		t.Fatal("un hilo de otro espacio:", err)
	}
	if ruta, cuerpo, err := EnvioGoogle("spaces/AAA", "spaces/AAA/threads/x", "hola"); err != nil || ruta != "spaces/AAA/messages?messageReplyOption=REPLY_MESSAGE_OR_FAIL" || cuerpo["text"] != "hola" {
		t.Fatal(ruta, cuerpo, err)
	}
	for _, c := range []struct {
		enlace string
		vale   bool
	}{
		{"https://graph.ejemplo.eu/v1.0/me/chats?$skiptoken=x", true},
		{"https://otro.ejemplo.eu/v1.0/me/chats?$skiptoken=x", false},
		{"http://graph.ejemplo.eu/v1.0/me/chats", false},
		{"https://graph.ejemplo.eu/beta/me", false},
		{"https://ana@graph.ejemplo.eu/v1.0/me", false},
	} {
		if MismoOrigen("https://graph.ejemplo.eu/v1.0", c.enlace) != c.vale {
			t.Errorf("%s", c.enlace)
		}
	}
	for _, u := range []string{"https://evil.example/abc", "http://meet.google.com/abc", "https://meet.google.com/"} {
		if _, err := EnlaceMeet(u); err == nil {
			t.Errorf("enlace de Meet aceptado: %s", u)
		}
	}
}
