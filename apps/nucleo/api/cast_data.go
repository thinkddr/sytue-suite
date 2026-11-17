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
