// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"strings"
	"testing"
)

// Los formatos nativos de Sytue Office solo los reconoce la edición local: sin
// FormatosOffice (las apps conectadas) un `.docus` no es de nadie, como en la web.
func TestLosFormatosDeOfficeSoloEnLaEdicionLocal(t *testing.T) {
	if _, ok := AppQueAbre("informe.docus"); ok {
		t.Fatal("la edición conectada no abre .docus")
	}
	FormatosOffice = true
	defer func() { FormatosOffice = false }()
	for nombre, quiero := range map[string]string{"a.docus": "docs", "B.CALCUS": "sheets", "c.diapos": "slides", "d.docx": "docs"} {
		if p, ok := AppQueAbre(nombre); !ok || p != quiero {
			t.Errorf("%s: %q, %v; quiero %q", nombre, p, ok, quiero)
		}
	}
	if got := strings.Join(ExtensionesOffice("sheets"), ","); got != "calcus,numbers,ods,ots,xlsx,xltx" {
		t.Errorf("ExtensionesOffice(sheets) = %q", got)
	}
	if got := strings.Join(Extensiones("sheets"), ","); strings.Contains(got, "calcus") {
		t.Errorf("Extensiones no cambia: %q", got)
	}
}
