// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"testing"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestToIMAPEnvelope(t *testing.T) {
	m := domain.Message{
		Subject:   "Hola",
		FromAddr:  "ada@sytue.eu",
		FromName:  "Ada Lovelace",
		To:        []string{"bob@example.com", "Carol <carol@example.com>"},
		Cc:        []string{"dave@example.com"},
		MessageID: "abc@sytue.eu",
		InReplyTo: "parent@sytue.eu",
	}
	env := toIMAPEnvelope(m)
	if env.Subject != "Hola" {
		t.Errorf("Subject = %q", env.Subject)
	}
	if len(env.From) != 1 || env.From[0].Mailbox != "ada" || env.From[0].Host != "sytue.eu" || env.From[0].Name != "Ada Lovelace" {
		t.Errorf("From = %+v", env.From)
	}
	if len(env.To) != 2 || env.To[1].Mailbox != "carol" || env.To[1].Name != "Carol" {
		t.Errorf("To = %+v", env.To)
	}
	if len(env.Cc) != 1 || env.Cc[0].Host != "example.com" {
		t.Errorf("Cc = %+v", env.Cc)
	}
	if env.MessageID != "abc@sytue.eu" {
		t.Errorf("MessageID = %q", env.MessageID)
	}
	if len(env.InReplyTo) != 1 || env.InReplyTo[0] != "parent@sytue.eu" {
		t.Errorf("InReplyTo = %v", env.InReplyTo)
	}
}

func TestSplitNameAddr(t *testing.T) {
	name, addr := splitNameAddr("Ada Lovelace <ada@sytue.eu>")
	if name != "Ada Lovelace" || addr != "ada@sytue.eu" {
		t.Fatalf("splitNameAddr = %q,%q", name, addr)
	}
	name, addr = splitNameAddr("bob@example.com")
	if name != "" || addr != "bob@example.com" {
		t.Fatalf("splitNameAddr sin nombre = %q,%q", name, addr)
	}
}

func TestNormalizeRange(t *testing.T) {
	if lo, hi := normalizeRange(2, 0, 9); lo != 2 || hi != 9 {
		t.Fatalf("2:* con star=9 => %d:%d", lo, hi)
	}
	if lo, hi := normalizeRange(5, 3, 9); lo != 3 || hi != 5 {
		t.Fatalf("rango invertido 5:3 => %d:%d", lo, hi)
	}
	if lo, hi := normalizeRange(0, 0, 7); lo != 7 || hi != 7 {
		t.Fatalf("*:* con star=7 => %d:%d", lo, hi)
	}
}
