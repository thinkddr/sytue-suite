// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MailData is the private mailbox surface used by the existing native screens.
// Unimplemented transport, sharing or account operations stay closed; callers
// retain their original Mail client separately for explicit public account work.
type MailData interface {
	PrivateStorageScope() storage.Scope
	Aliases(context.Context) (string, []MailAlias, error)
	Mailboxes(context.Context) ([]Mailbox, error)
	Messages(context.Context, string, string, int) ([]MailMessage, string, error)
	Search(context.Context, string, int, int) ([]MailMessage, error)
	Thread(context.Context, string) ([]MailMessage, error)
	Raw(context.Context, string) (io.ReadCloser, error)
	CreateMailbox(context.Context, string) (Mailbox, error)
	RenameMailbox(context.Context, string, string) (MailboxSetError, error)
	DeleteMailbox(context.Context, string) (MailboxSetError, error)
	SetFlagMany(context.Context, []string, string, bool) error
	MoveMany(context.Context, []string, string) error
	Delete(context.Context, string) error
	SaveDraft(context.Context, string, string, []byte) (MailMessage, error)
	Changes(context.Context, string) (MailCambios, error)
	Labels(context.Context) ([]MailLabel, error)
	LabelMessages(context.Context, string, string, int) ([]MailMessage, string, error)
	CreateLabel(context.Context, string, string) (MailLabel, error)
	UpdateLabel(context.Context, string, string, string) error
	DeleteLabel(context.Context, string) error
	ApplyLabel(context.Context, string, []string, bool) error
	Signature(context.Context) (string, error)
	SetSignature(context.Context, string) error
	Filters(context.Context) ([]MailFilter, error)
	CreateFilter(context.Context, MailFilter) (MailFilter, error)
	UpdateFilter(context.Context, MailFilter) error
	DeleteFilter(context.Context, string) error
	Vacation(context.Context) (MailVacation, error)
	SetVacation(context.Context, MailVacation) error
}

// MailRoutingData keeps explicit public delivery changes inside the private
// repository projection, without restoring an HTTP client to the facade.
type MailRoutingData interface {
	RefreshRouting(context.Context) (MailRoutingSnapshot, error)
	CreateRoutingAlias(context.Context, string) (MailAlias, error)
	DeleteRoutingAlias(context.Context, string) error
}
type MailQuotaData interface {
	Quota(context.Context) (MailQuota, error)
}

var ErrPrivateMailOperation = errors.New("mail: operation unavailable through private data")

// WithPrivateData never inherits a bearer, cookies or an HTTP fallback. Future
// API methods also fail closed until deliberately implemented by MailData.
func (m *Mail) WithPrivateData(data MailData) (*Mail, error) {
	if m == nil || m.c == nil || m.jmap == nil || data == nil || m.data != nil {
		return nil, ErrPrivateMailOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "mail" {
		return nil, ErrPrivateMailOperation
	}
	return &Mail{c: privateDataClient(m.c, ErrPrivateMailOperation), jmap: privateDataClient(m.jmap, ErrPrivateMailOperation), data: data}, nil
}

// MessageByteLimit exposes only the normalized capability already captured by
// the private repository. Missing private capability is unavailable, never a
// guessed legacy limit or an HTTP fallback.
func (m *Mail) MessageByteLimit() int64 {
	if m.data == nil {
		value, _ := limits.MessageBytes(0)
		return value
	}
	if value, ok := m.data.(interface{ MessageByteLimit() int64 }); ok {
		return value.MessageByteLimit()
	}
	return 0
}
