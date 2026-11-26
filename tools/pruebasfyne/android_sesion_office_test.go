// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"strings"
	"testing"
)

// Sytue Office (SYTUE_ARBOL=cmd-local) no comparte sesión con nadie: su SesionFirmas sale
// deshabilitada, sin pines ni productos, y android-sesion.py se niega a darle un broker o un
// pin (docs/roadmap.md, «Sytue Office»).

func TestAndroidSesionDeSytueOfficeNoConfiaEnNadieYQuedaDeshabilitada(t *testing.T) {
	var r androidSesionResultado
	pyRestoCorrer(t, androidSesionCargar+`
devolver({"java": m.java(False, {}, edicion="local")})
`, &r)
	for _, s := range []string{"HABILITADO = false", "PAQUETES = Collections.unmodifiableMap(pins);"} {
		if !strings.Contains(r.Java, s) {
			t.Fatalf("la SesionFirmas de la edición no contiene %q\n%s", s, r.Java)
		}
	}
	for _, s := range []string{"pins.put(", "products.put("} {
		if strings.Contains(r.Java, s) {
			t.Fatalf("la SesionFirmas de la edición confía en alguien (%q)\n%s", s, r.Java)
		}
	}
	if strings.Contains(strings.ReplaceAll(r.Java, "package eu.sytue.nativo;", ""), "eu.sytue.") {
		t.Fatalf("la SesionFirmas de la edición nombra un paquete de la conectada\n%s", r.Java)
	}
}

func TestAndroidSesionDeSytueOfficeNuncaAceptaUnBrokerNiUnPin(t *testing.T) {
	var r struct {
		Fallos []*pyRestoExcepcion `json:"fallos"`
	}
	pyRestoCorrer(t, androidSesionCargar+`
apps = {"eu.sytue.drive": ["a" * 64]}
devolver({"fallos": [
    fallo(m.java, True, {}, None, edicion="local"),
    fallo(m.java, False, apps, None, edicion="local"),
    fallo(m.java, False, {}, {"drive": "eu.sytue.drive"}, edicion="local"),
    fallo(m.java, False, {}, edicion="otra"),
]})
`, &r)
	casos := []string{"habilitada", "con pines", "con productos", "edición desconocida"}
	if len(r.Fallos) != len(casos) {
		t.Fatalf("%d resultados para %d casos", len(r.Fallos), len(casos))
	}
	for i, caso := range casos {
		t.Run(caso, func(t *testing.T) { pyRestoLanza(t, r.Fallos[i], "ValueError", "") })
	}
}
