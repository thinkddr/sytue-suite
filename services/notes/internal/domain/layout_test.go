// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// El árbol es la disposición por defecto: un cuaderno que no elige, la recibe.
func TestElArbolEsLaDisposicionPorDefecto(t *testing.T) {
	in := NotebookInput{Title: "Ideas"}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if in.Layout != LayoutTree {
		t.Errorf("layout por defecto = %q; quiero %q", in.Layout, LayoutTree)
	}
	for _, l := range []Layout{LayoutTree, LayoutFree, LayoutColumns, LayoutList} {
		if !ValidLayout(l) {
			t.Errorf("%q tendría que ser válida", l)
		}
	}
	if ValidLayout("grid") {
		t.Error("una disposición inventada se aceptó")
	}
}
