// SPDX-License-Identifier: Apache-2.0
package jmap

import "testing"

func TestPrivateMailAliasRoutingExceptionCannotAddressContentOrAnArbitraryPath(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	if !legacyMailControlPath("/api/v1/aliases/" + id) {
		t.Fatal("public alias deletion remained unavailable after private capture")
	}
	for _, path := range []string{"/api/v1/aliases/", "/api/v1/aliases/../messages", "/api/v1/aliases/" + id + "/raw", "/api/v1/aliases/%2e%2e", "/api/v1/aliases/" + id + "?account=other", "/api/v1/messages/" + id + "/raw"} {
		if legacyMailControlPath(path) {
			t.Fatal("alias routing exception reopened private content", path)
		}
	}
}
