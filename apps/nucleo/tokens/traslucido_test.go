// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"strings"
	"testing"
)

func TestLaAparienciaTraslucidaNoHeredaElOscuro(t *testing.T) {
	if Light != 0 || Dark != 1 || Translucent != 2 {
		t.Fatal("la tercera apariencia ha cambiado los valores de claro/oscuro")
	}
	const css = `:root {
 --sytue-bg: #e4e4e4;
 --sytue-surface: #ffffff;
 --sytue-text: #10141a;
 --sytue-ink-rgb: 16, 20, 26;
 --sytue-border: rgba(var(--sytue-ink-rgb), 0.5);
 --sytue-alias: var(--sytue-surface);
 }
 [data-theme='dark'] {
 --sytue-bg: #10141a;
 --sytue-text: #e4e4e4;
 --sytue-ink-rgb: 228, 228, 228;
 }
 [data-theme='translucent'] {
 --sytue-surface: rgba(255, 255, 255, 0.75);
 --sytue-ink-rgb: 40, 60, 80;
 }`
	raw, err := parseCrudas(css)
	if err != nil {
		t.Fatal(err)
	}
	for token, want := range map[string]color.NRGBA{
		"bg":      {228, 228, 228, 255},
		"text":    {16, 20, 26, 255},
		"surface": {255, 255, 255, 191},
		"alias":   {255, 255, 255, 191},
		"border":  {40, 60, 80, 128},
	} {
		if got := raw[Translucent][token]; got != want {
			t.Errorf("traslúcido %s = %v; contratoCSS %v", token, got, want)
		}
	}
	if raw[Light]["surface"] != (color.NRGBA{255, 255, 255, 255}) {
		t.Error("la reasignación traslúcida ha modificado la paleta clara")
	}
}

func TestSinBloqueTraslucidoNoSeDaLaTerceraAparienciaPorBuena(t *testing.T) {
	const css = `:root { --sytue-bg: #ffffff; }
 [data-theme='dark'] { --sytue-bg: #10141a; }`
	if _, err := parseCrudas(css); err == nil {
		t.Error("sin bloqueCSS traslúcido se ha inventado una tercera paleta")
	}
}

// Valores literales de [data-theme='translucent'] en la hoja canónica. Estos
// vectores no pasan por expandir ni por el aplanado que están comprobando.
func TestLaPaletaTraslucidaConservaLasCapasDeLaWeb(t *testing.T) {
	for token, want := range map[string]color.NRGBA{
		"bg":             {208, 228, 225, 255},
		"surface":        {255, 255, 255, 168},
		"surface-raised": {247, 251, 251, 255},
		"text":           {15, 29, 32, 255},
		"sidebar-bg":     {9, 84, 92, 235}, // el carril, de appearance.css: rgba(9, 84, 92, 0.92)
		"header-bg":      {255, 255, 255, 107},
		"card-blue":      {36, 87, 232, 41},
		"card-violet":    {101, 70, 197, 43},
		"card-mint":      {37, 118, 91, 43},
		"card-rose":      {182, 64, 104, 41},
		"card-amber":     {234, 214, 65, 66},
		"border-subtle":  {15, 29, 32, 15},
		"border":         {15, 29, 32, 31},
		"border-strong":  {15, 29, 32, 158},
		"overlay":        {15, 29, 32, 92},
	} {
		if got := Crudo(token, Translucent); got != want {
			t.Errorf("traslúcido %s = %v; literal CSS %v", token, got, want)
		}
	}
}

// El filete opaco va sobre la superficie ya compuesta, no sobre el blanco
// ignorando su transparencia. El velo y la superficie conservan su alfa.
func TestElBordeTraslucidoSeComponeSobreSuBase(t *testing.T) {
	const css = `:root {
 --sytue-bg: #e4e4e4;
 --sytue-surface: #ffffff;
 --sytue-border: rgba(0, 0, 0, 0.5);
 --sytue-overlay: rgba(0, 0, 0, 0.4);
 }
 [data-theme='dark'] { --sytue-bg: #10141a; }
 [data-theme='translucent'] { --sytue-surface: rgba(255, 255, 255, 0.5); }`
	palette, err := parsePaletas(css)
	if err != nil {
		t.Fatal(err)
	}
	if got := palette[Translucent]["border"]; got != (color.NRGBA{121, 121, 121, 255}) {
		t.Errorf("el borde es %v; negro50%% sobre superficie blanca50%%/gris228 da121", got)
	}
	if palette[Translucent]["surface"].A != 128 || palette[Translucent]["overlay"].A != 102 {
		t.Error("aplanar el filete ha borrado el alfa de las capas")
	}
}

func TestLasComillasDelSelectorNoCambianLasApariencias(t *testing.T) {
	simple, err := parseCrudas(coloresCSS)
	if err != nil {
		t.Fatal(err)
	}
	doble, err := parseCrudas(strings.ReplaceAll(coloresCSS, "'", `"`))
	if err != nil {
		t.Fatal(err)
	}
	for variante := range simple {
		for token, want := range simple[variante] {
			if got := doble[variante][token]; got != want {
				t.Errorf("variante%d token%s cambia con las comillasCSS: %v / %v", variante, token, got, want)
			}
		}
	}
}

// TestLosDialogosTraslucidosSonOpacos: el panel de los diálogos (`panel-surface`, el de
// Modal en la web y ui.Dialogo en Fyne) era un blanco al 80 % y en el modo traslúcido
// dejaba ver lo de detrás, así que costaba leer y usar un formulario. Ha de ser opaco
// en los dos colores de la suite, como los menús.
//
// VISTO EN ROJO con el `rgba(255, 255, 255, 0.8)` de antes: alfa 204.
func TestLosDialogosTraslucidosSonOpacos(t *testing.T) {
	t.Cleanup(func() { ElegirColor("") })
	for _, c := range []string{"", "cobalto"} {
		ElegirColor(c)
		for _, token := range []string{"panel-surface", "menu-surface"} {
			if got := Crudo(token, Translucent); got.A != 255 {
				t.Errorf("color %q: %s traslúcido = %v; un diálogo ha de tapar lo de detrás", c, token, got)
			}
		}
	}
}
