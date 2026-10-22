// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// La reconstrucción va por su ruta, con su fecha y su clave, y lo que cada fuente saca
// de una organización se avisa en ESA organización aunque la fuente dijera otra.
func TestReconstruirOrganizaciones(t *testing.T) {
	var llegados []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notifications/historic" || r.Header.Get("Authorization") != "Bearer s" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		llegados = append(llegados, v)
		if v["key"] == "roto" {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"result":"creado"}`))
	}))
	defer srv.Close()

	fecha := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)
	fuente := func(context.Context) ([]Historico, error) {
		return []Historico{
			{Notification: Notification{Tenant: "globex", Subject: "ana", Source: "mail", Type: "mail.message.received", Title: "x"}, Clave: "mail:1", Fecha: fecha},
			{Notification: Notification{Subject: "ana", Source: "mail", Type: "mail.message.received", Title: "x"}, Clave: "roto", Fecha: fecha},
			// Fuera de la ventana: no se manda.
			{Notification: Notification{Subject: "ana", Source: "mail", Type: "mail.message.received", Title: "x"}, Clave: "viejo", Fecha: fecha.Add(-VentanaHistorica)},
		}, nil
	}
	var out bytes.Buffer
	err := ReconstruirOrganizaciones(context.Background(), New(srv.URL, "s"), &out, []string{"NO VÁLIDA", "acme"}, fuente)
	if err == nil || !strings.Contains(err.Error(), "1 de 2") || !strings.Contains(err.Error(), "NO VÁLIDA") {
		t.Fatalf("un fallo tiene que acabar en error y decir cuántos: %v", err)
	}
	if got := out.String(); got != "== acme: 2 avisos\nmail.message.received creado 1\nmail.message.received error 1\n" {
		t.Fatalf("recuento: %q", got)
	}
	if len(llegados) != 2 || llegados[0]["tenant"] != "acme" || llegados[0]["key"] != "mail:1" ||
		llegados[0]["created_at"] != fecha.Format(time.RFC3339) || llegados[0]["level"] != "info" {
		t.Fatalf("lo que llega al Hub: %+v", llegados)
	}

	if err := ReconstruirOrganizaciones(context.Background(), nil, &out, []string{"acme"}, fuente); err == nil {
		t.Fatal("sin Hub no puede darse por reconstruido")
	}
	if err := ReconstruirOrganizaciones(context.Background(), New(srv.URL, "s"), &out, nil, fuente); err == nil {
		t.Fatal("sin organizaciones no hay nada que hacer, y tiene que decirlo")
	}
}

// Un título más largo de lo que el Hub admite se recorta en vez de perder el aviso.
func TestRecortarTitulo(t *testing.T) {
	largo := strings.Repeat("ñ", 250)
	got := RecortarTitulo(largo)
	if n := len([]rune(got)); n != MaxTituloRunas || !strings.HasSuffix(got, "…") {
		t.Fatalf("recortado a %d runas: %q", n, got)
	}
	if RecortarTitulo("corto") != "corto" {
		t.Fatal("un título corto no se toca")
	}
}
