// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cierre

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

func guardadas(t *testing.T, sesiones map[string]Sesion) ([]string, func(string) ([]byte, error)) {
	t.Helper()
	crudo := map[string][]byte{}
	for cuenta, s := range sesiones {
		datos, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		crudo[cuenta] = datos
	}
	crudo["rooms-matrix-session-roto"] = []byte("no es json")
	cuentas := make([]string, 0, len(crudo))
	for cuenta := range crudo {
		cuentas = append(cuentas, cuenta)
	}
	slices.Sort(cuentas)
	return cuentas, func(cuenta string) ([]byte, error) { return slices.Clone(crudo[cuenta]), nil }
}

// TestDeSujetoCierraUnaVezLosDispositivosDeEsaPersona: cada dispositivo de quien sale se
// cierra una vez y con su propio token; el de otra persona que comparte el llavero, el de
// un homeserver sin TLS y el que no tiene token no reciben nada.
func TestDeSujetoCierraUnaVezLosDispositivosDeEsaPersona(t *testing.T) {
	var mu sync.Mutex
	var vistos []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_matrix/client/v3/logout" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		vistos = append(vistos, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cuentas, leer := guardadas(t, map[string]Sesion{
		"rooms-matrix-session-mail":  {Homeserver: srv.URL, DeviceID: "MAIL", AccessToken: "token-mail", Subject: "ana"},
		"rooms-matrix-session-hub":   {Homeserver: srv.URL, DeviceID: "HUB", AccessToken: "token-hub", Subject: "ana"},
		"rooms-matrix-session-hub2":  {Homeserver: srv.URL, DeviceID: "HUB", AccessToken: "token-hub", Subject: "ana"},
		"rooms-matrix-session-bea":   {Homeserver: srv.URL, DeviceID: "BEA", AccessToken: "token-bea", Subject: "bea"},
		"rooms-matrix-session-plano": {Homeserver: "http://" + srv.Listener.Addr().String(), DeviceID: "PLANO", AccessToken: "token-plano", Subject: "ana"},
		"rooms-matrix-session-vacio": {Homeserver: srv.URL, DeviceID: "VACIO", Subject: "ana"},
	})
	total, cerrados := DeSujeto(t.Context(), srv.Client(), "ana", cuentas, leer)
	if total != 2 || cerrados != 2 {
		t.Errorf("total=%d cerrados=%d; quiero 2 y 2", total, cerrados)
	}
	slices.Sort(vistos)
	if !slices.Equal(vistos, []string{"Bearer token-hub", "Bearer token-mail"}) {
		t.Errorf("al homeserver han llegado %v", vistos)
	}
	if total, _ := DeSujeto(t.Context(), srv.Client(), "", cuentas, leer); total != 0 {
		t.Error("sin sujeto se cerraron dispositivos de alguien", total)
	}
}

// TestDeSujetoNoEsperaAUnHomeserverCaido: apagado o colgado, el cierre acaba dentro del
// plazo del contexto y dice que no se confirmó.
func TestDeSujetoNoEsperaAUnHomeserverCaido(t *testing.T) {
	colgado := make(chan struct{})
	lento := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-colgado }))
	defer lento.Close()
	defer close(colgado)
	apagado := httptest.NewTLSServer(http.NotFoundHandler())
	apagadoURL := apagado.URL
	apagado.Close()
	cuentas, leer := guardadas(t, map[string]Sesion{
		"rooms-matrix-session-lento":   {Homeserver: lento.URL, DeviceID: "LENTO", AccessToken: "t1", Subject: "ana"},
		"rooms-matrix-session-apagado": {Homeserver: apagadoURL, DeviceID: "APAGADO", AccessToken: "t2", Subject: "ana"},
	})
	ctx, parar := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer parar()
	inicio := time.Now()
	total, cerrados := DeSujeto(ctx, lento.Client(), "ana", cuentas, leer)
	if total != 2 || cerrados != 0 {
		t.Errorf("total=%d cerrados=%d; quiero 2 y 0", total, cerrados)
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Errorf("el cierre esperó %v a un homeserver caído", d)
	}
}

// TestDispositivoSoloAceptaUnReciboCompleto: la misma regla que el logout de Rooms.
func TestDispositivoSoloAceptaUnReciboCompleto(t *testing.T) {
	for _, tc := range []struct {
		nombre string
		status int
		cuerpo string
		hecho  bool
	}{
		{"cerrado", http.StatusOK, `{}`, true},
		{"ya_cerrado", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN"}`, true},
		{"cerrado_explicito", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":false}`, true},
		{"caducado", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":true}`, false},
		{"soft_no_booleano", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":"false"}`, false},
		{"prohibido", http.StatusForbidden, `{"errcode":"M_FORBIDDEN"}`, false},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.cuerpo))
			}))
			defer srv.Close()
			err := Dispositivo(t.Context(), srv.Client(), srv.URL, "t")
			if (err == nil) != tc.hecho || (err != nil && !errors.Is(err, ErrSinRecibo)) {
				t.Fatal("recibo mal interpretado", err)
			}
		})
	}
}
