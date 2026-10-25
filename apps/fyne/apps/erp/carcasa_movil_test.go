// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestElContenidoMovilReservaElCarrilSinCambiarLosCortes(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, ventana := range []float32{320, 390, 640, 767} {
			ocupado := float32(0)
			if forma == "marco" {
				ocupado = ui.AnchoCarrilMovil
			}
			l := &Lista{Ventana: ventana}
			if got, want := l.anchoMain(), ventana-ocupado; got != want {
				t.Fatalf("%s/%g: main=%g, quiero %g", forma, ventana, got, want)
			}
			if got, want := anchoUtilDe(ventana, 1280), ventana-ocupado-2*MargenLista; got != want {
				t.Fatalf("%s/%g: contenido=%g, quiero %g", forma, ventana, got, want)
			}
			if !l.esMovil() || l.haySM() != (ventana >= ui.CorteSM) {
				t.Fatal("el carril cambió el breakpoint de la ventana")
			}
		}
	}
}
