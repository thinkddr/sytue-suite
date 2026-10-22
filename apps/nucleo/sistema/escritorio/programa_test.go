// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"errors"
	"slices"
	"testing"
)

// TestCadaSistemaConoceASusClientesDeCorreo: los nombres que se le piden a cada sistema,
// y nada fuera de la lista. Outlook no existe en Linux ni en los móviles: ahí no se
// intenta, y quien llama enseña «ábrelo tú».
func TestCadaSistemaConoceASusClientesDeCorreo(t *testing.T) {
	for _, c := range []struct {
		goos, cual string
		quiero     []string
	}{
		{"darwin", "thunderbird", []string{"Thunderbird"}},
		{"darwin", "outlook", []string{"Microsoft Outlook"}},
		{"windows", "outlook", []string{"outlook.exe", "olk.exe"}},
		{"linux", "thunderbird", []string{"thunderbird"}},
		{"linux", "outlook", nil},
		{"android", "thunderbird", nil},
		{"darwin", "/bin/sh", nil},
	} {
		if got := nombresPrograma(c.goos, c.cual); !slices.Equal(got, c.quiero) {
			t.Errorf("%s/%s = %q, quiero %q", c.goos, c.cual, got, c.quiero)
		}
	}
	if err := AbrirPrograma("lo-que-sea"); !errors.Is(err, ErrNoDisponible) {
		t.Errorf("un programa fuera de la lista = %v", err)
	}
}
