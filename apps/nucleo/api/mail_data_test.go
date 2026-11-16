// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateMailFixture struct {
	MailData
	scope storage.Scope
	calls int
}

func (p *privateMailFixture) PrivateStorageScope() storage.Scope { return p.scope }
func (p *privateMailFixture) Aliases(context.Context) (string, []MailAlias, error) {
	p.calls++
	return "owner@example.test", []MailAlias{{ID: "writing", Address: "writing@example.test"}}, nil
}

func (p *privateMailFixture) Mailboxes(context.Context) ([]Mailbox, error) {
	p.calls++
	return []Mailbox{{ID: "private", Name: "Private mailbox"}}, nil
}

func (p *privateMailFixture) Raw(context.Context, string) (io.ReadCloser, error) {
	p.calls++
	return io.NopCloser(strings.NewReader("private MIME")), nil
}

func (p *privateMailFixture) Filters(context.Context) ([]MailFilter, error) {
	p.calls++
	return []MailFilter{{ID: "private-rule", Name: "Private rule"}}, nil
}

func (p *privateMailFixture) CreateFilter(_ context.Context, f MailFilter) (MailFilter, error) {
	p.calls++
	return f, nil
}

func (p *privateMailFixture) UpdateFilter(context.Context, MailFilter) error { p.calls++; return nil }

func (p *privateMailFixture) DeleteFilter(context.Context, string) error { p.calls++; return nil }

func (p *privateMailFixture) Vacation(context.Context) (MailVacation, error) {
	p.calls++
	return MailVacation{Body: "Private vacation"}, nil
}

func (p *privateMailFixture) SetVacation(context.Context, MailVacation) error { p.calls++; return nil }

func TestPrivateMailFacadeRoutesAllRuleAndVacationOperationsWithoutHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	data := &privateMailFixture{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}}
	private, err := NewMail(server.URL, nil, server.Client()).WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := private.Filters(t.Context()); err != nil || len(rows) != 1 || rows[0].ID != "private-rule" {
		t.Fatal(rows, err)
	}
	if row, err := private.CreateFilter(t.Context(), MailFilter{Name: "Rule"}); err != nil || row.Name != "Rule" {
		t.Fatal(row, err)
	}
	if err := private.UpdateFilter(t.Context(), MailFilter{}); err != nil {
		t.Fatal(err)
	}
	if err := private.DeleteFilter(t.Context(), "private-rule"); err != nil {
		t.Fatal(err)
	}
	if value, err := private.Vacation(t.Context()); err != nil || value.Body != "Private vacation" {
		t.Fatal(value, err)
	}
	if err := private.SetVacation(t.Context(), MailVacation{}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || data.calls != 6 {
		t.Fatal("private settings escaped to HTTP", calls.Load(), data.calls)
	}
}

func TestPrivateMailSettingsConcurrencyTokensNeverEnterHTTPJSON(t *testing.T) {
	for _, value := range []any{MailFilter{ID: "rule", PrivateRevision: "native-only-version"}, MailVacation{PrivateRevision: "native-only-version", Body: "Preserved vacation", UpdatedAt: time.Date(2026, 10, 30, 0, 0, 0, 123456789, time.UTC)}} {
		raw, err := json.Marshal(value)
		if err != nil || strings.Contains(string(raw), "native-only-version") || strings.Contains(string(raw), "PrivateRevision") {
			t.Fatal("native revision entered the service DTO", string(raw), err)
		}
	}
}

func TestPrivateMailFacadeRoutesContentAndClosesEveryLegacyHTTPFallback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	original := NewMail(server.URL, nil, server.Client())
	data := &privateMailFixture{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}}
	private, err := original.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := private.Mailboxes(t.Context())
	if err != nil || len(boxes) != 1 || boxes[0].ID != "private" {
		t.Fatal("private rows did not reach existing Mail API", err)
	}
	raw, err := private.Raw(t.Context(), "message")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(raw)
	_ = raw.Close()
	if string(body) != "private MIME" {
		t.Fatal("private MIME was not adopted")
	}
	if primary, aliases, err := private.Aliases(t.Context()); err != nil || primary != "owner@example.test" || len(aliases) != 1 || aliases[0].Address != "writing@example.test" {
		t.Fatal("private sender inventory escaped to HTTP", primary, aliases, err)
	}
	if primary, err := private.Address(t.Context()); err != nil || primary != "owner@example.test" {
		t.Fatal("private primary address was inferred from the legacy client", primary, err)
	}
	for name, operation := range map[string]func() error{
		"send": func() error {
			_, err := private.Send(t.Context(), "alice@example.test", []string{"bob@example.test"}, []byte("private body"), 0)
			return err
		},
		"schedule": func() error {
			_, err := private.Schedule(t.Context(), "alice@example.test", []string{"bob@example.test"}, []byte("private body"), time.Now().Add(time.Hour))
			return err
		},
		"shared-body": func() error { _, _, err := private.SharedMessages(t.Context(), "account", "box", "", 50); return err },
		"old-queue":   func() error { _, err := private.Scheduled(t.Context()); return err },
		"account":     func() error { _, err := private.Quota(t.Context()); return err },
		"sse":         func() error { return private.Llegadas(t.Context(), func() { t.Error("legacy event stream ran") }) },
	} {
		if err := operation(); !errors.Is(err, ErrPrivateMailOperation) {
			t.Fatal("legacy method escaped private boundary", name, err)
		}
	}
	if calls.Load() != 0 || data.calls != 4 {
		t.Fatal("private facade contacted original server", calls.Load(), data.calls)
	}
	if _, err := private.WithPrivateData(data); !errors.Is(err, ErrPrivateMailOperation) {
		t.Fatal("nested facade accepted", err)
	}
	if _, err := original.WithPrivateData(nil); !errors.Is(err, ErrPrivateMailOperation) {
		t.Fatal("nil private source accepted")
	}
	data.scope.Product = "drive"
	if _, err := original.WithPrivateData(data); !errors.Is(err, ErrPrivateMailOperation) {
		t.Fatal("another app's private repository accepted")
	}
}
