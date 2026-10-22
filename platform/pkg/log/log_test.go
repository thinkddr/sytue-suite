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

func TestNewAndLevels(t *testing.T) {
	if New(Options{Format: "json", Service: "sytue-id"}) == nil {
		t.Fatal("logger nil")
	}
	if parseLevel("debug") != slog.LevelDebug || parseLevel("bogus") != slog.LevelInfo {
		t.Fatal("parseLevel incorrecto")
	}
}

func TestContextRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := Into(context.Background(), l)

	ctx = With(ctx, "tenant", "acme")
	From(ctx).Info("hola")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["tenant"] != "acme" || m["msg"] != "hola" {
		t.Fatalf("atributos no propagados: %v", m)
	}
}

func TestFromDefaults(t *testing.T) {
	if From(context.Background()) == nil {
		t.Fatal("From debe devolver default, no nil")
	}
}
