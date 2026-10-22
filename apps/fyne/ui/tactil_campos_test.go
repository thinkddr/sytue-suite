// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "testing"

// TestConElDedoLosCamposMidenComoUnBoton: el `pointer-coarse:min-h-[max(44px,2.75rem)]`
// de Form.tsx. Con el dedo, el campo, el selector, la casilla y el interruptor con
// etiqueta miden 44, el suelo del botón; con el ratón, lo de siempre, y `sm` (cromo
// denso) no cambia. Medido en el barrido de diálogos (2026-10-16): 36 y 22 al lado de
// botones de 44 en todas las hojas con formulario.
func TestConElDedoLosCamposMidenComoUnBoton(t *testing.T) {
	medir := func() map[string]float32 {
		return map[string]float32{
			"campo":       NuevaEntrada("Título").MinSize().Height,
			"selector":    NuevoSelector([]string{"Trabajo", "Personal"}, nil).MinSize().Height,
			"casilla":     NuevaCasilla("Todo el día", nil).MinSize().Height,
			"radio":       NuevoRadio("Solo este", nil).MinSize().Height,
			"interruptor": NuevoConmutador("Avisarme", nil).MinSize().Height,
		}
	}
	raton := medir()
	if raton["campo"] >= SueloTactilBarra || raton["casilla"] >= SueloTactilBarra {
		t.Fatalf("con el ratón ya miden 44 o más (%v): el test no prueba nada", raton)
	}
	telefono(t)
	for nombre, alto := range medir() {
		if alto != SueloTactilBarra {
			t.Errorf("con el dedo, %s mide %v y debe medir %v", nombre, alto, SueloTactilBarra)
		}
	}
	denso := NuevaEntrada("Buscar")
	denso.Talla = CampoSM
	if h := denso.MinSize().Height; h >= SueloTactilBarra {
		t.Errorf("el campo `sm` (cromo denso) mide %v con el dedo y se queda en su alto", h)
	}
}
