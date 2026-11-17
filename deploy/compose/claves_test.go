// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compose no tiene código: solo el guardián de los ficheros de Docker
// Compose del repositorio, para que corra con `go test ./...` sin depender de
// tener Docker a mano.
package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// raiz es la raíz del repositorio vista desde este paquete.
const raiz = "../.."

// patronesDeCompose son los ficheros de Compose que se versionan. Los overlays de
// deploy/compose (override, frontlite, servidor, letsencrypt) entran por el primero.
var patronesDeCompose = []string{
	"deploy/compose/*.yml",
	"deploy/*/docker-compose*.yml",
	"scripts/e2e/docker-compose*.yml",
}

// claveRepetida es un hallazgo: la clave, la línea donde se repite y la primera.
type claveRepetida struct {
	donde   string
	clave   string
	linea   int
	primera int
}

func (c claveRepetida) String() string {
	return fmt.Sprintf("%s: %q repetida en la línea %d (ya estaba en la %d)", c.donde, c.clave, c.linea, c.primera)
}

// clavesRepetidas recorre el documento y devuelve toda clave repetida en un mismo
// mapa y toda variable repetida en el `environment` de un servicio, tanto en forma
// de mapa como de lista (`- CLAVE=valor`).
//
// Docker Compose rechaza el fichero entero con «mapping key … already defined»
// cuando un mapa repite clave, así que una sola línea duplicada deja sin arrancar
// la suite local. En forma de lista no se queja: se queda con una de las dos en
// silencio, que es peor. Pasó en el hub con CALENDAR_PUBLIC_URL, CONTACTS_PUBLIC_URL
// y NOTES_PUBLIC_URL, que aparecían con y sin ruta.
//
// La clave de fusión `<<` no cuenta: sobrescribir lo que trae un ancla
// (`<<: *go-env` y luego GOMEMLIMIT) es deliberado y YAML lo permite.
func clavesRepetidas(contenido []byte) ([]claveRepetida, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(contenido, &doc); err != nil {
		return nil, err
	}
	var hallazgos []claveRepetida
	var recorrer func(n *yaml.Node, ruta string)
	recorrer = func(n *yaml.Node, ruta string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, h := range n.Content {
				recorrer(h, ruta)
			}
		case yaml.SequenceNode:
			for i, h := range n.Content {
				recorrer(h, fmt.Sprintf("%s[%d]", ruta, i))
			}
		case yaml.MappingNode:
			vistas := map[string]int{}
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if k.Tag == "!!merge" || k.Value == "<<" {
					continue
				}
				if primera, ok := vistas[k.Value]; ok {
					hallazgos = append(hallazgos, claveRepetida{ruta, k.Value, k.Line, primera})
				} else {
					vistas[k.Value] = k.Line
				}
				hija := k.Value
				if ruta != "" {
					hija = ruta + "." + k.Value
				}
				if k.Value == "environment" && v.Kind == yaml.SequenceNode {
					hallazgos = append(hallazgos, variablesRepetidasEnLista(hija, v)...)
				}
				recorrer(v, hija)
			}
		case yaml.ScalarNode, yaml.AliasNode:
			// Hojas y referencias a anclas: no tienen claves propias.
		}
	}
	recorrer(&doc, "")
	return hallazgos, nil
}

func variablesRepetidasEnLista(donde string, lista *yaml.Node) []claveRepetida {
	var hallazgos []claveRepetida
	vistas := map[string]int{}
	for _, e := range lista.Content {
		if e.Kind != yaml.ScalarNode {
			continue
		}
		nombre, _, _ := strings.Cut(e.Value, "=")
		nombre = strings.TrimSpace(nombre)
		if primera, ok := vistas[nombre]; ok {
			hallazgos = append(hallazgos, claveRepetida{donde, nombre, e.Line, primera})
		} else {
			vistas[nombre] = e.Line
		}
	}
	return hallazgos
}

// TestLosComposeNoRepitenClaves falla si algún fichero de Compose del repositorio
// repite una clave, en particular una variable del `environment` de un servicio.
func TestLosComposeNoRepitenClaves(t *testing.T) {
	t.Parallel()

	var ficheros []string
	for _, p := range patronesDeCompose {
		m, err := filepath.Glob(filepath.Join(raiz, p))
		if err != nil {
			t.Fatalf("patrón %q: %v", p, err)
		}
		ficheros = append(ficheros, m...)
	}
	slices.Sort(ficheros)
	ficheros = slices.Compact(ficheros)
	// Un glob que no encuentra nada daría verde sin mirar nada.
	principal := filepath.Join(raiz, "deploy", "compose", "docker-compose.yml")
	if !slices.Contains(ficheros, principal) {
		t.Fatalf("no se encontró %s entre los ficheros de Compose: %v", principal, ficheros)
	}

	for _, f := range ficheros {
		contenido, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		hallazgos, err := clavesRepetidas(contenido)
		if err != nil {
			t.Errorf("%s no es YAML válido: %v", f, err)
			continue
		}
		for _, h := range hallazgos {
			t.Errorf("%s: %s", strings.TrimPrefix(f, raiz+"/"), h)
		}
	}
	t.Logf("revisados %d ficheros de Compose", len(ficheros))
}

// TestClavesRepetidasDetecta comprueba que el guardián de arriba ve lo que tiene
// que ver y deja pasar lo legítimo.
func TestClavesRepetidasDetecta(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		yaml   string
		quiere []string
	}{
		{
			nombre: "environment en forma de mapa con una variable dos veces",
			yaml: `services:
  hub:
    environment:
      CALENDAR_PUBLIC_URL: https://calendar.sytue.localhost
      MAIL_PUBLIC_URL: https://mail.sytue.localhost/mail
      CALENDAR_PUBLIC_URL: https://calendar.sytue.localhost/calendar
`,
			quiere: []string{`services.hub.environment: "CALENDAR_PUBLIC_URL" repetida en la línea 6 (ya estaba en la 4)`},
		},
		{
			nombre: "environment en forma de lista con una variable dos veces",
			yaml: `services:
  web:
    environment:
      - PORT=4321
      - HOST
      - PORT=4322
`,
			quiere: []string{`services.web.environment: "PORT" repetida en la línea 6 (ya estaba en la 4)`},
		},
		{
			nombre: "sobrescribir lo que trae un ancla no es repetir",
			yaml: `x-go-env: &go-env
  GOMEMLIMIT: 64MiB
  LOG_LEVEL: info
services:
  cast:
    environment:
      <<: *go-env
      GOMEMLIMIT: 1024MiB
`,
		},
		{
			nombre: "la misma variable en dos servicios distintos tampoco",
			yaml: `services:
  a:
    environment:
      PORT: "1"
  b:
    environment:
      PORT: "2"
`,
		},
		{
			nombre: "una clave repetida fuera de environment también tumba compose",
			yaml: `services:
  a:
    image: x
    image: y
`,
			quiere: []string{`services.a: "image" repetida en la línea 4 (ya estaba en la 3)`},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			hallazgos, err := clavesRepetidas([]byte(c.yaml))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, h := range hallazgos {
				got = append(got, h.String())
			}
			if !slices.Equal(got, c.quiere) {
				t.Errorf("hallazgos = %q, quiero %q", got, c.quiere)
			}
		})
	}
}
