// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"testing"
)

func TestPrivateMailLostMetadataACKAfterNewerEditAndDeviceRecoveryDoesNotRepublish(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "original accepted"); err == nil {
		t.Fatal("accepted publication ACK was not lost")
	}
	if err := f.repo.SetSignature(t.Context(), "newer remote edit"); err != nil {
		t.Fatal(err)
	}
	before := f.crypto.puts
	f.crypto.binding.Target.DeviceID = "FRESH-RECOVERED"
	if err := f.repo.SetSignature(t.Context(), "original accepted"); err != nil {
		t.Fatal("new device could not clean up old accepted metadata", err)
	}
	if f.crypto.puts != before {
		t.Fatal("old accepted metadata was republished")
	}
	got, err := f.repo.Signature(t.Context())
	if err != nil || got != "newer remote edit" {
		t.Fatal("ACK cleanup overwrote a later edit", err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	pending, err := f.repo.cfg.Journal.List(owner, writesNamespace)
	if err != nil || len(pending) != 0 {
		t.Fatal("accepted source journal was stranded", len(pending), err)
	}
}
