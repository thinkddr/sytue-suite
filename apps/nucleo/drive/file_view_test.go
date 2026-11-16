// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"
)

func TestPrivateFileViewRetainsSQLSensitivitySnapshotAndRejectsContradictions(t *testing.T) {
	for _, raw := range []string{
		`{"id":"file","name":"file.txt","label_id":"restricted","label_name":"Restricted","label_block_external_share":true}`,
		`{"id":"file","name":"file.txt","label":{"name":"Restricted","block_external_share":true}}`,
	} {
		file, err := fileView([]byte(raw), "file")
		if err != nil || file.Name != "file.txt" || file.Label == nil || file.Label.Name != "Restricted" || !file.Label.BlockExternalShare {
			t.Fatal("private file lost sensitivity restriction", file, err)
		}
	}
	for _, raw := range []string{
		`{"id":"file","label_id":"restricted","label_name":"Restricted","label_block_external_share":true,"label":{"name":"Public","block_external_share":false}}`,
		`{"id":"file","label_block_external_share":true}`,
		`{"id":"other"}`,
	} {
		if _, err := fileView([]byte(raw), "file"); err == nil {
			t.Fatal("inconsistent private label snapshot accepted", raw)
		}
	}
}
