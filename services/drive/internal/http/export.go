// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"net/http"
	"path"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// archivoExportado es una fila de files.<formato>: dónde quedó en el ZIP y qué era.
type archivoExportado struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Export entrega Drive para «Exportar tus datos» (GET /api/v1/export): cada
// archivo con sus bytes, en su carpeta, y la tabla files con lo que se sabe de él.
//
// Qué es de quién: en una cuenta personal, todo el árbol; en una organización, el
// árbol es de la organización y solo salen los archivos que subió la persona.
// Papelera, versiones anteriores y unidades de equipo no salen: no son el
// estado actual de lo suyo.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), subFromCtx(r), p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportar(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	root, err := h.svc.Root(ctx)
	if err != nil {
		return err
	}
	type pendiente struct {
		id   uuid.UUID
		ruta string
	}
	var filas []archivoExportado
	cola := []pendiente{{id: root.ID}}
	for len(cola) > 0 {
		c := cola[0]
		cola = cola[1:]
		subs, err := h.svc.ListSubfolders(ctx, c.id)
		if err != nil {
			return err
		}
		for _, s := range subs {
			cola = append(cola, pendiente{id: s.ID, ruta: path.Join(c.ruta, s.Name)})
		}
		for cursor := ""; ; {
			pagina, err := h.svc.ListFilesPage(ctx, c.id, cursor, 200)
			if err != nil {
				return err
			}
			for _, f := range pagina.Files {
				if !t.Personal() && f.OwnerSub != sub {
					continue
				}
				ruta := path.Join(c.ruta, f.Name)
				if err := h.exportarArchivo(ctx, p, f.ID, ruta); err != nil {
					return err
				}
				filas = append(filas, archivoExportado{
					Path: "files/" + ruta, Name: f.Name, SizeBytes: f.SizeBytes, ContentType: f.ContentType,
					Version: f.Version, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
				})
			}
			if pagina.NextCursor == "" || pagina.NextCursor == cursor {
				break
			}
			cursor = pagina.NextCursor
		}
	}
	return portabilidad.Tabla(p, "files", "file", portabilidad.Todas(filas))
}

func (h *Handlers) exportarArchivo(ctx context.Context, p *portabilidad.Paquete, id uuid.UUID, ruta string) error {
	f, rc, err := h.svc.OpenFileContent(ctx, id)
	if err != nil {
		return err
	}
	defer rc.Close()
	return p.FicheroDeTamano("files/"+ruta, f.UpdatedAt, f.SizeBytes, rc)
}
