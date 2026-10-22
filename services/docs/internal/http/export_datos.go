// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// exportData entrega los documentos para «Exportar tus datos» (GET
// /api/v1/export): cada documento PROPIO en Office Open XML —.docx, .xlsx,
// .pptx—, que es lo que abren y convierten Google Docs y Microsoft 365, y la
// tabla documents. Se materializa el estado de AHORA: la copia de la unidad solo
// se reescribe cuando alguien pulsa «Guardar en Drive», y podría ser de ayer.
func (h *Handler) exportData(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportarDocumentos(r.Context(), subj, p); err != nil {
		_ = p.Fallo(err)
		return
	}
	// A quién ha bloqueado para las peticiones de firma de fuera (ADR 0059): es suyo.
	if h.peticionesFirma != nil {
		bloqueos, err := h.peticionesFirma.BloqueosFirma(r.Context(), subj.Sub)
		if err == nil {
			err = portabilidad.Tabla(p, "signature_blocks", "block", portabilidad.Todas(bloqueos))
		}
		if err != nil {
			_ = p.Fallo(err)
			return
		}
	}
	_ = p.Cerrar()
}

func (h *Handler) exportarDocumentos(ctx context.Context, subj domain.Subject, p *portabilidad.Paquete) error {
	const pagina = 100
	f := domain.ListFilter{OwnerSub: subj.Sub, Limit: pagina}
	var todos []domain.Document
	for {
		docs, err := h.svc.List(ctx, subj, f)
		if err != nil {
			return err
		}
		for _, d := range docs {
			if d.OwnerSub != subj.Sub {
				continue
			}
			state, _, err := h.svc.LoadFull(ctx, subj, d.ID, h.hydrateFromS3)
			if err != nil {
				return err
			}
			var comments []domain.Comment
			if d.Kind == domain.KindDocument {
				if comments, err = h.store.ListComments(ctx, d.ID, true); err != nil {
					return err
				}
			}
			data, _, ext, err := domain.ExportWithComments(d.Kind, d.Title, state, domain.FormatOOXML, comments)
			if err != nil {
				return err
			}
			titulo := strings.TrimSpace(d.Title)
			if titulo == "" {
				titulo = d.ID
			}
			if err := p.Fichero("documents/"+titulo+"."+ext, d.UpdatedAt, bytes.NewReader(data)); err != nil {
				return err
			}
			todos = append(todos, d)
		}
		if len(docs) < pagina {
			break
		}
		ultimo := docs[len(docs)-1]
		f.BeforeUpdatedAt, f.BeforeID = &ultimo.UpdatedAt, ultimo.ID
	}
	return portabilidad.Tabla(p, "documents", "document", portabilidad.Todas(todos))
}
