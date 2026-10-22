// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// modeloVenta y modeloCompra reproducen lo que declaran los manifiestos de
// facturación, sin depender de que esos ficheros no cambien.
func modeloVenta() *catalogo.Modelo {
	return &catalogo.Modelo{
		Clave: "factura_venta", Secuencia: "FV",
		Campos: []catalogo.Campo{
			{Clave: "fecha", Tipo: catalogo.TipoFecha, Requerido: true},
			{Clave: "base", Tipo: catalogo.TipoDinero, Requerido: true},
			{Clave: "iva", Tipo: catalogo.TipoDinero},
			{Clave: "cliente", Tipo: catalogo.TipoRelacion, Modelo: "contacto"},
		},
		Contable: &catalogo.Contable{
			Diario: "ventas", CampoBase: "base", CampoIVA: "iva", CampoTercero: "cliente",
			CuentaContrapartida: "430000", CuentaIngresoGasto: "700000", CuentaIVA: "477000",
			Cobro: true,
		},
	}
}

func modeloCompra() *catalogo.Modelo {
	m := modeloVenta()
	m.Clave, m.Secuencia = "factura_compra", "FC"
	m.Contable = &catalogo.Contable{
		Diario: "compras", CampoBase: "base", CampoIVA: "iva", CampoTercero: "cliente",
		CuentaContrapartida: "400000", CuentaIngresoGasto: "600000", CuentaIVA: "472000",
		Cobro: false,
	}
	return m
}

func sumas(a Asiento) (int64, int64) {
	var debe, haber int64
	for _, ap := range a.Apuntes {
		debe += ap.Debe
		haber += ap.Haber
	}
	return debe, haber
}

func TestAsientoDeVentaConIVA(t *testing.T) {
	// 1.000,00 € + 21 % = 1.210,00 €, en céntimos.
	data := map[string]any{
		"fecha": "2026-09-12", "base": int64(100000), "iva": int64(21000),
		"cliente": "3f6c0f6e-0000-4000-8000-000000000001",
	}
	a, err := AsientoDe(modeloVenta(), "FV-2026-00001", data)
	if err != nil {
		t.Fatalf("AsientoDe: %v", err)
	}
	if len(a.Apuntes) != 3 {
		t.Fatalf("apuntes = %d, se esperaban 3", len(a.Apuntes))
	}
	debe, haber := sumas(a)
	if debe != haber || debe != 121000 {
		t.Fatalf("debe=%d haber=%d, se esperaba 121000 en ambos", debe, haber)
	}
	// El cliente DEBE el total, y es la única línea con tercero.
	if a.Apuntes[0].Cuenta != "430000" || a.Apuntes[0].Debe != 121000 {
		t.Errorf("contrapartida: %+v", a.Apuntes[0])
	}
	if a.Apuntes[0].Tercero == "" {
		t.Error("la línea de cliente tiene que llevar el tercero")
	}
	if a.Apuntes[1].Haber != 100000 || a.Apuntes[2].Haber != 21000 {
		t.Errorf("ingreso/IVA: %+v %+v", a.Apuntes[1], a.Apuntes[2])
	}
}

func TestAsientoDeCompraInvierteLosLados(t *testing.T) {
	data := map[string]any{"fecha": "2026-09-12", "base": int64(50000), "iva": int64(10500)}
	a, err := AsientoDe(modeloCompra(), "FC-2026-00001", data)
	if err != nil {
		t.Fatalf("AsientoDe: %v", err)
	}
	debe, haber := sumas(a)
	if debe != haber || debe != 60500 {
		t.Fatalf("debe=%d haber=%d, se esperaba 60500", debe, haber)
	}
	// En compras el gasto va al DEBE y el proveedor al HABER: justo al revés.
	if a.Apuntes[0].Cuenta != "600000" || a.Apuntes[0].Debe != 50000 {
		t.Errorf("gasto: %+v", a.Apuntes[0])
	}
	if a.Apuntes[2].Cuenta != "400000" || a.Apuntes[2].Haber != 60500 {
		t.Errorf("proveedor: %+v", a.Apuntes[2])
	}
}

func TestAsientoSinIVANoGeneraLineaDeIVA(t *testing.T) {
	// Una operación exenta. La línea de IVA a cero la rechazaría la base de datos
	// (`move_lines_un_solo_lado`), así que no debe existir.
	data := map[string]any{"fecha": "2026-09-12", "base": int64(30000)}
	a, err := AsientoDe(modeloVenta(), "FV-2026-00002", data)
	if err != nil {
		t.Fatalf("AsientoDe: %v", err)
	}
	if len(a.Apuntes) != 2 {
		t.Fatalf("apuntes = %d, se esperaban 2", len(a.Apuntes))
	}
	for _, ap := range a.Apuntes {
		if ap.Debe == 0 && ap.Haber == 0 {
			t.Errorf("apunte a cero: %+v", ap)
		}
	}
}

// TestAsientoRectificativo es el caso que se cuela con el signo al revés en
// cuanto se trata como una excepción: un abono es la misma factura en negativo.
func TestAsientoRectificativo(t *testing.T) {
	data := map[string]any{"fecha": "2026-09-12", "base": int64(-100000), "iva": int64(-21000)}
	a, err := AsientoDe(modeloVenta(), "FV-2026-00003", data)
	if err != nil {
		t.Fatalf("AsientoDe: %v", err)
	}
	debe, haber := sumas(a)
	if debe != haber || debe != 121000 {
		t.Fatalf("debe=%d haber=%d, se esperaba 121000", debe, haber)
	}
	// Los lados se han invertido: ahora el cliente está al HABER.
	if a.Apuntes[0].Cuenta != "430000" || a.Apuntes[0].Haber != 121000 || a.Apuntes[0].Debe != 0 {
		t.Errorf("un abono deja al cliente al haber: %+v", a.Apuntes[0])
	}
	for _, ap := range a.Apuntes {
		if ap.Debe < 0 || ap.Haber < 0 {
			t.Errorf("importe negativo en el libro: %+v", ap)
		}
	}
}

func TestAsientoRechazaDocumentoSinImporte(t *testing.T) {
	data := map[string]any{"fecha": "2026-09-12", "base": int64(0)}
	if _, err := AsientoDe(modeloVenta(), "FV-2026-00004", data); err == nil {
		t.Fatal("se esperaba error: un documento a cero no genera asiento")
	}
}

func TestAsientoRechazaFechaAusente(t *testing.T) {
	data := map[string]any{"base": int64(1000)}
	if _, err := AsientoDe(modeloVenta(), "FV-2026-00005", data); err == nil {
		t.Fatal("se esperaba error: sin fecha no hay ejercicio al que imputar")
	}
}

func TestAsientoRechazaImporteConDecimales(t *testing.T) {
	// Un float que no es entero solo puede venir de haber metido euros donde van
	// céntimos. Redondearlo en silencio sería perder dinero de verdad.
	data := map[string]any{"fecha": "2026-09-12", "base": 100.5}
	if _, err := AsientoDe(modeloVenta(), "FV-2026-00006", data); err == nil {
		t.Fatal("se esperaba error por importe con decimales")
	}
}
