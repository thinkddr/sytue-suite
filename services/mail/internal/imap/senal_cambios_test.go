// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// Lo que Thunderbird marca o mueve por IMAP manda la señal de cambios del widget de
// Mail a la dueña de la sesión, y una orden que no tocó ningún mensaje no la manda.
// VISTO EN ROJO sin la comprobación de `n` (avisaba con cero). Sin Hub (nil) no hace
// nada: lo cubre el nil-safe de AvisarCambio.
func TestLoQueCambiaPorIMAPMandaLaSenal(t *testing.T) {
	llega := make(chan string, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Tenant, Subject, Event string
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		llega <- e.Event + " " + e.Tenant + " " + e.Subject
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	s := &session{be: &backend{hub: hubnotify.New(hub.URL, "s")}, tenant: "acme", subject: "ana"}

	cero := 0
	s.cambio(&cero)
	select {
	case e := <-llega:
		t.Fatalf("sin mensajes tocados llega %q", e)
	case <-time.After(200 * time.Millisecond):
	}
	dos := 2
	s.cambio(&dos)
	select {
	case e := <-llega:
		if e != "datos acme ana" {
			t.Fatalf("llega %q, quiero «datos acme ana»", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la señal de IMAP no llega al Hub")
	}
}
