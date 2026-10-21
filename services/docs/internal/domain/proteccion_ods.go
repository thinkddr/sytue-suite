// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"encoding/xml"
	"strings"
)

// La protección de Calcus en un .ods (ver proteccion_fichero.go). ODF tiene `table:protected`
// por hoja y `table:structure-protected` para la estructura del libro, cada una con su
// `table:protection-key` (el digest de la contraseña) y el algoritmo con el que se calculó.
//
// El servicio guarda la contraseña con sal y 100 000 vueltas de SHA-512 (esquema de OOXML), que
// ODF no sabe expresar: la clave de `table:protection-key` lleva el hash en base64 con el
// algoritmo `xmlenc#sha512`, y la sal y las vueltas, sin las que no se puede verificar, van en
// atributos propios (`sytue:salt`, `sytue:spin`). LibreOffice no entiende esa combinación, así
// que una hoja exportada con contraseña no se desprotege con ninguna escrita allí (más bloqueado,
// nunca menos); Sytue la reconoce y la verifica. Una protección de LibreOffice con su clave
// (SHA-1 o SHA-256 sin sal) se importa SIN contraseña: el servicio no puede verificarla.
//
// Los permisos de «Proteger hoja» (insertar filas, ordenar…) no tienen equivalente en ODF
// estándar y viajan en `sytue:allowed`, la lista de los permitidos; ausente = ninguno.

const (
	algoritmoODS  = "http://www.w3.org/2001/04/xmlenc#sha512"
	protegidoODS  = ` table:protected="true"`
	estructuraODS = ` table:structure-protected="true"`
)

// permisosODS son los permisos del cuadro «Proteger hoja» que se escriben en sytue:allowed.
var permisosODS = []string{"formatCells", "insertRows", "deleteRows", "insertCols", "deleteCols", "sort", "autoFilter", "objects", "scenarios"}

// atributosTablaODS son los atributos de protección de una `table:table`; "" si no está protegida.
func atributosTablaODS(protect json.RawMessage) string {
	if !protegido(protect) {
		return ""
	}
	pm := map[string]json.RawMessage{}
	_ = json.Unmarshal(protect, &pm)
	var permitidos []string
	for _, p := range permisosODS {
		if dice(pm, p, true) {
			permitidos = append(permitidos, p)
		}
	}
	out := protegidoODS + ` sytue:allowed="` + strings.Join(permitidos, " ") + `"`
	if dice(pm, "selectCells", false) {
		out += ` sytue:select-locked="false"`
	}
	return out
}

// atributosLibroODS son los de `office:spreadsheet` cuando la estructura está protegida.
func atributosLibroODS(protect json.RawMessage) string {
	if !protegido(protect) {
		return ""
	}
	return estructuraODS
}

// protectDeODS convierte los atributos de una `table:table` en el `protect` del modelo (nil si
// la hoja no está protegida).
func protectDeODS(se xml.StartElement) json.RawMessage {
	if attrLocal(se, "protected") != "true" {
		return nil
	}
	protect := map[string]any{}
	for _, p := range strings.Fields(attrLocal(se, "allowed")) {
		for _, conocido := range permisosODS {
			if p == conocido {
				protect[p] = true
			}
		}
	}
	if attrLocal(se, "select-locked") == "false" {
		protect["selectCells"] = false
	}
	b, _ := json.Marshal(protect)
	return b
}

// lockDeODS lee la protección de estructura de `office:spreadsheet`.
func lockDeODS(se xml.StartElement) json.RawMessage {
	if attrLocal(se, "structure-protected") != "true" {
		return nil
	}
	return json.RawMessage("{}")
}
