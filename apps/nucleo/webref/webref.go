// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package webref le da a los tests los ficheros de la web contra los que se comparan
// los espejos escritos a mano de este módulo (tokens, keyframes, catálogos, reglas de
// caché).
//
// Existe por un falso verde: cada test hacía su propio `t.Skip` si el fichero no
// estaba, y una sincronización parcial del árbol los dejó a todos saltándose sin que
// nada lo dijera. Ahora solo se salta quien corre FUERA del monorepo; dentro, que
// falte el fichero es un fallo — o se movió y el espejo hay que revisarlo, o el árbol
// está a medias y el verde no vale.
package webref

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Read devuelve un fichero del repositorio, por su ruta desde la raíz.
func Read(t *testing.T, ruta string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for !esRaiz(dir) {
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Skip("fuera del monorepo: no hay web con la que comparar")
		}
		dir = padre
	}
	datos, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ruta)))
	if err != nil {
		t.Fatalf("%s: %v — si se ha movido, revisar el espejo que lo copia", ruta, err)
	}
	return string(datos)
}

// esRaiz reconoce la raíz del monorepo por lo que este módulo necesita de ella.
func esRaiz(dir string) bool {
	for _, hijo := range []string{"branding", "apps/nucleo/go.mod"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(hijo))); err != nil {
			return false
		}
	}
	return true
}

// SinControlEnLaWeb son los ids del registro común de comandos de la cinta que la web NO
// pinta para ese tipo de documento (`document`, `spreadsheet` o `presentation`), tal como
// los mide `test/e2e/paridad-cinta-web.mjs` y deja anotados `paridad-cinta-web.json`. Es la
// mitad web de la paridad: una brecha de la cinta nativa que la web también tiene no es
// una diferencia entre las dos, y una que la web no tiene sí lo es.
func SinControlEnLaWeb(t *testing.T, kind string) []string {
	t.Helper()
	var medido map[string][]string
	if err := json.Unmarshal([]byte(Read(t, "test/e2e/paridad-cinta-web.json")), &medido); err != nil {
		t.Fatalf("paridad-cinta-web.json: %v", err)
	}
	return medido[kind]
}
