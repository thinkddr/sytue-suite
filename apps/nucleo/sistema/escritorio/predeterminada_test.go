// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "testing"

// El nombre va tal cual lo deja el instalador en RegisteredApplications, con el espacio
// escapado: con un `+` Windows buscaría «Sytue+Calcus» y abriría la pantalla general.
func TestLaPaginaDePredeterminadasEsLaDeEstaApp(t *testing.T) {
	if got, want := paginaPredeterminadas("Sytue Calcus"), "ms-settings:defaultapps?registeredAppUser=Sytue%20Calcus"; got != want {
		t.Fatalf("página = %q, quiero %q", got, want)
	}
}

func TestSinExtensionesNoSeTocaNada(t *testing.T) {
	if err := Predeterminada("Sytue Hub", nil); err != nil {
		t.Fatalf("sin extensiones: %v", err)
	}
}

// Solo ficheros de Apple: no hay nada que reclamar, y no se llega al sistema (que en
// macOS le quitaría el .pages a Pages).
func TestLosDeAppleNoSePonenPorDefecto(t *testing.T) {
	if err := Predeterminada("Sytue Docus", []string{"pages", "numbers", "key"}); err != nil {
		t.Fatalf("solo de Apple: %v", err)
	}
}
