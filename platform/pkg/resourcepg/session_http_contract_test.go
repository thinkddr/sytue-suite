// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestSessionOriginContractRetainsSDKIdentifiersAndFixedService(t *testing.T) {
	q := SessionOriginRequest{Version: 1, Scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "docs"}, DeviceID: strings.Repeat("x", 200) + ":device", Nonce: strings.Repeat("b", 64)}
	if !q.ValidForService("docs") {
		t.Fatal("valid maintained SDK identifier was narrowed")
	}
	for _, service := range []string{"mail", "https://attacker.invalid", ""} {
		if q.ValidForService(service) {
			t.Fatal("request selected another service")
		}
	}
	for _, device := range []string{"", strings.Repeat("x", 256), "bad\nvalue", "bad value"} {
		q.DeviceID = device
		if q.ValidForService("docs") {
			t.Fatal("invalid SDK identifier accepted")
		}
	}
	for _, secret := range []string{"", strings.Repeat("A", 64), "short", strings.Repeat("g", 64)} {
		if ValidSessionServiceSecret(secret) {
			t.Fatal("malformed credential accepted")
		}
	}
	if !ValidSessionServiceSecret(strings.Repeat("a", 64)) {
		t.Fatal("canonical credential rejected")
	}
}
