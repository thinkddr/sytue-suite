// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLasPestanasDeDocusSonLasDeLaWeb: las pestañas de la cinta de Docus son, por
// IGUALDAD, las que la web pinta para un documento (su copia de build de `pestanas.json`):
// mismas, mismo orden, mismo rótulo.
func TestLasPestanasDeDocusSonLasDeLaWeb(t *testing.T) {
	web := []byte(webref.Read(t, editor.PestanasWeb))
	if p := editor.CotejarPestanas(registroDocus("es"), editor.Documento, web); len(p) > 0 {
		t.Error(strings.Join(p, "\n"))
	}
}
