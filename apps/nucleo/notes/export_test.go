// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

func TestPrivateNotebookExportRunsAllFormatsOnAuthenticatedDevice(t *testing.T) {
	r, _, source := notesFixture(t)
	const book = "12345678-1234-1234-1234-123456789abd"
	const note = "12345678-1234-1234-1234-123456789abe"
	source.rows["notebook/"+book] = []byte(`{"id":"` + book + `","owner_sub":"alice","title":"Private notebook","visibility":"private","layout":"list"}`)
	source.rows["note/"+note] = []byte(`{"id":"` + note + `","notebook_id":"` + book + `","owner_sub":"alice","kind":"text","title":"Private note","content":"<p>device-only content</p>","data":{"precise":900719925474099312345}}`)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"html", "md", "markdown", "txt", "pdf", "onex"} {
		t.Run(format, func(t *testing.T) {
			body, name, err := r.ExportNotebook(t.Context(), book, note, format)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(body)
			_ = body.Close()
			if err != nil || len(raw) == 0 || !strings.HasPrefix(name, "Private note.") {
				t.Fatal("private export missing", name, err)
			}
			if format == "pdf" {
				if !strings.HasPrefix(string(raw), "%PDF-") {
					t.Fatal("not a PDF")
				}
			} else if !strings.Contains(string(raw), "device-only content") {
				t.Fatal("private content omitted")
			}
			if (format == "onex" || format == "html") && !strings.Contains(string(raw), "900719925474099312345") {
				t.Fatal("precise data changed")
			}
		})
	}
	for _, input := range []struct {
		book, note, format string
		want               error
	}{{book, "", "unknown", model.ErrInvalidInput}, {book, "missing", "html", ErrNotFound}, {"missing", "", "html", ErrNotFound}} {
		body, _, err := r.ExportNotebook(t.Context(), input.book, input.note, input.format)
		if body != nil || !errors.Is(err, input.want) {
			t.Fatal("invalid export became readable", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if body, _, err := r.ExportNotebook(ctx, book, "", "html"); body != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled export became readable", err)
	}
	body, _, err := r.ExportNotebook(t.Context(), book, "", "html")
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err == nil || len(raw) != 0 {
		t.Fatal("closed owner returned private export bytes", err, len(raw))
	}
}
