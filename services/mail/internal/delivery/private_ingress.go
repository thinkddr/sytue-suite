// SPDX-License-Identifier: Apache-2.0
package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// SMTP is an external plaintext boundary. Admission has already reserved its
// bounded in-memory input; this path stores neither MIME nor its envelope in
// PostgreSQL, legacy shared blobs, notifications or local plaintext files.
func (in *Inbound) persistPrivateInbound(ctx context.Context, plan store.PrivateInboundPlan, raw []byte) error {
	metadata, err := json.Marshal(struct {
		ID         string    `json:"id"`
		MailboxID  string    `json:"mailbox_id"`
		Size       int64     `json:"size"`
		ReceivedAt time.Time `json:"received_at"`
	}{ID: uuid.NewString(), MailboxID: plan.MailboxID.String(), Size: int64(len(raw)), ReceivedAt: plan.ReceivedAt})
	if err != nil {
		return err
	}
	defer clear(metadata)
	task, blobID, err := objectstore.SealPrivateIngress(ctx, in.privateObjects, plan.Key, plan.TaskID, "mail.receive", metadata, bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	return in.store.CommitPrivateInbound(ctx, plan, task, blobID)
}
