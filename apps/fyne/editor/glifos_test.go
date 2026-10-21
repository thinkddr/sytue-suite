// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLosGlifosDeLaPortadaSonLosDeLaWeb compara ruta a ruta los glifos propios de la
// portada contra `KIND_PATH` y `GLYPHS` de `DocList.tsx`, en los dos sentidos: que no
// sobre ninguno aquí y que no falte ninguno de los que la web declara.
func TestLosGlifosDeLaPortadaSonLosDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, FuenteGlifos)
	for _, caso := range []struct {
		bloque string
		mapa   map[string]string
	}{
		{"const KIND_PATH: Record<DocKind, string> = {", GlifosTipo},
		{"const GLYPHS = {", GlifosPortada},
	} {
		web := rutasDelBloque(t, fuente, caso.bloque)
		for clave, suya := range web {
			switch nuestra, hay := caso.mapa[clave]; {
			case !hay:
				t.Errorf("%s declara %q y aquí no está", FuenteGlifos, clave)
			case nuestra != suya:
				t.Errorf("el glifo %q tiene otra ruta en la web:\n  web:  %s\n  aquí: %s", clave, suya, nuestra)
			}
		}
		for clave := range caso.mapa {
			if _, hay := web[clave]; !hay {
				t.Errorf("el glifo %q no existe en %s: o se retiró allí, o aquí se inventó", clave, FuenteGlifos)
			}
		}
	}
}

// rutasDelBloque saca las parejas `clave: 'ruta'` del objeto literal que empieza en
// `cabecera`, hasta su llave de cierre.
func rutasDelBloque(t *testing.T, fuente, cabecera string) map[string]string {
	t.Helper()
	i := strings.Index(fuente, cabecera)
	if i < 0 {
		t.Fatalf("%s ya no tiene «%s»: revisa el espejo", FuenteGlifos, cabecera)
	}
	cuerpo, _, _ := strings.Cut(fuente[i+len(cabecera):], "\n}")
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+):\s*'([^']+)'`).FindAllStringSubmatch(cuerpo, -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatalf("no se ha leído ningún glifo de «%s»", cabecera)
	}
	return out
}
