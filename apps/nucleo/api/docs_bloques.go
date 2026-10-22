// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"
)

// Bloques reutilizables de Sytue Docs (`GET /blocks`, `GET /blocks/{clave}`): los de Sytue
// y los de la organización, que salen vestidos con su marca. El mismo cable que el BFF
// `api/blocks.ts` de la web.

// DocBlock es un bloque que la cinta ofrece para insertar. Versions solo lo traen los de
// la organización: sus versiones publicadas, la vigente primero.
type DocBlock struct {
	Key      string               `json:"key"`
	Name     string               `json:"name"`
	Versions []DocTemplateVersion `json:"versions,omitempty"`
}

// DocBlocks es la lista de un tipo de documento.
type DocBlocks struct {
	Blocks    []DocBlock `json:"blocks"`
	OrgBlocks []DocBlock `json:"org_blocks"` //nolint:tagliatelle // el cable de la web
}

// Blocks pide los bloques de ese tipo, con los nombres en el idioma `locale`.
func (d *Docs) Blocks(ctx context.Context, kind DocKind, locale string) (DocBlocks, error) {
	return JSON[DocBlocks](ctx, d.c, Request{Path: "/blocks", Query: url.Values{"kind": {string(kind)}, "locale": {locale}}})
}

// Block pide el contenido de un bloque, listo para insertar: HTML en un documento, el JSON
// de una diapositiva en una presentación y el de unas celdas en una hoja.
func (d *Docs) Block(ctx context.Context, clave, locale string) (string, error) {
	out, err := JSON[struct {
		Content string `json:"content"`
	}](ctx, d.c, Request{Path: "/blocks/" + url.PathEscape(clave), Query: url.Values{"locale": {locale}}})
	return out.Content, err
}
