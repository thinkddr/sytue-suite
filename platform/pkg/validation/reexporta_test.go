// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import (
	"testing"

	devicevalidation "git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// TestReexportaElNucleo: los tipos son alias del núcleo, así que un error de validación
// hecho en un lado se reconoce en el otro (httpx lo traduce a 422 con sus campos).
func TestReexportaElNucleo(t *testing.T) {
	err := devicevalidation.New().Required("nombre", "").Err()
	verrs, ok := AsErrors(err)
	if !ok || len(verrs.Fields) != 1 || verrs.Fields[0].Field != "nombre" {
		t.Fatalf("AsErrors(%v) = %+v, %v", err, verrs, ok)
	}
	if New().Email("c", "no").Err() == nil || SanitizeText(" a  b ") != "a b" || TruncateRunes("áé", 1) != "á" {
		t.Fatal("la reexportación no se comporta como el núcleo")
	}
}
