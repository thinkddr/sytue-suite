// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
)

// DriveFileReplacer pins an editor's original file version. A private writer
// must reject a stale original instead of treating its result as a fresh upload.
type DriveFileReplacer interface {
	ReplaceFile(context.Context, File, string, io.Reader, int64) (File, error)
}

// ReplaceFile keeps the existing legacy upload route. Private editors require
// the conditional repository port and never fall back to an ordinary upload.
func (d *Drive) ReplaceFile(ctx context.Context, original File, contentType string, input io.Reader, size int64) (File, error) {
	if d.data != nil {
		writer, ok := d.data.(DriveFileReplacer)
		if !ok {
			return File{}, ErrPrivateDriveOperation
		}
		return conAviso[File](d)(writer.ReplaceFile(ctx, original, contentType, input, size))
	}
	return d.Upload(ctx, original.FolderID, original.Name, contentType, input, size)
}
