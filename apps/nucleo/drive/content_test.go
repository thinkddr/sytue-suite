// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestContentOriginIsExplicitBoundedAndNeverAppliesToMetadata(t *testing.T) {
	item := summary{SourceKind: "file", SourceID: "copy", SizeBytes: 7, Checksum: digest([]byte("content"))}
	physical := bodyOrigin{Kind: "file", ID: "original", Epoch: readerEpoch}
	pointer := bodyPointer{ObjectID: bodyID("file", "original", "binary"), Reference: storage.Reference{RoomID: "!private:test", EventID: "$body"}, Origin: physical}
	source, err := pointer.source(item, "binary")
	if err != nil || source.SourceID != "original" || source.Epoch != readerEpoch || source.SizeBytes != item.SizeBytes || source.Checksum != item.Checksum {
		t.Fatal("reference copy lost content binding", source, err)
	}
	if _, err := pointer.source(item, "metadata"); err == nil {
		t.Fatal("binary content origin substituted private metadata")
	}
	for _, origin := range []bodyOrigin{{Kind: "folder", ID: "original"}, {Kind: "file", ID: "different", Epoch: readerEpoch}, {Kind: "file", ID: "original", Epoch: "bad"}, {Kind: "file", ID: "original\x00", Epoch: readerEpoch}} {
		pointer.Origin = origin
		if _, err := pointer.source(item, "binary"); err == nil {
			t.Fatal("invalid or changed content origin accepted", origin)
		}
	}
}
