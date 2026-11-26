// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// El cliente de Slack: las tres formas de llamar (formulario, GET, JSON), `ok: false`
// como ErrSlack con su código, y sin token (la pasarela) ninguna cabecera de
// autorización.
func TestSlackLlamaDeLasTresFormas(t *testing.T) {
	var vistos []string
	h := doerFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		vistos = append(vistos, r.Method+" "+r.URL.String()+" "+r.Header.Get("Content-Type")+" "+r.Header.Get("Authorization")+" "+string(b))
		if strings.Contains(r.URL.Path, "chat.delete") {
			return respuesta(200, `{"ok":false,"error":"cant_delete_message"}`), nil
		}
		return respuesta(200, `{"ok":true,"channel":{"id":"C1"}}`), nil
	})
	s := &Slack{Base: "https://slack.ejemplo.eu/api", Token: TokenFijo("xoxp-1"), HTTP: h}
	var r struct {
		Channel CanalSlack `json:"channel"`
	}
	if err := s.Llamar(t.Context(), "conversations.info", url.Values{"channel": {"C1"}}, &r); err != nil || r.Channel.ID != "C1" {
		t.Fatal(r, err)
	}
	if err := s.Consultar(t.Context(), "conversations.list", url.Values{"limit": {"1"}}, nil); err != nil {
		t.Fatal(err)
	}
	sin := &Slack{Base: "https://slack.ejemplo.eu/api", HTTP: h}
	var e *ErrSlack
	if err := sin.Publicar(t.Context(), "chat.delete", map[string]string{"ts": "1.2"}, nil); !errors.As(err, &e) || e.Codigo != "cant_delete_message" || !strings.Contains(err.Error(), "cant_delete_message") {
		t.Fatal(err)
	}
	if err := s.Llamar(t.Context(), "../auth.revoke", nil, nil); !errors.Is(err, ErrEntrada) {
		t.Fatal(err)
	}
	want := []string{
		"POST https://slack.ejemplo.eu/api/conversations.info application/x-www-form-urlencoded Bearer xoxp-1 channel=C1",
		"GET https://slack.ejemplo.eu/api/conversations.list?limit=1  Bearer xoxp-1 ",
		`POST https://slack.ejemplo.eu/api/chat.delete application/json  {"ts":"1.2"}`,
	}
	if strings.Join(vistos, "\n") != strings.Join(want, "\n") {
		t.Fatalf("peticiones:\n%s", strings.Join(vistos, "\n"))
	}
	if h := HoraSlack("1700000000.000200"); !h.Equal(time.Unix(1700000000, 200000)) || h.Location() != time.UTC {
		t.Fatal(h)
	}
}

// Una página siguiente con el puerto por defecto escrito sigue siendo del mismo origen;
// otro puerto u otra máquina, no.
func TestMismoOrigenConElPuertoPorDefecto(t *testing.T) {
	for enlace, ok := range map[string]bool{
		"https://graph.ejemplo.eu:443/v1.0/me/chats?$skiptoken=x": true,
		"https://graph.ejemplo.eu/v1.0/me/chats":                  true,
		"https://graph.ejemplo.eu:8443/v1.0/me/chats":             false,
		"https://otro.ejemplo.eu/v1.0/me/chats":                   false,
		"https://graph.ejemplo.eu/v2.0/me":                        false,
	} {
		if MismoOrigen("https://graph.ejemplo.eu/v1.0", enlace) != ok {
			t.Error(enlace)
		}
	}
	if _, err := destino("https://graph.ejemplo.eu/v1.0", "https://otro.ejemplo.eu/v1.0/me"); !errors.Is(err, ErrEntrada) {
		t.Fatal("Pedir con una dirección de fuera:", err)
	}
}

// Los nombres de los ficheros de OneDrive, las personas de Teams y de Google Chat que se
// ponen en una ruta, y el id de un fichero en un mensaje de Teams.
func TestNombresYPersonasQueVanEnUnaRuta(t *testing.T) {
	if n := NombreOneDrive(" ../plan: 2027?.pdf. "); n != "_plan_ 2027_.pdf" {
		t.Fatal(n)
	}
	for p, ok := range map[string]bool{"ana@empresa.eu": true, "ana@empresa.eu')": false, "x/y@z": false} {
		if PersonaTeams.MatchString(p) != ok {
			t.Error(p)
		}
	}
	for p, ok := range map[string]bool{"users/123": true, "users/ana@empresa.eu": true, "users/../spaces": false, "users/a b@c": false} {
		if UsuarioChat.MatchString(p) != ok {
			t.Error(p)
		}
	}
	e := ElementoDrive{ETag: `"{8A1B2C3D-4E5F-4A6B-9C8D-7E6F5A4B3C2D},3"`}
	if e.IDAdjunto() != "8a1b2c3d-4e5f-4a6b-9c8d-7e6f5a4b3c2d" || (ElementoDrive{ETag: "x"}).IDAdjunto() != "" {
		t.Fatal(e.IDAdjunto())
	}
	if IDCompartido("https://a.sharepoint.com/x?y") != "u!aHR0cHM6Ly9hLnNoYXJlcG9pbnQuY29tL3g_eQ" {
		t.Fatal(IDCompartido("https://a.sharepoint.com/x?y"))
	}
	m := MensajeGraph{}
	m.Body.Content = `<img src="https://graph/v1.0/chats/c/messages/1/hostedContents/aWQ9MQ==/$value"><img src="x/hostedContents/aWQ9MQ==/$value">`
	if a := m.Alojados(); len(a) != 1 || a[0] != "aWQ9MQ==" {
		t.Fatal(a)
	}
}
