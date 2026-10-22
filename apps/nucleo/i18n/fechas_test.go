// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"strings"
	"testing"
	"time"
)

// FormatDayMonth es la fecha media sin el año: el renglón de una fila de lista. Los tres
// idiomas de aquí abajo ponen el año en sitios distintos, que es lo que hay que quitar
// bien; y el test se pone en rojo si `sinAno` se lleva por delante el mes.
func TestFormatDayMonthQuitaElAnoYSuSeparador(t *testing.T) {
	cuando := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct{ loc, quiero string }{
		{"es", "11 sept"},
		{"en", "11 Sept"},
		{"hu", "szept. 11."}, // el húngaro empieza por el año: se quita el literal de DETRÁS
	} {
		got := FormatDayMonth(Locale(c.loc), cuando)
		if got != c.quiero {
			t.Errorf("%s: %q, quiero %q", c.loc, got, c.quiero)
		}
		if strings.Contains(got, "2026") {
			t.Errorf("%s: el año sigue ahí (%q)", c.loc, got)
		}
	}
	// Y en los 26 idiomas: nunca el año, nunca vacío y nunca con espacios en los bordes.
	for _, l := range Locales {
		got := FormatDayMonth(l, cuando)
		if strings.Contains(got, "2026") || got == "" || strings.TrimSpace(got) != got {
			t.Errorf("%s: %q", l, got)
		}
	}
}
