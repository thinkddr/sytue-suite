// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func TestResourceFrameUsesSharedHybridEnvelopeAndRejectsPlaintextExtensions(t *testing.T) {
	identity, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := pqc.Seal(identity.Recipient(), []byte("resource-context"), []byte("private original payload"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := resource.EncodeFrame(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private original payload")) {
		t.Fatal("frame leaked payload")
	}
	opened, err := resource.DecodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	cleartext, err := pqc.Open(identity, []byte("resource-context"), opened)
	if err != nil || string(cleartext) != "private original payload" {
		t.Fatal("shared frame mismatch", err)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"v":1`, `"v":1,"body":"clear"`, 1), strings.Replace(string(raw), `"v":1`, `"v":1,"v":1`, 1), strings.Replace(string(raw), resource.FrameEventType, "m.room.message", 1), string(raw) + " {}"} {
		if _, err := resource.DecodeFrame([]byte(bad)); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
}
