// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOcultarLosPrimerosPasosEsDeUnaPersonaEnUnaOrganizacion: lo que oculta Ana en Acme
// no se lo encuentra oculto ni Ana en otra organización, ni otra persona con el mismo
// `sub` en otro Sytue ID, ni nadie sin sesión. Y el fichero no guarda quién es.
func TestOcultarLosPrimerosPasosEsDeUnaPersonaEnUnaOrganizacion(t *testing.T) {
	dir := aparte(t)
	ana := HuellaPrimerosPasos("https://id.sytue.eu", "u-ana", "acme")
	if PrimerosPasosOcultos(ana) {
		t.Fatal("sin ocultar ya sale oculta")
	}
	if err := OcultarPrimerosPasos(ana); err != nil {
		t.Fatal(err)
	}
	if err := OcultarPrimerosPasos(ana); err != nil { // dos veces no duplica
		t.Fatal(err)
	}
	if !PrimerosPasosOcultos(ana) {
		t.Fatal("ocultada, sigue saliendo")
	}
	if n := len(strings.Fields(Leer().PrimerosPasosOcultos)); n != 1 {
		t.Errorf("ocultar dos veces deja %d huellas", n)
	}
	for nombre, otra := range map[string]string{
		"otra organización": HuellaPrimerosPasos("https://id.sytue.eu", "u-ana", "beta"),
		"otro emisor":       HuellaPrimerosPasos("https://id.otra.eu", "u-ana", "acme"),
		"otra persona":      HuellaPrimerosPasos("https://id.sytue.eu", "u-bea", "acme"),
	} {
		if PrimerosPasosOcultos(otra) {
			t.Errorf("%s hereda el «Ocultar» de Ana", nombre)
		}
	}
	if HuellaPrimerosPasos("https://id.sytue.eu", "", "acme") != "" || PrimerosPasosOcultos("") {
		t.Error("sin sujeto no se recuerda nada")
	}
	crudo, err := os.ReadFile(filepath.Join(dir, ficheroAjustes))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(crudo), "u-ana") || strings.Contains(string(crudo), "acme") {
		t.Errorf("el fichero de ajustes dice quién ha entrado: %s", crudo)
	}
}
