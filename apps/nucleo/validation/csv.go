// SPDX-License-Identifier: Apache-2.0
package validation

// CSVSafe neutraliza la inyección de fórmulas CSV (CWE-1236): si una celda
// empieza por un carácter que Excel, LibreOffice o Google Sheets interpretarían
// como el comienzo de una fórmula, la prefija con una comilla simple para que la
// hoja de cálculo la trate como texto. Sin esto, un valor de usuario guardado en
// la base de datos (el nombre de un contacto, la descripción de un evento) se
// ejecuta al abrir el CSV exportado: =HYPERLINK(...) exfiltra, +cmd|... llega a
// ejecutar en Windows.
//
// Es el saneado de SALIDA de toda exportación CSV de la suite (registros de
// auditoría, contactos, gobernanza). El round-trip se revierte quitando la
// comilla sólo si protege uno de estos caracteres.
func CSVSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
