// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// bearerCtxKey transporta el token Bearer del usuario para reusarlo al llamar a
// Sytue Drive (propagación de identidad entre servicios de la suite).
type bearerCtxKey struct{}

// WithBearer inserta el token Bearer crudo en el contexto.
func WithBearer(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerCtxKey{}, token)
}

func bearerFrom(ctx context.Context) string {
	if v, ok := ctx.Value(bearerCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// DriveClient resuelve metadatos de adjuntos contra la API REST de Sytue Drive
// (GET /api/v1/files/{id}). Propaga el JWT del usuario para que Drive aplique su
// propia autorización/RLS. Clean-room: cliente HTTP propio, sin acoplamiento.
type DriveClient struct {
	baseURL string
	http    *http.Client
}

// NewDriveClient construye el cliente de Drive.
func NewDriveClient(baseURL string) *DriveClient {
	return &DriveClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// driveFile es la vista mínima del fichero que devuelve Drive.
type driveFile struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}

// Resolve consulta Drive por el fichero de un adjunto y devuelve sus metadatos como
// Attachment.
//
// Con canal (todo adjunto de un mensaje) va por la ruta de ÁMBITO DE CANAL
// (/channels/{channelID}/files/{fileID}), que es la única que ve un fichero subido al
// canal —invisible en «Mi unidad» por la RLS de ámbito— y que además exige MEMBRESÍA
// del canal: así, publicar un mensaje con un file_id ajeno (el de otro canal privado,
// por ejemplo) no cuela. Esa misma ruta resuelve, cayendo al ámbito personal, el
// fichero adjuntado POR REFERENCIA desde el Drive del usuario y los adjuntos
// heredados de antes de la migración de ámbito.
//
// Con channelID = uuid.Nil va por la ruta personal (/files/{fileID}): es el caso de
// los EMOJI personalizados, que son un activo del tenant y no de un canal.
func (c *DriveClient) Resolve(ctx context.Context, channelID, fileID uuid.UUID) (domain.Attachment, error) {
	token := bearerFrom(ctx)
	if token == "" {
		// Sin identidad propagada no podemos autorizar en Drive: conservar el id.
		return domain.Attachment{}, domain.ErrForbidden
	}
	url := fmt.Sprintf("%s/api/v1/files/%s", c.baseURL, fileID)
	if channelID != uuid.Nil {
		url = fmt.Sprintf("%s/api/v1/channels/%s/files/%s", c.baseURL, channelID, fileID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return domain.Attachment{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Attachment{}, fmt.Errorf("drive: solicitud: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusForbidden:
		return domain.Attachment{}, domain.ErrNotFound
	default:
		return domain.Attachment{}, fmt.Errorf("drive: respuesta %d", resp.StatusCode)
	}

	var f driveFile
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return domain.Attachment{}, fmt.Errorf("drive: decodificar: %w", err)
	}
	return domain.Attachment{
		FileID:      fileID,
		Name:        f.Name,
		ContentType: f.ContentType,
		SizeBytes:   f.SizeBytes,
	}, nil
}
