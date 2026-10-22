// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func newRedacting(buf *bytes.Buffer) *slog.Logger {
	h := NewRedactingHandler(slog.NewJSONHandler(buf, nil), RedactOptions{})
	return slog.New(h)
}

func TestRedactionHidesSecretsAndPII(t *testing.T) {
	var buf bytes.Buffer
	l := newRedacting(&buf)
	l.Info("login",
		"password", "hunter2",
		"user_email", "ada@sytue.eu",
		"access_token", "abc.def",
		"request_id", "req-1",
	)

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["password"] != redactedPlaceholder {
		t.Errorf("password no redactado: %v", m["password"])
	}
	if m["user_email"] != redactedPlaceholder {
		t.Errorf("email no redactado: %v", m["user_email"])
	}
	if m["access_token"] != redactedPlaceholder {
		t.Errorf("token no redactado: %v", m["access_token"])
	}
	if m["request_id"] != "req-1" {
		t.Errorf("request_id no debe redactarse: %v", m["request_id"])
	}
}

func TestRedactionInGroups(t *testing.T) {
	var buf bytes.Buffer
	l := newRedacting(&buf)
	l.Info("evt", slog.Group("user", slog.String("email", "x@y.z"), slog.String("id", "u1")))

	var m map[string]any
	_ = json.Unmarshal(buf.Bytes(), &m)
	user, _ := m["user"].(map[string]any)
	if user["email"] != redactedPlaceholder {
		t.Errorf("email en grupo no redactado: %v", user["email"])
	}
	if user["id"] != "u1" {
		t.Errorf("id en grupo alterado: %v", user["id"])
	}
}

func TestNewLoggerRedactsByDefault(t *testing.T) {
	// New() debe activar la redacción por defecto (seguridad por defecto).
	var buf bytes.Buffer
	base := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buf, nil), RedactOptions{}))
	base.Info("x", "secret", "s")
	if !bytes.Contains(buf.Bytes(), []byte(redactedPlaceholder)) {
		t.Fatal("New debe redactar por defecto")
	}
}

func TestRedactionWithContext(t *testing.T) {
	var buf bytes.Buffer
	l := newRedacting(&buf)
	ctx := Into(context.Background(), l)
	From(ctx).Info("x", "client_secret", "top")
	if !bytes.Contains(buf.Bytes(), []byte(redactedPlaceholder)) {
		t.Fatal("client_secret debe redactarse")
	}
}
