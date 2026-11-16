// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func submissionInput() model.Submission {
	return model.Submission{Operation: "11111111-1111-4111-8111-111111111111", Epoch: "22222222-2222-4222-8222-222222222222", From: "owner@example.test", Recipients: []string{"recipient@external.test"}, Consent: model.SMTPConsent, Generation: 1, Control: strings.Repeat("a", 64)}
}

func TestPrivateSubmissionTransportScopesStreamsAndNeverReplaysOrForwardsCookies(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	input := submissionInput()
	raw := []byte("From: owner@example.test\r\n\r\nPRIVATE-SUBMISSION-MIME")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if storage.ValidateRequestScope(r.Header, scope) != nil || r.URL.Path != "/api/v1/private-storage/submissions" || r.Method != http.MethodPost || r.Header.Get("Cookie") != "" {
			t.Error("submission escaped scope, route, verb or cookie isolation")
		}
		metadata, err := base64.RawURLEncoding.DecodeString(r.Header.Get(model.SubmissionHeader))
		if err != nil {
			t.Error(err)
		}
		var actual model.Submission
		if json.Unmarshal(metadata, &actual) != nil || actual.Validate() != nil || actual.Operation != input.Operation {
			t.Error("submission metadata changed")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, raw) {
			t.Error("streamed MIME changed")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar.SetCookies(address, []*http.Cookie{{Name: "unrelated-browser-cookie", Value: "synthetic-private-cookie"}})
	transport, err := NewMail(server.URL, nil, client).PrivateSubmissions(scope, input.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Submit(t.Context(), input, bytes.NewReader(raw), int64(len(raw))); err == nil || calls.Load() != 1 {
		t.Fatal("unauthorized MIME was replayed", calls.Load(), err)
	}
}

func TestPrivateSubmissionTransportNeverFollowsAContentRedirect(t *testing.T) {
	var leaked atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer outside.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, outside.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	input := submissionInput()
	transport, err := NewMail(server.URL, nil, server.Client()).PrivateSubmissions(storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}, input.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("PRIVATE-SUBMISSION-MIME")
	if _, err := transport.Submit(t.Context(), input, bytes.NewReader(raw), int64(len(raw))); err == nil || leaked.Load() != 0 {
		t.Fatal("redirect disclosed MIME to another origin", leaked.Load(), err)
	}
}

func TestPrivateFacadeCannotManufactureAnAuthenticatedSubmissionTransport(t *testing.T) {
	original := NewMail("https://closed.invalid", nil, nil)
	facade, err := original.WithPrivateData(&privateMailFixture{scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PrivateSubmissions(storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}, submissionInput().Epoch); err == nil {
		t.Fatal("private facade reopened its legacy authenticated transport")
	}
}
