// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import "testing"

// TestEsMultimedia: vídeo y audio reproducibles se abren con la app del sistema, como
// la web los abre en su reproductor; .avi/.mkv y las carpetas, no.
func TestEsMultimedia(t *testing.T) {
	for nombre, quiero := range map[string]bool{
		"v.mp4": true, "V.MOV": true, "a.mp3": true, "a.flac": true,
		"v.avi": false, "v.mkv": false, "t.txt": false, "sin": false,
	} {
		if got := EsMultimedia(Elemento{ID: "x", Nombre: nombre}); got != quiero {
			t.Errorf("%s: %v, quiero %v", nombre, got, quiero)
		}
	}
	if EsMultimedia(Elemento{ID: "x", Nombre: "c.mp4", Carpeta: true}) {
		t.Error("una carpeta no es multimedia")
	}
}
