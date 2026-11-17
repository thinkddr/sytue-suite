// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Subir el `.p12` tiene que contestar con un código que la interfaz sepa
// explicar ANTES de tocar la base de datos: la contraseña equivocada no es lo
// mismo que un fichero que no es un certificado, y ninguno de los dos llega al
// store (los manejadores van sin él a propósito: si lo tocaran, reventaría).
func TestGuardarCertificadoVerifactuExplicaElFallo(t *testing.T) {
	p12, err := os.ReadFile("../verifactu/testdata/moderno.p12")
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		nombre, codigo string
		cuerpo         map[string]any
	}{
		{"contraseña equivocada", "certificado_contrasena", map[string]any{"fichero": p12, "contrasena": "otra"}},
		{"no es un certificado", "certificado_invalido", map[string]any{"fichero": []byte("hola"), "contrasena": "secreta"}},
		{"PEM sin clave", "certificado_invalido", map[string]any{"fichero": []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n")}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cuerpo, _ := json.Marshal(c.cuerpo)
			r := httptest.NewRequest(http.MethodPut, "/api/v1/verifactu/certificado", bytes.NewReader(cuerpo))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			(&Handlers{}).GuardarCertificadoVerifactu(w, r)
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), c.codigo) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secreta") || strings.Contains(w.Body.String(), "otra") {
				t.Fatalf("la respuesta lleva la contraseña: %s", w.Body.String())
			}
		})
	}
}
