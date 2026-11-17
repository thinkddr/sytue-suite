// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

func TestGuardaVersionSoloAlEmpezarUnaTandaQueCambiaElTexto(t *testing.T) {
	ahora := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	antes := Note{Title: "Plan", Content: "<p>uno</p>", UpdatedAt: ahora.Add(-time.Hour)}
	cambia := NoteInput{Title: "Plan", Content: "<p>dos</p>"}
	reciente := &UltimaVersion{ReplacedBy: "ada", CreatedAt: ahora.Add(-time.Minute)}
	vieja := &UltimaVersion{ReplacedBy: "ada", CreatedAt: ahora.Add(-2 * TandaDeEdicion)}

	casos := []struct {
		nombre string
		antes  Note
		in     NoteInput
		ultima *UltimaVersion
		sub    string
		quiere bool
	}{
		{"primera edición", antes, cambia, nil, "ada", true},
		{"sin cambiar el texto (mover, marcar)", antes, NoteInput{Title: "Plan", Content: "<p>uno</p>"}, nil, "ada", false},
		{"nota recién creada y vacía", Note{UpdatedAt: antes.UpdatedAt}, cambia, nil, "ada", false},
		{"misma tanda de la misma persona", antes, cambia, reciente, "ada", false},
		{"otra persona", antes, cambia, reciente, "bob", true},
		{"tanda nueva tras la pausa", antes, cambia, vieja, "ada", true},
		{"sigue escribiendo sin pausa", Note{Title: "Plan", UpdatedAt: ahora.Add(-time.Minute)}, cambia, vieja, "ada", false},
		{"cifrada: cambia el criptograma", Note{ContentCipher: "encv1:a", UpdatedAt: antes.UpdatedAt}, NoteInput{ContentCipher: "encv1:b"}, nil, "ada", true},
	}
	for _, c := range casos {
		if got := GuardaVersion(c.antes, c.in, c.ultima, c.sub, ahora); got != c.quiere {
			t.Errorf("%s: GuardaVersion = %v, quiere %v", c.nombre, got, c.quiere)
		}
	}
}
