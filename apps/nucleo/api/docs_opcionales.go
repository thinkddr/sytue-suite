// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// Interfaces OPCIONALES de los datos privados de Docs (PLAN-EDICION-LOCAL §5.7 y O-09):
// lo que la fachada pide al servicio por HTTP y un almacén en el dispositivo puede hacer
// él mismo. Si el `DocsData` enchufado con [Docs.WithPrivateData] cumple una, el método
// de la fachada le delega; si no, sigue como hoy (por el cliente HTTP, que en modo
// privado falla cerrado). Ninguna cambia lo que hace la edición conectada.

// datos son los datos privados enchufados, o nil (también con un receptor nil).
func (d *Docs) datos() DocsData {
	if d == nil {
		return nil
	}
	return d.private
}

// DocsTrashData es la papelera: [Docs.Papelera], [Docs.Restaurar], [Docs.Purgar] y
// [Docs.Vaciar].
type DocsTrashData interface {
	TrashedDocuments(ctx context.Context, kind DocKind) ([]Document, error)
	RestoreDocument(ctx context.Context, id string) error
	PurgeDocument(ctx context.Context, id string) error
	EmptyTrash(ctx context.Context, kind DocKind, despues string) (fallidos int, siguiente string, err error)
}

// DocsCopyData es «Hacer una copia» ([Docs.CreateCopy]).
type DocsCopyData interface {
	CopyDocument(ctx context.Context, original Document, titulo string) (Document, error)
}

// DocsTemplatesData es la galería de plantillas ([Docs.Templates], [Docs.CreateFromTemplate]).
type DocsTemplatesData interface {
	DocumentTemplates(ctx context.Context, locale string) (DocTemplates, error)
	CreateDocumentFromTemplate(ctx context.Context, n DocNuevo) (Document, error)
}

// DocsThemesData son los temas ([Docs.Themes], [Docs.ApplyTheme], [Docs.PreviewTheme] y
// [Docs.SaveBrand]).
type DocsThemesData interface {
	DocumentThemes(ctx context.Context) (DocThemes, error)
	ApplyDocumentTheme(ctx context.Context, id, tema string, versionMarca int) (bool, error)
	PreviewDocumentTheme(ctx context.Context, id, tema string, versionMarca int) (DocThemePreview, error)
	SaveDocumentBrand(ctx context.Context, m DocBrand) (int, error)
}

// DocsBlocksData son los bloques reutilizables ([Docs.Blocks], [Docs.Block]).
type DocsBlocksData interface {
	DocumentBlocks(ctx context.Context, kind DocKind, locale string) (DocBlocks, error)
	DocumentBlock(ctx context.Context, clave, locale string) (string, error)
}
