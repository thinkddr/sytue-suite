// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package clave

import (
	"errors"
	"strings"
	"testing"
)

func TestHashYVerificar(t *testing.T) {
	h, err := Hash("correcta")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") || strings.Contains(h, "correcta") {
		t.Fatalf("formato inesperado: %s", h)
	}
	otro, _ := Hash("correcta")
	if otro == h {
		t.Error("dos hashes de la misma contraseña salieron iguales: falta la sal")
	}
	casos := []struct {
		nombre, pass, hash string
		ok, err            bool
	}{
		{"acierta", "correcta", h, true, false},
		{"falla", "otra", h, false, false},
		{"vacía", "", h, false, false},
		{"sin hash", "correcta", "", false, false},
		{"formato desconocido", "correcta", "$bcrypt$x", false, true},
		{"hash corrupto", "correcta", "$argon2id$v=19$m=65536,t=3,p=2$AAAA$", false, true},
	}
	for _, c := range casos {
		ok, err := Verificar(c.pass, c.hash)
		if ok != c.ok || (err != nil) != c.err {
			t.Errorf("%s: ok=%v err=%v", c.nombre, ok, err)
		}
	}
}

func TestHashRechazaLaVacia(t *testing.T) {
	if _, err := Hash(""); !errors.Is(err, ErrVacia) {
		t.Fatalf("err = %v", err)
	}
}
