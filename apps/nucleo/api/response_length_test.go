// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateOriginalStreamsWithoutContentLengthAndRejectsActualLengthMismatch(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	for _, body := range []string{"abc", "ab", "abcd"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pin, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || pin != scope {
					t.Error("wrong scope", err)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.(http.Flusher).Flush()
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := NewDrive(server.URL, nil, server.Client())
			source, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, "file", "source", 3)
			if err != nil {
				t.Fatal("chunked original rejected before streaming", err)
			}
			raw, err := io.ReadAll(source)
			_ = source.Close()
			if body == "abc" {
				if err != nil || string(raw) != body {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrResponseLength) {
				t.Fatal("wrong source length accepted", body, err)
			}
		})
	}
}
