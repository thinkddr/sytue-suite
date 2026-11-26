// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videos

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FO-09: un WebM se mide en Go (su duración y sus lados) y da un fotograma de póster en
// JPEG; lo que no es un vídeo no da nada, y no falla.
func TestMedirUnWebMEnGo(t *testing.T) {
	m := Medir(t.Context(), filepath.Join("..", "..", "video", "testdata", "dos-segundos.webm"))
	if m.Duracion < 2*time.Second || m.Duracion > 3*time.Second || m.Ancho != 320 || m.Alto != 180 {
		t.Errorf("medido: %v %dx%d", m.Duracion, m.Ancho, m.Alto)
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(m.Poster)); err != nil {
		t.Errorf("el póster no es un JPEG: %v", err)
	}
	nada := filepath.Join(t.TempDir(), "nada.webm")
	if err := os.WriteFile(nada, []byte("no es un vídeo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m := Medir(t.Context(), nada); m.Duracion != 0 || m.Ancho != 0 {
		t.Errorf("lo que no es un vídeo: %+v", m)
	}
}
