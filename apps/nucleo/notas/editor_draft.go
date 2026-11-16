// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"bytes"
	"slices"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// EditorDraft is an owned editor source in the existing counted Notes draft
// namespace. Version1 remains readable; version2 reconstructs exact plain HTML
// from its input while retaining independent HTML for optional notebook ciphers.
type EditorDraft struct {
	Version    int    `json:"version"`
	NoteID     string `json:"note_id"`
	NotebookID string `json:"notebook_id"`
	Revision   string `json:"revision"`
	HTML       string `json:"html,omitempty"`
	FromInput  bool   `json:"body_from_input,omitempty"`
	// Blocked drafts remain local until a newer client can safely migrate links.
	Blocked           bool          `json:"blocked,omitempty"`
	Input             api.NoteInput `json:"input"`
	ExpectedUpdatedAt time.Time     `json:"expected_updated_at"`
}

func (d EditorDraft) Valid() bool {
	if d.NoteID == "" || d.NotebookID == "" || d.Revision == "" {
		return false
	}
	if !utf8.ValidString(d.HTML) || !utf8.ValidString(d.Input.Content) {
		return false
	}
	if d.Version == 1 {
		return !d.FromInput && !d.Blocked
	}
	if d.Version == 3 {
		return d.Blocked && !d.FromInput
	}
	if d.Version != 2 || d.Blocked {
		return false
	}
	return !d.FromInput || (d.HTML == "" && d.Input.ContentCipher == "" && d.Input.CipherVersion == 0 && len(d.Input.ExpectedCrypto) == 0)
}

// EditorHTML reconstructs only a representation declared exact at write time.
// Version1 and optionally encrypted notebooks retain their original HTML.
func (d EditorDraft) EditorHTML() string {
	if d.Version == 2 && d.FromInput {
		return d.Input.Content
	}
	return d.HTML
}

// Clone gives an admitted writer ownership of mutable input slices and its CAS
// timestamp without serializing the document on the UI thread.
func (d EditorDraft) Clone() EditorDraft {
	out := d
	out.Input.Data = bytes.Clone(d.Input.Data)
	out.Input.ExpectedCrypto = bytes.Clone(d.Input.ExpectedCrypto)
	out.Input.Links = slices.Clone(d.Input.Links)
	if d.Input.ExpectedUpdatedAt != nil {
		at := *d.Input.ExpectedUpdatedAt
		out.Input.ExpectedUpdatedAt = &at
	}
	out.Input.X = cloneEditorPointer(d.Input.X)
	out.Input.Y = cloneEditorPointer(d.Input.Y)
	out.Input.Col = cloneEditorPointer(d.Input.Col)
	out.Input.Position = cloneEditorPointer(d.Input.Position)
	out.Input.Width = cloneEditorPointer(d.Input.Width)
	out.Input.GroupID = cloneEditorPointer(d.Input.GroupID)
	out.Input.Collapsed = cloneEditorPointer(d.Input.Collapsed)
	out.Input.Checked = cloneEditorPointer(d.Input.Checked)
	out.Input.Cancelled = cloneEditorPointer(d.Input.Cancelled)
	out.Input.Pinned = cloneEditorPointer(d.Input.Pinned)
	out.Input.Indexable = cloneEditorPointer(d.Input.Indexable)
	return out
}

func cloneEditorPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	owned := *value
	return &owned
}
