// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

// Líneas de factura y su desglose por tipo de IVA.
//
// El precio de una línea lleva el IVA INCLUIDO, que es como vende una tienda y
// como lo paga quien compra. Por eso el IVA se calcula por GRUPO y no por línea:
// la base de cada tipo es el total de sus líneas entre (1 + tipo), redondeada al
// céntimo, y la cuota es lo que falta hasta ese total. Base más cuota da
// exactamente lo que se cobró; con el IVA línea a línea, la suma de los
// redondeos se come o se inventa céntimos que el cliente nunca pagó.

// TiposIVA son los tipos que admite una línea, en porcentaje. El 0 no es un tipo
// del impuesto: es una operación exenta o no sujeta, y el porqué lo dice el
// régimen de IVA de la factura.
var TiposIVA = []int64{21, 10, 5, 4, 0}

// LineaFactura es una línea de `linea_factura`, ya leída.
type LineaFactura struct {
	Descripcion string
	// Cantidad es el decimal canónico tal como se guarda ("2", "1.5").
	Cantidad string
	// Precio es el precio unitario en céntimos, IVA incluido.
	Precio int64
	// Tipo es el tipo de IVA en porcentaje.
	Tipo int64
	// Importe es cantidad × precio, redondeado al céntimo: lo que se cobra.
	Importe int64
}

// GrupoIVA es la parte de la factura de un mismo tipo: lo cobrado (Total), su
// base y su cuota.
type GrupoIVA struct {
	Tipo, Total, Base, Cuota int64
}

// LineaDe lee una línea de `records.data`.
func LineaDe(data map[string]any) (LineaFactura, error) {
	l := LineaFactura{
		Descripcion: strings.TrimSpace(textoDe(data["descripcion"])),
		Cantidad:    textoDe(data["cantidad"]),
	}
	var err error
	if l.Precio, err = Centimos(data, "precio_con_iva"); err != nil {
		return l, err
	}
	tipo, err := strconv.ParseInt(textoDe(data["tipo_iva"]), 10, 64)
	if err != nil || !slices.Contains(TiposIVA, tipo) {
		return l, fmt.Errorf("la línea «%s» no tiene un tipo de IVA válido: %w", l.Descripcion, ErrDatos)
	}
	l.Tipo = tipo
	cantidad, ok := new(big.Rat).SetString(l.Cantidad)
	if !ok {
		return l, fmt.Errorf("la línea «%s» no tiene una cantidad válida: %w", l.Descripcion, ErrDatos)
	}
	if l.Importe, ok = alCentimo(cantidad.Mul(cantidad, new(big.Rat).SetInt64(l.Precio))); !ok {
		return l, fmt.Errorf("el importe de la línea «%s» es demasiado grande: %w", l.Descripcion, ErrDatos)
	}
	return l, nil
}

// Desglose agrupa las líneas por tipo de IVA, de mayor a menor.
func Desglose(lineas []LineaFactura) []GrupoIVA {
	totales := map[int64]int64{}
	for _, l := range lineas {
		totales[l.Tipo] += l.Importe
	}
	out := make([]GrupoIVA, 0, len(totales))
	for _, tipo := range TiposIVA {
		total, ok := totales[tipo]
		if !ok {
			continue
		}
		base := SinIVA(total, tipo)
		out = append(out, GrupoIVA{Tipo: tipo, Total: total, Base: base, Cuota: total - base})
	}
	return out
}

// SinIVA quita el IVA a un importe que lo lleva incluido: importe / (1 + tipo),
// al céntimo.
func SinIVA(centimos, tipo int64) int64 {
	// Cabe siempre en un int64: es menor, en valor absoluto, que el importe.
	base, _ := alCentimo(new(big.Rat).Mul(new(big.Rat).SetInt64(centimos), big.NewRat(100, 100+tipo)))
	return base
}

// alCentimo redondea al entero más cercano, las mitades lejos del cero (como se
// redondea un importe), y dice si cabe en un int64.
func alCentimo(r *big.Rat) (int64, bool) {
	n, err := strconv.ParseInt(r.FloatString(0), 10, 64)
	return n, err == nil
}

func textoDe(v any) string {
	s, _ := v.(string)
	return s
}
