// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"strings"
	"testing"
)

// TestMasContrasteResuelveElBloqueDeLaHoja: con «más contraste» (el `prefers-contrast:
// more` de la web) los tokens salen del bloque de la hoja, en cada color, forma y tema:
// todo borde es el contorno de control, el texto sutil es el atenuado, el velo de los
// diálogos tapa más que el de siempre y, en el traslúcido, la superficie, el carril y la
// hoja de Marco son opacos. Sin pedirlo, la paleta es la de siempre.
//
// Visto en ROJO sin apilar el bloque (ElegirMasContraste no cambia nada) y con el
// traslúcido apilado en todos los temas (el claro se queda con la superficie elevada).
func TestMasContrasteResuelveElBloqueDeLaHoja(t *testing.T) {
	t.Cleanup(func() { ElegirMasContraste(false); ElegirColor(""); ElegirForma(""); ElegirApp("") })
	temas := map[string]Variant{"claro": Light, "oscuro": Dark, "traslúcido": Translucent}
	for _, color := range []string{"", "cobalto", ColorDeCadaApp} {
		for _, forma := range []string{"", FormaCarril} {
			ElegirColor(color)
			ElegirApp("mail")
			ElegirForma(forma)
			ElegirMasContraste(false)
			normal := vigente()
			if !ElegirMasContraste(true) {
				t.Fatalf("%q/%q: ElegirMasContraste(true) no cambia nada", color, forma)
			}
			if !EsMasContraste() {
				t.Fatal("EsMasContraste no dice lo elegido")
			}
			for nombre, v := range temas {
				donde := color + "/" + forma + "/" + nombre
				for _, borde := range []string{"border", "border-subtle"} {
					if Color(borde, v) != Color("border-strong", v) {
						t.Errorf("%s: --sytue-%s = %v, no el contorno de control %v", donde, borde, Color(borde, v), Color("border-strong", v))
					}
				}
				if Color("text-subtle", v) != Color("text-muted", v) {
					t.Errorf("%s: el texto sutil no es el atenuado", donde)
				}
				if a, antes := Crudo("overlay", v).A, normal.crudas[v]["overlay"].A; a <= antes {
					t.Errorf("%s: el velo no tapa más (alfa %d, antes %d)", donde, a, antes)
				}
				if normal.paletas[v]["border"] == normal.paletas[v]["border-strong"] {
					t.Errorf("%s: sin pedirlo, el borde ya era el contorno de control", donde)
				}
				opacos := []string{"surface", "bg-subtle", "topbar-bg", "header-bg", "sidebar-bg", "rail"}
				if forma == "" {
					opacos = append(opacos, "sheet-bg")
				}
				for _, n := range opacos {
					c, antes := Crudo(n, v), normal.crudas[v][n]
					if v == Translucent && c.A != 0xff {
						t.Errorf("%s: --sytue-%s sigue siendo velo (alfa %d)", donde, n, c.A)
					}
					if v != Translucent && c != antes {
						t.Errorf("%s: --sytue-%s cambia fuera del traslúcido: %v → %v", donde, n, antes, c)
					}
				}
			}
		}
	}
}

// TestElBloqueDeMasContrasteEstaEnLaHoja: las puertas que lee el núcleo son las que
// escribe la hoja, dentro de su `@media`. Si alguien cambia el selector en branding/, los
// bloques dejarían de apilarse en silencio (apilarBloque no se queja de lo que no está).
func TestElBloqueDeMasContrasteEstaEnLaHoja(t *testing.T) {
	for hoja, aperturas := range map[string][]string{
		"colors.css":     {masContraste, masContrasteTraslucido},
		"appearance.css": {masContrasteTraslucido, masContrasteMarcoTraslucido},
	} {
		css := normalizar(map[string]string{"colors.css": coloresCSS, "appearance.css": aparienciaCSS}[hoja])
		for _, a := range aperturas {
			i := strings.Index(css, a)
			if i < 0 {
				t.Errorf("%s no trae el bloque %q", hoja, a)
				continue
			}
			media := strings.LastIndex(css[:i], "@media")
			if media < 0 || !strings.HasPrefix(css[media:], "@media (prefers-contrast: more) {") {
				t.Errorf("%s: el bloque %q no va dentro de `@media (prefers-contrast: more)`", hoja, a)
			}
		}
	}
}
