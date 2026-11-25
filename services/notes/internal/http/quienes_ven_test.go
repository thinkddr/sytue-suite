// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestLasEscriturasDeUnCuadernoAvisanAQuienesLoVen: cada ruta que escribe el contenido de
// un cuaderno o de una nota (`/notebooks/{id}`, sus notas, `/notes/{id}`…) pasa por
// quienesVen, que suma a la señal de cambios al dueño y a las personas con quien se
// compartió (ADR 0078). Quedan fuera las que no cambian lo que ven otras personas: los
// recordatorios y aplazar (de quien los pone), compartir y las peticiones de edición
// (ya avisan a quien toca), y exportar o importar. VISTO EN ROJO quitando el quienesVen
// de PUT /notes/{id}.
func TestLasEscriturasDeUnCuadernoAvisanAQuienesLoVen(t *testing.T) {
	fuente, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	ruta := regexp.MustCompile(`api\.(Post|Put|Patch|Delete)\("(/(?:notebooks|notes)/\{id\}[^"]*)", ([^\n]+)\)\n`)
	fuera := regexp.MustCompile(`/(people|edit-requests|reminder|postpone|export)`)
	vistas := 0
	for _, m := range ruta.FindAllStringSubmatch(string(fuente), -1) {
		if fuera.MatchString(m[2]) {
			continue
		}
		vistas++
		porNota := strings.HasPrefix(m[2], "/notes/")
		quiero := "h.quienesVen("
		if !strings.HasPrefix(m[3], quiero) || strings.HasSuffix(m[3], ", true)") != porNota {
			t.Errorf("%s %s no avisa a quienes ven lo escrito: %s", m[1], m[2], m[3])
		}
	}
	if vistas < 12 {
		t.Fatalf("solo %d escrituras de cuadernos y notas en server.go", vistas)
	}
}
