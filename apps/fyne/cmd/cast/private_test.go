// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
)

// Sin recursos verificados no hay plan: el fallo es el de recursos privados,
// nunca una importación legible de la fuente.
func TestCastPlanRejectsMissingResourcesBeforeConstruction(t *testing.T) {
	if _, err := castPlan(t.Context(), nil, nil); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatal(err)
	}
}
