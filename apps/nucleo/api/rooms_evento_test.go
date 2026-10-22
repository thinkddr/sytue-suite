// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaSalaDeUnEventoSeCreaPorSuUID: el cuerpo que manda el BFF de la web, y el enlace
// con `?rooms=` sobre la raíz pública.
func TestLaSalaDeUnEventoSeCreaPorSuUID(t *testing.T) {
	var cuerpo map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /api/v1/rooms/from-source" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		datos, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(datos, &cuerpo)
		_, _ = io.WriteString(w, `{"id":"sala 1"}`)
	}))
	defer srv.Close()
	enlace, err := NewRooms(srv.URL, nil, nil).SalaDeEvento(context.Background(), "u1@calendar.sytue.eu", "", "https://rooms.sytue.eu/")
	if err != nil || enlace != "https://rooms.sytue.eu/?rooms=sala+1" {
		t.Fatalf("enlace %q, %v", enlace, err)
	}
	if cuerpo["kind"] != "calendar" || cuerpo["ref_id"] != "u1@calendar.sytue.eu" || cuerpo["title"] != "Reunión" {
		t.Errorf("cuerpo %v", cuerpo)
	}
	if bff := webref.Read(t, "apps/frontend/apps/calendar/src/pages/api/conference.ts"); !strings.Contains(bff, "/api/v1/rooms/from-source") ||
		!strings.Contains(bff, "kind: 'calendar', ref_id: uid, title") || !strings.Contains(bff, "enlaceReunion(room.id") {
		t.Error("el BFF de Calendar cambió cómo crea la sala: revisar SalaDeEvento")
	}
}

// El enlace de una reunión llega a la app de Rooms por el esquema (lo que deja
// DesdeLaWeb al abrirla desde la página) y por https (App Links / Universal Links), en la
// raíz o con el prefijo, y con el parámetro antiguo. Lo demás no: otro host, otra ruta o
// algo que no es un id.
//
// VISTO EN ROJO (2026-09-25, en el frontal): sin comprobar el host, el enlace de
// `rooms.evil.eu` entraba en la reunión.
func TestLeerReunion(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"
	pagina := "https://rooms.sytue.eu/?rooms=" + id
	desdeWeb, ok := DesdeLaWeb("sytue.eu", mustURL(t, "sytue-rooms://open?url="+url.QueryEscape(pagina)))
	if !ok {
		t.Fatal("DesdeLaWeb no traduce la página de la reunión")
	}
	for _, c := range []struct {
		u  string
		ok bool
	}{
		{desdeWeb.String(), true},
		{pagina, true},
		{"https://rooms.sytue.eu/rooms/?rooms=" + id, true},
		{"https://ROOMS.sytue.eu/?meet=" + id, true},
		{"sytue-rooms:/rooms?rooms=" + id, true},
		{"https://rooms.evil.eu/?rooms=" + id, false},
		{"https://mail.sytue.eu/?rooms=" + id, false},
		{"https://rooms.sytue.eu/channels/x?rooms=" + id, false},
		{"https://rooms.sytue.eu/?rooms=../../admin", false},
		{"sytue-mail:/?rooms=" + id, false},
	} {
		got, ok := LeerReunion("sytue.eu", mustURL(t, c.u))
		if ok != c.ok || (ok && got != id) {
			t.Errorf("LeerReunion(%s) = %q, %v; quiero %v", c.u, got, ok, c.ok)
		}
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
