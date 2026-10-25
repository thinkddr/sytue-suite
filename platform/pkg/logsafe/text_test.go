// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package logsafe

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestTextEscapesUntrustedLineBreaks(t *testing.T) {
	for _, input := range []any{"request\r\nforged-entry", errors.New("request\r\nforged-entry")} {
		if got := Text(input); got != `request\r\nforged-entry` {
			t.Fatalf("escaped text = %q", got)
		}
	}
	if Text("ordinary value") != "ordinary value" {
		t.Fatal("plain text changed")
	}
}

func TestStructuredLogRetainsFieldsAndOneRecord(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	logger.Info("request", "error", Text(errors.New("bad\n{\"level\":\"INFO\"}")), "status", 400)
	if strings.Count(buffer.String(), "\n") != 1 {
		t.Fatal("input injected a log record")
	}
	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != float64(400) || record["error"] != `bad\n{"level":"INFO"}` {
		t.Fatalf("unexpected fields: %#v", record)
	}
}
