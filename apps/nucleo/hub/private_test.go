// SPDX-License-Identifier: Apache-2.0
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type testCatalog struct {
	entries  []records.Entry
	values   map[string][]byte
	returned [][]byte
	err      error
}

func (c *testCatalog) Entries() ([]records.Entry, error) { return c.entries, c.err }
func (c *testCatalog) ReadJSON(ctx context.Context, entry records.Entry) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value := bytes.Clone(c.values[entry.ID])
	c.returned = append(c.returned, value)
	return value, nil
}

func noticeFixture() Notice {
	return Notice{Version: 1, Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "hub"}, ID: "notice1", Source: "mail", Type: "mail.received", Level: "info", Title: "Private subject", Params: map[string]string{"from": "Alice"}, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func fixtureCatalog(t *testing.T, n Notice) *testCatalog {
	t.Helper()
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return &testCatalog{entries: []records.Entry{{Kind: noticeKind, ID: n.ID, State: "live"}}, values: map[string][]byte{n.ID: raw}}
}

func TestPrivateCatalogValidatesScopeAndDoesNotReturnPartialData(t *testing.T) {
	n := noticeFixture()
	c := fixtureCatalog(t, n)
	got, err := readCatalog(t.Context(), n.Scope, c)
	if err != nil || len(got) != 1 || got[0].Title != n.Title {
		t.Fatal(got, err)
	}
	for _, b := range c.returned[0] {
		if b != 0 {
			t.Fatal("plaintext buffer retained")
		}
	}
	for _, change := range []func(*testCatalog){
		func(c *testCatalog) { c.entries[0].ID = "another"; c.values["another"] = c.values[n.ID] },
		func(c *testCatalog) { c.entries = append(c.entries, c.entries[0]) },
		func(c *testCatalog) { c.entries[0].State = "trash" },
		func(c *testCatalog) { c.entries[0].Binary = true },
		func(c *testCatalog) { c.err = records.ErrConflict },
		func(c *testCatalog) {
			c.values[n.ID] = bytes.Replace(c.values[n.ID], []byte(`"alice"`), []byte(`"bob"`), 1)
		},
		func(c *testCatalog) {
			c.values[n.ID] = bytes.Replace(c.values[n.ID], []byte(`"v":1`), []byte(`"v":1,"deleted_at":"2026-01-01T00:00:00Z"`), 1)
		},
		func(c *testCatalog) {
			c.values[n.ID] = bytes.Replace(c.values[n.ID], []byte(`"alice"`), []byte(`"alice","subject":"bob"`), 1)
		},
	} {
		c := fixtureCatalog(t, n)
		change(c)
		got, err := readCatalog(t.Context(), n.Scope, c)
		if err == nil || got != nil {
			t.Fatal("accepted partial, conflicting or foreign catalog", got, err)
		}
	}
	if repo, err := New(Config{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}}); repo != nil || !errors.Is(err, ErrNotice) {
		t.Fatal("accepted another product", err)
	}
}

func TestPrivateNoticeRejectsUnsafeOrUnboundInputs(t *testing.T) {
	n := noticeFixture()
	for _, mutate := range []func(*Notice){
		func(n *Notice) { n.Scope.Subject = "bob" },
		func(n *Notice) { n.URL = "javascript:alert(1)" },
		func(n *Notice) { n.URL = "https://user:password@example.test" },
		func(n *Notice) { n.CreatedAt = time.Time{} },
		func(n *Notice) { n.Title = "" },
	} {
		bad := n
		mutate(&bad)
		if bad.validate(n.Scope, n.ID) == nil {
			t.Fatal("accepted invalid notice", bad)
		}
	}
}
