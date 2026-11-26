// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// FotoPorHuella busca, entre las fotos importadas directamente en este dispositivo
// (ImportarFoto), la que tiene ese original: el SHA-256 en hexadecimal de sus bytes.
// Las que están en la papelera no cuentan. ok es false si no hay ninguna.
//
// Es lo que deja abrir un fichero de imagen del disco («Abrir con», arrastrarlo o
// pasarlo en la línea de órdenes) sin importarlo dos veces: la segunda vez se abre la
// que ya estaba (PLAN-EDICION-LOCAL.md §5.11, FO-06).
func (r *Repository) FotoPorHuella(parent context.Context, sha256Hex string) (api.Photo, bool, error) {
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if len(sha256Hex) != 64 {
		return api.Photo{}, false, nil
	}
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Photo{}, false, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return api.Photo{}, false, err
	}
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return api.Photo{}, false, err
	}
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return api.Photo{}, false, err
		}
		asset, huella, err := r.huellaDeFila(raw)
		clear(raw)
		if err != nil {
			return api.Photo{}, false, err
		}
		if huella == "" || asset.DeletedAt != nil || asset.Owner != r.scope.Subject {
			continue
		}
		if huella == sha256Hex {
			return conEstado(photoView(asset)), true, nil
		}
	}
	return api.Photo{}, false, ctx.Err()
}

// HuellasConocidas son los SHA-256 (en hexadecimal, en minúsculas) de los originales
// importados directamente en este dispositivo, también los que están en la papelera: lo
// que «Importar del carrete» y las carpetas vigiladas ya no vuelven a traer, ni siquiera
// lo que la persona tiró (FO-04).
func (r *Repository) HuellasConocidas(parent context.Context) (map[string]bool, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return nil, err
	}
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(versions))
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		asset, huella, err := r.huellaDeFila(raw)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if huella == "" || asset.Owner != r.scope.Subject {
			continue
		}
		out[huella] = true
	}
	return out, ctx.Err()
}
