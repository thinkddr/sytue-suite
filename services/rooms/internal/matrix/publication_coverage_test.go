// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPublicationCoverageRequiresFreshAuthenticatedExactOrigin(t *testing.T) {
	for _, fault := range []string{"valid", "missing-secret", "upper-secret", "unavailable", "redirect", "nonce", "server", "coverage", "storage-only-v2", "instance", "mac", "boolean-version", "extra", "trailing", "oversize", "cancelled", "cached"} {
		t.Run(fault, func(t *testing.T) {
			secret := strings.Repeat("a", 64)
			calls := 0
			var cached []byte
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != publicationCoveragePath || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("Cache-Control") != "no-store" {
					t.Error("coverage changed fixed route or credential")
				}
				var challenge struct {
					Version int    `json:"v"`
					Nonce   string `json:"nonce"`
				}
				if json.NewDecoder(r.Body).Decode(&challenge) != nil || challenge.Version != 1 || !canonicalCoverageHex(challenge.Nonce) {
					t.Fatal("invalid random challenge")
				}
				if fault == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if fault == "redirect" {
					w.Header().Set("Location", "/wrong")
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				if fault == "oversize" {
					_, _ = w.Write([]byte(strings.Repeat("x", 4097)))
					return
				}
				if fault == "cached" && cached != nil {
					_, _ = w.Write(cached)
					return
				}
				value := map[string]any{"v": 1, "server": "matrix.sytue.eu", "coverage": publicationCoverageKind, "instance": strings.Repeat("b", 64), "nonce": challenge.Nonce}
				switch fault {
				case "nonce":
					value["nonce"] = strings.Repeat("c", 64)
				case "server":
					value["server"] = "other.example"
				case "coverage":
					value["coverage"] = "legacy"
				case "storage-only-v2":
					value["coverage"] = "storage-private-v2"
				case "instance":
					value["instance"] = ""
				case "boolean-version":
					value["v"] = true
				case "extra":
					value["extra"] = "ignored"
				}
				// Even a correctly MACed witness cannot change semantic scope.
				transcript := "sytue-publication-coverage-v1\n" + value["server"].(string) + "\n" + value["coverage"].(string) + "\n" + value["instance"].(string) + "\n" + value["nonce"].(string)
				mac := hmac.New(sha256.New, []byte(secret))
				_, _ = mac.Write([]byte(transcript))
				value["mac"] = hex.EncodeToString(mac.Sum(nil))
				if fault == "mac" {
					value["mac"] = strings.Repeat("0", 64)
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				cached = raw
				if fault == "trailing" {
					raw = append(raw, []byte("{}")...)
				}
				_, _ = w.Write(raw)
			})
			c.cfg.PublicationSecret = secret
			if fault == "missing-secret" {
				c.cfg.PublicationSecret = ""
			}
			if fault == "upper-secret" {
				c.cfg.PublicationSecret = strings.ToUpper(secret)
			}
			ctx := t.Context()
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			witness, err := c.RequirePublicationCoverage(ctx)
			if fault == "valid" || fault == "cached" {
				if err != nil || !witness.SameProcess(witness) {
					t.Fatal("fresh coverage rejected", err)
				}
				second, err := c.RequirePublicationCoverage(ctx)
				if fault == "valid" && (err != nil || !witness.SameProcess(second)) {
					t.Fatal("same process changed", err)
				}
				if fault == "cached" && err == nil {
					t.Fatal("replayed challenge authorized origin")
				}
			} else if err == nil {
				t.Fatal("invalid coverage authorized origin", fault)
			}
			if (fault == "missing-secret" || fault == "upper-secret" || fault == "cancelled") && calls != 0 {
				t.Fatal("invalid authority reached origin")
			}
		})
	}
	if (PublicationCoverage{}).SameProcess(PublicationCoverage{}) {
		t.Fatal("empty process witness accepted")
	}
	a := PublicationCoverage{server: "matrix.sytue.eu", instance: "a"}
	if a.SameProcess(PublicationCoverage{server: a.server, instance: "b"}) {
		t.Fatal("restarted origin reused drain witness")
	}
	var absent *Client
	if _, err := absent.RequirePublicationCoverage(t.Context()); err == nil {
		t.Fatal("absent module accepted")
	}
}
