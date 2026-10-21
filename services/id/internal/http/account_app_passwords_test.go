// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// La contraseña se enseña con guiones y se teclea de las dos formas: quien la
// copia de la pantalla se los lleva y quien la escribe a mano, no. Si las dos no
// normalizan igual, la mitad de la gente se queda fuera con la credencial buena.
func TestNormalizeAppPassword(t *testing.T) {
	claro, err := generarAppPassword()
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	if len(claro) != 24 || strings.Count(claro, "-") != 4 {
		t.Fatalf("formato inesperado: %q", claro)
	}
	sinGuiones := strings.ReplaceAll(claro, "-", "")
	for _, v := range []string{claro, sinGuiones, " " + strings.ToUpper(claro) + " ", strings.ReplaceAll(claro, "-", " ")} {
		if got := NormalizeAppPassword(v); got != sinGuiones {
			t.Fatalf("NormalizeAppPassword(%q) = %q, quería %q", v, got, sinGuiones)
		}
	}
	// El alfabeto excluye a propósito los caracteres que se confunden al copiar
	// de una pantalla a un móvil (l/1, o/0).
	for _, c := range sinGuiones {
		if !strings.ContainsRune(alfabetoAppPassword, c) {
			t.Fatalf("carácter fuera del alfabeto: %q en %q", c, claro)
		}
	}
	// Dos llamadas seguidas no pueden coincidir: son 20 caracteres aleatorios.
	otra, _ := generarAppPassword()
	if otra == claro {
		t.Fatal("dos contraseñas idénticas")
	}
}

// La verja del endpoint que verifica las contraseñas para Sytue Mail: sin el
// secreto compartido no se pasa. Con secreto vacío rechaza SIEMPRE (la ruta ni
// se monta, pero la defensa en profundidad no cuesta nada).
func TestInternalSecretAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	casos := []struct {
		nombre, secreto, cabecera string
		quiere                    int
	}{
		{"correcto", "s3cr3t", "Bearer s3cr3t", http.StatusNoContent},
		{"sin cabecera", "s3cr3t", "", http.StatusUnauthorized},
		{"otro secreto", "s3cr3t", "Bearer nope", http.StatusUnauthorized},
		{"sin prefijo", "s3cr3t", "s3cr3t", http.StatusUnauthorized},
		{"servidor sin secreto", "", "Bearer ", http.StatusUnauthorized},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := &Server{cfg: Config{InternalSecret: c.secreto}}
			req := httptest.NewRequest(http.MethodPost, "/internal/app-passwords/verify", http.NoBody)
			if c.cabecera != "" {
				req.Header.Set("Authorization", c.cabecera)
			}
			w := httptest.NewRecorder()
			s.internalSecretAuth(ok).ServeHTTP(w, req)
			if w.Code != c.quiere {
				t.Fatalf("status = %d, quería %d", w.Code, c.quiere)
			}
		})
	}
}
