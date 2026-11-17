// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// TestUniqueUploadName: el número va ANTES de la extensión («IMG_0001 (2).jpg»), la
// comparación de lo cogido la hace quien llama, y un nombre sin extensión no se parte.
func TestUniqueUploadName(t *testing.T) {
	cogidos := map[string]bool{"IMG_0001.jpg": true, "IMG_0001 (2).jpg": true, ".bashrc": true, "nota": true}
	hay := func(n string) (bool, error) { return cogidos[n], nil }
	for nombre, quiero := range map[string]string{
		"IMG_0001.jpg": "IMG_0001 (3).jpg",
		"IMG_0002.jpg": "IMG_0002.jpg",
		".bashrc":      ".bashrc (2)",
		"nota":         "nota (2)",
	} {
		if got, err := uniqueUploadName(nombre, hay); err != nil || got != quiero {
			t.Errorf("uniqueUploadName(%q) = %q, %v; quiero %q", nombre, got, err, quiero)
		}
	}
}
