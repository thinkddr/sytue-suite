// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
)

// NotesExportData produces a user-requested export entirely on the authorized
// device. It never delegates readable notebook content to a server.
type NotesExportData interface {
	ExportNotebook(context.Context, string, string, string) (io.ReadCloser, string, error)
}
