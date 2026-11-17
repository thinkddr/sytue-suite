// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMetadataEditPreservesUnknownJSONNumbersAndOriginalBytes(t *testing.T) {
	r := queryFixture(t, 0)
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	raw := []byte(`{"id":"file","folder_id":"root","name":"before.txt","size_bytes":0,"version":1,"extension":{"integer":900719925474099512345,"scientific":1.234567890123456789e+99}}`)
	original := bytes.Clone(raw)
	item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: "file", Payload: raw}, "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := r.changedMetadata(ctx, item, raw, metadataRequest{Operation: "rename", Kind: api.DriveFile, ID: "file", Value: "after.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated, []byte(`900719925474099512345`)) || !bytes.Contains(updated, []byte(`1.234567890123456789e+99`)) || !bytes.Contains(updated, []byte(`"name":"after.txt"`)) || !bytes.Equal(raw, original) {
		t.Fatal("edit rounded domain data or changed its source", string(updated))
	}
}
