// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"iter"
	"net/http"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Filas de la entrega de Fotos. Los bytes de cada foto NO van aquí: el original
// es un archivo de Sytue Drive y sale en la carpeta de Drive. Esto es lo que Fotos
// sabe de cada una y Drive no: cuándo y dónde se hizo, con qué cámara, qué sale y
// en qué álbumes está. drive_id la une con su fila de drive/files.
type (
	fotoExportada struct {
		ID          uuid.UUID  `json:"id"`
		DriveID     string     `json:"drive_id"`
		Name        string     `json:"name"`
		ContentType string     `json:"content_type"`
		SizeBytes   int64      `json:"size_bytes"`
		Width       int        `json:"width"`
		Height      int        `json:"height"`
		TakenAt     *time.Time `json:"taken_at"`
		UploadedAt  time.Time  `json:"uploaded_at"`
		Latitude    *float64   `json:"latitude"`
		Longitude   *float64   `json:"longitude"`
		Camera      string     `json:"camera"`
		Lens        string     `json:"lens"`
		Favorite    bool       `json:"favorite"`
		Description string     `json:"description"`
		Tags        []string   `json:"tags"`
	}
	albumExportado struct {
		ID        uuid.UUID   `json:"id"`
		Title     string      `json:"title"`
		Shared    bool        `json:"shared"`
		Photos    []uuid.UUID `json:"photos"`
		CreatedAt time.Time   `json:"created_at"`
		UpdatedAt time.Time   `json:"updated_at"`
	}
)

// ExportarDatos entrega Fotos para «Exportar tus datos» (GET /api/v1/export).
func (h *Handlers) ExportarDatos(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportarDatos(r.Context(), sub(r), p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportarDatos(ctx context.Context, duenyo string, p *portabilidad.Paquete) error {
	if err := portabilidad.Tabla(p, "photos", "photo", h.fotosDe(ctx, duenyo)); err != nil {
		return err
	}
	albumes, err := h.store.Albumes(ctx, duenyo)
	if err != nil {
		return err
	}
	filas := make([]albumExportado, 0, len(albumes))
	for _, a := range albumes {
		fotos, err := h.store.FotosDelAlbum(ctx, a.ID, duenyo)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(fotos))
		for i, f := range fotos {
			ids[i] = f.ID
		}
		filas = append(filas, albumExportado{
			ID: a.ID, Title: a.Titulo, Shared: a.Compartido, Photos: ids, CreatedAt: a.CreadoEn, UpdatedAt: a.ActualizadoEn,
		})
	}
	return portabilidad.Tabla(p, "albums", "album", portabilidad.Todas(filas))
}

// fotosDe recorre la línea de tiempo entera por páginas.
func (h *Handlers) fotosDe(ctx context.Context, duenyo string) iter.Seq2[fotoExportada, error] {
	return func(yield func(fotoExportada, error) bool) {
		f := store.FiltroLinea{Duenyo: duenyo, Limite: 200}
		for {
			fotos, siguiente, err := h.store.LineaDeTiempo(ctx, f)
			if err != nil {
				yield(fotoExportada{}, err)
				return
			}
			for _, a := range fotos {
				if !yield(fotoDe(a), nil) {
					return
				}
			}
			if siguiente == nil {
				return
			}
			f.Desde = siguiente
		}
	}
}

func fotoDe(a domain.Activo) fotoExportada {
	ancho, alto := a.Vistas()
	return fotoExportada{
		ID: a.ID, DriveID: a.DriveID, Name: a.Nombre, ContentType: a.Tipo, SizeBytes: a.Bytes,
		Width: ancho, Height: alto, TakenAt: a.TomadaEn, UploadedAt: a.SubidaEn,
		Latitude: a.Lat, Longitude: a.Lon, Camera: a.Camara, Lens: a.Objetivo,
		Favorite: a.Favorita, Description: a.Descripcion, Tags: a.Etiquetas,
	}
}
