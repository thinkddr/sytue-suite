// SPDX-License-Identifier: Apache-2.0
package search

import (
	"testing"
	"time"
)

func TestPrivateSearchMatcherPreservesBackendOperators(t *testing.T) {
	at := time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC)
	docs := []Document{
		{Body: "el viernes hay reunión de equipo", AuthorSub: "alice", AuthorName: "Ána", ChannelSlug: "general", ChannelName: "General", CreatedAt: at},
		{Body: "informe de cierre mensual listo", AuthorSub: "alice", AuthorName: "Ána", ChannelSlug: "general", ChannelName: "General", CreatedAt: at, Saved: true},
		{Body: "el cierre del informe va con retraso", AuthorSub: "bob", AuthorName: "Bób", ChannelSlug: "random", ChannelName: "Aleatorio", CreatedAt: at},
		{Body: "la subida del 50% ya se aprobó", AuthorSub: "alice", AuthorName: "Ána", ChannelSlug: "general", CreatedAt: at},
		{Body: "plan definitivo https://sytue.eu", AuthorSub: "alice", AuthorName: "Ána", ChannelSlug: "general", CreatedAt: at, Pinned: true, HasFile: true},
		{Body: "plan borrador, hablamos de http sin enlace", AuthorSub: "bob", AuthorName: "Bób", ChannelSlug: "random", CreatedAt: at},
	}
	cases := []struct {
		q    string
		want []int
	}{
		{"REUNION viernes", []int{0}},
		{`"informe de cierre"`, []int{1}},
		{"informe cierre", []int{1, 2}},
		{"50%", []int{3}},
		{"%", []int{3}},
		{"_", nil},
		{"plan -borrador", []int{4}},
		{`plan -"hablamos de http"`, []int{4}},
		{"from:ana in:#general", []int{0, 1, 3, 4}},
		{"from:bob", []int{2, 5}},
		{"after:2024-03-05 before:2024-03-06", []int{0, 1, 2, 3, 4, 5}},
		{"on:2024-03-06", nil},
		{"has:link", []int{4}},
		{"has:file", []int{4}},
		{"is:saved", []int{1}},
		{"is:pinned", []int{4}},
	}
	for _, tc := range cases {
		t.Run(tc.q, func(t *testing.T) {
			q := Parse(tc.q)
			want := map[int]bool{}
			for _, i := range tc.want {
				want[i] = true
			}
			for i, d := range docs {
				if got := q.Matches(d); got != want[i] {
					t.Fatalf("document %d match=%v want=%v", i, got, want[i])
				}
			}
		})
	}
	// Metadata prefilter must not inspect a body or guess encrypted attachments.
	if !Parse("needle has:file has:link").MetadataMatches(docs[0]) {
		t.Fatal("metadata prefilter inspected encrypted content")
	}
	if Parse("from:carol").MetadataMatches(docs[0]) {
		t.Fatal("metadata prefilter ignored author")
	}
}

func TestOpaqueMetadataCursorBindsActorScopeAndExactTies(t *testing.T) {
	row := Metadata{ID: "b8239b68-21e9-45b3-96c3-2e47663e0033", Tenant: "host", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)}
	raw := EncodeCursor(row, "own", "alice")
	cursor, err := DecodeCursor(raw, "own", "alice")
	if err != nil || cursor.ID != row.ID || cursor.Tenant != row.Tenant || !cursor.CreatedAt.Equal(row.CreatedAt) {
		t.Fatal("cursor changed the exact keyset position", err)
	}
	for _, pair := range [][2]string{{"other", "alice"}, {"own", "bob"}} {
		if _, err := DecodeCursor(raw, pair[0], pair[1]); err == nil {
			t.Fatal("foreign actor or organization reused cursor")
		}
	}
	if _, err := DecodeCursor("not-json", "own", "alice"); err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestQueryConcurrentUnicode(t *testing.T) {
	for range 12 {
		t.Run("independent-account", func(t *testing.T) {
			t.Parallel()
			q := Parse("arbol café")
			for range 100 {
				if !q.Matches(Document{Body: "ÁRBOL y Café, piñata, Straße, Łódź"}) {
					t.Fatal("concurrent folding changed the result")
				}
			}
		})
	}
}
