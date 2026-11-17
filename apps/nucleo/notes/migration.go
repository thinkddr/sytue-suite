// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"encoding/json"
	"errors"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// ErrAttachmentPending blocks activation until attachment bytes are private too.
var ErrAttachmentPending = errors.New("notes: attachment content must be privately imported before activation")

type (
	SnapshotState = records.SnapshotState
	SnapshotRow   = records.SnapshotRow
	SnapshotPage  = records.SnapshotPage
	Source        = records.Source
)

func (r *Repository) validatePersonal(kind, id string, raw []byte) error {
	var value struct {
		ID          json.RawMessage `json:"id"`
		ContentKind string          `json:"kind"`
		Owner       string          `json:"owner_sub"`
		Visibility  string          `json:"visibility"`
		PublicToken string          `json:"public_token"`
		RoomID      string          `json:"room_id"`
		TargetKind  string          `json:"target_kind"`
		Links       []struct {
			TargetKind string `json:"target_kind"`
		} `json:"links"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return ErrSource
	}
	if unresolvedAttachmentLink(value.TargetKind) {
		return ErrAttachmentPending
	}
	for _, link := range value.Links {
		if unresolvedAttachmentLink(link.TargetKind) {
			return ErrAttachmentPending
		}
	}
	if kind == "note" && unresolvedAttachment(value.ContentKind) {
		return ErrAttachmentPending
	}
	if !sourceIdentityMatches(kind, id, value.ID) || ((kind == "notebook" || kind == "note") && value.Owner == "") {
		return ErrSource
	}
	if value.Owner != "" && value.Owner != r.cfg.Scope.Subject {
		return ErrShared
	}
	if (kind == "notebook" || kind == "note") && (value.Visibility == "organization" || value.Visibility == "public" || value.PublicToken != "" || value.RoomID != "") {
		return ErrShared
	}
	return nil
}

func unresolvedAttachment(kind string) bool {
	switch kind {
	case "image", "file", "drive", "doc":
		return true
	default:
		return false
	}
}

func sourceIdentityMatches(kind, id string, raw json.RawMessage) bool {
	if len(raw) == 0 {
		return kind != "notebook" && kind != "note"
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text == id
	}
	if kind != "audit" {
		return false
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil || number.String() != id {
		return false
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}

func unresolvedAttachmentLink(kind string) bool {
	return kind == "drive_file" || kind == "docs_document"
}
