// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import "strings"

// JSON recorta el objeto JSON de una respuesta del modelo.
//
// Existe porque el modelo pone el vallado ```json … ``` aunque se le diga que no,
// y porque a veces antepone «Aquí tienes:». Se toma del primer `{` al último `}`:
// es tosco y es exactamente lo que hace falta. Vacío si no hay objeto.
//
// Vive aquí y no solo en platform/pkg/asistente porque lo usan también los clientes (la
// IA del ERP de Sytue Office, apps/nucleo/erp/ia); aquel lo reexporta.
func JSON(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return ""
	}
	return s[i : j+1]
}
