// SPDX-License-Identifier: Apache-2.0
package delivery

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/smtptest"
)

type privateSMTPFixture struct {
	sender   *SMTPSender
	server   *smtptest.Server
	received chan []byte
}

func (f *privateSMTPFixture) acceptedCount() int32 { return f.server.Accepted() }
func newPrivateSMTPFixture(t *testing.T, lostDataACK, lostQuitACK bool) *privateSMTPFixture {
	t.Helper()
	server := smtptest.New(t, smtptest.Config{LoseDataACK: lostDataACK, LoseQuitACK: lostQuitACK})
	sender := NewSMTPSenderConRelay(slog.New(slog.DiscardHandler), &Relay{Host: server.Host, Port: server.Port})
	sender.timeout = 5 * time.Second
	sender.privateTLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: server.Roots}
	return &privateSMTPFixture{sender: sender, server: server, received: server.Received}
}

func TestPrivateSMTPAcceptedDataSurvivesLostQuitACK(t *testing.T) {
	fixture := newPrivateSMTPFixture(t, false, true)
	raw := []byte("From: owner@example.test\r\nTo: receiver@example.test\r\nMessage-ID: <stable@private.test>\r\n\r\nPRIVATE-MIME-AND-ATTACHMENT\r\n")
	if _, err := fixture.sender.SendPrivate(t.Context(), "owner@example.test", []string{"receiver@example.test"}, raw); err != nil {
		t.Fatal("accepted DATA became a second delivery candidate", err)
	}
	if fixture.acceptedCount() != 1 || !bytes.Contains(<-fixture.received, []byte("PRIVATE-MIME-AND-ATTACHMENT")) {
		t.Fatal("SMTP did not accept the original message exactly once")
	}
}

func TestPrivateSMTPLostDataACKIsUncertainAndNeverTriesAnotherHost(t *testing.T) {
	fixture := newPrivateSMTPFixture(t, true, false)
	_, err := fixture.sender.SendPrivate(context.Background(), "owner@example.test", []string{"receiver@example.test"}, []byte("From: owner@example.test\r\n\r\nPRIVATE-MIME\r\n"))
	if !errors.Is(err, ErrSMTPAmbiguous) || fixture.acceptedCount() != 1 {
		t.Fatal("lost DATA ACK was treated as safe to resend", fixture.acceptedCount(), err)
	}
}
