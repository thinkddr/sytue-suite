// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestCalendarSourcePreservesScopeCapabilitiesAndOriginalBytes(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	now := time.Now().UTC()
	data := []byte(`{"future_integer":9007199254740993,"original":"` + strings.Repeat("ICS 漢字 / ", 5000) + `"}`)
	sum := sha256.Sum256(data)
	row := storage.StreamSourceRow{Kind: "object", ID: "12345678-1234-1234-1234-123456789abc", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope {
			t.Error("source account scope changed", err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/rows"):
			_ = json.NewEncoder(w).Encode(storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{row}})
		case strings.Contains(r.URL.Path, "/source/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("X-Sytue-Source-SHA256", row.SHA256)
			_, _ = w.Write(data)
		default:
			_ = json.NewEncoder(w).Encode(storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 1}, Shared: false, MaxBodyBytes: model.MaxRowBytes, PrivateIngressReady: false})
		}
	}))
	defer server.Close()
	client := NewCalendars(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	state, err := client.PrivateStorageState(t.Context(), scope)
	if err != nil || state.MaxBodyBytes != model.MaxRowBytes || state.PrivateIngressReady || state.Shared {
		t.Fatal("historical source became private ingress readiness", state, err)
	}
	page, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) != 1 || page.Rows[0] != row {
		t.Fatal("source descriptors changed", err)
	}
	body, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("original Calendar JSON changed", err)
	}
	bad := scope
	bad.Product = "contacts"
	if _, err := client.PrivateStorageState(t.Context(), bad); err == nil {
		t.Fatal("foreign product reached Calendar")
	}
	if _, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{Kind: "mime_message", ID: "foreign"}); err == nil {
		t.Fatal("foreign source kind reached Calendar")
	}
	if _, err := client.BeginPrivateStorageMigration(t.Context(), scope, ""); err == nil {
		t.Fatal("empty Begin became an ordinary state read")
	}
	if calls.Load() != 3 {
		t.Fatal("invalid source routing performed HTTP", calls.Load())
	}
}

func TestCalendarSourceRejectsOversizeAndChangedCommitment(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	var calls atomic.Int64
	row := storage.StreamSourceRow{Kind: "event", ID: "original", Size: 2, SHA256: strings.Repeat("a", 64)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Path, "/rows") {
			invalid := row
			invalid.Size = model.MaxRowBytes + 1
			_ = json.NewEncoder(w).Encode(storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{invalid}})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "2")
		w.Header().Set("X-Sytue-Source-SHA256", strings.Repeat("b", 64))
		_, _ = io.WriteString(w, "{}")
	}))
	defer server.Close()
	client := NewCalendars(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	if _, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{}); err == nil {
		t.Fatal("oversized Calendar source accepted")
	}
	if _, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row); err == nil {
		t.Fatal("changed source commitment accepted")
	}
	row.Size = model.MaxRowBytes + 1
	if _, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row); err == nil {
		t.Fatal("oversized request opened source")
	}
	row.Size = 2
	row.Kind = "mime_message"
	if _, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row); err == nil {
		t.Fatal("Calendar opened Mail source")
	}
	if calls.Load() != 2 {
		t.Fatal("rejected source descriptor still performed HTTP", calls.Load())
	}
}

func TestCalendarSourceBeginPreservesExplicitIngressRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("begin did not use shared POST request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"code":"private_calendar_ingress_required","message":"Service unavailable."}}`)
	}))
	defer server.Close()
	client := NewCalendars(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	_, err := client.BeginPrivateStorageMigration(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}, "12345678-1234-1234-1234-123456789abc")
	var failure *Error
	if !errors.As(err, &failure) || failure.Status != http.StatusServiceUnavailable || failure.Code() != "private_calendar_ingress_required" {
		t.Fatal("ingress rejection became a migration success or lost its policy", err)
	}
}

func TestCalendarSourceRejectsForeignSnapshotPages(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	for _, change := range []func(*storage.StreamSourcePage){
		func(p *storage.StreamSourcePage) { p.Scope.Subject = "bob" },
		func(p *storage.StreamSourcePage) { p.Epoch = "22345678-1234-1234-1234-123456789abc" },
		func(p *storage.StreamSourcePage) { p.Rows[0].Kind = "mime_message" },
	} {
		page := storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{{Kind: "event", ID: "original", Size: 2, SHA256: strings.Repeat("a", 64)}}}
		change(&page)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(page) }))
		client := NewCalendars(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
		_, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{})
		server.Close()
		if err == nil {
			t.Fatal("foreign source snapshot accepted", page)
		}
	}
}
