// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"math/big"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

func TestFacturaeRedondeoDeGrupo(t *testing.T) {
	lineas := []dominio.LineaFactura{{Descripcion: "A", Cantidad: "1", Precio: 3, Tipo: 21, Importe: 3}, {Descripcion: "B", Cantidad: "1", Precio: 3, Tipo: 21, Importe: 3}}
	detalle, impuestos, err := detalleFacturae(lineas, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if detalle[0].Bruto != "0.03" || detalle[1].Bruto != "0.02" || impuestos[0].Base.Total != "0.05" {
		t.Fatalf("redondeo: %+v %+v", detalle, impuestos)
	}
	if _, _, err := detalleFacturae(lineas, 4, 2); err == nil {
		t.Fatal("cabecera descuadrada admitida")
	}
}

func TestFacturaeMuchosCentimosNoGeneranBasesNegativas(t *testing.T) {
	lineas := make([]dominio.LineaFactura, 100)
	for i := range lineas {
		lineas[i] = dominio.LineaFactura{Descripcion: "Unidad", Cantidad: "1", Precio: 1, Tipo: 21, Importe: 1}
	}
	detalle, impuestos, err := detalleFacturae(lineas, 83, 17)
	if err != nil {
		t.Fatal(err)
	}
	var sumaBase, sumaCuota big.Rat
	for i, l := range detalle {
		base, okBase := new(big.Rat).SetString(l.Bruto)
		cuota, okCuota := new(big.Rat).SetString(l.Impuestos[0].Cuota.Total)
		precio, okPrecio := new(big.Rat).SetString(l.Precio)
		if !okBase || !okCuota || !okPrecio || base.Sign() < 0 || cuota.Sign() < 0 || precio.Sign() < 0 {
			t.Fatalf("línea %d negativa o inválida: %+v", i, l)
		}
		if new(big.Rat).Add(base, cuota).Cmp(big.NewRat(1, 100)) != 0 {
			t.Fatalf("línea %d no suma el céntimo vendido: %+v", i, l)
		}
		sumaBase.Add(&sumaBase, base)
		sumaCuota.Add(&sumaCuota, cuota)
		// Los empates se resuelven en el orden original y el resultado es estable.
		esperado := "0.00"
		if i < 83 {
			esperado = "0.01"
		}
		if l.Bruto != esperado {
			t.Fatalf("línea %d: base %s, esperada %s", i, l.Bruto, esperado)
		}
	}
	if sumaBase.Cmp(big.NewRat(83, 100)) != 0 || sumaCuota.Cmp(big.NewRat(17, 100)) != 0 || impuestos[0].Base.Total != "0.83" || impuestos[0].Cuota.Total != "0.17" {
		t.Fatalf("sumas: base=%s, cuota=%s, impuestos=%+v", sumaBase.RatString(), sumaCuota.RatString(), impuestos)
	}
}

func TestFacturaeReparteSoloDentroDeCadaTipo(t *testing.T) {
	lineas := []dominio.LineaFactura{
		{Descripcion: "A", Cantidad: "1", Precio: 1, Tipo: 21, Importe: 1},
		{Descripcion: "B", Cantidad: "1", Precio: 1, Tipo: 10, Importe: 1},
		{Descripcion: "C", Cantidad: "1", Precio: 4, Tipo: 21, Importe: 4},
		{Descripcion: "D", Cantidad: "1", Precio: 10, Tipo: 10, Importe: 10},
	}
	detalle, impuestos, err := detalleFacturae(lineas, 14, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, esperado := range []string{"0.01", "0.01", "0.03", "0.09"} {
		if detalle[i].Bruto != esperado {
			t.Fatalf("línea %d: %+v", i, detalle[i])
		}
	}
	if len(impuestos) != 2 || impuestos[0].Base.Total != "0.04" || impuestos[1].Base.Total != "0.10" {
		t.Fatalf("grupos mezclados: %+v", impuestos)
	}
}
