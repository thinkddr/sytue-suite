// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"reflect"
	"testing"
)

func linea(t *testing.T, cantidad string, precio int64, tipo string) LineaFactura {
	t.Helper()
	l, err := LineaDe(map[string]any{"descripcion": "x", "cantidad": cantidad, "precio_con_iva": float64(precio), "tipo_iva": tipo})
	if err != nil {
		t.Fatalf("LineaDe(%s × %d al %s): %v", cantidad, precio, tipo, err)
	}
	return l
}

func TestDesgloseAgrupaPorTipoDeMayorAMenor(t *testing.T) {
	got := Desglose([]LineaFactura{
		linea(t, "3", 250, "4"),   // pan: 7,50 €
		linea(t, "1", 1210, "21"), // vino: 12,10 €
		linea(t, "2", 550, "10"),  // 11,00 € de comida
		linea(t, "1", 490, "21"),  // envío: 4,90 €
	})
	want := []GrupoIVA{
		{Tipo: 21, Total: 1700, Base: 1405, Cuota: 295}, // 17,00 / 1,21 = 14,049…
		{Tipo: 10, Total: 1100, Base: 1000, Cuota: 100},
		{Tipo: 4, Total: 750, Base: 721, Cuota: 29}, // 7,50 / 1,04 = 7,211…
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("desglose\n got %+v\nwant %+v", got, want)
	}
}

// El IVA se redondea por grupo, no por línea: siete líneas de 3 céntimos al 21 %
// darían 7 céntimos de cuota sumando línea a línea (1 cada una) y son 4.
func TestDesgloseRedondeaPorGrupo(t *testing.T) {
	lineas := make([]LineaFactura, 0, 7)
	for range 7 {
		lineas = append(lineas, linea(t, "1", 3, "21"))
	}
	got := Desglose(lineas)
	if want := []GrupoIVA{{Tipo: 21, Total: 21, Base: 17, Cuota: 4}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestImporteDeLaLineaAlCentimo(t *testing.T) {
	for _, c := range []struct {
		cantidad string
		precio   int64
		importe  int64
	}{
		{"2", 1210, 2420},
		{"1.5", 199, 299},   // 298,5 → 299: la mitad, lejos del cero
		{"-1.5", 199, -299}, // una devolución redondea igual, con signo
		{"0.333", 1000, 333},
	} {
		if l := linea(t, c.cantidad, c.precio, "21"); l.Importe != c.importe {
			t.Errorf("%s × %d = %d, se esperaba %d", c.cantidad, c.precio, l.Importe, c.importe)
		}
	}
}

func TestLineaInvalida(t *testing.T) {
	for nombre, data := range map[string]map[string]any{
		"tipo que no existe": {"cantidad": "1", "precio_con_iva": float64(100), "tipo_iva": "7"},
		"sin tipo":           {"cantidad": "1", "precio_con_iva": float64(100)},
		"cantidad rota":      {"cantidad": "uno", "precio_con_iva": float64(100), "tipo_iva": "21"},
		"precio con decimal": {"cantidad": "1", "precio_con_iva": 1.5, "tipo_iva": "21"},
	} {
		if _, err := LineaDe(data); !errors.Is(err, ErrDatos) {
			t.Errorf("%s: se esperaba ErrDatos, llegó %v", nombre, err)
		}
	}
}
