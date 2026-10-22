// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"encoding/xml"
	"strconv"
	"strings"
)

// La protección de Calcus en un .xlsx (ROADMAP-OFFICE permisos-1): `sheetProtection` por
// hoja y `workbookProtection` para la estructura del libro, con el hash de la contraseña en
// el esquema de OOXML (algorithmName, hashValue, saltValue, spinCount) —que es justo el que
// guarda el servicio (proteccion.go), así que el de un fichero de Excel se importa tal cual.
// Al EXPORTAR no sale ni contraseña ni hash: las exportaciones se guardan y las descarga
// cualquier lector, y un hash se puede atacar sin conexión y sin el límite de intentos del
// servicio. El fichero sale protegido sin contraseña.
//
// Sentido de los atributos. En OOXML un permiso de `sheetProtection` vale 1 cuando la acción
// está BLOQUEADA con la hoja protegida; en el modelo de Calcus los permisos se guardan al
// revés (`insertRows: true` = permitido) y ausente = bloqueado, salvo `selectCells`, que
// ausente = permitido (criterio Excel). Aquí se invierten.
//
// Lo que no cabe en OOXML se pierde, siempre hacia el lado seguro (más bloqueado): los rangos
// editables por persona o grupo (`protect.ranges`) —`protectedRange` solo sabe de cuentas de
// Windows y, sin ellas, dejaría editar a cualquiera— y los permisos de tablas dinámicas e
// hipervínculos, que Calcus no tiene.

// permisosOOXML: permiso de Calcus → atributo (o atributos) de sheetProtection que lo bloquea.
var permisosOOXML = []struct {
	campo string
	attrs []string
}{
	{"formatCells", []string{"formatCells", "formatColumns", "formatRows"}},
	{"insertRows", []string{"insertRows"}},
	{"deleteRows", []string{"deleteRows"}},
	{"insertCols", []string{"insertColumns"}},
	{"deleteCols", []string{"deleteColumns"}},
	{"sort", []string{"sort"}},
	{"autoFilter", []string{"autoFilter"}},
	{"objects", []string{"objects"}},
	{"scenarios", []string{"scenarios"}},
}

