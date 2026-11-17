// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

var mailHTTPSourceScope = storage.Scope{Tenant: "acme", Subject: "user-1", Product: "mail"}

const mailHTTPSourceEpoch = "12345678-1234-1234-1234-123456789abc"

type privateSourceFixture struct {
	calls, begins int
	shared        bool
	otherMessage  bool
	state         storage.LegacyMigrationState
}

func (f *privateSourceFixture) PrivateStorageOwnsMessage(context.Context, string, uuid.UUID) (bool, error) {
	return !f.otherMessage, nil
}

func (f *privateSourceFixture) PrivateStorageShared(context.Context, string) (bool, error) {
	return f.shared, nil
}

func (f *privateSourceFixture) PrivateStorageState(context.Context, string) (storage.LegacyMigrationState, error) {
	f.calls++
	return f.state, nil
}

func (f *privateSourceFixture) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID, open func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error) {
	f.calls++
	f.begins++
	if owner != "user-1" {
		return f.state, store.ErrPrivateStorageEpoch
	}
	body, err := open(ctx, "internal-only-key")
	if err != nil {
		return f.state, err
	}
	_, err = io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return f.state, err
	}
	now := time.Now()
	f.state.Mode = "frozen"
	f.state.Epoch = epoch.String()
	f.state.StartedAt = &now
	return f.state, nil
}

func (f *privateSourceFixture) PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error) {
	f.calls++
	return storage.StreamSourcePage{Scope: mailHTTPSourceScope, Epoch: mailHTTPSourceEpoch, Rows: []storage.StreamSourceRow{}}, nil
}

func (f *privateSourceFixture) PrivateStorageSourceBody(context.Context, string, uuid.UUID, string, string) (store.PrivateSourceBody, error) {
	f.calls++
	return store.PrivateSourceBody{Descriptor: storage.StreamSourceRow{Kind: "message", ID: "msg", Size: 2, SHA256: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"}, Payload: []byte(`{}`)}, nil
}

type privateIngressFixture struct {
	prepares int
	ready    bool
}

func (f *privateIngressFixture) PreparePrivateIngress(context.Context, storage.Scope) error {
	f.prepares++
	return nil
}

func (f *privateIngressFixture) PrivateIngressReady(context.Context, storage.Scope) (bool, error) {
	return f.ready, nil
}

type privateBlobsFixture struct{ gets int }

func (f *privateBlobsFixture) Put(context.Context, tenancy.Tenant, string, io.Reader, int64, objectstore.PutOptions) error {
	return nil
}

func (f *privateBlobsFixture) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	f.gets++
	return io.NopCloser(bytes.NewReader([]byte("retained MIME"))), nil
}
func (f *privateBlobsFixture) Delete(context.Context, tenancy.Tenant, string) error { return nil }
func sourceRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	r := req(method, path, body, true, nil)
	header, err := storage.EncodeScopeHeader(mailHTTPSourceScope)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(storage.ScopeHeader, header)
	return r
}

func TestMailSourceCapabilitiesDoNotClaimPrivateIngressByFreezingHistory(t *testing.T) {
	source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "legacy"}}
	api := &API{privateSource: source, cfg: config.Config{MaxMessageBytes: 100 << 20}}
	response := httptest.NewRecorder()
	api.privateStorageState(response, sourceRequest(t, http.MethodGet, "/api/v1/private-storage/migration", ""))
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	var state storage.AppMigrationState
	if json.Unmarshal(response.Body.Bytes(), &state) != nil || state.Validate(mailHTTPSourceScope) != nil || state.PrivateIngressReady || state.MaxBodyBytes != 100<<20 {
		t.Fatal("lost authenticated capabilities", state)
	}
	blocked := httptest.NewRecorder()
	api.beginPrivateStorage(blocked, sourceRequest(t, http.MethodPost, "/api/v1/private-storage/migration", `{"epoch":"`+mailHTTPSourceEpoch+`"}`))
	if blocked.Code != http.StatusServiceUnavailable || source.begins != 0 {
		t.Fatal("froze account without a private ingress capability", blocked.Code, source.begins)
	}
}

