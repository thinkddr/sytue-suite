// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"fmt"
	"strings"
)

// Lo que tiene que cumplir una factura al emitirse y un documento de venta al
// convertirse, sin base de datos. Lo usan el almacén de Postgres de services/syerp y el
// de SQLite de Sytue Office (apps/nucleo/local/erp): las dos ediciones rechazan lo
// mismo con las mismas palabras (PLAN-EDICION-LOCAL §6.13, vectores dorados).

// LimiteSimplificada es el tope de una factura simplificada: 3.000 €, IVA incluido
// (RD 1619/2012, art. 4.1, para las ventas al por menor y el resto de su lista).
//
// ponytail: el tope general de 400 € depende de la actividad de la organización,
// que el ERP no sabe; se vigila el de 3.000, que es el de las tiendas.
const LimiteSimplificada = 300000

// RegimenesSinSimplificada son los regímenes en los que el RD 1619/2012 (art. 4.3)
// no deja expedir factura simplificada: la entrega intracomunitaria, lo que no se
// localiza en España (las ventas a distancia a otro Estado, entre otras) y la
// inversión del sujeto pasivo, que necesita identificar al cliente.
var RegimenesSinSimplificada = map[string]bool{
	"iva_entrega_ue": true,
	"iva_no_sujeta":  true,
	"iva_inversion":  true,
}

// CuadreFactura comprueba una factura que se emite con sus líneas ya leídas: que su
// base y su IVA sean los de sus líneas, si las tiene, y que una simplificada pueda
// serlo. Devuelve el desglose de las líneas (nada si no tiene).
func CuadreFactura(data map[string]any, lineas []LineaFactura) ([]GrupoIVA, error) {
	base, err := Centimos(data, "base")
	if err != nil {
		return nil, err
	}
	iva, err := Centimos(data, "iva")
	if err != nil {
		return nil, err
	}
	grupos := Desglose(lineas)
	if len(grupos) > 0 {
		bases, cuotas := SumaGrupos(grupos)
		if bases != base || cuotas != iva {
			return nil, fmt.Errorf("la base y el IVA de la factura (%s € y %s €) tienen que ser los de sus líneas (%s € y %s €): %w",
				Importe(base), Importe(iva), Importe(bases), Importe(cuotas), ErrDatos)
		}
	}
	if textoDe(data["tipo_factura"]) == "F2" {
		switch {
		case RegimenesSinSimplificada[textoDe(data["regimen_iva"])]:
			return nil, fmt.Errorf("en este régimen de IVA no se puede emitir una factura simplificada (RD 1619/2012, art. 4.3): emite una completa: %w",
				ErrDatos)
		case base+iva > LimiteSimplificada:
			return nil, fmt.Errorf("una factura simplificada no puede pasar de 3.000 € con el IVA incluido (RD 1619/2012, art. 4.1): emite una completa: %w",
				ErrDatos)
		}
	}
	return grupos, nil
}

// SumaGrupos son las bases y las cuotas de un desglose.
func SumaGrupos(grupos []GrupoIVA) (bases, cuotas int64) {
	for _, g := range grupos {
		bases += g.Base
		cuotas += g.Cuota
	}
	return bases, cuotas
}

// ComprobarImportesVenta comprueba, al convertir un presupuesto o un pedido, que la
// cabecera del documento de origen sea la de sus líneas (`grupos`, su desglose) y que
// el total sea la base más el IVA.
func ComprobarImportesVenta(data map[string]any, grupos []GrupoIVA) error {
	base, err := Centimos(data, "base")
	if err != nil {
		return err
	}
	iva, err := Centimos(data, "iva")
	if err != nil {
		return err
	}
	bases, cuotas := SumaGrupos(grupos)
	if len(grupos) > 0 && (base != bases || iva != cuotas) {
		return fmt.Errorf("revisa la base y el IVA del documento: no coinciden con sus líneas: %w", ErrDatos)
	}
	if data["total"] != nil {
		total, err := Centimos(data, "total")
		if err != nil {
			return err
		}
		if total != base+iva {
			return fmt.Errorf("revisa el total del documento: no coincide con su base e IVA: %w", ErrDatos)
		}
	}
	return nil
}

// TotalesDeLineas es la cabecera que corresponde a unas líneas: la suma de las bases y
// de las cuotas de su desglose, y el total. Es el recálculo que hace que una factura
// con líneas cuadre (CuadreFactura) sin teclear la cabecera a mano.
func TotalesDeLineas(lineas []LineaFactura) (base, iva, total int64) {
	base, iva = SumaGrupos(Desglose(lineas))
	return base, iva, base + iva
}

// PrefijoDiario convierte el nombre del diario en el prefijo de sus asientos
// (ventas → VENTAS-2026-00001). Seis letras como mucho, que es lo que admite la
// forma de la referencia.
func PrefijoDiario(diario string) string {
	p := ""
	for _, r := range diario {
		if r >= 'a' && r <= 'z' {
			p += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			p += string(r)
		}
		if len(p) == 6 {
			break
		}
	}
	if len(p) < 2 {
		return "DIARIO"
	}
	return p
}

// Importe escribe céntimos como la AEAT: punto decimal y dos cifras («-12.30»).
func Importe(centimos int64) string {
	signo := ""
	if centimos < 0 {
		signo, centimos = "-", -centimos
	}
	return fmt.Sprintf("%s%d.%02d", signo, centimos/100, centimos%100)
}

// Emitible dice si una factura de venta que pasa a `estado` SE EMITE ahora (sin
// VERI*FACTU, es la que tiene que cuadrar): pasa a confirmada o pagada desde borrador
// o anulada y no viene de otro sistema (`externo_id`).
func Emitible(estadoActual string, data map[string]any, estado string) bool {
	if estado != "confirmado" && estado != "pagado" {
		return false
	}
	return (estadoActual == "borrador" || estadoActual == "anulado") && strings.TrimSpace(textoDe(data["externo_id"])) == ""
}
