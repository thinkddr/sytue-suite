// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package office

import "testing"

func TestCatalog(t *testing.T) {
	exports := Exports()
	want := []string{"pdf", "ooxml", "odf", "html", "markdown"}
	if len(exports) != len(want) {
		t.Fatalf("exports: %v", exports)
	}
	for i, command := range exports {
		if command.ID != want[i] {
			t.Errorf("order: %s", command.ID)
		}
		for _, kind := range []string{"document", "spreadsheet", "presentation"} {
			if command.Labels[kind] == "" {
				t.Errorf("missing label: %s/%s", command.ID, kind)
			}
		}
		if !command.Available(false, false) || command.Available(true, false) || command.Available(false, true) {
			t.Errorf("availability: %s", command.ID)
		}
	}
	exports[0].Labels["document"] = "mutated"
	if Exports()[0].Labels["document"] == "mutated" {
		t.Fatal("shared mutable catalog")
	}
}