func TestMailSourceScopeAndCollaborationFailBeforeAnyMigrationEffect(t *testing.T) {
	for _, fault := range []string{"missing", "duplicate", "other-account", "shared"} {
		t.Run(fault, func(t *testing.T) {
			source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "legacy"}}
			ingress := &privateIngressFixture{ready: true}
			blobs := &privateBlobsFixture{}
			api := &API{privateSource: source, privateIngress: ingress, blobs: blobs}
			request := sourceRequest(t, http.MethodPost, "/api/v1/private-storage/migration", `{"epoch":"`+mailHTTPSourceEpoch+`"}`)
			switch fault {
			case "missing":
				request.Header.Del(storage.ScopeHeader)
			case "duplicate":
				request.Header.Add(storage.ScopeHeader, request.Header.Get(storage.ScopeHeader))
			case "other-account":
				other := mailHTTPSourceScope
				other.Subject = "other"
				header, _ := storage.EncodeScopeHeader(other)
				request.Header.Set(storage.ScopeHeader, header)
			case "shared":
				source.shared = true
			}
			response := httptest.NewRecorder()
			api.beginPrivateStorage(response, request)
			if response.Code < 400 || source.begins != 0 || ingress.prepares != 0 || blobs.gets != 0 {
				t.Fatal("invalid source scope had effects", response.Code, source.begins, ingress.prepares, blobs.gets)
			}
		})
	}
}

func TestMailSourceBeginRequiresExplicitVerifiedIngressAndRetainsOriginal(t *testing.T) {
	source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "legacy", Rows: 1}}
	ingress := &privateIngressFixture{ready: true}
	blobs := &privateBlobsFixture{}
	api := &API{privateSource: source, privateIngress: ingress, blobs: blobs, cfg: config.Config{MaxMessageBytes: 50 << 20}}
	response := httptest.NewRecorder()
	api.beginPrivateStorage(response, sourceRequest(t, http.MethodPost, "/api/v1/private-storage/migration", `{"epoch":"`+mailHTTPSourceEpoch+`"}`))
	if response.Code != http.StatusOK || source.begins != 1 || ingress.prepares != 1 || blobs.gets != 1 {
		t.Fatal("explicit migration did not use bridge and retained source", response.Code, response.Body.String())
	}
	var state storage.AppMigrationState
	if json.Unmarshal(response.Body.Bytes(), &state) != nil || state.Validate(mailHTTPSourceScope) != nil || !state.PrivateIngressReady || state.Mode != "frozen" {
		t.Fatal("bad frozen capability", state)
	}
}

func TestMailLegacyContentGateDoesNotApplyToAnotherAccount(t *testing.T) {
	for _, path := range []string{"/api/v1/messages/" + mailHTTPSourceEpoch + "/raw", "/api/v1/mailboxes", "/jmap", "/api/v1/export", "/api/v1/shared-mailboxes/other/mailboxes", "/api/v1/admin/audit-log", "/api/v1/quota", "/api/v1/private-storage/migration"} {
		t.Run(path, func(t *testing.T) {
			source := &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "frozen"}}
			api := &API{privateSource: source}
			called := false
			handler := api.guardLegacyPrivateMail(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, sourceRequest(t, http.MethodGet, path, ""))
			if legacyMailControlPath(path) {
				if !called || response.Code != http.StatusNoContent {
					t.Fatal("personal freeze blocked other scope/control", response.Code)
				}
			} else if called || response.Code != http.StatusConflict {
				t.Fatal("legacy personal content escaped private gate", response.Code)
			}
		})
	}
}

func TestMailFrozenPersonalGateLeavesDelegatedMessageACLToItsExistingHandler(t *testing.T) {
	source := &privateSourceFixture{otherMessage: true, state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "frozen"}}
	api := &API{privateSource: source}
	called := false
	handler := api.guardLegacyPrivateMail(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNotFound) }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, sourceRequest(t, http.MethodGet, "/api/v1/messages/"+uuid.NewString()+"/raw", ""))
	if !called || response.Code != http.StatusNotFound {
		t.Fatal("personal gate replaced the other mailbox's ACL result", response.Code)
	}
}
