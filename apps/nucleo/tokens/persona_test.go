// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import "testing"

// El hash de `peerColor`: «Ana» es el cielo en la web, y un carácter fuera del plano
// básico cuenta como sus DOS unidades UTF-16 (U+1F600 es D83D DE00: 55357·31 + 56832 =
// 1772899, resto 1). Contado por runas daría 128512, resto 4, «avatar-5»: se vio en ROJO
// así.
func TestPersonaEsElHashDeLaWeb(t *testing.T) {
	for clave, quiero := range map[string]string{"Ana": "avatar-3", "\U0001F600": "avatar-2", "": "avatar-1"} {
		if got := Persona(clave); got != quiero {
			t.Errorf("Persona(%q) = %s, y la web dice %s", clave, got, quiero)
		}
		Color(Persona(clave), Light) // que el token exista
	}
}
