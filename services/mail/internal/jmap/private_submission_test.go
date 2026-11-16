// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
)

type privateSubmissionFixture struct {
	calls   int
	failure error
}

func (f *privateSubmissionFixture) SubmitRaw(context.Context, string, []string, []byte) error {
	panic("private request entered legacy plaintext submission")
}

func (f *privateSubmissionFixture) Schedule(context.Context, string, []string, []byte, time.Time, string) (uuid.UUID, error) {
	panic("private request entered legacy plaintext queue")
}

func (f *privateSubmissionFixture) SubmitPrivate(_ context.Context, owner string, input model.Submission, raw []byte) (model.SubmissionReceipt, error) {
	f.calls++
	if owner != "user-1" {
		return model.SubmissionReceipt{}, errors.New("wrong owner")
	}
	fingerprint, err := input.Fingerprint(raw)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	if f.failure != nil {
		return model.SubmissionReceipt{}, f.failure
	}
	return model.SubmissionReceipt{Operation: input.Operation, Fingerprint: fingerprint, States: []string{"accepted"}}, nil
}

func (f *privateSubmissionFixture) PrivateSubmissionReceipt(context.Context, string, string, string) (model.SubmissionReceipt, error) {
	return model.SubmissionReceipt{}, nil
}

type submissionReadCounter struct {
	reads int
	raw   io.Reader
}

func (r *submissionReadCounter) Read(out []byte) (int, error) { r.reads++; return r.raw.Read(out) }
func (r *submissionReadCounter) Close() error                 { return nil }

func TestPrivateSubmissionHTTPRejectsChangedScopeEpochAndMissingConsentBeforeReadingMIME(t *testing.T) {
	for _, fault := range []string{"scope", "epoch", "consent"} {
		t.Run(fault, func(t *testing.T) {
			source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "frozen", Epoch: mailHTTPSourceEpoch}}
			sender := &privateSubmissionFixture{}
			handler := &API{privateSource: source, privateIngress: &privateIngressFixture{ready: true}, outbound: sender, cfg: config.Config{MaxMessageBytes: 1 << 20}}
			input := model.Submission{Operation: uuid.NewString(), Epoch: mailHTTPSourceEpoch, From: "owner@example.test", Recipients: []string{"recipient@external.test"}, Consent: model.SMTPConsent, Generation: 1, Control: strings.Repeat("a", 64)}
			if fault == "epoch" {
				input.Epoch = uuid.NewString()
			}
			if fault == "consent" {
				input.Consent = ""
			}
			request := sourceRequest(t, http.MethodPost, "/api/v1/private-storage/submissions", "")
			if fault == "scope" {
				request.Header.Del(storage.ScopeHeader)
			}
			metadata, _ := json.Marshal(input)
			request.Header.Set(model.SubmissionHeader, base64.RawURLEncoding.EncodeToString(metadata))
			request.Header.Set("Content-Type", "message/rfc822")
			body := &submissionReadCounter{raw: strings.NewReader("PRIVATE-MIME-NOT-TO-BE-READ")}
			request.Body = body
			response := httptest.NewRecorder()
			handler.submitPrivateMessage(response, request)
			if response.Code < 400 || body.reads != 0 || sender.calls != 0 {
				t.Fatal("invalid authority disclosed MIME", response.Code, body.reads, sender.calls)
			}
		})
	}
}

func TestPrivateSubmissionHTTPDoesNotReflectParserOrRelayContent(t *testing.T) {
	source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "frozen", Epoch: mailHTTPSourceEpoch}}
	sender := &privateSubmissionFixture{failure: errors.New("REMOTE-REFLECTED-PRIVATE-SUBJECT-AND-BODY")}
	handler := &API{privateSource: source, privateIngress: &privateIngressFixture{ready: true}, outbound: sender, cfg: config.Config{MaxMessageBytes: 1 << 20}}
	input := model.Submission{Operation: uuid.NewString(), Epoch: mailHTTPSourceEpoch, From: "owner@example.test", Recipients: []string{"recipient@external.test"}, Consent: model.SMTPConsent, Generation: 1, Control: strings.Repeat("a", 64)}
	request := sourceRequest(t, http.MethodPost, "/api/v1/private-storage/submissions", "From: owner@example.test\r\n\r\nPRIVATE-BODY")
	metadata, _ := json.Marshal(input)
	request.Header.Set(model.SubmissionHeader, base64.RawURLEncoding.EncodeToString(metadata))
	request.Header.Set("Content-Type", "message/rfc822")
	response := httptest.NewRecorder()
	handler.submitPrivateMessage(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "PRIVATE") || sender.calls != 1 {
		t.Fatal("private relay details escaped into HTTP errors", response.Code, sender.calls)
	}
}
