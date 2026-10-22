// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"fmt"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

var identidadTPV = []string{"tpv_operacion", "tpv_huella"}

func conservarIdentidadTPV(previo, data map[string]any) error {
	for _, clave := range identidadTPV {
		valor, hay := data[clave]
		texto, esTexto := valor.(string)
		if hay && (!esTexto || texto != cadenaDe(previo[clave])) {
			return fmt.Errorf("la identidad de la operación TPV no se puede cambiar: %w", domain.ErrDatos)
		}
		if valor, hay := previo[clave]; hay {
			data[clave] = valor
		}
	}
	return nil
}

// Los UUID de cobro solo los asigna la transacción del TPV. Un alta genérica,
// importación o plan de IA no puede hacerse pasar por un cobro ya registrado.
func rechazarIdentidadTPV(data map[string]any) error {
	for _, clave := range identidadTPV {
		if _, hay := data[clave]; hay {
			return fmt.Errorf("%s es un campo interno del TPV: %w", clave, domain.ErrDatos)
		}
	}
	return nil
}

// Una factura de venta con `externo_id` se tiene por IMPORTADA —ya declarada en
// el ERP de donde viene— y no entra en la cadena de VERI*FACTU al emitirse
// (registroQueToca). Por eso ese campo solo lo ponen las importaciones y los
// conectores: escrito a mano en un alta o una edición, sacaba de la cadena una
// factura emitida aquí (NEG-14).
func rechazarExternoFactura(modelo string, data map[string]any) error {
	if modelo == "factura_venta" && cadenaDe(data["externo_id"]) != "" {
		return fmt.Errorf("externo_id solo lo ponen las importaciones: %w", domain.ErrDatos)
	}
	return nil
}

func conservarExternoFactura(previo, data map[string]any) error {
	antes := cadenaDe(previo["externo_id"])
	if ahora := cadenaDe(data["externo_id"]); ahora != "" && ahora != antes {
		return fmt.Errorf("externo_id no se puede cambiar: %w", domain.ErrDatos)
	}
	if antes != "" {
		data["externo_id"] = previo["externo_id"]
	} else {
		delete(data, "externo_id")
	}
	return nil
}
