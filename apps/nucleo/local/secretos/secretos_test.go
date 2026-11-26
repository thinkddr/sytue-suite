// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package secretos

import (
	"os"
	"testing"
)

func TestReservaEnFicheroCon0600(t *testing.T) {
	a := SoloFichero(t.TempDir())
	if v, err := a.Leer("ia"); err != nil || v != "" {
		t.Fatal(v, err)
	}
	if err := a.Guardar("ia", "sk-prueba"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Leer("ia"); v != "sk-prueba" {
		t.Fatal(v)
	}
	st, err := os.Stat(a.fichero("ia"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatal("permisos", st.Mode(), err)
	}
	if err := a.Guardar("ia", ""); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Leer("ia"); v != "" {
		t.Fatal("no se borró")
	}
}
