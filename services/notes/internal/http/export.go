// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"bytes"
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Export entrega los cuadernos para «Exportar tus datos» (GET /api/v1/export):
// cada cuaderno PROPIO en su HTML nativo —el volcado completo, que además se ve
// al abrirlo— y las tablas de cuadernos y notas. Los de otros que se pueden leer
// (los de la organización) no son datos de quien exporta y no salen.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), sub(r), p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportar(ctx context.Context, sujeto string, p *portabilidad.Paquete) error {
	todos, err := h.store.ListNotebooks(ctx, sujeto)
	if err != nil {
		return err
	}
	var (
		propios []domain.Notebook
		notas   []domain.Note
	)
	for _, nb := range todos {
		if nb.OwnerSub != sujeto {
			continue
		}
		// ExportNotebook deja la traza de auditoría de cada cuaderno, como cuando se
		// exporta uno suelto desde la aplicación.
		nb, deNb, err := h.store.ExportNotebook(ctx, nb.ID, sujeto)
		if err != nil {
			return err
		}
		html := domain.Exportar(nb, deNb, domain.FormatoHTML)
		nombre := domain.NombreDeFicheroExportado(nb.Title, domain.FormatoHTML)
		if err := p.Fichero("notebooks/"+nombre, nb.UpdatedAt, bytes.NewReader(html)); err != nil {
			return err
		}
		propios = append(propios, nb)
		notas = append(notas, deNb...)
	}
	if err := portabilidad.Tabla(p, "notebooks", "notebook", portabilidad.Todas(propios)); err != nil {
		return err
	}
	return portabilidad.Tabla(p, "notes", "note", portabilidad.Todas(notas))
}
