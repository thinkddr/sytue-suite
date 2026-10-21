// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

// Normalize: recorta, normaliza el color y rechaza lo inválido con ErrInvalidInput
// (que el handler traduce a 400).
func TestSensitivityLabelNormalize(t *testing.T) {
	l := SensitivityLabel{
		Name:        "  Confidencial  ",
		Description: "  Solo para el equipo  ",
		Color:       "  #AABBCC ",
		Rank:        30,
	}
	if err := l.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if l.Name != "Confidencial" || l.Description != "Solo para el equipo" {
		t.Fatalf("no recortó: %+v", l)
	}
	if l.Color != "#aabbcc" {
		t.Fatalf("color=%q, se esperaba minúsculas", l.Color)
	}

	bad := []struct {
		name string
		in   SensitivityLabel
	}{
		{"nombre vacío", SensitivityLabel{Name: "   "}},
		{"nombre largo", SensitivityLabel{Name: strings.Repeat("x", maxLabelName+1)}},
		{"descripción larga", SensitivityLabel{Name: "ok", Description: strings.Repeat("x", maxLabelDesc+1)}},
		{"rank negativo", SensitivityLabel{Name: "ok", Rank: -1}},
		{"rank enorme", SensitivityLabel{Name: "ok", Rank: maxLabelRank + 1}},
		{"color sin #", SensitivityLabel{Name: "ok", Color: "aabbcc"}},
		{"color corto", SensitivityLabel{Name: "ok", Color: "#abc"}},
		{"color no hex", SensitivityLabel{Name: "ok", Color: "#zzzzzz"}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			err := in.Normalize()
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("se esperaba ErrInvalidInput, got %v", err)
			}
		})
	}

	// El color es OPCIONAL: vacío es válido (la UI usa el neutro).
	sin := SensitivityLabel{Name: "Público"}
	if err := sin.Normalize(); err != nil {
		t.Fatalf("color vacío debería ser válido: %v", err)
	}
}
