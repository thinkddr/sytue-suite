// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestEmojiEsLaFuenteDeColorEstableYSuLicenciaVaSeparadaDelFS(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(Emoji())); got != "15671215ab769fdc7162a045d56fd7d7e477c51b04e6b3c761d914d8fdd6cc44" {
		t.Fatalf("Noto Color Emoji del tag estable cambió: SHA256=%s", got)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(LicenciaEmoji())); got != "6a73f9541c2de74158c0e7cf6b0a58ef774f5a780bf191f2d7ec9cc53efe2bf2" {
		t.Fatalf("la licencia original de Noto Color Emoji cambió: SHA256=%s", got)
	}
	if !strings.Contains(string(LicenciaEmoji()), "SIL OPEN FONT LICENSE Version 1.1") {
		t.Fatal("falta la licencia OFL original")
	}
	if _, err := fs.ReadFile(FS, "NotoColorEmoji.ttf"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("el FS usado por el IdP incluye la fuente de emoji: %v", err)
	}
}
