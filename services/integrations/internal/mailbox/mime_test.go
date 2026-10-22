// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestMessageRoundTripKeepsRecipientsPrivate(t *testing.T) {
	out := Outgoing{To: []Address{{Name: "Ana López", Email: "ana@example.com"}}, Bcc: []Address{{Email: "hidden@example.com"}}, Subject: "Revisión del día", Text: "Hola, mañana.\nSegunda línea.", HTML: "<p>Hola</p>", InReplyTo: "parent@example.com", Attachments: []Attachment{{Name: "datos.csv", Type: "text/csv", ContentBase64: base64.StdEncoding.EncodeToString([]byte("a,b\n1,2"))}}}
	raw, err := EncodeMessage("sender@example.org", out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("hidden@example.com")) || bytes.Contains(raw, []byte("Bcc:")) {
		t.Fatal("Bcc disclosed in MIME")
	}
	got, err := DecodeMessage(raw, "message-1", "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "message-1" || got.FolderID != "INBOX" || got.Subject != out.Subject || len(got.From) != 1 || got.From[0].Email != "sender@example.org" || got.To[0] != out.To[0] || !strings.Contains(got.Text, "mañana") || got.HTML != out.HTML || len(got.Attachments) != 1 || got.Attachments[0].ContentBase64 != out.Attachments[0].ContentBase64 {
		t.Fatalf("lost message content: %+v", got)
	}
}

func TestEncodeRejectsHeaderAndEnvelopeInjection(t *testing.T) {
	valid := Outgoing{To: []Address{{Email: "ana@example.com"}}, Subject: "hello", Text: "body"}
	for _, change := range []func(*Outgoing){
		func(o *Outgoing) { o.Subject = "hello\r\nBcc: victim@example.net" },
		func(o *Outgoing) { o.To = []Address{{Email: "ana@example.com\r\nRCPT TO:<victim@example.net>"}} },
		func(o *Outgoing) { o.To = []Address{{Name: "Ana\nBcc: X", Email: "ana@example.com"}} },
		func(o *Outgoing) { o.InReplyTo = "x@example.org> <another@example.org" },
		func(o *Outgoing) { o.To = nil },
		func(o *Outgoing) {
			o.Attachments = []Attachment{{Name: "x\r\nContent-Type: text/html", Type: "text/plain"}}
		},
	} {
		out := valid
		change(&out)
		if _, err := EncodeMessage("sender@example.org", out); !errors.Is(err, ErrInvalid) {
			t.Errorf("unsafe outgoing accepted: %v", err)
		}
	}
	if _, err := EncodeMessage("Other <other@example.net>", valid); !errors.Is(err, ErrInvalid) {
		t.Fatal("sender display-name form accepted")
	}
}

func TestDecodeLegacyCharsetAndLimits(t *testing.T) {
	raw := []byte("From: Ana <ana@example.com>\r\nTo: b@example.net\r\nSubject: =?ISO-8859-1?Q?Mañana?=\r\nContent-Type: text/plain; charset=iso-8859-1\r\n\r\nma\xf1ana")
	m, err := DecodeMessage(raw, "1", "INBOX")
	if err != nil || m.Text != "mañana" {
		t.Fatalf("legacy charset: %+v %v", m, err)
	}
	if _, err := DecodeMessage(make([]byte, MaxMessageBytes+1), "1", "INBOX"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized input: %v", err)
	}
	if _, err := EncodeMessage("a@example.com", Outgoing{To: []Address{{Email: "b@example.com"}}, Text: strings.Repeat("x", MaxMessageBytes+1)}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized output: %v", err)
	}
}

func TestHTMLOnlyMessageProvidesTextWithoutFetchingResources(t *testing.T) {
	raw := []byte("From: a@example.eu\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<head><style>secret</style></head><p>Hello &amp; welcome</p><img src=\"https://tracker.invalid/pixel\"><script>bad()</script>")
	m, err := DecodeMessage(raw, "html", "INBOX")
	if err != nil || m.Text != "Hello & welcome" || strings.Contains(m.Snippet, "tracker") {
		t.Fatalf("HTML fallback: %#v %v", m, err)
	}
}
