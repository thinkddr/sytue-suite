// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
)

// PhotosDirectIngestData retains OS inputs and derives their media on the authorized
// device. job is one explicit import's stable UUID, not a path or filename.
type PhotosDirectIngestData interface {
	ImportarFoto(context.Context, string, string, string, io.Reader, int64) (Photo, error)
	ReanudarImportaciones(context.Context) ([]Photo, error)
}

func (p *Photos) CanImportPrivatePhotos() bool {
	if p == nil || p.data == nil {
		return false
	}
	_, ok := p.data.(PhotosDirectIngestData)
	return ok
}

// ImportarFoto never sends an original to the legacy Drive or Photos endpoints.
func (p *Photos) ImportarFoto(ctx context.Context, job, name, contentType string, input io.Reader, size int64) (Photo, error) {
	if p != nil && p.data != nil {
		if ingest, ok := p.data.(PhotosDirectIngestData); ok {
			return ingest.ImportarFoto(ctx, job, name, contentType, input, size)
		}
	}
	return Photo{}, ErrPrivatePhotosOperation
}

func (p *Photos) ReanudarImportaciones(ctx context.Context) ([]Photo, error) {
	if p != nil && p.data != nil {
		if ingest, ok := p.data.(PhotosDirectIngestData); ok {
			return ingest.ReanudarImportaciones(ctx)
		}
	}
	return nil, ErrPrivatePhotosOperation
}
