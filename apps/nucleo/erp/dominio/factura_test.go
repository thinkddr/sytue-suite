// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func lineas(t *testing.T, datos ...map[string]any) []LineaFactura {
	t.Helper()
	out := make([]LineaFactura, 0, len(datos))
	for _, d := range datos {
		l, err := LineaDe(d)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func TestCuadreFactura(t *testing.T) {
	ls := lineas(t,
		map[string]any{"descripcion": "Corte", "cantidad": "2", "precio_con_iva": int64(1210), "tipo_iva": "21"},
		map[string]any{"descripcion": "Libro", "cantidad": "1", "precio_con_iva": int64(1040), "tipo_iva": "4"},
	)
	base, iva, total := TotalesDeLineas(ls)
	if base != 3000 || iva != 460 || total != 3460 {
		t.Fatalf("totales de las líneas: %d %d %d", base, iva, total)
	}
	// La cabecera que cuadra pasa y devuelve el desglose.
	g, err := CuadreFactura(map[string]any{"base": json.Number("3000"), "iva": int64(460)}, ls)
	if err != nil || len(g) != 2 {
		t.Fatalf("cuadra: %v %+v", err, g)
	}
	// La que no, falla con las cifras de las dos.
	_, err = CuadreFactura(map[string]any{"base": int64(3000), "iva": int64(400)}, ls)
	if !errors.Is(err, ErrDatos) || !strings.Contains(err.Error(), "(30.00 € y 4.00 €)") || !strings.Contains(err.Error(), "(30.00 € y 4.60 €)") {
		t.Fatalf("descuadre: %v", err)
	}
	// Sin líneas no se compara; una simplificada de más de 3.000 € no vale.
	if _, err := CuadreFactura(map[string]any{"base": int64(300000), "iva": int64(1), "tipo_factura": "F2"}, nil); !errors.Is(err, ErrDatos) {
		t.Fatalf("simplificada grande: %v", err)
	}
	if _, err := CuadreFactura(map[string]any{"base": int64(100), "tipo_factura": "F2", "regimen_iva": "iva_inversion"}, nil); !errors.Is(err, ErrDatos) {
		t.Fatalf("simplificada en inversión: %v", err)
	}
}

func TestComprobarImportesVenta(t *testing.T) {
	ls := lineas(t, map[string]any{"descripcion": "x", "cantidad": "1", "precio_con_iva": int64(121), "tipo_iva": "21"})
	if err := ComprobarImportesVenta(map[string]any{"base": int64(100), "iva": int64(21), "total": int64(121)}, Desglose(ls)); err != nil {
		t.Fatal(err)
	}
	if err := ComprobarImportesVenta(map[string]any{"base": int64(100), "iva": int64(21), "total": int64(120)}, nil); !errors.Is(err, ErrDatos) {
		t.Fatalf("total mal: %v", err)
	}
	if err := ComprobarImportesVenta(map[string]any{"base": int64(99), "iva": int64(22)}, Desglose(ls)); !errors.Is(err, ErrDatos) {
		t.Fatalf("cabecera distinta de las líneas: %v", err)
	}
}

func TestEmitibleYPrefijo(t *testing.T) {
	if !Emitible("borrador", nil, "confirmado") || Emitible("confirmado", nil, "pagado") || Emitible("borrador", map[string]any{"externo_id": "x"}, "pagado") {
		t.Fatal("Emitible")
	}
	for diario, p := range map[string]string{"ventas": "VENTAS", "compras": "COMPRA", "banco": "BANCO", "x": "DIARIO"} {
		if PrefijoDiario(diario) != p {
			t.Errorf("PrefijoDiario(%q) = %q", diario, PrefijoDiario(diario))
		}
	}
	if _, err := Centimos(map[string]any{"x": json.Number("1.5")}, "x"); !errors.Is(err, ErrDatos) {
		t.Fatal("un json.Number con decimales no es un importe")
	}
}
