// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"regexp"
	"strconv"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

const rutaPaletaWeb = "apps/frontend/packages/config/src/paleta-usuario.ts"

// El espejo es la paleta de la web, tono a tono y en su orden, y busca el más cercano con
// el mismo peso de la luz: si no, el mismo calendario saldría de un color en cada cliente.
func TestLaPaletaDeUsuarioEsLaDeLaWeb(t *testing.T) {
	web := webref.Read(t, rutaPaletaWeb)
	halladas := regexp.MustCompile(`\{ id: '([a-z]+)', valor: '(#[0-9a-f]{6})' \}`).FindAllStringSubmatch(web, -1)
	tonos := make([]ColorDeUsuario, 0, len(halladas))
	for _, m := range halladas {
		tonos = append(tonos, ColorDeUsuario{m[1], m[2]})
	}
	if len(tonos) != len(PaletaDeUsuario) {
		t.Fatalf("la web tiene %d tonos y el espejo %d", len(tonos), len(PaletaDeUsuario))
	}
	for i, c := range tonos {
		if PaletaDeUsuario[i] != c {
			t.Errorf("tono %d: la web dice %v y el espejo %v", i, c, PaletaDeUsuario[i])
		}
	}
	m := regexp.MustCompile(`const W_LUZ = ([0-9.]+);`).FindStringSubmatch(web)
	if m == nil {
		t.Fatal("la web ya no declara W_LUZ")
	}
	if w, _ := strconv.ParseFloat(m[1], 64); w != pesoLuz {
		t.Errorf("la web pesa la luz %v y el espejo %v", w, pesoLuz)
	}
}

// Los mismos casos que `paleta-usuario.test.ts`: lo guardado con las paletas de antes se
// pinta con el tono que su dueño reconocería.
func TestUnColorGuardadoSePintaConSuTono(t *testing.T) {
	for guardado, quiero := range map[string]string{
		"#a31f38": "red", "#b0470f": "clay", "#94590f": "brass", "#146a47": "green",
		"#0f7052": "teal", "#0e6c84": "cyan", "#1e50a8": "blue", "#2e5a8c": "blue",
		"#4338ca": "indigo", "#6b3e9e": "plum", "#a23e8c": "rose", "#334155": "slate",
		"#86198f": "plum", "#be123c": "red", "#b0413a": "red", "#8a6a1e": "brass",
		"#565b62": "slate", "#5f7a5a": "sage", "#7a5480": "plum", "#9c5470": "rose",
		"#047857": "teal", "#b91c1c": "red", "#4b5563": "slate",
		"#f00": "red", "#00ff00": "green", "#000000": "slate", "#ffffff": "slate",
		// Los tokens de Mail y un tono escrito como lo escribe un cliente CalDAV.
		"slate": "slate", "brass": "brass", "rose": "rose", "#2A72C7FF": "blue",
	} {
		c, ok := TonoDeUsuario(guardado)
		if !ok || c.ID != quiero {
			t.Errorf("TonoDeUsuario(%q) = %q, %v; quiero %q", guardado, c.ID, ok, quiero)
		}
	}
	for _, nada := range []string{"", "rojo", "#12345", "var(--sytue-primary)"} {
		if c, ok := TonoDeUsuario(nada); ok {
			t.Errorf("TonoDeUsuario(%q) = %q; no debería haber tono", nada, c.ID)
		}
	}
	reserva := color.NRGBA{1, 2, 3, 255}
	if PintarDeUsuario("", reserva) != reserva {
		t.Error("sin color no se pinta la reserva")
	}
	if PintarDeUsuario("#2a72c7", reserva) != (color.NRGBA{0x2a, 0x72, 0xc7, 0xff}) {
		t.Error("un tono de la paleta no se pinta tal cual")
	}
}
