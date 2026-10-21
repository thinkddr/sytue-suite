// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"errors"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func baseMsg() domain.Message {
	return domain.Message{
		UID:        10,
		Subject:    "Factura mensual",
		FromAddr:   "ada@sytue.eu",
		FromName:   "Ada",
		To:         []string{"bob@example.com"},
		Cc:         []string{"carol@example.com"},
		Snippet:    "Adjunto la factura",
		MessageID:  "mid@sytue.eu",
		Flags:      []string{"\\Seen"},
		SizeBytes:  1000,
		ReceivedAt: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		SentAt:     time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC),
	}
}

func TestMatchCriteriaNil(t *testing.T) {
	if !matchCriteria(baseMsg(), 1, nil) {
		t.Fatal("criterio nil debe casar siempre")
	}
}

func TestMatchCriteriaSeqAndUID(t *testing.T) {
	m := baseMsg()
	// seq incluido.
	c := &imap.SearchCriteria{SeqNum: []imap.SeqSet{imap.SeqSetNum(5)}}
	if !matchCriteria(m, 5, c) {
		t.Error("seq 5 debe casar")
	}
	if matchCriteria(m, 6, c) {
		t.Error("seq 6 no debe casar")
	}
	// UID.
	c = &imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(imap.UID(10))}}
	if !matchCriteria(m, 1, c) {
		t.Error("uid 10 debe casar")
	}
	c = &imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(imap.UID(99))}}
	if matchCriteria(m, 1, c) {
		t.Error("uid 99 no debe casar")
	}
}

func TestMatchCriteriaDates(t *testing.T) {
	m := baseMsg()
	// SINCE anterior a la recepción -> casa.
	if !matchCriteria(m, 1, &imap.SearchCriteria{Since: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}) {
		t.Error("SINCE debe casar")
	}
	// SINCE posterior -> no casa.
	if matchCriteria(m, 1, &imap.SearchCriteria{Since: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}) {
		t.Error("SINCE posterior no debe casar")
	}
	// BEFORE posterior -> casa.
	if !matchCriteria(m, 1, &imap.SearchCriteria{Before: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}) {
		t.Error("BEFORE debe casar")
	}
	// SENTSINCE / SENTBEFORE.
	if !matchCriteria(m, 1, &imap.SearchCriteria{SentSince: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}) {
		t.Error("SENTSINCE debe casar")
	}
	if matchCriteria(m, 1, &imap.SearchCriteria{SentBefore: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}) {
		t.Error("SENTBEFORE anterior no debe casar")
	}
}

func TestMatchCriteriaBodyTextFlagsSize(t *testing.T) {
	m := baseMsg()
	if !matchCriteria(m, 1, &imap.SearchCriteria{Body: []string{"factura"}}) {
		t.Error("BODY factura debe casar")
	}
	if matchCriteria(m, 1, &imap.SearchCriteria{Text: []string{"inexistente"}}) {
		t.Error("TEXT inexistente no debe casar")
	}
	if !matchCriteria(m, 1, &imap.SearchCriteria{Flag: []imap.Flag{imap.FlagSeen}}) {
		t.Error("FLAG \\Seen debe casar")
	}
	if matchCriteria(m, 1, &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}) {
		t.Error("NOT \\Seen no debe casar (el mensaje está visto)")
	}
	if matchCriteria(m, 1, &imap.SearchCriteria{Larger: 2000}) {
		t.Error("LARGER 2000 no debe casar (mensaje de 1000)")
	}
	if !matchCriteria(m, 1, &imap.SearchCriteria{Larger: 500}) {
		t.Error("LARGER 500 debe casar")
	}
	if matchCriteria(m, 1, &imap.SearchCriteria{Smaller: 500}) {
		t.Error("SMALLER 500 no debe casar")
	}
}

