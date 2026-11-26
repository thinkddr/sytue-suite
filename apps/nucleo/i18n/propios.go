// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"fmt"
	"slices"
	"strings"
)

// Propios es una tabla de textos propios de una app o un paquete de la edición (PLAN
// §5.14, C-12): los que no tienen clave en el catálogo de la web. Base lleva el español
// y el inglés, escritos a mano; Traducciones, los otros veinticuatro idiomas de
// `i18n.Locales`, traducidos del inglés. Cada paquete con una tabla así comprueba en sus
// pruebas que `Faltan()` está vacío.
type Propios struct {
	Base         map[string][2]string
	Traducciones map[Locale]map[string]string
}

// En es el texto de la clave en un idioma: el suyo, o el inglés si falta, o la clave si
// no está. Las variantes regionales («pt-BR») usan las de su idioma.
func (p Propios) En(idioma Locale, clave string) string {
	t, ok := p.Base[clave]
	if !ok {
		return clave
	}
	base, _, _ := strings.Cut(string(idioma), "-")
	switch base {
	case "es":
		return t[0]
	case "en":
		return t[1]
	}
	if v, ok := p.Traducciones[Locale(base)][clave]; ok && v != "" {
		return v
	}
	return t[1]
}

// Faltan dice lo que no cuadra: idiomas sin tabla, claves que faltan o sobran en uno,
// textos vacíos y verbos de formato (%s, %d…) que no coinciden con los del inglés.
func (p Propios) Faltan() []string {
	var out []string
	for clave, v := range p.Base {
		if strings.TrimSpace(v[0]) == "" || strings.TrimSpace(v[1]) == "" {
			out = append(out, fmt.Sprintf("%q vacío en español o en inglés", clave))
		}
		if verbos(v[0]) != verbos(v[1]) {
			out = append(out, fmt.Sprintf("%q: el español y el inglés no llevan los mismos huecos", clave))
		}
	}
	for _, loc := range Locales {
		if loc == "es" || loc == "en" || strings.Contains(string(loc), "-") {
			continue
		}
		tr, ok := p.Traducciones[loc]
		if !ok {
			out = append(out, fmt.Sprintf("%s: no hay traducciones", loc))
			continue
		}
		for clave, v := range p.Base {
			t := tr[clave]
			switch {
			case strings.TrimSpace(t) == "":
				out = append(out, fmt.Sprintf("%s: falta %q", loc, clave))
			case verbos(t) != verbos(v[1]):
				out = append(out, fmt.Sprintf("%s: %q no lleva los huecos del inglés", loc, clave))
			}
		}
		for clave := range tr {
			if _, ok := p.Base[clave]; !ok {
				out = append(out, fmt.Sprintf("%s: sobra %q", loc, clave))
			}
		}
	}
	for loc := range p.Traducciones {
		if loc == "es" || loc == "en" || !IsLocale(string(loc)) {
			out = append(out, fmt.Sprintf("traducciones de %q, que no es un idioma más de la suite", loc))
		}
	}
	slices.Sort(out)
	return out
}

// verbos son los verbos de formato de un texto, en orden («%s %d»).
func verbos(s string) string {
	var b strings.Builder
	for i := 0; i < len(s)-1; i++ {
		if s[i] != '%' {
			continue
		}
		if s[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && strings.IndexByte("+-# 0123456789.[]", s[j]) >= 0 {
			j++
		}
		if j < len(s) {
			b.WriteString(s[i : j+1])
			b.WriteByte(' ')
		}
		i = j
	}
	return b.String()
}
