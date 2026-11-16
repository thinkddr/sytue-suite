// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type thumbnailNoMedia struct {
	*queryRecords
	calls int
}

func (m *thumbnailNoMedia) StorageObject(context.Context, storage.Reference, string) (storage.ObjectVersion, error) {
	m.calls++
	return storage.ObjectVersion{}, ErrSource
}

func TestPrivateThumbnailRejectsSizeTypeAndHiddenAncestorsBeforeMedia(t *testing.T) {
	for _, test := range []struct {
		name, contentType string
		size              int64
		hidden            bool
		want              error
	}{
		{"large", "image/png", storage.MaxDriveThumbnailBytes + 1, false, ErrUnsupportedMedia},
		{"type", "application/octet-stream", 12, false, ErrUnsupportedMedia},
		{"hidden", "image/png", 12, true, ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := queryFixture(t, 0)
			root := "root"
			var deleted *time.Time
			if test.hidden {
				now := time.Now().UTC()
				deleted = &now
			}
			addQueryRow(t, r, "folder", "parent", api.Folder{ID: "parent", Name: "Parent", ParentID: &root, DeletedAt: deleted})
			addQueryRow(t, r, "file", "image", api.File{ID: "image", Name: "Private image", FolderID: "parent", ContentType: test.contentType, SizeBytes: test.size})
			media := &thumbnailNoMedia{queryRecords: r.cfg.Crypto.(*queryRecords)}
			r.cfg.Crypto = media
			body, err := r.Thumbnail(t.Context(), "image")
			if body != nil || !errors.Is(err, test.want) || media.calls != 0 {
				t.Fatal("private thumbnail crossed a pre-media guard", body, err, media.calls)
			}
		})
	}
}
