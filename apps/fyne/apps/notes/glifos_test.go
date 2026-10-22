// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLosGlifosSonLosDeLaWeb compara ruta a ruta los glifos propios de Notepads contra
// `glyphs.tsx`, que es de donde salen.
//
// Se comprueban los dos sentidos que tienen sentido —que la clave exista allí y que la
// ruta sea la misma— pero NO que estén todas: de las cuarenta y tantas de `glyphs.tsx`
// se portan las que las pantallas de esta ola USAN, y exigir el fichero entero
// obligaría a traerse la barra del editor enriquecido para poder pintar el árbol.
//
// VISTO EN ROJO (2026-09-22, en el piloto), con tres inversiones deshechas después:
// cambiando un `4` por un `5` en `NotaD` dice que la ruta no es la de la web;
// añadiendo `"inventado": "M0 0"` a `Glifos` dice que esa clave no existe en el `.tsx`;
// y cambiando `FuenteGlifos` a una ruta que no está, `webref.Read` falla en vez de
// saltarse el test en silencio, que es justo para lo que existe ese paquete.
func TestLosGlifosSonLosDeLaWeb(t *testing.T) {
	web := rutasDeGlyphsTSX(t)
	for clave, nuestra := range notes.Glifos {
		switch suya, hay := web[clave]; {
		case !hay:
			t.Errorf("el glifo %q no existe en %s: o se retiró allí, o aquí se inventó",
				clave, notes.FuenteGlifos)
		case suya != nuestra:
			t.Errorf("el glifo %q tiene otra ruta en la web:\n  web:  %s\n  aquí: %s",
				clave, suya, nuestra)
		}
	}
}

// TestNingunGlifoEstaDosVeces: dos claves con la misma ruta es una copia, y la ruta que
// se corrija en la web solo llegaría a una de ellas.
//
// Con una excepción DELIBERADA y medida: `download` y `upload` son el mismo dibujo con
// la flecha al revés, y `undo`/`redo` lo mismo. No son iguales entre sí; lo que este
// test caza es que alguien pegue la misma cadena dos veces por descuido.
func TestNingunGlifoEstaDosVeces(t *testing.T) {
	visto := map[string]string{}
	for clave, d := range notes.Glifos {
		if otra, ya := visto[d]; ya {
			t.Errorf("%q y %q tienen la MISMA ruta: una de las dos está copiada", clave, otra)
		}
		visto[d] = clave
	}
}

// TestElTipoDeNotaTieneSuGlifo recorre los tipos que el servicio puede devolver y
// comprueba que el que lleva icono lo saca de esta tabla y no de otro sitio.
//
// Los tipos son los de `GLIFO` en NoteCard.tsx; `text` es el único sin icono, que es lo
// que hace la web («el rótulo de las que no son texto»).
func TestElTipoDeNotaTieneSuGlifo(t *testing.T) {
	conIcono := []string{
		"link", "image", "file", "drive", "doc", "event", "room",
		"crossref", "color", "launcher", "todo",
	}
	for _, tipo := range conIcono {
		if notes.GlifoDeTipo(tipo) == "" {
			t.Errorf("el tipo de nota %q se queda sin icono", tipo)
		}
	}
	if notes.GlifoDeTipo("text") != "" {
		t.Error("una nota de texto no lleva icono de tipo en la web")
	}
	if notes.GlifoDeTipo("loquesea") != "" {
		t.Error("un tipo que esta versión no conoce no debe pintar un icono cualquiera")
	}
}

// rutasDeGlyphsTSX lee el `const PATHS = { … } as const;` de Notepads.
//
// El patrón NO es el de Mail y Drive (`<path d="…" />`): aquí las rutas son cadenas
// entre comillas simples, y las largas las parte Prettier en la línea siguiente. Por eso
// este ayudante vive en este paquete y no en `iconos`.
func rutasDeGlyphsTSX(t *testing.T) map[string]string {
	t.Helper()
	fuente := webref.Read(t, notes.FuenteGlifos)
	i := strings.Index(fuente, "const PATHS = {")
	j := strings.Index(fuente, "} as const;")
	if i < 0 || j < i {
		t.Fatalf("%s ya no declara `const PATHS = { … } as const;`: rehacer el espejo",
			notes.FuenteGlifos)
	}
	re := regexp.MustCompile(`(?m)^ {2}([A-Za-z][A-Za-z0-9]*):\s*(?:\n\s+)?'([^']*)',`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:j], -1) {
		out[m[1]] = m[2]
	}
	if len(out) < 30 {
		t.Fatalf("solo se han leído %d glifos de %s: el patrón ya no vale",
			len(out), notes.FuenteGlifos)
	}
	return out
}
