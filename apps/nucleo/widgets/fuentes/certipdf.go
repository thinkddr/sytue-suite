// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"cmp"
	"context"
	"path"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// maxRecientesPDF es lo que la portada de CertiPDF enseña: con más, el total no se da.
const maxRecientesPDF = 48

// CertiPDF lee las peticiones de firma recibidas (las pendientes, primero) y los PDF
// recientes. Cada lectura falla por su lado. Peticiones: 2.
func CertiPDF(_ Comunes, pdf *api.PDF) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		var (
			lecturas   sync.WaitGroup
			peticiones api.PDFPeticionesFirma
			errP       error
		)
		lecturas.Go(func() { peticiones, errP = pdf.PeticionesFirma(ctx) })
		lista, errL := pdf.Lista(ctx)
		lecturas.Wait()
		if err := todoFalla(errP, errL); err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		items := make([]widgets.Item, 0, 2*widgets.MaxItems)
		if errP == nil {
			firmas, pendientes := itemsDeFirmas(peticiones)
			d.Counters = append(d.Counters, widgets.Counter{Kind: "pending", Value: pendientes})
			items = append(items, firmas...)
		}
		if errL == nil {
			if len(lista) < maxRecientesPDF {
				d.Counters = append(d.Counters, widgets.Counter{Kind: "files", Value: int64(len(lista))})
			}
			items = append(items, itemsDePDF(lista)...)
		}
		d.Items = widgets.SanitizeItems(items)
		return d, nil
	}
}

func itemsDeFirmas(ps api.PDFPeticionesFirma) ([]widgets.Item, int64) {
	var pendientes int64
	items := make([]widgets.Item, 0, widgets.MaxItems)
	for _, r := range ps.Recibidas {
		if r.Estado != "" && r.Estado != "pending" && r.Estado != "pendiente" {
			continue
		}
		pendientes++
		if llenos(items) {
			continue
		}
		titulo := strings.TrimSpace(path.Base(r.Ruta))
		if titulo == "" || titulo == "." || titulo == "/" {
			titulo = strings.TrimSpace(cmp.Or(r.NombrePide, r.Solicitante))
		}
		if titulo == "" {
			continue
		}
		items = append(items, widgets.Item{
			Title: titulo, Subtitle: r.NombrePide, Badge: "FIRMAR",
			StartAt: unix(r.Creada), Color: "#d83b35", Unread: true,
		})
	}
	return items, pendientes
}

func itemsDePDF(lista []api.File) []widgets.Item {
	items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
	for _, f := range lista {
		if nombre := strings.TrimSpace(f.Name); nombre != "" {
			items = append(items, widgets.Item{Title: nombre, Badge: "PDF", StartAt: unix(f.UpdatedAt)})
		}
		if llenos(items) {
			break
		}
	}
	return items
}
