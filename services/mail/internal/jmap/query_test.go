// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios de la lógica pura de la API JMAP (parseo de filtros, claves de
// blob, mapeo de keywords) — sin BD/red.
package jmap

import (
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestParseEmailQueryFilter(t *testing.T) {
	mbID := uuid.New()
	raw := rawArgs(`{
		"filter": {"inMailbox": "` + mbID.String() + `", "text": "factura", "from": "ada", "hasKeyword": "$flagged", "notKeyword": "$seen"},
		"limit": 25,
		"position": 10
	}`)
	f := parseEmailQueryFilter(raw)
	if f.MailboxID != mbID {
		t.Errorf("MailboxID = %v, quiero %v", f.MailboxID, mbID)
	}
	if f.Text != "factura" || f.From != "ada" {
		t.Errorf("Text/From = %q/%q", f.Text, f.From)
	}
	if !f.Flagged {
		t.Error("hasKeyword=$flagged debe activar Flagged")
	}
	if !f.Unseen {
		t.Error("notKeyword=$seen debe activar Unseen")
	}
	if f.Limit != 25 || f.Offset != 10 {
		t.Errorf("Limit/Offset = %d/%d", f.Limit, f.Offset)
	}
}

func TestParseEmailQueryFilterHasAttachment(t *testing.T) {
	f := parseEmailQueryFilter(rawArgs(`{"filter":{"subject":"factura","hasAttachment":true}}`))
	if !f.HasAttachment {
		t.Error("hasAttachment=true debe activar HasAttachment en el filtro del store")
	}
	if f.Subject != "factura" {
		t.Errorf("Subject = %q", f.Subject)
	}
	// Ausente ⇒ false (no restringe).
	if parseEmailQueryFilter(rawArgs(`{"filter":{"text":"x"}}`)).HasAttachment {
		t.Error("sin hasAttachment no debe restringir por adjunto")
	}
}

func TestParseEmailQueryFilterDefaults(t *testing.T) {
	f := parseEmailQueryFilter(nil)
	if f.Limit != 50 {
		t.Errorf("límite por defecto = %d, quiero 50", f.Limit)
	}
	if f.Offset != 0 {
		t.Errorf("offset por defecto = %d", f.Offset)
	}
}

func TestParseEmailQueryFilterClampsLimit(t *testing.T) {
	f := parseEmailQueryFilter(rawArgs(`{"limit": 100000}`))
	if f.Limit != maxQueryLimit {
		t.Errorf("límite excesivo debe recortarse a %d, got %d", maxQueryLimit, f.Limit)
	}
	f = parseEmailQueryFilter(rawArgs(`{"limit": -5}`))
	if f.Limit != maxQueryLimit {
		t.Errorf("límite negativo => %d, quiero %d", f.Limit, maxQueryLimit)
	}
	f = parseEmailQueryFilter(rawArgs(`{"position": -3}`))
	if f.Offset != 0 {
		t.Errorf("position negativa debe normalizarse a 0, got %d", f.Offset)
	}
}

func TestBlobKey(t *testing.T) {
	valid := []string{
		"blobs/" + uuid.NewString() + "/" + uuid.NewString(),
		"blobs/a/b",
	}
	for _, v := range valid {
		if k, ok := blobKey(v); !ok || k != v {
			t.Errorf("blobKey(%q) = %q,%v; quiero válido", v, k, ok)
		}
	}
	invalid := []string{
		"",
		"../etc/passwd",
		"blobs/../secrets/x",
		"messages/x/y", // prefijo no permitido en subida genérica
		"/absolute/path",
		"nope/blobs/x",
	}
	for _, v := range invalid {
		if _, ok := blobKey(v); ok {
			t.Errorf("blobKey(%q) debería rechazarse", v)
		}
	}
}

func TestKeywordToFlag(t *testing.T) {
	cases := map[string]string{
		"$seen":     domain.FlagSeen,
		"$flagged":  domain.FlagFlagged,
		"$draft":    domain.FlagDraft,
		"$answered": domain.FlagAnswered,
		"$deleted":  domain.FlagDeleted,
		"custom":    "custom",
	}
	for kw, want := range cases {
		if got := keywordToFlag(kw); got != want {
			t.Errorf("keywordToFlag(%q) = %q, quiero %q", kw, got, want)
		}
	}
}

func TestSetError(t *testing.T) {
	if setError(domain.ErrNotFound)["type"] != "notFound" {
		t.Error("ErrNotFound => notFound")
	}
	if setError(domain.ErrMailboxExists)["type"] != "alreadyExists" {
		t.Error("ErrMailboxExists => alreadyExists")
	}
	if setError(domain.ErrSpecialMailbox)["type"] != "forbidden" {
		t.Error("ErrSpecialMailbox => forbidden")
	}
}
