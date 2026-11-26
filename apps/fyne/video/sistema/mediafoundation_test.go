// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/video/sistema/mediafoundation"
)

// El lector de Media Foundation entrega el PCM al muestreo de la tarjeta del
// Reproductor (mono a 48 kHz): si uno cambia, el otro también.
func TestMediaFoundationEntregaAlMuestreoDelReproductor(t *testing.T) {
	if mediafoundation.Muestreo != llamada.Muestreo {
		t.Fatalf("Media Foundation entrega a %d Hz y el Reproductor suena a %d", mediafoundation.Muestreo, llamada.Muestreo)
	}
}
