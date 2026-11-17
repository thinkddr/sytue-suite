// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestCanonicalWireMarkerPreservesSignedCipherAndRetry(t *testing.T) {
	raw := []byte("{\"session_id\":\"session\",\"ciphertext\":\"opaque\",\"algorithm\":\"m.megolm.v1.aes-sha2\",\"device_id\":\"DEVICE\"}")
	marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}
	marked, err := publication.AttachMarker(raw, marker)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := publication.WireDigest(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	after, actual, err := publication.WireDigest(marked, true)
	if err != nil || before != after || actual != marker {
		t.Fatal("admission marker changed signed cipher", err)
	}
	normalized, parsed, err := publication.CanonicalWire(marked, true)
	if err != nil || parsed != marker {
		t.Fatal(err)
	}
	original, _, err := publication.CanonicalWire(raw, false)
	if err != nil || !bytes.Equal(normalized, original) {
		t.Fatal("marker altered immutable journal core", err)
	}
	again, err := publication.AttachMarker(marked, marker)
	if err != nil || !bytes.Equal(again, marked) {
		t.Fatal("same durable attempt is not idempotent", err)
	}
	replacement := marker
	replacement.ID = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	if _, err := publication.AttachMarker(marked, replacement); err == nil {
		t.Fatal("retry silently replaced original admission")
	}
	if _, _, err := publication.CanonicalWire(raw, true); err == nil {
		t.Fatal("required admission omitted")
	}
	duplicate := []byte("{\"algorithm\":\"m.megolm.v1.aes-sha2\",\"algorithm\":\"m.megolm.v1.aes-sha2\",\"ciphertext\":\"opaque\",\"session_id\":\"session\"}")
	if _, _, err := publication.CanonicalWire(duplicate, false); err == nil {
		t.Fatal("duplicate signed field accepted")
	}
}
