// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

const rejillaWeb = "apps/frontend/apps/bases/src/components/Rejilla.tsx"

// claseCon devuelve el atributo `class` de Rejilla.tsx que contiene `ancla`. El ancla
// es lo que identifica el elemento (la cabecera de un campo, la celda…) y no las
// medidas, que son justo lo que se quiere leer: si el ancla se pierde, el test lo dice
// en vez de comparar contra otro elemento.
func claseCon(t *testing.T, fuente, ancla string) string {
	t.Helper()
	var halladas []string
	for _, m := range regexp.MustCompile("class=(?:\"([^\"]*)\"|\\{`([^`]*)`\\})").FindAllStringSubmatch(fuente, -1) {
		if c := m[1] + m[2]; strings.Contains(c, ancla) {
			halladas = append(halladas, c)
		}
	}
	if len(halladas) != 1 {
		t.Fatalf("Rejilla.tsx tiene %d clases con %q y se esperaba una: revisad el ancla", len(halladas), ancla)
	}
	return halladas[0]
}

// paso lee la utilidad de espaciado `prefijo-N` de una clase y la pasa a píxeles con la
// misma escala que Tailwind (N × 0,25 rem).
func paso(t *testing.T, clase, prefijo string) float32 {
	t.Helper()
	var hallado []float32
	for _, u := range strings.Fields(clase) {
		if n, ok := strings.CutPrefix(u, prefijo+"-"); ok {
			v, err := strconv.ParseFloat(n, 32)
			if err != nil {
				t.Fatalf("%q no es un paso de espaciado", u)
			}
			hallado = append(hallado, tema.Espacio(float32(v)))
		}
	}
	if len(hallado) != 1 {
		t.Fatalf("la clase %q tiene %d utilidades %s-N y se esperaba una", clase, len(hallado), prefijo)
	}
	return hallado[0]
}

// TestLasMedidasDeLaRejillaSonLasDeLaWeb contrasta las medidas de rejilla.go con las
// clases de Tailwind de Rejilla.tsx, leyéndolas del fichero. Las clases se parsean y no
// se comparan como cadena para que el error diga qué medida se fue y a cuánto: la
// rejilla nativa llegó a citar `py-2`, `size={14}`, `gap-1.5` y `min-w-32` cuando la
// web ya llevaba `py-3`, `size={15}`, `gap-2` y `min-w-36`, y nada lo vigilaba.
func TestLasMedidasDeLaRejillaSonLasDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, rejillaWeb)

	cabeza := claseCon(t, fuente, "px-3 py-3 font-semibold")
	celda := claseCon(t, fuente, "sytue-focus-ring-inset")
	titulo := claseCon(t, fuente, "? 'justify-end' : ''}")
	numero := claseCon(t, fuente, "w-12 border-b px-2 py-2 text-start font-normal")
	chips := claseCon(t, fuente, "flex max-w-72 flex-wrap")
	casilla := claseCon(t, fuente, "accent-primary pointer-events-none")

	for _, c := range []struct {
		que           string
		suya, nuestra float32
	}{
		{"el relleno horizontal de la cabecera de un campo (margenCelda)", paso(t, cabeza, "px"), margenCelda},
		{"el relleno vertical de la cabecera de un campo (margenCabeza)", paso(t, cabeza, "py"), margenCabeza},
		{"el relleno horizontal de una celda (margenCelda)", paso(t, celda, "px"), margenCelda},
		{"el ancho mínimo de una celda (minCelda)", paso(t, celda, "min-w"), minCelda},
		{"el hueco entre el glifo y el nombre del campo (huecoCabeza)", paso(t, titulo, "gap"), huecoCabeza},
		{"el relleno horizontal de la columna de números (margenFila0)", paso(t, numero, "px"), margenFila0},
		{"el hueco entre chips (huecoChips)", paso(t, chips, "gap"), huecoChips},
		{"el lado de la casilla (ladoCasilla)", paso(t, casilla, "h"), ladoCasilla},
		{"el ancho de la casilla (ladoCasilla)", paso(t, casilla, "w"), ladoCasilla},
	} {
		if c.suya != c.nuestra {
			t.Errorf("%s: la web pide %v px y aquí son %v", c.que, c.suya, c.nuestra)
		}
	}

	// La cabecera es `text-sm` (de la tabla) con su relleno arriba y abajo.
	if !strings.Contains(claseCon(t, fuente, "bases-grid w-full"), "text-sm") {
		t.Error("la tabla ya no es `text-sm`: revisad altoCabeza y el escalón de los títulos")
	}
	if quiero := 2*paso(t, cabeza, "py") + tema.TextoSM.Linea; altoCabeza != quiero {
		t.Errorf("altoCabeza es %v y la cabecera de la web mide %v", altoCabeza, quiero)
	}

	icono := regexp.MustCompile(`<Icon name=\{ICONO_DE_TIPO\[c\.kind\]\} size=\{(\d+)\}`).FindStringSubmatch(fuente)
	if icono == nil {
		t.Fatal("Rejilla.tsx ya no pinta el glifo de tipo de la cabecera con `size={N}`")
	}
	if lado, _ := strconv.ParseFloat(icono[1], 32); float32(lado) != ladoIconoCabeza {
		t.Errorf("el glifo de tipo de la cabecera es `size={%s}` en la web y aquí mide %v", icono[1], ladoIconoCabeza)
	}

	// ponytail: el tope de 20 rem por columna es deliberado (rejilla.go lo explica);
	// la web no tiene tope y no hay clase con la que compararlo.
	if !strings.Contains(claseCon(t, fuente, "bases-grid w-full"), "min-w-max") {
		t.Error("la tabla de la web ya no es `min-w-max`: revisad si maxCelda sigue teniendo sentido")
	}
}
