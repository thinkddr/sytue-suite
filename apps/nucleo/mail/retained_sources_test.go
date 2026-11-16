// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateMailRetainedSourcesExposeEveryExactOriginalWithoutSQL(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	f.source.forbidPages = true
	pages, opens := f.source.pages, f.source.opens
	var descriptors []storage.StreamSourceRow
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := f.repo.RetainedSourcePage(t.Context(), cursor, 2)
		if err != nil || page.Validate(testScope, testEpoch, cursor) != nil {
			t.Fatal("invalid retained page", err)
		}
		descriptors = append(descriptors, page.Rows...)
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if len(descriptors) != len(f.source.rows) {
		t.Fatal("source disappeared")
	}
	for _, row := range descriptors {
		reader, err := f.repo.OpenRetainedSource(t.Context(), row)
		if err != nil {
			t.Fatal(row.Kind, err)
		}
		raw, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal(readErr, closeErr)
		}
		original := f.source.bodies[storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}]
		if !bytes.Equal(raw, original) {
			t.Fatal("original changed", row.Kind)
		}
		clear(raw)
	}
	if f.source.pages != pages || f.source.opens != opens {
		t.Fatal("encrypted recovery returned to SQL")
	}
}

func TestPrivateMailRetainedReaderRejectsDescriptorForgeryAndDrains(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	row := f.source.rows[0]
	altered := row
	altered.SHA256 = digest([]byte("foreign"))
	if _, err := f.repo.OpenRetainedSource(t.Context(), altered); !errors.Is(err, ErrRecord) {
		t.Fatal("changed descriptor admitted", err)
	}
	view, cancel := context.WithCancel(t.Context())
	reader, err := f.repo.OpenRetainedSource(view, row)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = f.repo.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatal("retired reader remained readable", err)
	}
	_ = reader.Close()
}
