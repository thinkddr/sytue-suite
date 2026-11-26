// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type CastData interface {
	PrivateStorageScope() storage.Scope
	Videos(context.Context, string, string, string, string, int) (CastPage, error)
	Video(context.Context, string) (CastVideo, error)
	Play(context.Context, string) (CastPlay, error)
}

// CastPrivateMedia never exposes a provider URL or a plaintext spool file.
// Its implementation owns cancellation, authenticated seeking and retirement.
type CastPrivateMedia interface {
	io.ReadSeekCloser
	Size() int64
}

type CastMediaData interface {
	OpenMedia(context.Context, string) (CastPrivateMedia, error)
}

var ErrPrivateCastOperation = errors.New("cast: operation unavailable through private data")

// UsesPrivateData reports whether this facade is bound to the device's private
// repository, whose recipient sharing is not yet available.
func (c *Cast) UsesPrivateData() bool { return c != nil && c.data != nil }

func (c *Cast) WithPrivateData(data CastData) (*Cast, error) {
	if c == nil || c.c == nil || c.pub == nil || c.raiz == nil || c.data != nil || data == nil {
		return nil, ErrPrivateCastOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "cast" {
		return nil, ErrPrivateCastOperation
	}
	return &Cast{c: privateDataClient(c.c, ErrPrivateCastOperation), pub: privateDataClient(c.pub, ErrPrivateCastOperation), raiz: privateDataClient(c.raiz, ErrPrivateCastOperation), data: data}, nil
}

// CastExploreData es OPCIONAL: un CastData que además tiene su propia portada y sus
// canales. La cumple la videoteca de Sytue Office (`local/videoteca`), donde «publicado»
// es un estado local que enseña el vídeo en la portada de ESA app en ESE equipo (D-26).
// El repositorio cifrado no la cumple, y entonces Explore y Channel siguen cerrados.
type CastExploreData interface {
	Explore(ctx context.Context, cursor string, limite int) (CastExplore, error)
	Channel(ctx context.Context, org, cursor string) (CastChannel, error)
}

// NewPrivateCast es un Cast sin transporte HTTP, enchufado solo a data: lo que data no
// atiende falla con ErrPrivateCastOperation sin abrir ninguna conexión.
func NewPrivateCast(data CastData) (*Cast, error) {
	return NewCast("", nil, nil).WithPrivateData(data)
}
