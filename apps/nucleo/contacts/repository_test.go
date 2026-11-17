// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type contactsCatalog struct {
	entries  []records.Entry
	raw      map[string][]byte
	returned [][]byte
	failAt   int
	failure  error
}

func (c *contactsCatalog) Entries() ([]records.Entry, error) {
	return append([]records.Entry(nil), c.entries...), nil
}

func (c *contactsCatalog) ReadJSON(ctx context.Context, entry records.Entry) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.failure != nil && len(c.returned) == c.failAt {
		return nil, c.failure
	}
	raw := bytes.Clone(c.raw[rowKey(entry.Kind, entry.ID)])
	c.returned = append(c.returned, raw)
	return raw, nil
}

func catalogRows(rows []Row) *contactsCatalog {
	catalog := &contactsCatalog{raw: map[string][]byte{}}
	for _, row := range rows {
		catalog.entries = append(catalog.entries, records.Entry{Kind: row.Kind, ID: row.ID, State: "live"})
		catalog.raw[rowKey(row.Kind, row.ID)] = bytes.Clone(row.JSON)
	}
	return catalog
}

func TestPrivateCatalogValidatesWholeGraphAndOwnsVerifiedBuffers(t *testing.T) {
	catalog := catalogRows(activeRows(t))
	snapshot, err := readCatalog(t.Context(), testScope(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Rows()) != 9 {
		t.Fatal("lost source rows")
	}
	people, err := snapshot.Contacts()
	if err != nil || len(people) != 1 || people[0].ID != contactID {
		t.Fatal("unusable projection", err)
	}
	for _, raw := range catalog.returned {
		for _, value := range raw {
			if value != 0 {
				t.Fatal("verified temporary bytes were retained")
			}
		}
	}
	// A complete encrypted aggregate overrides the source without changing its
	// immutable baseline inventory or publishing half of an updated contact.
	rows := activeRows(t)
	replaceField(t, rows, 2, "note", "private edit")
	change := contactAggregate(rows)
	raw, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	catalog = catalogRows(activeRows(t))
	catalog.entries = append(catalog.entries, records.Entry{Kind: change.Kind, ID: change.ID, State: "live"})
	catalog.raw[rowKey(change.Kind, change.ID)] = raw
	snapshot, err = readCatalog(t.Context(), testScope(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	people, err = snapshot.Contacts()
	if err != nil || people[0].Note != "private edit" {
		t.Fatal("aggregate not applied", err)
	}
}

func TestPrivateCatalogNeverReturnsPartialOrForeignData(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, *contactsCatalog)
	}{
		{"missing-book", func(_ *testing.T, c *contactsCatalog) { c.entries = c.entries[1:] }},
		{"duplicate-entry", func(_ *testing.T, c *contactsCatalog) { c.entries = append(c.entries, c.entries[0]) }},
		{"wrong-entry-id", func(_ *testing.T, c *contactsCatalog) {
			c.entries[0].ID = cardID
			c.raw[rowKey("book", cardID)] = c.raw[rowKey("book", bookID)]
		}},
		{"foreign-owner", func(t *testing.T, c *contactsCatalog) {
			rows := activeRows(t)
			replaceField(t, rows, 0, "owner_sub", "bob")
			c.raw[rowKey("book", bookID)] = rows[0].JSON
		}},
		{"invalid-date", func(t *testing.T, c *contactsCatalog) {
			rows := activeRows(t)
			replaceField(t, rows, 2, "birthday", "invalid")
			c.raw[rowKey("contact", contactID)] = rows[2].JSON
		}},
		{"later-read-failure", func(_ *testing.T, c *contactsCatalog) { c.failure = errors.New("device unavailable"); c.failAt = 3 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := catalogRows(activeRows(t))
			test.change(t, catalog)
			snapshot, err := readCatalog(t.Context(), testScope(), catalog)
			if err == nil || snapshot != nil {
				t.Fatal("accepted partial/foreign private agenda")
			}
			if catalog.failure != nil && !errors.Is(err, catalog.failure) {
				t.Fatal("lost original device failure", err)
			}
			for _, raw := range catalog.returned {
				for _, value := range raw {
					if value != 0 {
						t.Fatal("error retained temporary plaintext")
					}
				}
			}
		})
	}
	if source, err := NewSource(nil); source != nil || !errors.Is(err, ErrSnapshot) {
		t.Fatal("accepted nil migration source")
	}
	config := Config{Scope: testScope()}
	config.Scope.Product = "notes"
	if repository, err := New(config); repository != nil || !errors.Is(err, ErrSnapshot) {
		t.Fatal("accepted another product before checking crypto")
	}
}
