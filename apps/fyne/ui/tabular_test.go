// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElContadorLlevaCifrasTabulares es el `tabular-nums` de `<Badge count>`: medido en
// Chromium a raíz de 16px (2026-10-03), «3» mide 18 (el suelo de
// `min-w-[18px]`), «12» 23,109 y «99+» 30,656. En proporcional, «12» se quedaba en el suelo de 18.
func TestElContadorLlevaCifrasTabulares(t *testing.T) {
	for _, c := range []struct {
		texto string
		web   float32
	}{{"3", 18}, {"12", 23.109}, {"99+", 30.656}} {
		if got := MedidaContador(c.texto).Width; got < c.web-0.05 || got > c.web+0.05 {
			t.Errorf("el contador «%s» mide %v de ancho y la web %v", c.texto, got, c.web)
		}
	}
}

// TestLasCifrasTabularesSonLasDeLaFuente: el `tnum` de Figtree, no el ancho del dígito más
// ancho (lo que hacía Calendar): «11:11» y «08:00» miden lo mismo, y cada cifra lo que la
// fuente dice, que en SemiBold a 12px son 7,562 en Chromium..
func TestLasCifrasTabularesSonLasDeLaFuente(t *testing.T) {
	if a, b := MedirTabular("11:11", tema.TextoSM, Normal), MedirTabular("08:00", tema.TextoSM, Normal); a != b {
		t.Errorf("«11:11» mide %v y «08:00» %v con `tabular-nums`", a, b)
	}
	if got := MedirTabular("0", tema.TextoXS, SemiNegrita); got < 7.532 || got > 7.592 {
		t.Errorf("la cifra tabular mide %v y Chromium 7,562", got)
	}
}

// TestElPomoDelConmutadorLlevaSuSombra compara con Chromium a raíz de 16px
// (2026-10-04, ×2, con la paleta petróleo): bajo el pomo apagado el carril
// #e8efef se oscurece a #e3eaea con `shadow-xs`. Sin sombra, la fila conserva
// el color del carril.
func TestElPomoDelConmutadorLlevaSuSombra(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	c := NuevoConmutador("", nil)
	img := colocado(t, c, fyne.NewSize(AnchoCarril, AltoCarril), 6)
	x := int((6 + MargenPomo + LadoPomo/2) * escala)
	y := int((6+MargenPomo+LadoPomo)*escala) + 1
	got, quiero := px(img, x, y), hex(0xe3eaea)
	if !cerca(got, quiero) {
		t.Errorf("bajo el pomo el carril es %v; Chromium pinta %v con shadow-xs", got, quiero)
	}
}
