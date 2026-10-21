// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package catalogo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Materializar verifica los objetos existentes y publica los bytes canónicos que
// falten. Un catálogo anunciado siempre tiene bytes reales en el almacén.
func (c *Catalogo) Materializar(ctx context.Context, objetos *objectstore.Client) error {
	t := tenancy.Tenant{Slug: "shared"}
	for _, rec := range c.recursos {
		rc, err := objetos.Get(ctx, t, rec.Clave)
		if err == nil {
			b, readErr := io.ReadAll(io.LimitReader(rc, int64(len(rec.Contenido))+1))
			_ = rc.Close()
			if readErr != nil {
				return fmt.Errorf("catalogo: leer %s: %w", rec.Nombre, readErr)
			}
			hash := sha256.Sum256(b)
			if hex.EncodeToString(hash[:]) == rec.SHA256 {
				continue
			}
		} else if !errors.Is(err, objectstore.ErrNotFound) {
			return fmt.Errorf("catalogo: comprobar %s: %w", rec.Nombre, err)
		}
		if err := objetos.Put(ctx, t, rec.Clave, bytes.NewReader(rec.Contenido), int64(len(rec.Contenido)), objectstore.PutOptions{ContentType: rec.Tipo}); err != nil {
			return fmt.Errorf("catalogo: publicar %s: %w", rec.Nombre, err)
		}
	}
	return nil
}
