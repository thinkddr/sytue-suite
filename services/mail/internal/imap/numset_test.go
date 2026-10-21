// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios de la resolución de conjuntos seq/uid y del match de SEARCH
// (lógica pura, sin BD/red).
package imap

import (
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// msgs de prueba: UIDs no contiguos para distinguir seq (posición) de UID.
func testMsgs() []domain.Message {
	return []domain.Message{
		{UID: 10, Subject: "uno", FromAddr: "a@x.eu", Flags: []string{domain.FlagSeen}, SizeBytes: 100},
		{UID: 20, Subject: "dos", FromAddr: "b@x.eu", Flags: []string{}, SizeBytes: 200},
		{UID: 35, Subject: "tres", FromAddr: "a@x.eu", Flags: []string{domain.FlagFlagged}, SizeBytes: 300},
	}
}

func seqsOf(sel []selected) []uint32 {
	out := make([]uint32, len(sel))
	for i, s := range sel {
		out[i] = s.Seq
	}
	return out
}

func TestResolveSeqSet(t *testing.T) {
	msgs := testMsgs()

	// Secuencia 2:3 => posiciones 2 y 3.
	var set goimap.SeqSet
	set.AddRange(2, 3)
	got := seqsOf(resolveSet(msgs, set))
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("SeqSet 2:3 => %v", got)
	}

	// Comodín 2:* => posiciones 2 y 3.
	var star goimap.SeqSet
	star.AddRange(2, 0) // 0 == "*"
	got = seqsOf(resolveSet(msgs, star))
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("SeqSet 2:* => %v", got)
	}

	// Número suelto.
	got = seqsOf(resolveSet(msgs, goimap.SeqSetNum(1)))
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("SeqSetNum(1) => %v", got)
	}
}

func TestResolveUIDSet(t *testing.T) {
	msgs := testMsgs()

	// UID 20 => seq 2.
	got := resolveSet(msgs, goimap.UIDSetNum(20))
	if len(got) != 1 || got[0].Seq != 2 || got[0].Msg.UID != 20 {
		t.Fatalf("UIDSetNum(20) => %+v", got)
	}

	// Rango UID 10:20 => seqs 1,2.
	var set goimap.UIDSet
	set.AddRange(10, 20)
	sel := resolveSet(msgs, set)
	if len(sel) != 2 || sel[0].Msg.UID != 10 || sel[1].Msg.UID != 20 {
		t.Fatalf("UIDSet 10:20 => %+v", sel)
	}

	// UID *:* selecciona el mayor (35).
	var star goimap.UIDSet
	star.AddRange(0, 0)
	sel = resolveSet(msgs, star)
	if len(sel) != 1 || sel[0].Msg.UID != 35 {
		t.Fatalf("UIDSet *:* => %+v", sel)
	}
}

func TestResolveSetEmpty(t *testing.T) {
	if got := resolveSet(nil, goimap.SeqSetNum(1)); got != nil {
		t.Fatalf("resolveSet(nil) => %v", got)
	}
}

func TestMatchCriteria(t *testing.T) {
	msgs := testMsgs()

	// UNSEEN: solo el segundo (sin \Seen).
	crit := &goimap.SearchCriteria{NotFlag: []goimap.Flag{domain.FlagSeen}}
	assertMatch(t, msgs, crit, []uint32{2, 3})

	// FLAGGED: solo el tercero.
	crit = &goimap.SearchCriteria{Flag: []goimap.Flag{domain.FlagFlagged}}
	assertMatch(t, msgs, crit, []uint32{3})

	// HEADER FROM a@x.eu: primero y tercero.
	crit = &goimap.SearchCriteria{Header: []goimap.SearchCriteriaHeaderField{{Key: "From", Value: "a@x.eu"}}}
	assertMatch(t, msgs, crit, []uint32{1, 3})

	// TEXT "dos": segundo.
	crit = &goimap.SearchCriteria{Text: []string{"dos"}}
	assertMatch(t, msgs, crit, []uint32{2})

	// LARGER 150: segundo y tercero.
	crit = &goimap.SearchCriteria{Larger: 150}
	assertMatch(t, msgs, crit, []uint32{2, 3})

	// NOT (FLAGGED): primero y segundo.
	crit = &goimap.SearchCriteria{Not: []goimap.SearchCriteria{{Flag: []goimap.Flag{domain.FlagFlagged}}}}
	assertMatch(t, msgs, crit, []uint32{1, 2})

	// OR (uno, tres) por subject: primero y tercero.
	crit = &goimap.SearchCriteria{Or: [][2]goimap.SearchCriteria{{
		{Header: []goimap.SearchCriteriaHeaderField{{Key: "Subject", Value: "uno"}}},
		{Header: []goimap.SearchCriteriaHeaderField{{Key: "Subject", Value: "tres"}}},
	}}}
	assertMatch(t, msgs, crit, []uint32{1, 3})
}

func TestMatchCriteriaSince(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	msgs := []domain.Message{
		{UID: 1, ReceivedAt: base.AddDate(0, 0, -2)},
		{UID: 2, ReceivedAt: base},
	}
	crit := &goimap.SearchCriteria{Since: base.AddDate(0, 0, -1)}
	assertMatch(t, msgs, crit, []uint32{2})
}

func assertMatch(t *testing.T, msgs []domain.Message, crit *goimap.SearchCriteria, want []uint32) {
	t.Helper()
	var got []uint32
	for i, m := range msgs {
		seq := uint32(i + 1)
		if matchCriteria(m, seq, crit) {
			got = append(got, seq)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("matchCriteria => %v, quiero %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchCriteria => %v, quiero %v", got, want)
		}
	}
}
