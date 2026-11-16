// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MailDraftRevision is one editor revision, composed exactly once before its
// first attempt. Its MIME is admitted into the existing encrypted local journal.
type MailDraftRevision struct {
	Editor   string
	Revision string
	ID       string
	From     string
	MIME     []byte
}

// MailDraftAttempt identifies an admitted immutable revision, not a fresh edit.
// It contains no message contents, sender address or transport credentials.
type MailDraftAttempt struct {
	Scope    storage.Scope `json:"scope"`
	Epoch    string        `json:"epoch"`
	Editor   string        `json:"editor"`
	Revision string        `json:"revision"`
	ID       string        `json:"id"`
}

// PrivateMailDraftRevisions separates admitting a revision from replaying it.
// Commit never accepts replacement MIME; callers retain its handle until ACK.
type PrivateMailDraftRevisions interface {
	PrepareDraftRevision(context.Context, MailDraftRevision) (MailDraftAttempt, error)
	CommitDraftRevision(context.Context, MailDraftAttempt) (MailMessage, error)
	PendingDraftRevisions(context.Context) ([]MailDraftAttempt, error)
}

// PrivateDraftRevisions never returns an HTTP implementation. The bool reports
// private mode; a private facade with no revision port must remain closed.
func (m *Mail) PrivateDraftRevisions() (PrivateMailDraftRevisions, bool) {
	if m == nil || m.data == nil {
		return nil, false
	}
	port, _ := m.data.(PrivateMailDraftRevisions)
	return port, true
}
