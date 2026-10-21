// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLaProteccionPrecedeLosDatosYConservaElDestinoSiFalla(t *testing.T) {
	for _, fallar := range []bool{false, true} {
		t.Run(map[bool]string{false: "protección aplicada", true: "dispositivo bloqueado"}[fallar], func(t *testing.T) {
			dir := t.TempDir()
			destino := filepath.Join(dir, "snapshot.json")
			if err := os.WriteFile(destino, []byte("anterior"), permisoFichero); err != nil {
				t.Fatal(err)
			}
			bloqueado := errors.New("el sistema no permite proteger el archivo")
			llamadas := 0
			err := escribirProtegido(destino, []byte("nuevo privado"), false, func(temporal string) error {
				llamadas++
				datos, err := os.ReadFile(temporal)
				if err != nil || len(datos) != 0 {
					t.Fatal("se han escrito datos antes de aplicar la protección")
				}
				if fallar {
					return bloqueado
				}
				return nil
			})
			if llamadas != 1 || (fallar && !errors.Is(err, bloqueado)) || (!fallar && err != nil) {
				t.Fatalf("no se respeta el resultado del sistema: llamadas=%d error=%v", llamadas, err)
			}
			esperado := "nuevo privado"
			if fallar {
				esperado = "anterior"
			}
			datos, err := os.ReadFile(destino)
			if err != nil || string(datos) != esperado {
				t.Fatal("se publica sin protección o se pierde el archivo anterior")
			}
			ficheros, err := os.ReadDir(dir)
			if err != nil || len(ficheros) != 1 || ficheros[0].Name() != "snapshot.json" {
				t.Fatal("queda un temporal tras terminar")
			}
		})
	}
}
