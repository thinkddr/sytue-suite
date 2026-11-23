// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Prueba que el informe que se publica se puede LEER.
//
// POR QUÉ. El cuerpo iba en base64 y las incidencias eran un churro que había que
// decodificar a mano para saber qué había fallado. Lo que se comprueba aquí es
// justo eso: que lo que sale por el cable es JSON válido, que el log va en claro y
// que no lleva los códigos de color del terminal.
//
// Con un `curl` de mentira delante, que en vez de publicar guarda lo que le pasan.
func TestElInformeQueSePublicaSePuedeLeer(t *testing.T) {
	tmp := t.TempDir()

	// Un log como los de verdad: con colores, comillas y una barra invertida.
	log := filepath.Join(tmp, "log.txt")
	guiontest.Escribir(t, log, "compilando…\n"+
		"\033[93mwarning\033[0m: algo\n"+
		`error TS2741: falta "erp" en C:\ruta`+"\n")

	// Guarda el JSON que le pasan por --data @fichero y dice que fue bien.
	guiontest.Doble(t, tmp, "curl", `for i in $(seq 1 $#); do
  if [ "${!i}" = "--data" ]; then j=$((i+1)); cp "${!j#@}" "$SALIDA"; fi
done
exit 0`)

	enviado := filepath.Join(tmp, "enviado.json")
	guiontest.Guion(t, guiontest.Opciones{Env: []string{
		"SALIDA=" + enviado, guiontest.ConPATH(tmp),
		"GITHUB_TOKEN=x", "GITHUB_API_URL=https://api.ejemplo.test",
		"GITHUB_REPOSITORY=org/repo", "GITHUB_WORKFLOW=ci", "GITHUB_REF_NAME=main",
		"GITHUB_SHA=abcdef1234567890", "GITHUB_RUN_ID=7", "GITHUB_SERVER_URL=https://ejemplo.test",
	}}, ".github/actions/informe/accion.sh", log, "fallo quality · main · abcdef1")

	crudo, err := os.ReadFile(enviado) //nolint:gosec // G304: fichero de la propia prueba, en su directorio temporal.
	if err != nil || len(crudo) == 0 {
		t.Fatal("no se envió nada")
	}

	var informe struct {
		Titulo string `json:"title"`
		Cuerpo string `json:"body"`
	}
	t.Run("lo que se envía es JSON válido", func(t *testing.T) {
		if err := json.Unmarshal(crudo, &informe); err != nil {
			t.Fatalf("lo enviado no es JSON: %v\n%s", err, crudo)
		}
	})
	t.Run("el log va EN CLARO, no en base64", func(t *testing.T) {
		if !strings.Contains(informe.Cuerpo, "error TS2741") {
			t.Fatalf("el cuerpo no trae el error en claro:\n%s", informe.Cuerpo)
		}
	})
	t.Run("no quedan códigos de color", func(t *testing.T) {
		if strings.Contains(informe.Cuerpo, "\033") {
			t.Fatalf("el cuerpo lleva secuencias de escape:\n%q", informe.Cuerpo)
		}
	})
	t.Run("las comillas y la barra sobreviven", func(t *testing.T) {
		if !strings.Contains(informe.Cuerpo, `falta "erp" en C:\ruta`) {
			t.Fatalf("las comillas o la barra invertida no llegan como estaban:\n%s", informe.Cuerpo)
		}
	})
	t.Run("dice dónde falló", func(t *testing.T) {
		if !strings.Contains(informe.Cuerpo, "ejemplo.test/org/repo/actions/runs/7") {
			t.Fatalf("el cuerpo no enlaza la ejecución:\n%s", informe.Cuerpo)
		}
	})
	t.Run("el título es el que se pidió", func(t *testing.T) {
		if informe.Titulo != "fallo quality · main · abcdef1" {
			t.Fatalf("esperaba: %s\nobtuvo:   %s", "fallo quality · main · abcdef1", informe.Titulo)
		}
	})
}
