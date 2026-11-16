// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func listFixture(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "eu-test-1", AccessKey: "synthetic", SecretKey: "synthetic", Bucket: "retained-photos"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func listXML(keys []string, truncated bool, token string) string {
	var body strings.Builder
	fmt.Fprintf(&body, `<ListBucketResult><Name>retained-photos</Name><IsTruncated>%t</IsTruncated><NextContinuationToken>%s</NextContinuationToken>`, truncated, token)
	for i, key := range keys {
		body.WriteString("<Contents><Key>")
		_ = xml.EscapeText(&body, []byte(key))
		fmt.Fprintf(&body, "</Key><Size>%d</Size></Contents>", i+1)
	}
	body.WriteString("</ListBucketResult>")
	return body.String()
}

func TestListPreservesHistoricalPathsAndBindsCursorToTenantAndPrefix(t *testing.T) {
	var calls atomic.Int64
	c := listFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if q.Get("list-type") != "2" || q.Get("prefix") != "tenants/acme/a/photo/" || q.Get("max-keys") != "3" || q.Get("delimiter") != "" {
			t.Error("unscoped or unbounded provider request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		keys := []string{"tenants/acme/a/photo/old/original.png", "tenants/acme/a/photo/old/thumb.jpg", "tenants/acme/a/photo/current/thumb.png"}
		// The provider returns byte-sorted names, irrespective of current recipes.
		keys = []string{keys[2], keys[0], keys[1]}
		if after := q.Get("start-after"); after != "" {
			if after != keys[1] {
				t.Error("cursor failed to retain last accepted key")
			}
			keys = keys[2:]
		}
		_, _ = w.Write([]byte(listXML(keys, false, "")))
	})
	tenant := tenancy.Tenant{Slug: "acme"}
	first, err := c.List(t.Context(), tenant, "a/photo", "", 2)
	if err != nil || len(first.Objects) != 2 || first.Next == "" || first.Objects[0].Key != "a/photo/current/thumb.png" {
		t.Fatal(first, err)
	}
	for _, change := range []struct{ tenant, prefix, cursor string }{{"other", "a/photo", first.Next}, {"acme", "a/other", first.Next}, {"acme", "a/photo", "invalid"}} {
		before := calls.Load()
		_, err := c.List(t.Context(), tenancy.Tenant{Slug: change.tenant}, change.prefix, change.cursor, 2)
		if !errors.Is(err, ErrList) || calls.Load() != before {
			t.Fatal("changed cursor reached provider", err)
		}
	}
	second, err := c.List(t.Context(), tenant, "a/photo/", first.Next, 2)
	if err != nil || len(second.Objects) != 1 || second.Next != "" || second.Objects[0].Key != "a/photo/old/thumb.jpg" {
		t.Fatal(second, err)
	}
}

func TestListFailsClosedForMalformedTruncatedOrEscapedProviderPages(t *testing.T) {
	for _, value := range []struct {
		name      string
		keys      []string
		truncated bool
		token     string
		status    int
	}{
		{"escape", []string{"tenants/other/a/photo/private"}, false, "", 200},
		{"prefix sibling", []string{"tenants/acme/a/photograph/private"}, false, "", 200},
		{"unordered", []string{"tenants/acme/a/photo/z", "tenants/acme/a/photo/a"}, false, "", 200},
		{"duplicate", []string{"tenants/acme/a/photo/a", "tenants/acme/a/photo/a"}, false, "", 200},
		{"truncated without token", nil, true, "", 200},
		{"provider error", nil, false, "", 403},
	} {
		t.Run(value.name, func(t *testing.T) {
			c := listFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(value.status)
				_, _ = w.Write([]byte(listXML(value.keys, value.truncated, value.token)))
			})
			page, err := c.List(t.Context(), tenancy.Tenant{Slug: "acme"}, "a/photo", "", 10)
			if err == nil || len(page.Objects) != 0 || page.Next != "" {
				t.Fatal("incomplete listing became successful inventory", page, err)
			}
		})
	}
}

func TestListRejectsUnsafePrefixesAndCancellationBeforeProvider(t *testing.T) {
	c := listFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid list reached provider") })
	for _, prefix := range []string{"", "/a", "../a", "a/../b", "shared/a", "a\\b", "a\x00b"} {
		if _, err := c.List(t.Context(), tenancy.Tenant{Slug: "acme"}, prefix, "", 1); !errors.Is(err, ErrList) {
			t.Fatal(prefix, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.List(ctx, tenancy.Tenant{Slug: "acme"}, "a/photo", "", 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