func unoOCero(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// sheetProtectionXlsx es el elemento de una hoja protegida (`protect` es el objeto del
// modelo); "" si la hoja no lo está.
func sheetProtectionXlsx(protect json.RawMessage) string {
	if !protegido(protect) {
		return ""
	}
	pm := map[string]json.RawMessage{}
	_ = json.Unmarshal(protect, &pm) // una lista queda como objeto vacío
	var b strings.Builder
	b.WriteString(`<sheetProtection sheet="1"`)
	for _, p := range permisosOOXML {
		bloqueado := !dice(pm, p.campo, true)
		for _, a := range p.attrs {
			b.WriteString(` ` + a + `="` + unoOCero(bloqueado) + `"`)
		}
	}
	// Seleccionar celdas bloqueadas está permitido salvo que el modelo diga que no.
	b.WriteString(` selectLockedCells="` + unoOCero(dice(pm, "selectCells", false)) + `"/>`)
	return b.String()
}

// workbookProtectionXlsx bloquea la estructura del libro; "" si no lo está.
func workbookProtectionXlsx(protect json.RawMessage) string {
	if !protegido(protect) {
		return ""
	}
	return `<workbookProtection lockStructure="1"/>`
}

// xlsxSheetProtection es el elemento leído de una hoja.
type xlsxSheetProtection struct {
	Sheet               string `xml:"sheet,attr"`
	AlgorithmName       string `xml:"algorithmName,attr"`
	HashValue           string `xml:"hashValue,attr"`
	SaltValue           string `xml:"saltValue,attr"`
	SpinCount           string `xml:"spinCount,attr"`
	Objects             string `xml:"objects,attr"`
	Scenarios           string `xml:"scenarios,attr"`
	FormatCells         string `xml:"formatCells,attr"`
	InsertColumns       string `xml:"insertColumns,attr"`
	InsertRows          string `xml:"insertRows,attr"`
	DeleteColumns       string `xml:"deleteColumns,attr"`
	DeleteRows          string `xml:"deleteRows,attr"`
	SelectLockedCells   string `xml:"selectLockedCells,attr"`
	Sort                string `xml:"sort,attr"`
	AutoFilter          string `xml:"autoFilter,attr"`
	WorkbookAlgorithm   string `xml:"workbookAlgorithmName,attr"`
	WorkbookHash        string `xml:"workbookHashValue,attr"`
	WorkbookSalt        string `xml:"workbookSaltValue,attr"`
	WorkbookSpin        string `xml:"workbookSpinCount,attr"`
	LockStructure       string `xml:"lockStructure,attr"`
	LegacyPasswordValue string `xml:"password,attr"`
}

// verdad lee un xsd:boolean con su valor por omisión.
func verdad(v string, pordefecto bool) bool {
	switch strings.TrimSpace(v) {
	case "1", "true":
		return true
	case "0", "false":
		return false
	}
	return pordefecto
}

// protectDeXlsx convierte el sheetProtection de una hoja en el `protect` del modelo y,
// si trae una contraseña que el servicio sabe verificar, en su hash. Una protección con
// otra contraseña (la de 16 bits de Excel antiguo, SHA-1…) se importa SIN contraseña: más
// vale que la quite quien importa, que es su propietario, a no poder comprobarla nunca.
// nil si la hoja no está protegida (`sheet` ausente o 0).
func protectDeXlsx(sp *xlsxSheetProtection) (protect map[string]any, h *ClaveWire) {
	if sp == nil || !verdad(sp.Sheet, false) {
		return nil, nil
	}
	protect = map[string]any{}
	permitido := func(attr string, bloqueadoPorDefecto bool) bool { return !verdad(attr, bloqueadoPorDefecto) }
	for campo, v := range map[string]bool{
		"formatCells": permitido(sp.FormatCells, true),
		"insertRows":  permitido(sp.InsertRows, true),
		"deleteRows":  permitido(sp.DeleteRows, true),
		"insertCols":  permitido(sp.InsertColumns, true),
		"deleteCols":  permitido(sp.DeleteColumns, true),
		"sort":        permitido(sp.Sort, true),
		"autoFilter":  permitido(sp.AutoFilter, true),
		"objects":     permitido(sp.Objects, false),
		"scenarios":   permitido(sp.Scenarios, false),
	} {
		if v {
			protect[campo] = true
		}
	}
	if verdad(sp.SelectLockedCells, false) {
		protect["selectCells"] = false
	}
	if w := (ClaveWire{Algoritmo: sp.AlgorithmName, Sal: sp.SaltValue, Vueltas: atoiSeguro(sp.SpinCount), Hash: sp.HashValue}); sp.HashValue != "" {
		if _, err := w.aHash(); err == nil {
			h = &w
		}
	}
	return protect, h
}

// lockDeXlsx lee workbookProtection de workbook.xml: (true, hash) si la estructura del
// libro está bloqueada.
func lockDeXlsx(raw []byte) (bloqueado bool, h *ClaveWire) {
	var libro struct {
		WP *xlsxSheetProtection `xml:"workbookProtection"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &libro) != nil || libro.WP == nil || !verdad(libro.WP.LockStructure, false) {
		return false, nil
	}
	if w := (ClaveWire{Algoritmo: libro.WP.WorkbookAlgorithm, Sal: libro.WP.WorkbookSalt, Vueltas: atoiSeguro(libro.WP.WorkbookSpin), Hash: libro.WP.WorkbookHash}); w.Hash != "" {
		if _, err := w.aHash(); err == nil {
			h = &w
		}
	}
	return true, h
}

func atoiSeguro(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
