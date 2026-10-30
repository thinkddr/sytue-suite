// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package service

import (
	"encoding/base64"
	"io"
	"mime"
	"net/mail"
	"strings"
	"testing"
)

func TestAlertMailRejectsInjectedHeaders(t *testing.T) {
	for _, input := range [][3]string{
		{"sender@example.eu\r\nBcc: attacker@example.eu", "to@example.eu", "Subject"},
		{"sender@example.eu", "to@example.eu\nBcc: attacker@example.eu", "Subject"},
		{"sender@example.eu", "to@example.eu", "Subject\r\nBcc: attacker@example.eu"},
		{"sender@example.eu", "to@example.eu", "Subject\x00hidden"},
		{"sender@example.eu", "one@example.eu, two@example.eu", "Subject"},
		{"not an address", "to@example.eu", "Subject"},
	} {
		if _, err := mailMessage(input[0], input[1], input[2], "body"); err == nil {
			t.Fatal("unsafe mail headers were accepted")
		}
	}
}

func TestAlertMailMIMEProtectsBodyAndPreservesContent(t *testing.T) {
	body := "Proyecto: prueba\r\n\r\nBcc: attacker@example.eu\r\n.\r\nDATA\r\n<script>alert(1)</script>\n" + strings.Repeat("á", 2000)
	message, err := mailMessage("Sytue <sender@example.eu>", "Ana <to@example.eu>", "Alerta · caída", body)
	if err != nil {
		t.Fatal(err)
	}
	if message.from != "sender@example.eu" || message.to != "to@example.eu" {
		t.Fatal("invalid SMTP envelope")
	}
	parsed, err := mail.ReadMessage(strings.NewReader(message.content))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("Bcc") != "" || parsed.Header.Get("Content-Transfer-Encoding") != "base64" || parsed.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatal("unexpected MIME headers")
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != "Alerta · caída" {
		t.Fatalf("subject round trip: %q, %v", subject, err)
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, parsed.Body))
	if err != nil || string(decoded) != body {
		t.Fatal("body did not round trip")
	}
	encoded := strings.SplitN(message.content, "\r\n\r\n", 2)[1]
	for _, line := range strings.Split(encoded, "\r\n") {
		if len(line) > 76 {
			t.Fatal("MIME line too long")
		}
	}
}
