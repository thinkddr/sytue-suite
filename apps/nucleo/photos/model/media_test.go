// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMediaInventoryRequiresEveryOriginalAndExactDescriptor(t *testing.T) {
	for _, scenario := range []string{"complete", "catalog only", "missing original", "wrong hash", "wrong size", "extra binary"} {
		t.Run(scenario, func(t *testing.T) {
			rows := retainedRows(t)
			rows[0].JSON = bytes.Replace(rows[0].JSON, []byte(`9007199254740993,`), []byte(`8,`), 1)
			sum := sha256.Sum256([]byte("original"))
			media := MediaRow{ID: photoID, Tenant: "acme", Owner: "alice", AssetID: photoID, Role: "original", Size: 8, SHA256: hex.EncodeToString(sum[:]), ContentType: "image/heic"}
			raw, err := json.Marshal(media)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "catalog only" {
				rows = append(rows, Row{Kind: "media", ID: media.ID, JSON: raw})
			}
			blobs := make([]storage.StreamSourceRow, 1, 2)
			blobs[0] = media.Descriptor()
			switch scenario {
			case "catalog only", "missing original":
				blobs = nil
			case "wrong hash":
				blobs[0].SHA256 = hex.EncodeToString(make([]byte, 32))
			case "wrong size":
				blobs[0].Size++
			case "extra binary":
				extra := blobs[0]
				extra.ID = "unrelated"
				blobs = append(blobs, extra)
			}
			snapshot, err := NewSnapshot(photoScope(), rows)
			if err != nil {
				t.Fatal(err)
			}
			err = snapshot.ValidateMedia(blobs)
			if scenario == "complete" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrSnapshot) {
				t.Fatal("incomplete media inventory accepted", err)
			}
		})
	}
}
