// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type lostSharedIndexACK struct {
	*cryptoFixture
	lose bool
}

func (c *lostSharedIndexACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := c.cryptoFixture.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
	var h header
	if err == nil && json.Unmarshal(commit.Metadata, &h) == nil && h.Type == "outbound" && c.lose {
		c.lose = false
		return version, errors.New("synthetic shared index ACK lost")
	}
	return version, err
}

func TestSharedPrivateOutboundRetiresOldMetadataACKAfterAnotherDeviceCanceled(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Shared metadata ACK original")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	replaceTestCrypto(t, f.repo, &lostSharedIndexACK{cryptoFixture: f.crypto, lose: true})
	attempt, err := f.repo.PrepareOutbound(t.Context(), api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CommitOutbound(t.Context(), attempt); err == nil {
		t.Fatal("accepted shared metadata index ACK was not lost")
	}
	fresh, _ := freshOutboundRepository(t, f, transport)
	if err := fresh.DiscoverOutbound(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := fresh.CancelPrivateOutbound(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	count := f.crypto.puts
	status, err := f.repo.CommitOutbound(t.Context(), attempt)
	if err != nil || !status.Receipt.Canceled || transport.sent != 0 {
		t.Fatal("original owner did not reconcile cancellation", err)
	}
	if f.crypto.puts != count {
		t.Fatal("old owner republished obsolete shared metadata")
	}
	pending, err := f.repo.cfg.Journal.List(f.repo.outboundOwner(), writesNamespace)
	if err != nil || len(pending) != 0 {
		t.Fatal("accepted old shared metadata ACK stranded its WAL", len(pending), err)
	}
}
