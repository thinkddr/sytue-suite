// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

// TestElMenuNoRepiteLaMarca: los títulos de pantalla llevan « — Sytue One»; en el menú de
// la bandeja de Sytue One, no.
func TestElMenuNoRepiteLaMarca(t *testing.T) {
	for titulo, quiero := range map[string]string{
		"Notifications — Sytue One": "Notifications",
		"Sytue One — Home":          "Home",
		"Search in Sytue One":       "Search in Sytue One",
		"Welcome to Sytue":          "Welcome to Sytue",
		"Sytue One":                 "Sytue One",
	} {
		if got := etiquetaMenu(titulo); got != quiero {
			t.Errorf("etiquetaMenu(%q) = %q, quiero %q", titulo, got, quiero)
		}
	}
}
