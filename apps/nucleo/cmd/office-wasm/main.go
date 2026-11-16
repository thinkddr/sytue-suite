// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Command office-wasm loads the original document engines only on demand. It
// owns no SSSS, recipient identity, vault or network credential. Persistent and
// collaborative content is read/written through the host's scoped crypto kernel.
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func main() {
	webport.Serve(webport.Module{ID: "office", Version: 1, Execute: func(string, js.Value) (any, error) { return nil, webport.ErrRejected }, Async: map[string]func(context.Context, js.Value) (any, error){"office.import": importDocument, "office.export": exportDocument, "office.pdf": exportPDF}})
}

func importDocument(ctx context.Context, req js.Value) (any, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	raw, err := webport.BytesArg(req.Get("payload"), model.MaxImportBytes)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	comments := req.Get("comments")
	if comments.Type() != js.TypeUndefined && comments.Type() != js.TypeBoolean {
		return nil, webport.ErrRejected
	}
	includeComments := comments.Type() == js.TypeBoolean && comments.Bool()
	locale := ""
	if value := req.Get("locale"); value.Type() == js.TypeString {
		locale = value.String()
	} else if value.Type() != js.TypeUndefined {
		return nil, webport.ErrRejected
	}
	result, err := model.ImportLocalized(req.Get("filename").String(), raw, includeComments, locale)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	if err = webport.RevalidateJS(ctx, port, identity); err != nil {
		return nil, err
	}
	return webport.BytesValue(encoded), nil
}

func stateAndComments(req js.Value) (*model.State, []model.Comment, error) {
	raw, err := webport.BytesArg(req.Get("state"), int(^uint(0)>>1))
	if err != nil {
		return nil, nil, err
	}
	defer clear(raw)
	state, err := model.StateFromJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	var comments []model.Comment
	if req.Get("comments").Type() != js.TypeUndefined {
		raw, err := webport.BytesArg(req.Get("comments"), int(^uint(0)>>1))
		if err != nil {
			return nil, nil, err
		}
		defer clear(raw)
		if json.Unmarshal(raw, &comments) != nil {
			return nil, nil, webport.ErrRejected
		}
	}
	return state, comments, nil
}

func exportDocument(ctx context.Context, req js.Value) (any, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	state, comments, err := stateAndComments(req)
	if err != nil {
		return nil, err
	}
	kind := model.Kind(req.Get("kind").String())
	if !kind.Valid() {
		return nil, webport.ErrRejected
	}
	raw, mime, extension, err := model.ExportWithComments(kind, req.Get("title").String(), state, model.BlobFormat(req.Get("format").String()), comments)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	if err = webport.RevalidateJS(ctx, port, identity); err != nil {
		return nil, err
	}
	return map[string]any{"body": webport.BytesValue(raw), "content_type": mime, "extension": extension}, nil
}

func exportPDF(ctx context.Context, req js.Value) (any, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	state, _, err := stateAndComments(req)
	if err != nil {
		return nil, err
	}
	kind := model.Kind(req.Get("kind").String())
	if !kind.Valid() {
		return nil, webport.ErrRejected
	}
	raw, report, err := model.ExportPDFAccesible(kind, req.Get("title").String(), req.Get("locale").String(), state)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	encoded, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	if err = webport.RevalidateJS(ctx, port, identity); err != nil {
		return nil, err
	}
	return map[string]any{"body": webport.BytesValue(raw), "report": webport.BytesValue(encoded), "content_type": "application/pdf", "extension": "pdf"}, nil
}
