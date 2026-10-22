// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"testing"

	temafyne "fyne.io/fyne/v2/theme"
)

func TestLasTresAparienciasMandanSobreElSistema(t *testing.T) {
	if Claro != 0 || Oscuro != 1 || Translucido != 2 {
		t.Fatalf("las variantes han cambiado de valor: %v, %v, %v", Claro, Oscuro, Translucido)
	}
	t.Cleanup(func() { Elegir("") })
	for _, c := range []struct {
		nombre string
		v      Variante
	}{{"claro", Claro}, {"oscuro", Oscuro}, {"traslucido", Translucido}} {
		t.Run(c.nombre, func(t *testing.T) {
			Elegir("")
			if !Elegir(c.nombre) {
				t.Fatal("una apariencia nueva tiene que pedir repintar")
			}
			if De(temafyne.VariantLight) != c.v || De(temafyne.VariantDark) != c.v {
				t.Errorf("%q no manda sobre ambas variantes del sistema", c.nombre)
			}
			if Elegir(c.nombre) {
				t.Error("repetir la misma elección no tiene que pedir repintar")
			}
			if !Elegir("") {
				t.Error("volver a Sistema tiene que pedir repintar")
			}
			if De(temafyne.VariantLight) != Claro || De(temafyne.VariantDark) != Oscuro {
				t.Error("Sistema no sigue la variante de Fyne")
			}
		})
	}
}

func TestUnaAparienciaDesconocidaConservaElRespaldoDelSistema(t *testing.T) {
	t.Cleanup(func() { Elegir("") })
	Elegir("traslucido")
	if !Elegir("desconocido") || De(temafyne.VariantDark) != Oscuro || De(temafyne.VariantLight) != Claro {
		t.Error("una preferencia desconocida tiene que devolver el mando al sistema")
	}
}
