// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func activeRows(t *testing.T) []Row {
	t.Helper()
	rows := completeRows()
	for _, index := range []int{0, 1, 2} {
		replaceField(t, rows, index, "deleted_at", nil)
	}
	return rows
}

func contactAggregate(rows []Row) Aggregate {
	return Aggregate{Version: 1, Scope: testScope(), Kind: "contact_state", ID: contactID, Rows: []Row{rows[1], rows[2], rows[5], rows[6]}}
}

func TestAggregatePublishesContactAndAllChannelsTogetherWithoutChangingBaseline(t *testing.T) {
	rows := activeRows(t)
	baseline, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	original := baseline.Rows()
	replaceField(t, rows, 2, "note", "Edited privately")
	replaceField(t, rows, 5, "email", "new@example.invalid")
	change := contactAggregate(rows)
	current, err := baseline.WithAggregates([]Aggregate{change})
	if err != nil {
		t.Fatal(err)
	}
	people, err := current.Contacts()
	if err != nil || len(people) != 1 || people[0].Note != "Edited privately" || people[0].Emails[0].Email != "new@example.invalid" {
		t.Fatal("partial aggregate", err)
	}
	again := baseline.Rows()
	for i, row := range original {
		if !bytes.Equal(row.JSON, again[i].JSON) {
			t.Fatal("mutated retained baseline")
		}
	}
	// A caller cannot alter the accepted revision by reusing its buffers.
	clear(change.Rows[0].JSON)
	if _, err := current.Contacts(); err != nil {
		t.Fatal("caller changed current snapshot", err)
	}
}

func TestAggregateRejectsPartialCardsForeignRowsAndCollisions(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, *Aggregate)
	}{
		{"missing-card", func(_ *testing.T, a *Aggregate) { a.Rows = a.Rows[1:] }},
		{"foreign-row", func(t *testing.T, a *Aggregate) { replaceField(t, a.Rows, 2, "contact_id", groupID) }},
		{"foreign-scope", func(_ *testing.T, a *Aggregate) { a.Scope.Subject = "bob" }},
		{"wrong-identity", func(_ *testing.T, a *Aggregate) { a.ID = groupID }},
		{"extra-table", func(_ *testing.T, a *Aggregate) { a.Rows = append(a.Rows, completeRows()[4]) }},
		{"wrong-object", func(t *testing.T, a *Aggregate) { replaceField(t, a.Rows, 1, "object_id", groupCardID) }},
		{"duplicate-row", func(_ *testing.T, a *Aggregate) { a.Rows = append(a.Rows, a.Rows[0]) }},
		{"wrong-trash-state", func(_ *testing.T, a *Aggregate) { now := time.Now(); a.DeletedAt = &now }},
		{"invalid-purge", func(_ *testing.T, a *Aggregate) { a.Purged = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := activeRows(t)
			baseline, err := NewSnapshot(testScope(), rows)
			if err != nil {
				t.Fatal(err)
			}
			change := contactAggregate(rows)
			test.change(t, &change)
			if _, err := baseline.WithAggregates([]Aggregate{change}); !errors.Is(err, ErrSnapshot) {
				t.Fatal("accepted partial/foreign aggregate", err)
			}
		})
	}
	rows := activeRows(t)
	baseline, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	change := contactAggregate(rows)
	if _, err := baseline.WithAggregates([]Aggregate{change, change}); !errors.Is(err, ErrSnapshot) {
		t.Fatal("accepted duplicate current revisions")
	}
}

func TestAggregatePurgeRetainsUnresolvedGroupMembershipAndBookDeleteIsOneRevision(t *testing.T) {
	baseline, err := NewSnapshot(testScope(), activeRows(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	purge := Aggregate{Version: 1, Scope: testScope(), Kind: "contact_state", ID: contactID, Purged: true, DeletedAt: &now}
	current, err := baseline.WithAggregates([]Aggregate{purge})
	if err != nil {
		t.Fatal(err)
	}
	people, err := current.Contacts()
	if err != nil || len(people) != 0 {
		t.Fatal("purge retained contact", err)
	}
	groups, err := current.Groups()
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatal("purge erased group members", err)
	}
	for _, row := range current.Rows() {
		if row.Kind == "member" && row.ID == memberID {
			var member map[string]json.RawMessage
			if json.Unmarshal(row.JSON, &member) != nil || string(member["contact_id"]) != "null" {
				t.Fatal("retained dangling resolved hint")
			}
		}
	}
	rows := activeRows(t)
	replaceField(t, rows, 0, "deleted_at", now.Format(time.RFC3339Nano))
	deleted := Aggregate{Version: 1, Scope: testScope(), Kind: "book_state", ID: bookID, Rows: rows[:1], DeletedAt: &now}
	current, err = baseline.WithAggregates([]Aggregate{deleted})
	if err != nil {
		t.Fatal(err)
	}
	people, err = current.Contacts()
	if err != nil || len(people) != 1 || !people[0].BookDeleted {
		t.Fatal("partial book deletion", err)
	}
	groups, err = current.Groups()
	if err != nil || len(groups) != 1 || !groups[0].BookDeleted {
		t.Fatal("partial group visibility", err)
	}
}
