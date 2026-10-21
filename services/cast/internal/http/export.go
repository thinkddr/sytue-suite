// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Export — GET /api/v1/export: «Exportar tus datos» (CAST-ENCARGO §5.2,
// platform/pkg/portabilidad). Los metadatos de MIS vídeos y con quién los
// comparto. Los ficheros no van: son gigas y ya se descargan uno a uno con
// `download_url`. Lo que otros subieron y puedo ver no es mío y no sale.
//
// Van también mis comentarios (en cualquier vídeo de cualquier organización), la
// lista de personas de TODOS mis vídeos, aunque hoy no estén en `people` —la lista
// se conserva al cambiar la visibilidad y vuelve a mandar si se restaura—, a quién
// he bloqueado y las denuncias que he puesto.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), quienDe(r), p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

type videoExportado struct {
	ID              uuid.UUID  `json:"id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Visibility      string     `json:"visibility"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DurationMS      *int64     `json:"duration_ms"`
	Width           *int32     `json:"width"`
	Height          *int32     `json:"height"`
	BytesOriginal   int64      `json:"bytes_original"`
	BytesTotal      int64      `json:"bytes_total"`
	CommentsEnabled bool       `json:"comments_enabled"`
	ChatEnabled     bool       `json:"chat_enabled"`
	DownloadEnabled bool       `json:"download_enabled"`
	Views           int64      `json:"views"`
	PublishedAt     *time.Time `json:"published_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type personaExportada struct {
	VideoID  uuid.UUID `json:"video_id"`
	UserID   string    `json:"user_id"`
	Name     string    `json:"name"`
	Username string    `json:"username"`
	AddedAt  time.Time `json:"added_at"`
}

func (h *Handlers) exportar(ctx context.Context, quien service.Quien, p *portabilidad.Paquete) error {
	var (
		videos   []videoExportado
		personas []personaExportada
		cursor   string
	)
	for {
		pag, err := h.svc.Biblioteca(ctx, quien, service.Filtro{Mios: true, Cursor: cursor, Limite: 100})
		if err != nil {
			return err
		}
		for _, v := range pag.Videos {
			videos = append(videos, videoExportado{
				ID: v.ID, Kind: string(v.Kind), Status: string(v.Status), Visibility: string(v.Visibility),
				Title: v.Title, Description: v.Description, DurationMS: v.DurationMS, Width: v.Width, Height: v.Height,
				BytesOriginal: v.BytesOriginal, BytesTotal: v.BytesTotal,
				CommentsEnabled: v.CommentsEnabled, ChatEnabled: v.ChatEnabled, DownloadEnabled: v.DownloadEnabled,
				Views: v.Views, PublishedAt: v.PublishedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
			})
			ps, err := h.svc.Personas(ctx, quien, v.ID)
			if err != nil {
				return err
			}
			for _, per := range ps {
				personas = append(personas, personaExportada{VideoID: v.ID, UserID: per.UserID, Name: per.Name, Username: per.Username, AddedAt: per.AddedAt})
			}
		}
		if pag.Siguiente == "" {
			break
		}
		cursor = pag.Siguiente
	}
	if err := portabilidad.Tabla(p, "videos", "video", portabilidad.Todas(videos)); err != nil {
		return err
	}
	if err := portabilidad.Tabla(p, "video_people", "person", portabilidad.Todas(personas)); err != nil {
		return err
	}
	if h.social == nil {
		return nil
	}
	q := socialDe(quien)
	comentarios, err := h.social.ComentariosDe(ctx, q)
	if err != nil {
		return err
	}
	if err := portabilidad.Tabla(p, "comments", "comment", portabilidad.Todas(comentarios)); err != nil {
		return err
	}
	// A quién ha bloqueado y qué ha denunciado ella (ADR 0074). No quién la
	// bloqueó ni quién la denunció: eso es de otras personas.
	bloqueos, err := h.social.Bloqueados(ctx, q)
	if err != nil {
		return err
	}
	if err := portabilidad.Tabla(p, "blocks", "block", portabilidad.Todas(bloqueos)); err != nil {
		return err
	}
	denuncias, err := h.social.DenunciasDe(ctx, q)
	if err != nil {
		return err
	}
	return portabilidad.Tabla(p, "reports", "report", portabilidad.Todas(denuncias))
}
