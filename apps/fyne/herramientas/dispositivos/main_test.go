// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// TestCierraCadaDispositivoDeLaTiradaUnaVez: las sesiones de Matrix que dejó la tirada en
// su llavero (y un cierre pendiente del mismo dispositivo) se cierran una vez cada una, con
// su propio token; lo que no es una sesión de ese homeserver no recibe nada.
//
// VISTO EN ROJO sin comprobar el homeserver: el token de «otro» salía hacia el servidor.
func TestCierraCadaDispositivoDeLaTiradaUnaVez(t *testing.T) {
	keyring.MockInit()
	var mu sync.Mutex
	var vistos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost || r.URL.Path != "/_matrix/client/v3/logout" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		vistos = append(vistos, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer token-caducado" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	guardar := func(cuenta string, v any) {
		datos, _ := json.Marshal(v)
		if err := auth.GuardarSecreto(cuenta, datos); err != nil {
			t.Fatal(err)
		}
	}
	guardar("rooms-matrix-session-a", sesion{Homeserver: srv.URL, DeviceID: "SYTUE_A", AccessToken: "token-a"})
	guardar("matrix-logout-a", map[string]any{"version": 1, "account": "a", "session": sesion{Homeserver: srv.URL, DeviceID: "SYTUE_A", AccessToken: "token-a"}})
	guardar("rooms-matrix-session-b", sesion{Homeserver: srv.URL, DeviceID: "SYTUE_B", AccessToken: "token-caducado"})
	guardar("rooms-matrix-session-otro", sesion{Homeserver: "https://matrix.foreign.test", DeviceID: "SYTUE_C", AccessToken: "token-otro"})
	guardar("rooms-matrix-master-a", map[string]string{"clave": "no-es-una-sesion"})

	var cuentas []string
	for _, p := range prefijos {
		c, err := auth.CuentasCon(p)
		if err != nil {
			t.Fatal(err)
		}
		cuentas = append(cuentas, c...)
	}
	total, cerrados := cerrar(context.Background(), cuentas, auth.LeerSecreto, srv.Client(), srv.URL)
	if total != 2 || cerrados != 2 {
		t.Errorf("total=%d cerrados=%d; quiero 2 y 2", total, cerrados)
	}
	slices.Sort(vistos)
	if !slices.Equal(vistos, []string{"Bearer token-a", "Bearer token-caducado"}) {
		t.Errorf("al homeserver han llegado %v", vistos)
	}
}
