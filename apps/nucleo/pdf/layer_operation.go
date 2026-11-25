// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"fmt"
	"slices"
	"strings"
)

// LAS CAPAS QUE SE SUPERPONEN A CADA PÁGINA: marca de agua, fondo, numeración
// —corriente o Bates— y encabezado o pie.
//
// Vivían en el servicio de Docs (`aplicarCapa` de `pdfpages.go`), así que solo el
// servidor sabía componerlas: con el Drive en la bóveda (ADR 0086) el estudio del
// navegador y la app nativa no podían poner un sello ni numerar un PDF que el
// servidor no ve. Ahora la composición es del motor, y el servicio, la sesión del
// dispositivo (`DeviceSession.Pages`) y la fachada local de las apps nativas
// llaman a la MISMA función con los mismos campos que `/pdf/pages`.

// MaxLayerText acota el texto de una capa: se dibuja en CADA página, y una cadena
// de un megabyte daría un fichero absurdo.
const MaxLayerText = 500

// layerOperations son las `op` de `/pdf/pages` que superponen texto.
var layerOperations = []string{"marca-agua", "fondo", "numerar", "bates", "encabezado", "pie"}

// IsLayerOperation dice si `op` superpone una capa (y no reordena páginas).
func IsLayerOperation(op string) bool { return slices.Contains(layerOperations, strings.TrimSpace(op)) }

// ApplyLayerOperation superpone la capa `op` a todas las páginas de `doc` con los
// campos de `/pdf/pages`: `texto`, `posicion` (numerar), `desde` y `digitos`
// (bates). Reescribe el documento, así que uno firmado pide antes la confirmación
// de invalidar sus firmas (`pdf_invalidates_signatures`), como organizar páginas.
func ApplyLayerOperation(op string, doc *PDFDoc, fields map[string]string) ([]byte, error) {
	op = strings.TrimSpace(op)
	if !IsLayerOperation(op) {
		return nil, inputError(fmt.Sprintf("Unknown op: %q.", op))
	}
	text := strings.TrimSpace(fields["texto"])
	if op != "numerar" && op != "bates" && text == "" {
		return nil, inputError("The text to be written is missing.")
	}
	if len([]rune(text)) > MaxLayerText {
		return nil, inputError(fmt.Sprintf("The text is too long (maximum %d characters).", MaxLayerText))
	}
	if err := pageSourcesMayBeRewritten([]*PDFDoc{doc}); err != nil {
		return nil, err
	}
	switch op {
	case "marca-agua":
		return MarcaDeAguaPDF(doc, text)
	case "fondo":
		return FondoPDF(doc, text)
	case "numerar":
		return NumerarPaginasPDF(doc, text, LayerPosition(fields["posicion"]))
	case "bates":
		from, err := ParseOptionalInteger(fields["desde"], 1, 1_000_000_000)
		if err != nil {
			return nil, err
		}
		digits, err := ParseOptionalInteger(fields["digitos"], 6, 12)
		if err != nil {
			return nil, err
		}
		return NumeracionBatesPDF(doc, text, from, digits)
	case "encabezado":
		return EncabezadoPiePDF(doc, text, CapaSuperiorCen)
	default: // "pie"
		return EncabezadoPiePDF(doc, text, CapaInferiorCen)
	}
}

// LayerPosition traduce el nombre de posición que manda el cliente. Uno
// desconocido cae en el pie centrado en vez de dar error: es una preferencia
// estética, y fallar por ella tiraría el trabajo entero.
func LayerPosition(s string) PosicionCapa {
	switch strings.TrimSpace(s) {
	case "superior-izquierda":
		return CapaSuperiorIzq
	case "superior-centro":
		return CapaSuperiorCen
	case "superior-derecha":
		return CapaSuperiorDer
	case "inferior-izquierda":
		return CapaInferiorIzq
	case "inferior-derecha":
		return CapaInferiorDer
	default:
		return CapaInferiorCen
	}
}
