// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// vectoresRevision es `revisionCuenta.vectores.json`, el fichero que recorren la web y
// este espejo.
type vectoresRevision struct {
	Revision []struct {
		Nombre   string          `json:"nombre"`
		Datos    DatosRevision   `json:"datos"`
		Esperado []PuntoRevision `json:"esperado"`
	} `json:"revision"`
	PrimerosPasos []struct {
		Nombre   string             `json:"nombre"`
		Datos    DatosPrimerosPasos `json:"datos"`
		Esperado []PasoPendiente    `json:"esperado"`
	} `json:"primerosPasos"` //nolint:tagliatelle // el nombre lo fija el vector
}

// TestLaRevisionYLosPrimerosPasosSonLosDeLaWeb recorre los vectores de la web: la tarjeta
// del Hub y la de la portada tienen que decirle lo mismo a la misma persona.
//
// Visto en ROJO quitando la passkey como segundo factor (protegida = MFAEnabled): fallan
// los dos casos de «una passkey basta».
func TestLaRevisionYLosPrimerosPasosSonLosDeLaWeb(t *testing.T) {
	var v vectoresRevision
	if err := json.Unmarshal([]byte(webref.Read(t, "apps/frontend/packages/shell/src/revisionCuenta.vectores.json")), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Revision) == 0 || len(v.PrimerosPasos) == 0 {
		t.Fatal("los vectores han llegado vacíos: el test no probaría nada")
	}
	for _, c := range v.Revision {
		if got := RevisionDeCuenta(c.Datos); !reflect.DeepEqual(got, c.Esperado) {
			t.Errorf("revisión «%s»:\n  Go   %+v\n  web  %+v", c.Nombre, got, c.Esperado)
		}
	}
	for _, c := range v.PrimerosPasos {
		if got := PrimerosPasos(c.Datos); !reflect.DeepEqual(got, c.Esperado) {
			t.Errorf("primeros pasos «%s»:\n  Go   %+v\n  web  %+v", c.Nombre, got, c.Esperado)
		}
	}
}

// TestLasAnclasSonLasDeLaWeb: un ancla que no existe en Mi cuenta abre la página por
// arriba, sin llevar a ninguna parte.
func TestLasAnclasSonLasDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/shell/src/revisionCuenta.ts")
	for clave, ancla := range AnclasRevision {
		if !strings.Contains(fuente, string(clave)+": '"+ancla+"'") {
			t.Errorf("el ancla de %s es %q y la web no la declara así en ANCLAS", clave, ancla)
		}
	}
}

// TestLaCargaDeLosPrimerosPasos junta las tres respuestas como el BFF del portal: lo que
// falla no inventa pasos, y sin ninguna respuesta no hay tarjeta.
func TestLaCargaDeLosPrimerosPasos(t *testing.T) {
	casos := []struct {
		nombre             string
		me, llaves, contra int // código HTTP de cada ruta
		hay                bool
		claves             []ClavePaso
	}{
		{"cuenta nueva", 200, 200, 200, true, []ClavePaso{PasoDosPasos, PasoRecuperacion, PasoFoto, PasoDispositivos}},
		{"sin passkeys no se pide la verificación", 200, 500, 200, true, []ClavePaso{PasoRecuperacion, PasoFoto, PasoDispositivos}},
		{"sin contraseñas no se pide conectar", 200, 200, 500, true, []ClavePaso{PasoDosPasos, PasoRecuperacion, PasoFoto}},
		{"sin /me no hay tarjeta", 500, 200, 200, true, []ClavePaso{}},
		{"sin nada", 500, 500, 500, false, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			mux := http.NewServeMux()
			responder := func(codigo int, cuerpo string) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(codigo)
					if codigo == 200 {
						_, _ = w.Write([]byte(cuerpo))
					}
				}
			}
			mux.HandleFunc("/api/v1/me", responder(c.me, `{"id":"u-1","mfa_enabled":false,"has_avatar":false}`))
			mux.HandleFunc("/api/v1/webauthn/credentials", responder(c.llaves, `{"credentials":[]}`))
			mux.HandleFunc("/api/v1/me/app-passwords", responder(c.contra, `{"app_passwords":[]}`))
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			pasos, hay := NewAccount(srv.URL, nil, srv.Client()).PrimerosPasos(context.Background(), true)
			if hay != c.hay {
				t.Fatalf("hay = %v, y debía ser %v", hay, c.hay)
			}
			// nil y vacío no son lo mismo: sin respuesta no hay lista.
			var claves []ClavePaso
			if pasos != nil {
				claves = make([]ClavePaso, 0, len(pasos))
				for _, p := range pasos {
					claves = append(claves, p.Clave)
				}
			}
			if !reflect.DeepEqual(claves, c.claves) {
				t.Errorf("pasos %v, y debían ser %v", claves, c.claves)
			}
		})
	}
}
