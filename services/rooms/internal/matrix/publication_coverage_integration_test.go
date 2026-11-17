//go:build integration

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"os"
	"strings"
	"testing"
)

func TestPublicationCoverageRealSynapseFreshProcessWitness(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ORIGIN_FIXTURE") != "1" {
		t.Skip("disposable remote Synapse required")
	}
	c := New(Config{InternalURL: "http://127.0.0.1:18978", HomeserverURL: "https://storage.test:18978", ServerName: "storage.test:18978", SSOProviderID: "artificial", AdminToken: "not-an-admin-token", PublicationSecret: strings.Repeat("0", 64)})
	first, err := c.RequirePublicationCoverage(t.Context())
	if err != nil {
		t.Fatal("real module witness rejected", err)
	}
	second, err := c.RequirePublicationCoverage(t.Context())
	if err != nil || !first.SameProcess(second) {
		t.Fatal("fresh nonces changed process witness", err)
	}
	c.cfg.PublicationSecret = strings.Repeat("1", 64)
	if _, err := c.RequirePublicationCoverage(t.Context()); err == nil {
		t.Fatal("wrong shared authority accepted")
	}
}
