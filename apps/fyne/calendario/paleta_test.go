// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las muestras de un calendario son la paleta de colores de usuario, con el nombre del
// catálogo común, y un calendario guardado con un tono de la paleta de antes (el granate
// #a31f38) se ve marcado y pintado en su tono más cercano, el rojo. Visto en ROJO con los
// catorce tonos joya: catorce muestras con nombres de Calendar y el granate, tal cual.
func TestLasMuestrasSonLaPaletaDeUsuario(t *testing.T) {
	test.NewTempApp(t)
	g := nuevasMuestras(&Pantalla{}, "#a31f38", false, nil)
	if len(g.muestras) != len(tokens.PaletaDeUsuario) {
		t.Fatalf("%d muestras y la paleta tiene %d", len(g.muestras), len(tokens.PaletaDeUsuario))
	}
	for i, m := range g.muestras {
		if m.nombre != tokens.PaletaDeUsuario[i].Clave() {
			t.Errorf("muestra %d: nombre %q", i, m.nombre)
		}
	}
	if g.elegida < 0 || g.muestras[g.elegida].hex != "#c44744" {
		t.Errorf("el granate de antes no marca el rojo (elegida %d)", g.elegida)
	}
	if got, quiero := colorDe("#a31f38"), tokens.PaletaDeUsuario[0].NRGBA(); got != quiero {
		t.Errorf("el granate de antes se pinta %v y no %v", got, quiero)
	}
	if colorDe("") != AcentoPorDefecto {
		t.Error("sin color no se pinta el acento por defecto")
	}
}
