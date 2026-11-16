// SPDX-License-Identifier: Apache-2.0
package api

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateMailLimitFixture struct {
	privateMailFixture
	limit int64
}

func (p *privateMailLimitFixture) MessageByteLimit() int64 { return p.limit }
func TestPrivateMailMIMEBudgetUsesCapturedCapabilityWithoutHTTPFallback(t *testing.T) {
	raw := NewMail("https://closed.invalid", nil, nil)
	if raw.MessageByteLimit() != eml.DefaultMessageByteLimit {
		t.Fatal("legacy MIME budget changed")
	}
	fixture := privateMailFixture{scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}}
	unavailable, err := raw.WithPrivateData(&fixture)
	if err != nil || unavailable.MessageByteLimit() != 0 {
		t.Fatal("unproven private MIME budget became a legacy fallback", err)
	}
	captured, err := raw.WithPrivateData(&privateMailLimitFixture{privateMailFixture: fixture, limit: 96 << 20})
	if err != nil || captured.MessageByteLimit() != 96<<20 || fixture.calls != 0 {
		t.Fatal("authenticated larger capability was silently reduced or fetched overHTTP", err)
	}
}
