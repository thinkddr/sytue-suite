// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

var (
	reSombra = regexp.MustCompile(`--shadow-(xs|sm|md|lg|xl):\s*([^;]+);`)
	reCapa   = regexp.MustCompile(`^(-?[\d.]+)(?:px)? (-?[\d.]+)(?:px)? ([\d.]+)(?:px)? (rgba\([^)]+\))$`)
)

// sombrasDe lee las `--shadow-*` del primer bloque que abre `apertura` en theme.css.
func sombrasDe(t *testing.T, css, apertura string) map[string][]CapaSombra {
	t.Helper()
	i := strings.Index(css, apertura)
	if i < 0 {
		t.Fatalf("theme.css no tiene el bloque %q", apertura)
	}
	cuerpo := css[i+len(apertura):]
	cuerpo = cuerpo[:strings.IndexByte(cuerpo, '}')]
	out := map[string][]CapaSombra{}
	for _, m := range reSombra.FindAllStringSubmatch(cuerpo, -1) {
		var capas []CapaSombra
		for _, c := range strings.Split(m[2], "),") {
			c = strings.TrimSpace(c)
			if !strings.HasSuffix(c, ")") {
				c += ")"
			}
			p := reCapa.FindStringSubmatch(c)
			if p == nil {
				t.Fatalf("--shadow-%s: no sé leer %q", m[1], c)
			}
			f := func(s string) float32 { v, _ := strconv.ParseFloat(s, 32); return float32(v) }
			col, ok := tokens.ParseColor(p[4])
			if !ok {
				t.Fatalf("--shadow-%s: color ilegible %q", m[1], p[4])
			}
			capas = append(capas, CapaSombra{DesplX: f(p[1]), DesplY: f(p[2]), Desenfoque: f(p[3]), Color: col})
		}
		out[m[1]] = capas
	}
	return out
}

// TestLasSombrasSonLasDeLaWeb lee las cinco elevaciones de `@sytue/config/theme.css` en
// claro y en oscuro —por sus DOS puertas, como los colores en
// `tokens.TestLosTokensSonLosDeLaWeb`— y las compara con las que pinta `ui.Sombra`.
//
// Visto en ROJO el 2026-09-27, sin sabotear nada: theme.css solo endurecía las sombras
// con `[data-theme='dark']`, y quien seguía el tema del SISTEMA veía en la web las del
// claro (marina al 12 %) sobre fondo casi negro, mientras el escritorio pintaba negro al
// 50 %. Se arregló en la hoja, que es la que se contradecía.
func TestLasSombrasSonLasDeLaWeb(t *testing.T) {
	css := strings.ReplaceAll(webref.Read(t, "apps/frontend/packages/config/src/theme.css"), `"`, "'")
	claro := sombrasDe(t, css, "@theme {")
	niveles := []string{"xs", "sm", "md", "lg", "xl"}
	comparar := func(que string, web map[string][]CapaSombra, v Variante) {
		for n, nombre := range niveles {
			w, ok := web[nombre]
			if !ok {
				w = claro[nombre] // lo que el bloque no redefine se hereda del claro
			}
			got := Sombra(NivelSombra(n), v)
			if len(got) != len(w) {
				t.Errorf("%s --shadow-%s: la web tiene %d capas y Fyne %d", que, nombre, len(w), len(got))
				continue
			}
			for i := range w {
				if got[i] != w[i] {
					t.Errorf("%s --shadow-%s capa %d: Fyne %+v, web %+v", que, nombre, i, got[i], w[i])
				}
			}
		}
	}
	comparar("claro", claro, Claro)
	comparar("oscuro elegido", sombrasDe(t, css, "[data-theme='dark'] {"), Oscuro)
	comparar("oscuro del sistema", sombrasDe(t, css, ":root:not([data-theme]) {"), Oscuro)
}
