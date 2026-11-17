// SPDX-License-Identifier: Apache-2.0
//go:build integration

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Fixture-only HTTP: authentication is Synapse's maintained API, never a
// production credential, password-based app connection or encryption protocol.
func cutoverPolicyHTTP(ctx context.Context, t *testing.T, token, method, path string, body, out any, want int) {
	t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:18599"+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != want {
		t.Fatalf("fixture %s %s returned status %d, expected %d", method, path, response.StatusCode, want)
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out); err != nil {
			t.Fatal(err)
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	}
}

func cutoverPolicyLogin(ctx context.Context, t *testing.T, user string) string {
	t.Helper()
	var reply struct {
		Token string `json:"access_token"`
	}
	cutoverPolicyHTTP(ctx, t, "", http.MethodPost, "/_matrix/client/v3/login", map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": user}, "password": "artificial-cutover-controller"}, &reply, http.StatusOK)
	if reply.Token == "" {
		t.Fatal("empty fixture login")
	}
	return reply.Token
}

func TestMatrixStorageCutoverPolicyAgainstSynapseAndPostgres(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_CUTOVER_POLICY") != "1" {
		t.Skip("requires disposable remote Synapse and PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	admin := cutoverPolicyLogin(ctx, t, "rooms_controller")
	controller := New(Config{InternalURL: "http://127.0.0.1:18599", HomeserverURL: "https://storage.test:18600", ServerName: "storage.test:18600", SSOProviderID: "oidc-sytue", AdminToken: admin})
	controller.SetIdentityGate(st)
	st.SetMatrixGate(controller)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "drive"}
	provision := func(ctx context.Context, room string, readOnly bool) (string, error) {
		return controller.EnsureStorageRoomPolicy(ctx, scope.Tenant, scope.Subject, scope.Product, room, readOnly)
	}
	source, err := st.EnsureMatrixStorageRoomsWithPolicy(ctx, scope.Subject, scope.Product, storage.LegacyIdentity, provision)
	if err != nil || len(source) != 1 {
		t.Fatal("source provisioning", err)
	}
	actor, err := controller.UserID(scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	cutoverPolicyHTTP(ctx, t, admin, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(actor), map[string]any{"password": "artificial-cutover-controller", "logout_devices": false}, nil, http.StatusOK)
	human := cutoverPolicyLogin(ctx, t, actor)
	cutoverPolicyHTTP(ctx, t, human, http.MethodPost, "/_matrix/client/v3/join/"+url.PathEscape(source[0]), map[string]any{}, nil, http.StatusOK)
	opaque := map[string]string{"algorithm": domain.MatrixEncryption, "ciphertext": "artificial-policy-only-opaque-event", "session_id": "policy-fixture"}
	send := func(token, room string, want int) storage.Reference {
		var reply struct {
			EventID string `json:"event_id"`
		}
		var out any
		if want == http.StatusOK {
			out = &reply
		}
		cutoverPolicyHTTP(ctx, t, token, http.MethodPut, "/_matrix/client/v3/rooms/"+url.PathEscape(room)+"/send/m.room.encrypted/"+uuid.NewString(), opaque, out, want)
		return storage.Reference{RoomID: room, EventID: reply.EventID}
	}
	send(human, source[0], http.StatusOK)
	epoch := uuid.NewString()
	if _, err := st.BeginMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch); err != nil {
		t.Fatal(err)
	}
	fence := func(ctx context.Context, room string) error {
		_, err := controller.EnsureStorageRoomPolicy(ctx, scope.Tenant, scope.Subject, scope.Product, room, true)
		return err
	}
	state, err := st.FreezeMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch, fence)
	if err != nil || state.Phase != "frozen" {
		t.Fatal("freeze policy", state, err)
	}
	send(human, source[0], http.StatusForbidden)
	if _, err := st.EnsureMatrixStorageRoomsWithPolicy(ctx, scope.Subject, scope.Product, storage.LegacyIdentity, provision); err != nil {
		t.Fatal("read-only bootstrap", err)
	}
	send(human, source[0], http.StatusForbidden)
	isolated, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, scope.Subject, scope.Product, storage.IsolatedIdentity, func(ctx context.Context, room string) (string, error) {
		return controller.EnsureIsolatedStorageRoom(ctx, scope, room)
	})
	if err != nil || len(isolated) != 1 {
		t.Fatal("isolated provisioning", err)
	}
	owner, err := controller.StorageUserID(scope)
	if err != nil {
		t.Fatal(err)
	}
	cutoverPolicyHTTP(ctx, t, admin, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(owner), map[string]any{"password": "artificial-cutover-controller", "logout_devices": false}, nil, http.StatusOK)
	token := cutoverPolicyLogin(ctx, t, owner)
	cutoverPolicyHTTP(ctx, t, token, http.MethodPost, "/_matrix/client/v3/join/"+url.PathEscape(isolated[0]), map[string]any{}, nil, http.StatusOK)
	receipt := send(token, isolated[0], http.StatusOK)
	validate := func(ctx context.Context) error {
		return controller.ValidateStorageEvent(ctx, domain.MatrixEventRef{RoomID: receipt.RoomID, EventID: receipt.EventID}, scope)
	}
	state, err = st.CompleteMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch, receipt, fence, validate)
	if err != nil || state.Phase != "isolated" {
		t.Fatal("public receipt routing/sender", state, err)
	}
	if _, err := st.EnsureMatrixStorageRoomsWithPolicy(ctx, scope.Subject, scope.Product, storage.LegacyIdentity, provision); err != nil {
		t.Fatal(err)
	}
	send(human, source[0], http.StatusForbidden)
	reopened := store.New(h.Pool, nil)
	persisted, err := reopened.MatrixStorageCutover(ctx, scope.Subject, scope.Product)
	if err != nil || persisted.Receipt == nil || *persisted.Receipt != receipt {
		t.Fatal("durable cutover receipt", persisted, err)
	}
	t.Logf("real PostgreSQL+Synapse policy: %d retained source, legacy writes403 before/after bootstrap, isolated scope sender checked and receipt persisted; payload is an opaque policy fixture, actual SDK crypto is tested separately", len(source))
}
