// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"testing"
)

func TestHashtags(t *testing.T) {
	casos := []struct {
		texto string
		want  []string
	}{
		{"Llamar a #Clientes y cerrar #plan-2026 (#urgente)", []string{"Clientes", "plan-2026", "urgente"}},
		{"#reunión\n#日本 al empezar línea", []string{"reunión", "日本"}},
		// Lo que NO es un hashtag: anclas, entidades, números, colores, títulos de Markdown.
		{"ver https://sitio.eu/#ancla y texto#pegado", nil},
		{"it&#39;s el #1 con &#x27;comillas&#x27;", nil},
		{"fondo #ff0000 y ## Título", nil},
		// Un color sin cifras es una palabra, y el guion final no es de la etiqueta.
		{"la #decade del #plan- y #b2b", []string{"decade", "plan", "b2b"}},
		{"#a #a", []string{"a", "a"}},
	}
	for _, c := range casos {
		if got := Hashtags(c.texto); !slices.Equal(got, c.want) {
			t.Errorf("Hashtags(%q) = %q, se esperaba %q", c.texto, got, c.want)
		}
	}
}

func TestHashtagDeConsulta(t *testing.T) {
	for q, want := range map[string]string{
		" #plan ":       "plan",
		"#plan-2026":    "plan-2026",
		"#plan urgente": "",
		"#plan #otro":   "",
		"plan":          "",
		"#":             "",
		"#ff0000":       "",
	} {
		got, ok := HashtagDeConsulta(q)
		if got != want || ok != (want != "") {
			t.Errorf("HashtagDeConsulta(%q) = %q, %v; se esperaba %q", q, got, ok, want)
		}
	}
}
