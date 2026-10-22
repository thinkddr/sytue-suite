// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// Cambiar o quitar la foto avisa al Hub para esa persona con la versión NUEVA —la
// misma cadena que el ETag—, por la puerta de ESTADO y no por la de avisos: las
// barras abiertas la vuelven a pedir, y la campana no se entera. Y la foto sale
// con `no-cache`: con el `max-age=3600` de antes la barra de otra app seguía
// pintando la vieja al recargar durante una hora.
func TestCambiarLaFotoAvisaAlHubYNoSeQuedaEnCache(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "foto@id.test", "")
	tok := l.token(t, u.ID, "acme", nil)

	llegan := make(chan hubnotify.Evento, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e hubnotify.Evento
		if r.URL.Path != "/internal/events" || r.Header.Get("Authorization") != "Bearer s" ||
			json.NewDecoder(r.Body).Decode(&e) != nil {
			t.Errorf("al Hub le llega %s %s sin el secreto o sin cuerpo", r.Method, r.URL.Path)
		}
		llegan <- e
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	l.srv.hub = clienteHub(Config{HubBaseURL: hub.URL, HubInternalSecret: "s"})

	esperar := func() map[string]any {
		t.Helper()
		select {
		case e := <-llegan:
			if e.Tenant != "acme" || e.Subject != u.ID || e.Event != hubnotify.EventoAvatar {
				t.Fatalf("evento %+v, quiero avatar para acme/%s", e, u.ID)
			}
			d, _ := e.Data.(map[string]any)
			return d
		case <-time.After(5 * time.Second):
			t.Fatal("Sytue ID no avisó al Hub del cambio de foto")
			return nil
		}
	}

	const png = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/avatar", tok, map[string]string{"photo": png}); rr.Code != http.StatusOK {
		t.Fatalf("PUT avatar: %d %s", rr.Code, rr.Body)
	}
	d := esperar()
	rr, _ := l.do(t, http.MethodGet, "/api/v1/me/avatar", tok, nil)
	if etag := strings.Trim(rr.Header().Get("ETag"), `"`); etag == "" || d["version"] != etag {
		t.Fatalf("el evento lleva la versión %v y la foto sale con ETag %q", d["version"], etag)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Fatalf("Cache-Control de la foto = %q: el navegador la sirve sin preguntar y se ve la vieja", cc)
	}

	if rr, _ := l.do(t, http.MethodDelete, "/api/v1/me/avatar", tok, nil); rr.Code != http.StatusOK {
		t.Fatalf("DELETE avatar: %d", rr.Code)
	}
	if d := esperar(); d["version"] != "" {
		t.Fatalf("al quitar la foto el evento lleva la versión %v, quiero vacía", d["version"])
	}
}
