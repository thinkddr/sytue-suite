// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidLabelName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Trabajo", true},
		{"Trabajo/Proyectos", true}, // '/' admitido para jerarquía por nombre
		{"  con espacios  ", true},
		{"", false},
		{"   ", false},
		{"con\x00nul", false},
		{"con\ttab", false},
		{"con\nsalto", false},
		{strings.Repeat("x", 128), true},
		{strings.Repeat("x", 129), false},
	}
	for _, c := range cases {
		if got := ValidLabelName(c.name); got != c.want {
			t.Errorf("ValidLabelName(%q) = %v, quiero %v", c.name, got, c.want)
		}
	}
}

func TestValidLabelColor(t *testing.T) {
	if !ValidLabelColor("") {
		t.Error("'' (neutro) debe ser válido")
	}
	for _, c := range LabelColors {
		if !ValidLabelColor(c) {
			t.Errorf("color de paleta %q debe ser válido", c)
		}
	}
	for _, bad := range []string{"neon", "#ff0000", "red; color:evil", "SLATE"} {
		if ValidLabelColor(bad) {
			t.Errorf("color fuera de paleta %q no debe ser válido", bad)
		}
	}
}

// Los tokens que se aceptan son los ids de la paleta de colores de usuario de la web,
// en su orden: si la web estrena un tono y aquí no se añade, el selector de Mail ofrece
// un color que el servicio rechaza al guardar. Go no puede importar TypeScript, así que
// el test lee el fichero y compara.
func TestLabelColorsEsLaPaletaDeUsuario(t *testing.T) {
	ruta := filepath.Join("..", "..", "..", "..", "apps", "frontend", "packages", "config", "src", "paleta-usuario.ts")
	src, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se encuentra %s (%v): ¿se movió de sitio? el guardián habría quedado desactivado", ruta, err)
	}
	tonos := regexp.MustCompile(`\{ id: '([a-z]+)', valor: '#[0-9a-f]{6}' \}`).FindAllStringSubmatch(string(src), -1)
	web := make([]string, 0, len(tonos))
	for _, m := range tonos {
		web = append(web, m[1])
	}
	if !slices.Equal(web, LabelColors) {
		t.Errorf("LabelColors = %v; la paleta de la web es %v", LabelColors, web)
	}
	// Las etiquetas guardadas con la paleta sobria de antes siguen siendo válidas.
	for _, c := range []string{"slate", "brass", "sage", "clay", "blue", "plum", "teal", "rose"} {
		if !ValidLabelColor(c) {
			t.Errorf("el token de antes %q ya no es válido", c)
		}
	}
}

// El keyword de una etiqueta es su id: estable y válido como atom IMAP / keyword
// JMAP (solo [0-9a-f-]), sin caracteres que rompan FLAGS/keywords.
func TestLabelKeyword(t *testing.T) {
	id := uuid.New()
	l := Label{ID: id}
	kw := l.Keyword()
	if kw != id.String() {
		t.Fatalf("Keyword() = %q, quiero %q", kw, id.String())
	}
	if strings.ContainsAny(kw, " ()[]{}%*\"\\") {
		t.Fatalf("keyword %q contiene caracteres no válidos para IMAP/JMAP", kw)
	}
}
