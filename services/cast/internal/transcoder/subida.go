// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// subirDirectorio sube todo lo que hay bajo `dir` con la clave `prefijo` + ruta
// relativa (con barras de S3, sea cual sea el sistema de ficheros) y devuelve
// cuántos bytes subió. Un fichero vacío se sube igual: un playlist sin nada
// dentro es un error de ffmpeg que se verá al reproducir, no un motivo para
// perder el resto.
func (w *trabajador) subirDirectorio(ctx context.Context, t tenancy.Tenant, dir, prefijo string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, ruta)
		if err != nil {
			return err
		}
		clave := prefijo + filepath.ToSlash(rel)
		n, err := w.subirFichero(ctx, t, ruta, clave)
		if err != nil {
			return err
		}
		total += n
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("subir %q: %w", prefijo, err)
	}
	return total, nil
}

// subirFichero sube un fichero con su Content-Type y devuelve su tamaño.
func (w *trabajador) subirFichero(ctx context.Context, t tenancy.Tenant, ruta, clave string) (int64, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: ruta dentro del temporal del trabajo, la recorre WalkDir
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if err := w.objetos.Put(ctx, t, clave, f, info.Size(), objectstore.PutOptions{ContentType: service.TipoDe(clave)}); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// tamanoDe dice cuánto ocupa un objeto, o 0 si no existe: lo que se va a
// sobrescribir se resta de la cuota antes de sumar lo nuevo.
func (w *trabajador) tamanoDe(ctx context.Context, t tenancy.Tenant, clave string) (int64, error) {
	o, err := w.objetos.GetRange(ctx, t, clave, "bytes=0-0")
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("medir %q: %w", clave, err)
	}
	_ = o.Body.Close()
	return o.Total, nil
}
