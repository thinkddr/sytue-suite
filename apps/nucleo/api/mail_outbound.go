// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type MailOutgoing struct {
	Operation, DraftID, From string
	Recipients               []string
	MIME                     []byte
	Consent                  string
	ReleaseAt                time.Time
}

type MailOutgoingAttempt struct {
	Scope     storage.Scope `json:"scope"`
	Epoch     string        `json:"epoch"`
	Operation string        `json:"operation"`
}

type MailOutgoingStatus struct {
	Attempt   MailOutgoingAttempt
	Receipt   model.SubmissionReceipt
	ReleaseAt time.Time
	Queued    bool
}

type PrivateMailOutbound interface {
	PrepareOutbound(context.Context, MailOutgoing) (MailOutgoingAttempt, error)
	CommitOutbound(context.Context, MailOutgoingAttempt) (MailOutgoingStatus, error)
	CancelPrivateOutbound(context.Context, MailOutgoingAttempt) error
	PendingOutbound(context.Context) ([]MailOutgoingAttempt, error)
	Scheduled(context.Context) ([]MailScheduled, error)
	Reschedule(context.Context, string, time.Time) error
	CancelOutbound(context.Context, string) error
}

func (m *Mail) PrivateOutbound() (PrivateMailOutbound, bool) {
	if m == nil || m.data == nil {
		return nil, false
	}
	port, _ := m.data.(PrivateMailOutbound)
	return port, true
}
