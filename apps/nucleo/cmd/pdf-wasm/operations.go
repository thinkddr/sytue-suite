// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

// Tope de los campos de una operación (el JSON de anotaciones, valores…): el mismo
// que el motor, con holgura para el resto de campos.
const maxFieldsBytes = pdf.MaxOperationJSONBytes + 64<<10

// documentos traduce JavaScript ↔ la sesión del motor. Los errores del MOTOR (una
// contraseña, un documento firmado) vuelven como valor `{error: {codigo, mensaje}}`
// —el estudio los enseña con su frase—; los del protocolo, como rechazo.
type documentos struct{ session *pdf.DeviceSession }

func newDocumentos() *documentos { return &documentos{session: pdf.NewDeviceSession()} }

func fallo(err error) any {
	f := pdf.DescribeDeviceError(err)
	return map[string]any{"error": map[string]any{"codigo": f.Code, "mensaje": f.Message}}
}

func texto(value js.Value) (string, error) {
	switch value.Type() {
	case js.TypeUndefined:
		return "", nil
	case js.TypeString:
		return value.String(), nil
	default:
		return "", webport.ErrRejected
	}
}

// campos lee `campos`: un JSON {clave: texto} en bytes, nunca en una cadena que
// acabe en un registro.
func campos(req js.Value) (map[string]string, error) {
	value := req.Get("campos")
	if value.Type() == js.TypeUndefined {
		return map[string]string{}, nil
	}
	raw, err := webport.BytesArg(value, maxFieldsBytes)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	out := map[string]string{}
	if json.Unmarshal(raw, &out) != nil {
		return nil, webport.ErrRejected
	}
	return out, nil
}

func edicion(edit pdf.DeviceEdit) any {
	out := map[string]any{"body": webport.BytesValue(edit.Data), "content_type": edit.ContentType}
	clear(edit.Data)
	if edit.Archival != "" {
		out["pdfa"] = edit.Archival
	}
	if edit.ImportedFields > 0 {
		out["importados"] = edit.ImportedFields
	}
	if c := edit.Compression; c != nil {
		out["antes"], out["despues"], out["ahorro"] = c.Antes, c.Despues, c.Ahorro()
	}
	return out
}

func (d *documentos) abrirJS(ctx context.Context, req js.Value) (any, error) {
	raw, err := webport.BytesArg(req.Get("pdf"), pdf.MaxPDFBytes)
	if err != nil {
		return nil, err
	}
	clave, err := texto(req.Get("clave"))
	if err != nil {
		clear(raw)
		return nil, err
	}
	id, resumen, err := d.session.Open(raw, clave)
	if err != nil {
		clear(raw)
		return fallo(err), nil
	}
	encoded, err := json.Marshal(resumen)
	if err != nil || ctx.Err() != nil {
		d.session.Close(id)
		return nil, webport.ErrRejected
	}
	defer clear(encoded)
	return map[string]any{"doc": id, "contenido": webport.BytesValue(encoded)}, nil
}

func (d *documentos) cerrarJS(_ context.Context, req js.Value) (any, error) {
	id, err := texto(req.Get("doc"))
	if err != nil {
		return nil, err
	}
	return d.session.Close(id), nil
}

func (d *documentos) renderJS(ctx context.Context, req js.Value) (any, error) {
	id, err := texto(req.Get("doc"))
	if err != nil {
		return nil, err
	}
	fields, err := campos(req)
	if err != nil {
		return nil, err
	}
	result, err := d.session.Render(ctx, id, fields)
	if err != nil {
		return fallo(err), nil
	}
	defer clear(result.Data)
	return map[string]any{"body": webport.BytesValue(result.Data), "content_type": result.ContentType}, nil
}

func (d *documentos) anotarJS(_ context.Context, req js.Value) (any, error) {
	id, err := texto(req.Get("doc"))
	if err != nil {
		return nil, err
	}
	autor, err := texto(req.Get("autor"))
	if err != nil {
		return nil, err
	}
	fields, err := campos(req)
	if err != nil {
		return nil, err
	}
	edit, err := d.session.Annotate(id, fields, autor)
	if err != nil {
		return fallo(err), nil
	}
	return edicion(edit), nil
}

func (d *documentos) formularioJS(_ context.Context, req js.Value) (any, error) {
	id, err := texto(req.Get("doc"))
	if err != nil {
		return nil, err
	}
	nombre, err := texto(req.Get("nombre"))
	if err != nil {
		return nil, err
	}
	fields, err := campos(req)
	if err != nil {
		return nil, err
	}
	var datos []byte
	if value := req.Get("datos"); value.Type() != js.TypeUndefined {
		if datos, err = webport.BytesArg(value, pdf.MaxOperationFileBytes); err != nil {
			return nil, err
		}
		defer clear(datos)
	}
	edit, err := d.session.Form(id, fields, nombre, datos)
	if err != nil {
		return fallo(err), nil
	}
	return edicion(edit), nil
}

func (d *documentos) paginasJS(_ context.Context, req js.Value) (any, error) {
	lista := req.Get("docs")
	if lista.Type() != js.TypeObject || !lista.InstanceOf(js.Global().Get("Array")) {
		return nil, webport.ErrRejected
	}
	n := lista.Length()
	if n < 1 || n > pdf.MaxDeviceDocuments {
		return nil, webport.ErrRejected
	}
	ids := make([]string, 0, n)
	for i := range n {
		id, err := texto(lista.Index(i))
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	fields, err := campos(req)
	if err != nil {
		return nil, err
	}
	edit, err := d.session.Pages(ids, fields)
	if err != nil {
		return fallo(err), nil
	}
	return edicion(edit), nil
}

func (d *documentos) herramientaJS(_ context.Context, req js.Value) (any, error) {
	id, err := texto(req.Get("doc"))
	if err != nil {
		return nil, err
	}
	fields, err := campos(req)
	if err != nil {
		return nil, err
	}
	edit, err := d.session.Tool(id, fields)
	if err != nil {
		return fallo(err), nil
	}
	return edicion(edit), nil
}
