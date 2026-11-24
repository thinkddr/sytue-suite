// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/url"
	"testing"
)

// TestUnEnlaceSinGaleriaEsperaASuMontaje: la fila de un álbum o «Añadir fotos» del
// widget pueden arrancar la app, y llegan antes de que la galería esté montada. Se
// guardan y se atienden al montarla (antes se perdían); lo que no es de Photos, no.
func TestUnEnlaceSinGaleriaEsperaASuMontaje(t *testing.T) {
	for enlace, guardado := range map[string]bool{
		"sytue-photos:/?action=upload":   true,
		"sytue-photos:/album?id=al1":     true,
		"sytue-photos:/?action=create":   false,
		"sytue-photos:/ajustes?id=al1":   false,
		"sytue-photos:/?action=upload&x": false,
	} {
		u, err := url.Parse(enlace)
		if err != nil {
			t.Fatal(err)
		}
		host := &photosStorage{}
		host.atender(u)
		if got := host.pendiente.Load(); (got != nil) != guardado {
			t.Errorf("%s: guardado %v, quiero %v", enlace, got != nil, guardado)
		}
	}
}
