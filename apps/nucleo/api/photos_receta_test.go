// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "testing"

// Los vectores los imprimió services/syphotos/internal/domain el 2026-09-08 y son los
// mismos de editor/receta.test.ts: si alguno cambia, la app nombra derivadas distintas
// que el servicio.
func TestLaHuellaEsLaDeGo(t *testing.T) {
	completa := Receta{Version: 1, Pasos: []PasoReceta{
		{Tipo: PasoRecorte, Params: map[string]float64{"x": 0.1, "y": 0.2, "ancho": 0.5, "alto": 0.25}},
		{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}},
		{Tipo: PasoEspejo, Eje: "h"},
		{Tipo: PasoTemperatura, Params: map[string]float64{"valor": -0.333333333}},
	}}
	exp := Receta{Version: 1, Pasos: []PasoReceta{{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0.5}}}}
	for _, c := range []struct {
		r                Receta
		canonica, huella string
	}{
		{RecetaVacia(), `{"pasos":[],"version":1}`, "ec5adbfaaa1f"},
		{exp, `{"pasos":[{"params":{"valor":"0.500000"},"tipo":"exposicion"}],"version":1}`, "53600d98c18e"},
		{completa, `{"pasos":[{"params":{"alto":"0.250000","ancho":"0.500000","x":"0.100000","y":"0.200000"},"tipo":"recorte"},{"params":{"grados":"90.000000"},"tipo":"giro"},{"eje":"h","tipo":"espejo"},{"params":{"valor":"-0.333333"},"tipo":"temperatura"}],"version":1}`, "1119c7c11660"},
	} {
		if got := c.r.Canonica(); got != c.canonica {
			t.Errorf("canónica:\n  %s\ny Go da\n  %s", got, c.canonica)
		}
		if got := c.r.Huella(); got != c.huella {
			t.Errorf("huella %s y Go da %s", got, c.huella)
		}
	}
}
