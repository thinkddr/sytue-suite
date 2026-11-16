// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"strings"
	"testing"
)

func TestMetadataProofBindsFreshRequestEvidenceAndPurpose(t *testing.T) {
	secret := strings.Repeat("a", 64)
	nonce, err := FreshMetadataNonce()
	if err != nil || !ValidMetadataNonce(nonce) {
		t.Fatal("fresh nonce unavailable", err)
	}
	another, err := FreshMetadataNonce()
	if err != nil || another == nonce {
		t.Fatal("nonce reused", err)
	}
	request := map[string]any{"nonce": nonce, "scope": "acme/docs/owner", "device": "SDK_A"}
	response := map[string]any{"revision": "9007199254740993", "active": true}
	mac, err := MetadataMAC(secret, "session-v1", request, response)
	if err != nil || VerifyMetadataMAC(secret, "session-v1", request, response, mac) != nil {
		t.Fatal("original metadata proof rejected", err)
	}
	for _, change := range []string{"nonce", "scope", "device", "revision", "active", "purpose", "secret", "mac"} {
		t.Run(change, func(t *testing.T) {
			q := map[string]any{"nonce": nonce, "scope": "acme/docs/owner", "device": "SDK_A"}
			r := map[string]any{"revision": "9007199254740993", "active": true}
			key, domain, tag := secret, "session-v1", mac
			switch change {
			case "nonce":
				q["nonce"] = another
			case "scope":
				q["scope"] = "acme/mail/owner"
			case "device":
				q["device"] = "SDK_B"
			case "revision":
				r["revision"] = "9007199254740993x"
			case "active":
				r["active"] = false
			case "purpose":
				domain = "source-v1"
			case "secret":
				key = strings.Repeat("b", 64)
			case "mac":
				tag = strings.Repeat("0", 64)
			}
			if VerifyMetadataMAC(key, domain, q, r, tag) == nil {
				t.Fatal("changed metadata authenticated", change)
			}
		})
	}
	if _, err := MetadataMAC(secret, "session-v1", request, strings.Repeat("x", 256<<10)); err == nil {
		t.Fatal("unbounded metadata authenticated")
	}
	for _, bad := range []string{"", strings.Repeat("A", 64), strings.Repeat("0", 63), strings.Repeat("g", 64)} {
		if ValidMetadataSecret(bad) || ValidMetadataNonce(bad) {
			t.Fatal("noncanonical proof input accepted")
		}
	}
}
