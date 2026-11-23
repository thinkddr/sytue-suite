// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"testing"
)

// La huella del objeto `protect` es de contenido, no de la contraseña: dos objetos
// que solo difieren en `password`/`hasPassword` dan la misma huella, y por tanto el
// SHA-256 de huellaJSON nunca ve la contraseña. Si alguien dejara de quitarla, este
// test fallaría (y el SHA-256 sin sal pasaría a ser un hash débil de contraseña).
func TestHuellaProtectNoDependeDeLaContrasena(t *testing.T) {
	base := map[string]json.RawMessage{"slides": json.RawMessage(`["s1"]`), "master": json.RawMessage(`true`)}
	conClave := map[string]json.RawMessage{
		"slides": json.RawMessage(`["s1"]`), "master": json.RawMessage(`true`),
		"password": json.RawMessage(`"s3creto"`), "hasPassword": json.RawMessage(`true`),
	}
	otraClave := map[string]json.RawMessage{
		"slides": json.RawMessage(`["s1"]`), "master": json.RawMessage(`true`),
		"password": json.RawMessage(`"otra"`),
	}
	h := huellaProtect(base)
	if huellaProtect(conClave) != h || huellaProtect(otraClave) != h {
		t.Fatal("la huella de `protect` depende de la contraseña")
	}
	cambiado := map[string]json.RawMessage{"slides": json.RawMessage(`["s2"]`), "master": json.RawMessage(`true`)}
	if huellaProtect(cambiado) == h {
		t.Fatal("la huella no distingue un cambio de lo bloqueado")
	}
}
