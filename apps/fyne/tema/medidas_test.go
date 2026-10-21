// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"regexp"
	"strconv"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestLaRaizEsLaDeLaWeb(t *testing.T) {
	css := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	raiz := regexp.MustCompile(`(?s):where\(html\)\s*\{(.*?)\}`).FindStringSubmatch(css)
	if raiz == nil {
		t.Fatal("theme.css no declara la base html de la suite")
	}
	declaracion := regexp.MustCompile(`font-size:\s*([0-9.]+)px;`).FindStringSubmatch(raiz[1])
	if declaracion == nil {
		t.Fatal("theme.css no declara la raíz en px; revisar Rem antes de cambiar su unidad")
	}
	px, err := strconv.ParseFloat(declaracion[1], 32)
	if err != nil || float32(px) != Rem {
		t.Fatalf("la raíz web es %s px y Fyne usa %v", declaracion[1], Rem)
	}
}

func TestLosRadiosSonLosDeLaWeb(t *testing.T) {
	css := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	for nombre, radio := range map[string]float32{
		"sm": RadioSM, "md": RadioMD, "lg": RadioLG,
		"xl": RadioXL, "2xl": Radio2XL, "pill": RadioPill,
	} {
		propiedad := "--radius-" + nombre
		declaracion := regexp.MustCompile(regexp.QuoteMeta(propiedad) + `:\s*([0-9.]+)px;`).FindStringSubmatch(css)
		if declaracion == nil {
			t.Fatalf("theme.css no declara %s en px", propiedad)
		}
		px, err := strconv.ParseFloat(declaracion[1], 32)
		if err != nil || float32(px) != radio {
			t.Errorf("%s: web %s px, Fyne %v; revisar branding/tokens/spacing.json y sync-branding", propiedad, declaracion[1], radio)
		}
	}
}

// TestLaEscalaEsLaExactaDeLaWeb ata cada peldaño de la escala tipográfica al valor que
// la web calcula desde rem. Por IGUALDAD, no por proximidad: en cuanto se admite un
// margen, alguien redondea «solo un poquito» y el error se hereda en las dieciséis apps.
//
// Los pares rem→px son los de theme.css escritos a mano: si salieran de `escalon`, el
// test se comprobaría a sí mismo.
func TestLaEscalaEsLaExactaDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		nombre     string
		e          Escalon
		rem, linea float32 // lo que dice theme.css
	}{
		{"text-2xs", Texto2XS, 0.6875, 1},
		{"text-xs", TextoXS, 0.75, 1},
		{"text-sm", TextoSM, 0.8125, 1.125},
		{"text-base", TextoBase, 0.875, 1.375},
		{"text-md", TextoMD, 1, 1.5},
		{"text-lg", TextoLG, 1.125, 1.625},
		{"text-xl", TextoXL, 1.375, 1.875},
		{"text-2xl", Texto2XL, 1.75, 2.25},
		{"text-3xl", Texto3XL, 2.25, 2.625},
		{"text-4xl", Texto4XL, 3, 3.25},
	} {
		if quiero := c.rem * Rem; c.e.Tam != quiero {
			t.Errorf("%s mide %v y en la web son %v px (%.4f rem × %v)", c.nombre, c.e.Tam, quiero, c.rem, Rem)
		}
		if quiero := c.linea * Rem; c.e.Linea != quiero {
			t.Errorf("%s: interlineado %v y en la web %v px", c.nombre, c.e.Linea, quiero)
		}
	}
	// El texto sin clase hereda la raíz de 16px; text-base pide explícitamente 14px.
	if TextoRaiz.Tam != 16 || TextoRaiz.Linea != 22 {
		t.Errorf("el texto sin clase es %v/%v y la web declara 16 px con caja de 22", TextoRaiz.Tam, TextoRaiz.Linea)
	}
}

// El rem de 16 deja enteros los peldaños tipográficos actuales, pero Tailwind admite
// pasos fraccionarios. Esos medios y cuartos de píxel siguen llegando al pintor.
func TestLaEscalaConservaSusDecimales(t *testing.T) {
	for paso, quiero := range map[float32]float32{0.125: 0.5, 0.3125: 1.25} {
		if got := Espacio(paso); got != quiero {
			t.Errorf("Espacio(%v) = %v, quiero %v; no redondear antes de pintar", paso, got, quiero)
		}
	}
}

// TestElEspaciadoEsElDeTailwind comprueba la escala de espaciado contra los pasos que
// más se usan en la suite, calculados a mano desde `0.25rem` con el rem de 16.
func TestElEspaciadoEsElDeTailwind(t *testing.T) {
	for paso, quiero := range map[float32]float32{
		1: 4, 2: 8, 2.5: 10, 3: 12, 4: 16, 5: 20, 6: 24, 8: 32, 9: 36, 11: 44, 12: 48,
	} {
		if got := Espacio(paso); got != quiero {
			t.Errorf("Espacio(%v) = %v, quiero %v", paso, got, quiero)
		}
	}
}
