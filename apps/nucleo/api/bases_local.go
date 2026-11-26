// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
)

// Lo que un repositorio de Bases en el dispositivo puede hacer además de BasesData
// (PLAN-EDICION-LOCAL.md §5.7 y §6.12, B-04). Cada grupo es opcional: la fachada lo usa
// si los datos privados lo cumplen y, si no, falla cerrada como siempre.

// BasesSchemaData cambia fórmulas, opciones, el orden de los campos y las vistas.
type BasesSchemaData interface {
	SetFieldFormula(context.Context, string, string) error
	SetFieldChoices(context.Context, string, []string) error
	ReorderFields(context.Context, string, []string) error
	UpdateView(context.Context, string, string, BasesViewConfig) (BasesView, error)
}

// BasesQueryData lee con la búsqueda y el orden de un momento.
type BasesQueryData interface {
	ListRecordsCon(context.Context, string, string, string, int, BasesLectura) (BasesRecordPage, error)
}

// BasesImportData crea una tabla a partir de un CSV.
type BasesImportData interface {
	ImportCSV(context.Context, string, string, []byte) (BasesTable, error)
}

// BasesAttachmentData guarda y sirve los adjuntos de los registros.
type BasesAttachmentData interface {
	UploadAttachment(context.Context, string, string, string, []byte) (BasesAttachment, error)
	DownloadAttachment(context.Context, string, string) (io.ReadCloser, error)
	DownloadThumb(context.Context, string, string) (io.ReadCloser, error)
	DeleteAttachment(context.Context, string, string) error
}

// BasesCommentData guarda los comentarios de los registros.
type BasesCommentData interface {
	ListComments(context.Context, string) ([]BasesComment, error)
	CreateComment(context.Context, string, string, string) (BasesComment, error)
	DeleteComment(context.Context, string) error
}

// BasesAuditData es la actividad de una base que guarda el propio dispositivo (B-06). Un
// repositorio que no la lleva la cumple igual y falla con ErrPrivateBasesOperation, que
// es lo de siempre.
type BasesAuditData interface {
	ListAudit(context.Context, string, int64, int) ([]BasesAuditEntry, error)
}
