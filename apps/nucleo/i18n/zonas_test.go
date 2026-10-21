// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"slices"
	"testing"
	"time"
	_ "time/tzdata" // las zonas, también donde el sistema no las trae
)

// TestLasZonasSonLasDeIntl: la lista es la del motor de la web (cientos, con Madrid y
// Nueva York), y la etiqueta es la de `zoneLabel`, con el desfase del instante.
func TestLasZonasSonLasDeIntl(t *testing.T) {
	z := ZonasHorarias()
	if len(z) < 300 || !slices.Contains(z, "Europe/Madrid") || !slices.Contains(z, "America/New_York") {
		t.Fatalf("zonas.json trae %d zonas: regenerar en Linux con gen-i18n.ts", len(z))
	}
	verano := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	invierno := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		zona   string
		en     time.Time
		quiero string
	}{
		{"Europe/Madrid", verano, "Europe/Madrid (GMT+02:00)"},
		{"America/New_York", invierno, "America/New York (GMT-05:00)"},
		{"Asia/Kolkata", verano, "Asia/Kolkata (GMT+05:30)"},
		{"UTC", verano, "UTC (GMT+00:00)"},
	} {
		if got := EtiquetaZona(c.zona, c.en); got != c.quiero {
			t.Errorf("EtiquetaZona(%s) = %q y zoneLabel da %q", c.zona, got, c.quiero)
		}
	}
}