func TestMatchCriteriaNotAndOr(t *testing.T) {
	m := baseMsg()
	// NOT sobre un subcriterio que casa -> el conjunto no casa.
	c := &imap.SearchCriteria{Not: []imap.SearchCriteria{{Body: []string{"factura"}}}}
	if matchCriteria(m, 1, c) {
		t.Error("NOT(BODY factura) no debe casar")
	}
	// OR con una rama que casa.
	c = &imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{
		{Body: []string{"inexistente"}},
		{Body: []string{"factura"}},
	}}}
	if !matchCriteria(m, 1, c) {
		t.Error("OR con una rama verdadera debe casar")
	}
}

func TestMatchHeader(t *testing.T) {
	m := baseMsg()
	cases := []struct {
		key, val string
		want     bool
	}{
		{"from", "ada", true},
		{"from", "zzz", false},
		{"subject", "factura", true},
		{"to", "bob", true},
		{"cc", "carol", true},
		{"message-id", "mid", true},
		{"subject", "", true},    // valor vacío casa si el campo no está vacío
		{"x-custom", "", true},   // cabecera no indexada con valor vacío
		{"x-custom", "v", false}, // cabecera no indexada con valor no vacío
	}
	for _, c := range cases {
		if got := matchHeader(m, c.key, c.val); got != c.want {
			t.Errorf("matchHeader(%q,%q) = %v, quiero %v", c.key, c.val, got, c.want)
		}
	}
}

func TestSplitAtAndNameAddr(t *testing.T) {
	local, host := splitAt("ada@sytue.eu")
	if local != "ada" || host != "sytue.eu" {
		t.Errorf("splitAt = %q/%q", local, host)
	}
	// Sin @ -> todo local, host vacío.
	local, host = splitAt("noat")
	if local != "noat" || host != "" {
		t.Errorf("splitAt sin @ = %q/%q", local, host)
	}
	name, addr := splitNameAddr("Ada <ada@sytue.eu>")
	if name != "Ada" || addr != "ada@sytue.eu" {
		t.Errorf("splitNameAddr = %q/%q", name, addr)
	}
	name, addr = splitNameAddr("plain@x.eu")
	if name != "" || addr != "plain@x.eu" {
		t.Errorf("splitNameAddr plano = %q/%q", name, addr)
	}
}

func TestOneAddr(t *testing.T) {
	if oneAddr("", "Ada") != "" {
		t.Error("addr vacío debe dar vacío")
	}
	if oneAddr("ada@x.eu", "") != "ada@x.eu" {
		t.Error("sin nombre devuelve la dirección")
	}
	if oneAddr("ada@x.eu", "Ada") != "Ada <ada@x.eu>" {
		t.Error("con nombre compone name <addr>")
	}
}

func TestAddrsSkipsInvalid(t *testing.T) {
	// Entrada sin parte local se descarta; lista resultante puede quedar nil.
	if out := addrs("", "@nolocal"); out != nil {
		t.Errorf("direcciones inválidas deben descartarse, got %+v", out)
	}
	out := addrs("Ada <ada@sytue.eu>", "bob@example.com")
	if len(out) != 2 || out[0].Mailbox != "ada" || out[1].Host != "example.com" {
		t.Errorf("addrs válidas = %+v", out)
	}
}

func TestMapIMAPErr(t *testing.T) {
	cases := []error{
		domain.ErrNotFound,
		domain.ErrMailboxExists,
		domain.ErrSpecialMailbox,
		domain.ErrMailboxNotEmpty,
		domain.ErrInvalidAddress,
	}
	for _, in := range cases {
		got := mapIMAPErr(in)
		var ie *imap.Error
		if !errors.As(got, &ie) {
			t.Errorf("mapIMAPErr(%v) debe ser *imap.Error, got %T", in, got)
		}
	}
	// Error desconocido se devuelve tal cual.
	base := errors.New("x")
	if !errors.Is(mapIMAPErr(base), base) {
		t.Error("error desconocido debe pasar sin traducir")
	}
}
