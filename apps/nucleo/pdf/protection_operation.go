// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"fmt"
	"strings"
)

// ProtectionOperationResult retains the warning that an untouched XMP block may
// override edited document properties in another reader.
type ProtectionOperationResult struct {
	Data         []byte
	UnchangedXMP bool
}

// ApplyProtectionOperation edits the supplied document in memory. Its passwords,
// metadata and redaction areas never require a server-side operation.
func ApplyProtectionOperation(doc *PDFDoc, fields map[string]string) (ProtectionOperationResult, error) {
	if doc == nil {
		return ProtectionOperationResult{}, inputError("The PDF is missing.")
	}
	var result ProtectionOperationResult
	var err error
	switch op := strings.TrimSpace(fields["op"]); op {
	case "proteger":
		if fields["usuario"] == "" && fields["dueno"] == "" {
			return result, inputError("Protecting needs at least one password.")
		}
		result.Data, err = ProtegerPDF(doc, OpcionesProteccion{
			ContrasenaUsuario: fields["usuario"],
			ContrasenaDueno:   fields["dueno"],
			Permitir:          ParseProtectionPermissions(fields["permisos"]),
		})
	case "desproteger":
		result.Data, err = DesprotegerPDF(doc)
	case "metadatos":
		var metadata Metadatos
		if err := DecodeOperationJSON(fields["metadatos"], "metadatos", &metadata); err != nil {
			return result, err
		}
		result.Data, result.UnchangedXMP, err = EditarMetadatosPDF(doc, metadata)
	case "sanear":
		result.Data, err = SanearMetadatosPDF(doc)
	case "redactar":
		var areas []AreaRedaccion
		if err := DecodeOperationJSON(fields["areas"], "areas", &areas); err != nil {
			return result, err
		}
		result.Data, err = RedactarPDF(doc, areas)
	default:
		return result, inputError(fmt.Sprintf("%q is not a valid operation. Use proteger, desproteger, metadatos, sanear or redactar.", op))
	}
	return result, err
}

// ParseProtectionPermissions preserves the existing allow-list: an omitted
// permission remains denied. Accessibility follows the PDF specification.
func ParseProtectionPermissions(raw string) Permisos {
	var permissions Permisos
	for part := range strings.SplitSeq(raw, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "imprimir":
			permissions.Imprimir = true
		case "imprimir-alta":
			permissions.ImprimirEnAlta = true
		case "modificar":
			permissions.Modificar = true
		case "copiar":
			permissions.Copiar = true
		case "anotar":
			permissions.Anotar = true
		case "rellenar":
			permissions.RellenarCampos = true
		case "ensamblar":
			permissions.Ensamblar = true
		}
	}
	permissions.Accesibilidad = true
	return permissions
}
