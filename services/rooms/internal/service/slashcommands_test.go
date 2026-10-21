// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestComandoLlegaComoUnSlashCommandDeSlack: el cuerpo y la firma tienen que ser los que
// espera un comando ya escrito para Slack. Si `command` o `text` no vinieran donde toca,
// el bot no sabría ni qué le piden.
func TestComandoLlegaComoUnSlashCommandDeSlack(t *testing.T) {
	var vals url.Values
	var firma, ts, ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		vals, _ = url.ParseQuery(string(raw))
		ct = r.Header.Get("Content-Type")
		firma = r.Header.Get(compat.SlackSignatureHeader)
		ts = r.Header.Get(compat.SlackTimestampHeader)

		// Lo que hace un bot de Slack antes de mirar nada más.
		mac := hmac.New(sha256.New, []byte(testSecret))
		mac.Write([]byte("v0:" + ts + ":" + string(raw)))
		if want := "v0=" + hex.EncodeToString(mac.Sum(nil)); firma != want {
			t.Errorf("firma = %q, el receptor calcula %q", firma, want)
		}
		_, _ = w.Write([]byte(`{"text":"desplegando *prod*"}`))
	}))
	defer srv.Close()

	svc := &Service{outgoing: srv.Client()}
	cmd := domain.SlashCommand{
		Name: "desplegar", Tenant: "acme", RequestURL: srv.URL,
		Token: "verif", SigningSecret: testSecret,
	}
	canal := uuid.New()
	texto, tipo, err := svc.doSlash(context.Background(), cmd, canal, "u-1", "Ana", "prod --forzar")
	if err != nil {
		t.Fatalf("doSlash: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", ct)
	}
	for k, want := range map[string]string{
		"command": "/desplegar", "text": "prod --forzar", "token": "verif",
		"user_id": "u-1", "user_name": "Ana", "channel_id": canal.String(), "team_id": "acme",
	} {
		if got := vals.Get(k); got != want {
			t.Fatalf("%s = %q, quiero %q", k, got, want)
		}
	}
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil {
		t.Fatalf("sin timestamp utilizable: %q", ts)
	}
	// La respuesta llega ya traducida al Markdown de Rooms.
	if texto != "desplegando **prod**" {
		t.Fatalf("respuesta = %q", texto)
	}
	// Sin `response_type` declarado ⇒ efímera: solo la ve quien ejecutó.
	if tipo != domain.SlashResponseEphemeral {
		t.Fatalf("tipo = %q, quiero efímero por defecto", tipo)
	}
}

// TestComandoPublicoCuandoLoPide: `in_channel` es lo único que debe acabar publicado.
func TestComandoPublicoCuandoLoPide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response_type":"in_channel","text":"listo"}`))
	}))
	defer srv.Close()

	svc := &Service{outgoing: srv.Client()}
	cmd := domain.SlashCommand{Name: "x", RequestURL: srv.URL, SigningSecret: testSecret}
	_, tipo, err := svc.doSlash(context.Background(), cmd, uuid.New(), "u", "U", "")
	if err != nil {
		t.Fatalf("doSlash: %v", err)
	}
	if tipo != domain.SlashResponseInChannel {
		t.Fatalf("tipo = %q, quiero in_channel", tipo)
	}
}

// TestComandoSinCuerpoEsAceptacion: un 200 vacío significa «recibido, contesto luego».
// Tratarlo como error haría que el usuario viera un fallo donde no lo hay.
func TestComandoSinCuerpoEsAceptacion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &Service{outgoing: srv.Client()}
	cmd := domain.SlashCommand{Name: "x", RequestURL: srv.URL}
	texto, _, err := svc.doSlash(context.Background(), cmd, uuid.New(), "u", "U", "")
	if err != nil {
		t.Fatalf("un 200 vacío no es un error: %v", err)
	}
	if texto != "" {
		t.Fatalf("texto = %q, quiero vacío", texto)
	}
}

// TestComandoQueFallaSeDistingue: el 5xx del tercero tiene que llegar como ErrSlashFailed
// para que el handler responda 502 y no un 500 que mandaría a mirar los logs de Rooms.
func TestComandoQueFallaSeDistingue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := &Service{outgoing: srv.Client()}
	cmd := domain.SlashCommand{Name: "x", RequestURL: srv.URL}
	if _, _, err := svc.doSlash(context.Background(), cmd, uuid.New(), "u", "U", ""); err == nil {
		t.Fatal("un 500 del comando se dio por bueno")
	}
}

// TestComandoNoLlegaARedInterna: la URL la pone un admin, pero un admin también puede
// equivocarse —o ser engañado— y apuntarla al interior. La misma barrera que los
// webhooks salientes.
func TestComandoNoLlegaARedInterna(t *testing.T) {
	svc := &Service{}
	for _, u := range []string{
		"http://127.0.0.1:8085/x",
		"http://10.0.0.5/x",
		"http://[::1]/x",
		"http://169.254.169.254/latest/meta-data/",
		"file:///etc/passwd",
	} {
		cmd := domain.SlashCommand{Name: "x", RequestURL: u}
		if _, _, err := svc.postSlash(context.Background(), cmd, uuid.New(), "u", "U", ""); err == nil {
			t.Fatalf("se permitió llamar a %s", u)
		}
	}
}
