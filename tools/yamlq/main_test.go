// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func salida(t *testing.T, yaml string, soloRepetidas bool) (string, error) {
	t.Helper()
	// procesar escribe por la salida estándar; para el test basta con
	// comprobar el error y, cuando interesa el JSON, decodificar aparte.
	return capturar(t, func() error { return procesar([]byte(yaml), soloRepetidas) })
}

// UNA CLAVE REPETIDA TIENE QUE SALTAR. Es la razón de ser de esta herramienta:
// en YAML la segunda pisa a la primera EN SILENCIO, y un parser corriente se
// queda con la última y da verde sobre un fichero roto. Un job con dos `if`
// pierde la condición de arriba sin que nada falle a la vista.
func TestClaveRepetida(t *testing.T) {
	_, err := salida(t, "jobs:\n  a:\n    if: uno\n    runs-on: docker\n    if: dos\n", true)
	if err == nil {
		t.Fatal("no detectó la clave repetida")
	}
	if !strings.Contains(err.Error(), "«if»") {
		t.Fatalf("el error no nombra la clave: %v", err)
	}
	// Y dice DÓNDE están las dos, que es lo que evita tener que buscarlas.
	if !strings.Contains(err.Error(), "línea 5") || !strings.Contains(err.Error(), "en la 3") {
		t.Fatalf("el error no da las dos líneas: %v", err)
	}
}

// La misma clave en mapas DISTINTOS es legítima y no debe saltar: si saltara,
// ningún workflow pasaría —todos los jobs tienen su `runs-on`—.
func TestMismaClaveEnMapasDistintos(t *testing.T) {
	if _, err := salida(t, "a:\n  x: 1\nb:\n  x: 2\n", true); err != nil {
		t.Fatalf("falso positivo: %v", err)
	}
}

// Se recorren TODOS los documentos. Un manifiesto de Kubernetes renderizado son
// decenas separados por `---`, y quedarse con el primero es la forma silenciosa
// de no comprobar nada.
func TestMultidocumento(t *testing.T) {
	out, err := salida(t, "kind: Service\n---\nkind: Deployment\n", false)
	if err != nil {
		t.Fatalf("procesar: %v", err)
	}
	lineas := strings.Split(strings.TrimSpace(out), "\n")
	if len(lineas) != 2 {
		t.Fatalf("documentos emitidos = %d, quiero 2:\n%s", len(lineas), out)
	}
	if !strings.Contains(lineas[1], "Deployment") {
		t.Fatalf("el segundo documento no salió: %q", lineas[1])
	}
	// Y el duplicado se detecta también en el SEGUNDO documento, no solo en el
	// primero.
	if _, err := salida(t, "kind: Service\n---\na: 1\na: 2\n", true); err == nil {
		t.Fatal("no miró más allá del primer documento")
	}
}

// Un documento vacío —el `---` final de tantos ficheros— no emite `null`: quien
// lo reciba por `jq` tendría que filtrarlo en cada llamada.
func TestDocumentoVacio(t *testing.T) {
	out, err := salida(t, "kind: Service\n---\n", false)
	if err != nil {
		t.Fatalf("procesar: %v", err)
	}
	if strings.Contains(out, "null") {
		t.Fatalf("emitió un documento nulo:\n%s", out)
	}
}

func TestYAMLInvalido(t *testing.T) {
	if _, err := salida(t, "a: [1, 2\n", false); err == nil {
		t.Fatal("aceptó YAML inválido")
	}
}

// capturar recoge lo que `procesar` escribe por la salida estándar. Redirigirla
// es más simple que meterle un io.Writer a la herramienta solo para el test:
// escribir por stdout es su interfaz real, la que consume `jq`.
func capturar(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	previo := os.Stdout
	os.Stdout = w
	errFn := fn()
	os.Stdout = previo
	_ = w.Close()
	datos, _ := io.ReadAll(r)
	return string(datos), errFn
}
