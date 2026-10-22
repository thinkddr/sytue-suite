// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"maps"
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLosErroresSonLosDeLaWeb compara `erroresPDF` con `ERRORES` de `pdf/errores.ts`, por
// IGUALDAD y en los dos sentidos. Un código que solo traduce uno de los dos sale crudo en
// el otro —o, desde que la frase del motor no se enseña, como el genérico—, y es justo lo
// que no se ve hasta que alguien se encuentra ese fallo.
func TestLosErroresSonLosDeLaWeb(t *testing.T) {
	t.Parallel()
	fuente := webref.Read(t, "apps/frontend/packages/office/src/pdf/errores.ts")
	web := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s+([a-z_]+): '(pdf\.[a-zA-Z.]+)',`).FindAllStringSubmatch(fuente, -1) {
		web[m[1]] = m[2]
	}
	if len(web) < 20 {
		t.Fatalf("solo %d códigos leídos de errores.ts: ¿ha cambiado su forma?", len(web))
	}
	if !maps.Equal(web, erroresPDF) {
		var faltan, sobran []string
		for k, v := range web {
			if erroresPDF[k] != v {
				faltan = append(faltan, k+"→"+v)
			}
		}
		for k, v := range erroresPDF {
			if web[k] != v {
				sobran = append(sobran, k+"→"+v)
			}
		}
		slices.Sort(faltan)
		slices.Sort(sobran)
		t.Errorf("los mapas de errores no son el mismo.\nla web y aquí no: %v\naquí y la web no: %v", faltan, sobran)
	}
}
