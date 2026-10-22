// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// TestFailRegistraLos4xx fija que un rechazo deja rastro con su código.
//
// El fallo que evita: hasta el 2026-08-12 solo se registraban los 5xx, así que
// un 400 se veía en los registros como «status=400» y nada más —igual un JSON
// mal formado que un tope de gasto superado—. Auditando producción aparecieron
// tres intentos de despliegue rechazados y no había forma de saber por cuál de
// las seis validaciones.
//
// Y comprueba lo que NO debe salir: el mensaje, que se compone con Params y
// puede llevar dentro lo que escribió el usuario.
func TestFailRegistraLos4xx(t *testing.T) {
	casos := []struct {
		nombre   string
		err      error
		registra bool
		nivel    string
	}{
		{"400 con código", &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "el valor SECRETO-DEL-USUARIO no vale"}, true, "INFO"},
		{"404", &Error{Status: http.StatusNotFound, Code: "not_found", Message: "no está"}, true, "INFO"},
		{"500 sigue en error", &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "roto"}, true, "ERROR"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := log.Into(t.Context(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
			r := httptest.NewRequest(http.MethodGet, "/x", http.NoBody).WithContext(ctx)

			Fail(httptest.NewRecorder(), r, c.err)

			salida := buf.String()
			if !c.registra {
				if salida != "" {
					t.Fatalf("no debería registrar nada, y registró: %s", salida)
				}
				return
			}
			if salida == "" {
				t.Fatal("no registró nada: un rechazo sin rastro es el fallo que este test evita")
			}
			var linea map[string]any
			if err := json.Unmarshal([]byte(strings.SplitN(salida, "\n", 2)[0]), &linea); err != nil {
				t.Fatalf("línea de log no parseable: %v (%s)", err, salida)
			}
			if got := linea["level"]; got != c.nivel {
				t.Errorf("nivel = %v, se esperaba %s", got, c.nivel)
			}
			var esperado *Error
			if !errors.As(c.err, &esperado) {
				t.Fatalf("el caso no lleva un *Error: %T", c.err)
			}
			if got := linea["code"]; got != esperado.Code {
				t.Errorf("falta el código: %v", got)
			}
			// Lo que importa de verdad: el mensaje del usuario NO viaja al log
			// en un 4xx. En el 5xx sí va la causa, que es interna.
			if c.nivel == "INFO" && strings.Contains(salida, "SECRETO-DEL-USUARIO") {
				t.Error("el mensaje llegó al registro: puede llevar datos del usuario")
			}
		})
	}
}
