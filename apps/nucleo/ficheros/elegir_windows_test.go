// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

import (
	"strings"
	"testing"
)

// TestElTituloNoLlegaAlGuionComoCodigo (FYS-12): un título con comillas
// tipográficas o rectas no aparece tal cual en el guion de PowerShell.
func TestElTituloNoLlegaAlGuionComoCodigo(t *testing.T) {
	titulo := "L’app'; Remove-Item C:\\x; '"
	_, args, err := ordenElegir(titulo, []string{"pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if guion := args[len(args)-1]; strings.Contains(guion, "Remove-Item") || strings.Contains(guion, "’") {
		t.Fatalf("el título va en claro en el guion: %s", guion)
	}
}
