// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pagos

import (
	"errors"
	"testing"
)

// Cada campo que Verificar compara, cambiado de uno en uno, tumba la verificación; y
// lo esperado con huecos no casa con un estado que tenga los mismos huecos.
func TestVerificarExigeCadaCampo(t *testing.T) {
	esperado := Esperado{Pago: "pay-1", Importe: 2910, Modo: ModoReal, Comercio: "pfl_abc"}
	bueno := Estado{
		Estado: EstadoPagado, Importe: 2910, Divisa: DivisaEUR, Modo: ModoReal, Comercio: "pfl_abc",
		Metadatos: map[string]string{MetadatoPago: "pay-1"},
	}
	if err := Verificar(esperado, bueno); err != nil {
		t.Fatalf("el pago esperado no verifica: %v", err)
	}
	casos := map[string]func(e *Estado){
		"importe":   func(e *Estado) { e.Importe = 2909 },
		"divisa":    func(e *Estado) { e.Divisa = "USD" },
		"metadatos": func(e *Estado) { e.Metadatos = map[string]string{MetadatoPago: "pay-2"} },
		"sin id":    func(e *Estado) { e.Metadatos = nil },
		"modo":      func(e *Estado) { e.Modo = ModoPrueba },
		"comercio":  func(e *Estado) { e.Comercio = "pfl_otro" },
	}
	for nombre, cambiar := range casos {
		e := bueno
		cambiar(&e)
		if err := Verificar(esperado, e); !errors.Is(err, ErrNoCasa) {
			t.Errorf("%s: un pago distinto verifica (%v)", nombre, err)
		}
	}

	vacio := Estado{Divisa: DivisaEUR, Metadatos: map[string]string{MetadatoPago: ""}}
	for nombre, esp := range map[string]Esperado{
		"sin pago":     {Importe: 0},
		"sin comercio": {Pago: "pay-1", Importe: 2910, Modo: ModoReal},
		"sin modo":     {Pago: "pay-1", Importe: 2910, Comercio: "pfl_abc"},
	} {
		e := vacio
		e.Importe, e.Modo, e.Comercio = esp.Importe, esp.Modo, esp.Comercio
		e.Metadatos = map[string]string{MetadatoPago: esp.Pago}
		if err := Verificar(esp, e); !errors.Is(err, ErrNoCasa) {
			t.Errorf("%s: lo esperado con huecos casa con un estado igual de vacío (%v)", nombre, err)
		}
	}
}
