// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

var ErrPrivateDataOnly = errors.New("docs: operation unavailable through private data")

type privateClosedRoundTripper struct{}

func (privateClosedRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, ErrPrivateDataOnly
}

func newPrivateClosedClient() *Client {
	return &Client{
		base: "https://private.invalid",
		http: &http.Client{Transport: privateClosedRoundTripper{}},
	}
}

// DocsData backs a Docs client from the encrypted private repository without
// exposing any legacy HTTP client fallback.
type DocsData interface {
	ListDocuments(ctx context.Context, f DocsFilter) ([]Document, string, error)
	CreateDocumentFromSource(ctx context.Context, kind DocKind, titulo, fuente string) (Document, bool, error)
	DocumentBySource(ctx context.Context, fuente string) (Document, bool, error)
	GetDocument(ctx context.Context, id string) (Document, string, error)
	ImportDocument(ctx context.Context, nombre string, datos []byte, comentarios bool) (Importado, error)
	ImportComments(ctx context.Context, id string, comentarios []ImportadoComentario) ([]string, error)
	RenameDocument(ctx context.Context, id, titulo string) error
	DeleteDocument(ctx context.Context, id string) error
	StarDocument(ctx context.Context, id string, destacado bool) error
	DocumentContent(ctx context.Context, id, clave string) (DocContent, error)
	SaveDocumentContent(ctx context.Context, id, clave, valor string, dot DocDot, lamport uint64, autoritativo bool, baseSeq int64) (int64, error)
	SaveDocumentCoedition(ctx context.Context, id, clave, valor string, dot DocDot, lamport uint64, autoritativo bool, chars []coedicion.Op) (int64, error)
	DocumentOpsSince(ctx context.Context, id string, desde int64) ([]DocOpSecuenciada, error)
	DocumentPresence(ctx context.Context, id, conn string) ([]DocMiembro, error)
	SaveDocumentChecked(ctx context.Context, id, clave, valor string, base DocContent, dot DocDot, autoritativo bool) (DocContent, error)
	DocumentVersions(ctx context.Context, id string) ([]DocVersion, error)
	RestoreDocumentVersion(ctx context.Context, id, version string) error
	CreateDocumentVersion(ctx context.Context, id, label string) (DocVersion, error)
	ExportDocument(ctx context.Context, id, formato string, plantilla bool, accesible *bool) ([]byte, error)
	DocumentSharing(ctx context.Context, id string) ([]DocPermission, []DocShare, error)
	GrantDocumentPermission(ctx context.Context, id, kind, principal, role string) error
	RevokeDocumentPermission(ctx context.Context, id, kind, principal string) error
	CreateDocumentShare(ctx context.Context, id, role string) (DocShare, error)
	RevokeDocumentShare(ctx context.Context, id, enlace string) error
	DocumentComments(ctx context.Context, id string, conResueltos bool) ([]DocComment, error)
	CreateDocumentComment(ctx context.Context, id, padre, cuerpo string) (DocComment, error)
	CreateAnchoredDocumentComment(ctx context.Context, id, cuerpo, marca, texto string) (DocComment, error)
	CreateCellDocumentComment(ctx context.Context, id, cuerpo string, a AnclaCelda) (DocComment, error)
	ResolveDocumentComment(ctx context.Context, id, comentario string, resuelto bool) error
	DeleteDocumentComment(ctx context.Context, id, comentario string) error
}

// SlidesData backs a Slides client from the encrypted private repository without
// exposing any legacy HTTP client fallback.
type SlidesData interface {
	ListSlides(ctx context.Context, q, cursor string) ([]SlidesDocument, string, error)
	CreateSlidePresentation(ctx context.Context, titulo string) (SlidesDocument, error)
	GetSlidePresentation(ctx context.Context, id string) (SlidesDocument, string, error)
	RenameSlidePresentation(ctx context.Context, id, titulo string) error
	DeleteSlidePresentation(ctx context.Context, id string) error
	SlideContent(ctx context.Context, id string) (SlidesContent, error)
	SaveSlideContent(ctx context.Context, id string, base SlidesContent, actor string, contador uint64, modelo string) (SlidesContent, error)
}

// NewPrivateDocs returns a fail-closed Docs client with no HTTP transport.
func NewPrivateDocs() *Docs {
	return &Docs{c: newPrivateClosedClient(), privateMode: true}
}

// WithPrivateData detaches any legacy HTTP client and binds Docs to the
// encrypted repository.
func (d *Docs) WithPrivateData(data DocsData) (*Docs, error) {
	if data == nil {
		return nil, ErrPrivateDataOnly
	}
	return &Docs{c: newPrivateClosedClient(), private: data, privateMode: true}, nil
}

// NewPrivateSlides returns a fail-closed Slides client with no HTTP transport.
func NewPrivateSlides() *Slides {
	return &Slides{c: newPrivateClosedClient(), privateMode: true}
}

// WithPrivateData detaches any legacy HTTP client and binds Slides to the
// encrypted repository.
func (s *Slides) WithPrivateData(data SlidesData) (*Slides, error) {
	if data == nil {
		return nil, ErrPrivateDataOnly
	}
	out := &Slides{c: newPrivateClosedClient(), private: data, privateMode: true}
	if docsData, ok := data.(DocsData); ok {
		out.docsPrivate = docsData
	}
	return out, nil
}

// Docs returns the companion Docs facade bound to the same private data source
// (or nil when the Slides receiver is nil).
func (s *Slides) Docs() *Docs {
	if s == nil {
		return nil
	}
	if s.docsPrivate != nil {
		return &Docs{c: newPrivateClosedClient(), private: s.docsPrivate, privateMode: true}
	}
	if s.privateMode {
		return NewPrivateDocs()
	}
	return &Docs{c: s.c}
}
