// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Operation limits are shared by authorized devices and the legacy HTTP adapter.
const (
	MaxOperationJSONBytes = 8 << 20
	MaxOperationFileBytes = 256 << 10
)

// FormOperationInput contains only caller-supplied operation data. The caller
// owns authorization and PDF parsing; this engine performs no I/O or storage.
type FormOperationInput struct {
	Fields       map[string]string
	Filename     string
	Data         []byte
	DataProvided bool
}

// FormOperationResult retains the existing PDF/JSON/XFDF response contract.
type FormOperationResult struct {
	Data           []byte
	ContentType    string
	ImportedFields int
}

// DecodeOperationJSON preserves strict field validation and input limits across
// native and HTTP operations. More than one JSON value is never an edit request.
func DecodeOperationJSON(raw, field string, destination any) error {
	if strings.TrimSpace(raw) == "" {
		return inputError(fmt.Sprintf("The %q field is missing.", field))
	}
	if len(raw) > MaxOperationJSONBytes {
		return &OperationFailure{Status: 413, Code: "payload_too_large", Message: "The file is larger than the maximum allowed size."}
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return inputError(fmt.Sprintf("The %q field is not valid JSON: %v", field, err))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return inputError(fmt.Sprintf("The %q field must contain a single JSON value.", field))
	}
	return nil
}

// UploadedDocumentName retains the informational file name, without exposing a
// caller's local path in exported XFDF. It never resolves a filesystem location.
func UploadedDocumentName(name string) string {
	if index := strings.LastIndexAny(name, `/\`); index >= 0 {
		return name[index+1:]
	}
	return name
}

// ApplyFormOperation is the single dispatcher for the existing seven form
// operations. It calls the maintained PDF editor and XFDF codec directly.
func ApplyFormOperation(doc *PDFDoc, input FormOperationInput) (FormOperationResult, error) {
	if doc == nil {
		return FormOperationResult{}, inputError("The PDF is missing.")
	}
	operation := strings.TrimSpace(input.Fields["op"])
	if (operation == "crear" || operation == "aplanar") && doc.conservaFirmas() {
		return FormOperationResult{}, fmt.Errorf("%w: creating or flattening fields changes a signed document", ErrInvalidaFirmas)
	}
	switch operation {
	case "", "leer":
		return formJSONResult(map[string]any{"campos": doc.CamposDelFormulario()})
	case "detectar":
		page, err := ParseOptionalInteger(input.Fields["pagina"], 1, doc.NumPaginas())
		if err != nil {
			return FormOperationResult{}, err
		}
		proposals, err := doc.DetectarCamposPDF(page - 1)
		if err != nil {
			return FormOperationResult{}, err
		}
		return formJSONResult(map[string]any{"propuestas": proposals})
	case "exportar-datos":
		data, err := doc.ExportarXFDF(UploadedDocumentName(input.Filename))
		return FormOperationResult{Data: data, ContentType: "application/vnd.adobe.xfdf"}, err
	case "crear":
		var fields []CampoNuevo
		if err := DecodeOperationJSON(input.Fields["campos"], "campos", &fields); err != nil {
			return FormOperationResult{}, err
		}
		data, err := AnadirCamposPDF(doc, fields)
		return formPDFResult(data), err
	case "rellenar":
		var values map[string]string
		if err := DecodeOperationJSON(input.Fields["valores"], "valores", &values); err != nil {
			return FormOperationResult{}, err
		}
		data, err := RellenarFormularioPDF(doc, values)
		return formPDFResult(data), err
	case "aplanar":
		data, err := AplanarFormularioPDF(doc)
		return formPDFResult(data), err
	case "importar-datos":
		return importFormOperation(doc, input)
	default:
		return FormOperationResult{}, inputError(fmt.Sprintf("%q is not a valid operation. Use leer, detectar, crear, rellenar, aplanar, exportar-datos or importar-datos.", operation))
	}
}

func formJSONResult(value any) (FormOperationResult, error) {
	data, err := json.Marshal(value)
	return FormOperationResult{Data: data, ContentType: "application/json"}, err
}

func formPDFResult(data []byte) FormOperationResult {
	return FormOperationResult{Data: data, ContentType: "application/pdf"}
}

func importFormOperation(doc *PDFDoc, input FormOperationInput) (FormOperationResult, error) {
	if !input.DataProvided {
		return FormOperationResult{}, inputError(`The "datos" file is missing.`)
	}
	if len(input.Data) > MaxOperationFileBytes {
		return FormOperationResult{}, inputError(`The "datos" file is too large.`)
	}
	values, err := ImportarXFDF(input.Data)
	if err != nil {
		return FormOperationResult{}, err
	}
	data, err := RellenarFormularioPDF(doc, values)
	result := formPDFResult(data)
	result.ImportedFields = len(values)
	return result, err
}

// FormImportedFieldsHeader is present only for a successful import operation.
func FormImportedFieldsHeader(result FormOperationResult) string {
	if result.ImportedFields == 0 {
		return ""
	}
	return strconv.Itoa(result.ImportedFields)
}
