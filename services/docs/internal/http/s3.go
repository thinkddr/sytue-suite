// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// authnClaims extrae los claims de Sytue ID del contexto del request.
func authnClaims(r *http.Request) (authn.Claims, bool) {
	return authn.FromContext(r.Context())
}

// hydrateFromS3 descarga y deserializa un snapshot CRDT alojado en Object
// Storage (cuando el estado base es demasiado grande para guardarlo inline).
func (h *Handler) hydrateFromS3(ctx context.Context, blobKey string) (*domain.State, error) {
	if h.objects == nil {
		return nil, fmt.Errorf("docs: object storage no configurado")
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	rc, err := h.objects.Get(ctx, t, blobKey)
	if err != nil {
		return nil, fmt.Errorf("docs: descargar snapshot S3: %w", err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("docs: leer snapshot S3: %w", err)
	}
	return domain.StateFromJSON(raw)
}

// uploadSnapshot sube un snapshot CRDT grande a Object Storage bajo `key`.
func (h *Handler) uploadSnapshot(ctx context.Context, key string, data []byte) error {
	if h.objects == nil {
		return fmt.Errorf("docs: object storage no configurado")
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	// Comprimido: un snapshot CRDT es JSON y baja un 79 % medido; solo se lee
	// por el servicio (hydrateFromS3), nunca por URL prefirmada. Los exports NO:
	// se sirven prefirmados y llegarían al navegador tal cual están.
	return h.objects.Put(ctx, t, key, bytes.NewReader(data), int64(len(data)),
		objectstore.PutOptions{ContentType: "application/json", Comprimir: true})
}

// uploadExport sube un export (OOXML/ODF/PDF/...) a Object Storage.
func (h *Handler) uploadExport(ctx context.Context, key, contentType string, data []byte) error {
	if h.objects == nil {
		return fmt.Errorf("docs: object storage no configurado")
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return h.objects.Put(ctx, t, key, bytes.NewReader(data), int64(len(data)),
		objectstore.PutOptions{ContentType: contentType})
}

// snapshotKey deriva la clave S3 (relativa al prefijo de tenant) de un snapshot.
func snapshotKey(docID string, seq int64) string {
	return fmt.Sprintf("docs/%s/snapshots/%d.json", docID, seq)
}

// exportKey deriva la clave S3 de un export. Lleva un identificador ÚNICO, y no solo
// (documento, secuencia, formato), porque esa terna NO determina el contenido:
//
//   - `RenameDocument` cambia el título SIN tocar `seq` (documents.go), y el título va
//     dentro del fichero exportado. Exportar, renombrar y volver a exportar producía dos
//     ficheros distintos bajo la misma clave: el segundo pisaba al primero.
//   - `listExports` seguía mostrando las dos filas, y descargar la vieja entregaba el
//     contenido de la nueva. Un export histórico cambiaba retroactivamente, con el
//     `size_bytes` de su fila ya sin corresponder al objeto.
//
// Un export es un artefacto INMUTABLE con historial: cada uno merece su clave. Las filas
// antiguas siguen funcionando porque la descarga usa el `blob_key` guardado, no lo
// recalcula.
func exportKey(docID string, seq int64, id, ext string) string {
	return fmt.Sprintf("docs/%s/exports/%d-%s.%s", docID, seq, id, ext)
}
