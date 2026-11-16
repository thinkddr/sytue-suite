// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"errors"
	"testing"
)

func TestRetainedSourceIDsPreserveDatabaseAuditNumbers(t *testing.T) {
	r := &Repository{cfg: Config{Scope: testScope}}
	for _, value := range []struct {
		kind, id, body string
		valid          bool
	}{
		{"audit", "9007199254740993123", `{"id":9007199254740993123,"detail":{"future":900719925474099312345}}`, true},
		{"audit", "9007199254740993123", `{"id":9007199254740993124}`, false},
		{"audit", "9", `{"id":9.0}`, false},
		{"audit", "9223372036854775808", `{"id":9223372036854775808}`, false},
		{"tag_mark", "mark", `{"note_id":"note","tag_id":"tag","state_id":"state"}`, true},
		{"note", "note", `{"id":1,"owner_sub":"alice"}`, false},
		{"notebook", "book", `{"id":"book","owner_sub":"alice","visibility":"private"}`, true},
	} {
		err := r.validatePersonal(value.kind, value.id, []byte(value.body))
		if (err == nil) != value.valid {
			t.Errorf("%s %s valid=%v: %v", value.kind, value.id, value.valid, err)
		}
	}
}

func TestRetainedSourceRejectsPlainAttachmentPointersBeforeActivation(t *testing.T) {
	r := &Repository{cfg: Config{Scope: testScope}}
	for _, raw := range []string{
		`{"id":"link","target_kind":"drive_file","target_id":"file"}`,
		`{"id":"link","target_kind":"docs_document","target_id":"document"}`,
		`{"id":"note","owner_sub":"alice","links":[{"target_kind":"drive_file","target_id":"file"}]}`,
	} {
		if err := r.validatePersonal("note_link", "link", []byte(raw)); !errors.Is(err, ErrAttachmentPending) {
			t.Fatal("attachment pointer treated as privately copied content", err)
		}
	}
}
