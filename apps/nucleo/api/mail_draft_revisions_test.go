// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type mailRevisionProbe struct {
	*privateMailFixture
	prepared, committed int
}

func (p *mailRevisionProbe) PrepareDraftRevision(_ context.Context, input MailDraftRevision) (MailDraftAttempt, error) {
	p.prepared++
	return MailDraftAttempt{Scope: p.scope, Editor: input.Editor, Revision: input.Revision, ID: "draft"}, nil
}

func (p *mailRevisionProbe) CommitDraftRevision(_ context.Context, attempt MailDraftAttempt) (MailMessage, error) {
	p.committed++
	return MailMessage{ID: attempt.ID}, nil
}

func (p *mailRevisionProbe) PendingDraftRevisions(context.Context) ([]MailDraftAttempt, error) {
	return nil, nil
}

func TestMailDraftRevisionPortHasNoLegacyOrUnsupportedPrivateHTTPFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	original := NewMail(server.URL, nil, server.Client())
	if port, private := original.PrivateDraftRevisions(); port != nil || private {
		t.Fatal("legacy HTTP client became a private revision owner")
	}
	unsupported, err := original.WithPrivateData(&privateMailFixture{scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}})
	if err != nil {
		t.Fatal(err)
	}
	if port, private := unsupported.PrivateDraftRevisions(); port != nil || !private {
		t.Fatal("unsupported private revision port did not remain closed")
	}
	fixture := &mailRevisionProbe{privateMailFixture: &privateMailFixture{scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}}}
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	port, active := private.PrivateDraftRevisions()
	if port == nil || !active {
		t.Fatal("private revision port was not adopted")
	}
	attempt, err := port.PrepareDraftRevision(t.Context(), MailDraftRevision{Editor: "editor", Revision: "revision", MIME: []byte("PRIVATE-REVISION")})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := port.CommitDraftRevision(t.Context(), attempt); err != nil || result.ID != "draft" {
		t.Fatal(result, err)
	}
	if requests.Load() != 0 || fixture.prepared != 1 || fixture.committed != 1 {
		t.Fatal("revision work entered legacy HTTP", requests.Load(), fixture.prepared, fixture.committed)
	}
}
