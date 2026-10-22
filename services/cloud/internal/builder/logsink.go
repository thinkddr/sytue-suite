// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"bytes"
	"context"
	"fmt"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// S3LogSink persiste los logs de build en el Object Storage S3 (bucket
// build-cache), bajo el prefijo del tenant. Implementa builder.LogSink.
type S3LogSink struct {
	blobs *objectstore.Client
}

// NewS3LogSink construye el sink sobre el cliente de objectstore dado.
func NewS3LogSink(blobs *objectstore.Client) *S3LogSink { return &S3LogSink{blobs: blobs} }

// PutBuildLog sube el log de una build y devuelve su clave (relativa al prefijo
// de tenant que añade objectstore).
func (s *S3LogSink) PutBuildLog(ctx context.Context, tenant, buildID string, logs []byte) (string, error) {
	t, err := tenancy.New(tenant)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("build-logs/%s.log", buildID)
	// Comprimido: un log de build es texto que se lee por el servicio, no por URL.
	if err := s.blobs.Put(ctx, t, key, bytes.NewReader(logs), int64(len(logs)),
		objectstore.PutOptions{ContentType: "text/plain; charset=utf-8", Comprimir: true}); err != nil {
		return "", fmt.Errorf("subir log de build: %w", err)
	}
	return key, nil
}
