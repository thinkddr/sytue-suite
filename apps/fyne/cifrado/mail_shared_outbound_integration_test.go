// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package cifrado

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

type nativeSharedOutboundProof struct {
	sent, canceled api.MailOutgoingAttempt
	raw            []byte
	puts           int
}

// The original SDK owner publishes future jobs, then retires before SMTP.
// No key or journal is transferred to the SSSS-recovered device.
func prepareNativeSharedOutbound(ctx context.Context, t *testing.T, repository *privateMail.Repository, facade *api.Mail, snapshot *storagetest.MailSnapshot, provider *storagetest.CipherGateway) *nativeSharedOutboundProof {
	t.Helper()
	if snapshot == nil {
		t.Fatal("shared outbound proof requires the original retained SQL fixture")
	}
	primary, _, err := facade.Aliases(ctx)
	if err != nil || primary == "" {
		t.Fatal("missing authenticated sender", err)
	}
	proof := &nativeSharedOutboundProof{}
	for index, subject := range []string{"PRIVATE-NATIVE-SHARED-SENT", "PRIVATE-NATIVE-SHARED-CANCELED"} {
		raw, err := eml.Componer(eml.Salida{De: primary, Para: []string{"recipient@external.test"}, Asunto: subject, Texto: "Entire original signature follows:\n" + snapshot.Signature, Adjuntos: []eml.Adjunto{{Nombre: "private.txt", Tipo: "text/plain", Datos: []byte("PRIVATE-NATIVE-SHARED-ATTACHMENT")}}}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		input := api.MailOutgoing{Operation: uuid.NewString(), From: primary, Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)}
		attempt, err := repository.PrepareOutbound(ctx, input)
		if err != nil {
			clear(raw)
			t.Fatal("SDK producer could not capture original", err)
		}
		status, err := repository.CommitOutbound(ctx, attempt)
		if err != nil || !status.Queued {
			clear(raw)
			t.Fatal("SDK producer did not publish E2E future job", err)
		}
		if index == 0 {
			proof.sent = attempt
			proof.raw = raw
			t.Cleanup(func() { clear(proof.raw) })
		} else {
			proof.canceled = attempt
			clear(raw)
		}
	}
	snapshotProvider := provider.Snapshot()
	proof.puts = snapshotProvider.Puts
	for _, cipher := range snapshotProvider.Data {
		if bytes.Contains(cipher, []byte("PRIVATE-NATIVE-SHARED")) || bytes.Contains(cipher, []byte("Entire original signature follows")) {
			t.Fatal("provider received readable outbox content")
		}
	}
	t.Log("MAIL_SHARED_OUTBOUND_ORIGINAL_DEVICE_PUBLISHED_E2E_JOBS")
	return proof
}

func completeNativeSharedOutbound(ctx context.Context, t *testing.T, proof *nativeSharedOutboundProof, repository *privateMail.Repository, facade *api.Mail, provider *storagetest.CipherGateway, transport *nativeMailFixtureTransport, client *http.Client, origin string) {
	t.Helper()
	if rows, err := repository.PendingOutbound(ctx); err != nil || len(rows) != 0 {
		t.Fatal("fresh SSSS owner did not start with empty local outbox", len(rows), err)
	}
	if err := repository.DiscoverOutbound(ctx); err != nil {
		t.Fatal("fresh SDK could not discover encrypted jobs", err)
	}
	rows, err := repository.Scheduled(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatal("fresh SDK did not recover both encrypted schedules", len(rows), err)
	}
	if err := repository.CancelPrivateOutbound(ctx, proof.canceled); err != nil {
		t.Fatal("fresh SDK could not cancel shared job", err)
	}
	if err := repository.CancelPrivateOutbound(ctx, proof.canceled); err != nil {
		t.Fatal("lost local cancellation callback did not reconcile", err)
	}
	due := time.Now().Add(20 * time.Millisecond)
	if err := repository.Reschedule(ctx, proof.sent.Operation, due); err != nil {
		t.Fatal("fresh SDK could not revise encrypted schedule", err)
	}
	timer := time.NewTimer(time.Until(due) + time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	transport.loseSubmit.Store(true)
	if _, err := repository.CommitOutbound(ctx, proof.sent); err == nil || !transport.lostSubmit.Load() {
		t.Fatal("real HTTP SMTP acceptance ACK was not lost", err)
	}
	if pending, err := repository.PendingOutbound(ctx); err != nil || len(pending) != 1 {
		t.Fatal("lost SMTP HTTP ACK discarded the retained shared job", len(pending), err)
	}
	status, err := repository.CommitOutbound(ctx, proof.sent)
	if err != nil || !status.Receipt.Accepted() {
		t.Fatal("fresh SDK could not reconcile accepted original", err)
	}
	if transport.submitted.Load() != 1 {
		t.Fatal("lost HTTP acceptance resent MIME", transport.submitted.Load())
	}
	if status, err := repository.CommitOutbound(ctx, proof.sent); err != nil || !status.Receipt.Accepted() {
		t.Fatal("lost local acceptance callback did not reconcile", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/fixture/outbound-proof", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer synthetic-live-device-session")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var observed struct {
		Accepted int32
		Raw      []byte
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&observed) != nil {
		t.Fatal("real SMTP proof unavailable")
	}
	defer clear(observed.Raw)
	if observed.Accepted != 1 || !bytes.Equal(bytes.ReplaceAll(proof.raw, []byte("\r\n"), []byte("\n")), observed.Raw) {
		t.Fatal("real SMTP duplicated, truncated or recomposed original", observed.Accepted, len(proof.raw), len(observed.Raw))
	}
	mailboxes, err := facade.Mailboxes(ctx)
	if err != nil {
		t.Fatal("could not project finalized private mailboxes", err)
	}
	foundSent, foundDraft := false, false
	for _, box := range mailboxes {
		if box.Role != "sent" && box.Role != "drafts" {
			continue
		}
		messages, _, err := facade.Messages(ctx, box.ID, "", 50)
		if err != nil {
			t.Fatal("could not page finalized private messages", err)
		}
		for _, message := range messages {
			if message.Subject == "PRIVATE-NATIVE-SHARED-CANCELED" {
				foundDraft = box.Role == "drafts" && message.Has("\\Draft")
			}
			if message.Subject != "PRIVATE-NATIVE-SHARED-SENT" {
				continue
			}
			foundSent = box.Role == "sent"
			body, err := facade.Raw(ctx, message.ID)
			if err != nil {
				t.Fatal("could not read finalized private MIME", err)
			}
			raw, err := io.ReadAll(body)
			closeErr := body.Close()
			equal := bytes.Equal(raw, proof.raw)
			clear(raw)
			if err != nil || closeErr != nil || !equal {
				t.Fatal("encrypted Sent lost original full MIME", err, closeErr)
			}
		}
	}
	if !foundSent || !foundDraft {
		t.Fatal("fresh SDK did not finish encrypted Sent and editable canceled Draft", foundSent, foundDraft)
	}
	if provider.Snapshot().Puts != proof.puts {
		t.Fatal("fresh SDK reuploaded original MIME on adoption")
	}
	if pending, err := repository.PendingOutbound(ctx); err != nil || len(pending) != 0 {
		t.Fatal("final shared receipt retained an incomplete outbox", len(pending), err)
	}
	t.Log("MAIL_SHARED_OUTBOUND_FRESH_SDK_SSSS_REAL_PG_TLS_SMTP_LOST_HTTP_ACK_EXACTLY_ONCE_PASS")
}
