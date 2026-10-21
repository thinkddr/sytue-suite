// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command yamlq lee YAML y escribe JSON, para que los gates consulten con `jq`
// en vez de con Python.
//
// POR QUÉ EXISTE. Media docena de comprobaciones de `deploy/production/tests`
// abrían un `python3 -c` con PyYAML para leer un manifiesto o un workflow. Eso
// mete un intérprete y una librería de terceros en el camino crítico del CI, y
// se notó: el runner compartido de git.gay no trae PyYAML, así que hubo que
// añadirle un instalador propio a la acción de toolchain para que cuatro gates
// dejaran de morir. Aquí ya hay un compilador de Go y `gopkg.in/yaml.v3` en el
// go.mod; una herramienta de sesenta líneas quita esa dependencia entera.
//
// Hace DOS cosas y ninguna más, porque para el resto ya está `jq`:
//
//	yamlq            <ficheros…>   YAML → JSON (un documento por línea)
//	yamlq -repetidas <ficheros…>   ¿alguna clave repetida dentro de un mapa?
//
// Sin ficheros lee de la entrada estándar. Con `-repetidas` no imprime nada si
// todo está bien y sale 1 nombrando fichero, clave y línea si no.
//
// LO DE LAS CLAVES REPETIDAS NO ES UN EXTRA. En YAML la segunda pisa a la
// primera EN SILENCIO: un job con dos `if` pierde la condición de arriba sin que
// nada falle a la vista, y Forgejo además rechaza el fichero al preparar un
// `workflow_dispatch`. Un parser corriente no sirve para detectarlo —PyYAML
// también se quedaba con la última—, así que se recorre el árbol de nodos, que
// es donde los duplicados todavía existen.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	repetidas := flag.Bool("repetidas", false, "solo comprobar claves repetidas dentro de un mismo mapa")
	flag.Parse()

	entradas := flag.Args()
	if len(entradas) == 0 {
		entradas = []string{"-"}
	}

	fallos := 0
	for _, ruta := range entradas {
		datos, err := leer(ruta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "yamlq: %v\n", err)
			fallos++
			continue
		}
		if err := procesar(datos, *repetidas); err != nil {
			fmt.Fprintf(os.Stderr, "yamlq: %s: %v\n", ruta, err)
			fallos++
		}
	}
	if fallos > 0 {
		os.Exit(1)
	}
}

func leer(ruta string) ([]byte, error) {
	if ruta == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(ruta)
}

// procesar recorre TODOS los documentos del fichero: un manifiesto de Kubernetes
// renderizado por Helm son decenas separados por `---`, y quedarse con el
// primero es la manera silenciosa de no comprobar nada.
// procesar NO recibe la ruta: quien llama ya la pone en el mensaje
// («yamlq: <ruta>: <error>»), así que aquí solo sobraba.
func procesar(datos []byte, soloRepetidas bool) error {
	dec := yaml.NewDecoder(bytes.NewReader(datos))
	for n := 1; ; n++ {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("documento %d: %w", n, err)
		}
		if err := buscarRepetidas(&doc); err != nil {
			return fmt.Errorf("documento %d: %w", n, err)
		}
		if soloRepetidas {
			continue
		}
		var v any
		if err := doc.Decode(&v); err != nil {
			return fmt.Errorf("documento %d: %w", n, err)
		}
		// Un documento vacío —el `---` final de tantos ficheros— decodifica a
		// nulo. Emitirlo llenaría la salida de `null` que luego `jq` tendría que
		// filtrar en cada llamada.
		if v == nil {
			continue
		}
		salida, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("documento %d: %w", n, err)
		}
		fmt.Println(string(salida))
	}
}

// buscarRepetidas baja por el árbol de NODOS y no por el mapa ya construido:
// cuando yaml.v3 termina de decodificar en un `map[string]any`, el duplicado ya
// se perdió. Aquí las dos claves siguen ahí, cada una con su línea.
func buscarRepetidas(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		vistas := make(map[string]int, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			clave := n.Content[i]
			if antes, ya := vistas[clave.Value]; ya {
				return fmt.Errorf("clave «%s» repetida en la línea %d (ya estaba en la %d)",
					clave.Value, clave.Line, antes)
			}
			vistas[clave.Value] = clave.Line
		}
	}
	for _, hijo := range n.Content {
		if err := buscarRepetidas(hijo); err != nil {
			return err
		}
	}
	return nil
}
