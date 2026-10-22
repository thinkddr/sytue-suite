// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapositivas

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

func TestPosicionValidar(t *testing.T) {
	ok, err := Posicion{Index: 2, Total: 10, Title: "  Presupuesto \n del\tpozo  ", Ref: "docs:abc-123"}.Validar()
	if err != nil || ok.Title != "Presupuesto del pozo" {
		t.Fatalf("válida: %+v %v", ok, err)
	}
	if p, _ := (Posicion{Total: 1, Title: strings.Repeat("ñ", 500)}).Validar(); len([]rune(p.Title)) != MaxTitulo {
		t.Errorf("el título se recorta a %d runas, no a %d", MaxTitulo, len([]rune(p.Title)))
	}
	for nombre, p := range map[string]Posicion{
		"sin diapositivas":        {Index: 0, Total: 0},
		"demasiadas":              {Index: 0, Total: MaxDiapositivas + 1},
		"índice negativo":         {Index: -1, Total: 5},
		"índice fuera del total":  {Index: 5, Total: 5},
		"referencia con espacios": {Index: 0, Total: 1, Ref: "docs: abc"},
		"referencia sin origen":   {Index: 0, Total: 1, Ref: "abc"},
		"referencia con ruta":     {Index: 0, Total: 1, Ref: "docs:../../etc"},
	} {
		if _, err := p.Validar(); !errors.Is(err, domain.ErrEntradaInvalida) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}
