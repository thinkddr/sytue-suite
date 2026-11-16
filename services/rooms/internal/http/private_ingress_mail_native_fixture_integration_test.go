// SPDX-License-Identifier: Apache-2.0
//go:build integration

package chathttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/mailtest"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	backendmatrix "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// This source is explicitly a retained SQL fixture, not Mail's production SQL
// adapter. Authority, queue, routing/freeze, SDK, MIME execution and ACK are real.
type nativeMailSQLFixture struct {
	mu                     sync.Mutex
	state                  storage.AppMigrationState
	snapshot               storagetest.MailSnapshot
	inbox                  string
	retired                bool
	opens                  int
	handlers               *Handlers
	legacyUser, legacyRoom string
	outbound               *mailtest.Relay
	serviceConfig          config.Config
	nativeService          *service.Service
}

func prepareNativeMailSQLFixture(ctx context.Context, t *testing.T, st *store.Store, origin *backendmatrix.Client, admin *mautrix.Client, endpoint string, scope storage.Scope, snapshot *storagetest.MailSnapshot) (*nativeMailSQLFixture, error) {
	t.Helper()
	rooms, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, scope.Subject, scope.Product, storage.LegacyIdentity, func(ctx context.Context, room string) (string, error) {
		return origin.EnsureStorageRoom(ctx, scope.Tenant, scope.Subject, scope.Product, room)
	})
	if err != nil {
		return nil, err
	}
	user, err := origin.UserID(scope.Subject)
	if err != nil {
		return nil, err
	}
	_, err = admin.MakeRequest(ctx, http.MethodPut, endpoint+"/_synapse/admin/v2/users/"+url.PathEscape(user), map[string]any{"password": "artificial-disposable-ingress-password", "logout_devices": false}, nil)
	if err != nil {
		return nil, err
	}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			UserID string `json:"user_id"`
		}
		if r.URL.Path != "/internal/people/orgs" || r.Header.Get("Authorization") != "Bearer artificial-native-mail-identity" || json.NewDecoder(r.Body).Decode(&request) != nil || request.UserID != scope.Subject {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"orgs": []string{scope.Tenant}, "personal": false})
	}))
	t.Cleanup(identity.Close)
	_, serverName, found := strings.Cut(admin.UserID.String(), ":")
	if !found {
		return nil, storage.ErrRecord
	}
	serviceConfig := config.Config{MatrixInternalURL: endpoint, MatrixHomeserverURL: "https://" + serverName, MatrixServerName: serverName, MatrixSSOProviderID: "oidc-sytue", MatrixAdminToken: admin.AccessToken, IDURL: identity.URL, MatrixIdentitySecret: "artificial-native-mail-identity"}
	if os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1" {
		serviceConfig.MatrixPublicationSecret = mailBrowserPublicationSecret
		serviceConfig.MatrixPublicationRequireMarker = os.Getenv("SYTUE_MAIL_NATIVE_STRICT_PROOF") == "1"
	}
	svc := service.New(serviceConfig, st, nil, nil)
	body := []byte(`{"id":"inbox","name":"Inbox","role":"inbox","parent_id":null}`)
	digest := sha256.Sum256(body)
	result := &nativeMailSQLFixture{state: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}, MaxBodyBytes: 2 << 20, PrivateIngressReady: true}, handlers: &Handlers{svc: svc}, serviceConfig: serviceConfig, nativeService: svc, legacyUser: user, legacyRoom: rooms[0], inbox: "inbox", snapshot: storagetest.MailSnapshot{Rows: []storagetest.MailSnapshotRow{{Descriptor: storage.StreamSourceRow{Kind: "mailbox", ID: "inbox", Size: int64(len(body)), SHA256: hex.EncodeToString(digest[:])}, Body: body}}}}
	if snapshot != nil {
		result.snapshot = *snapshot
		result.state = snapshot.State
		result.inbox = snapshot.InboxID
	}
	return result, nil
}

func (f *nativeMailSQLFixture) mount(api chi.Router) {
	if f.outbound != nil {
		f.outbound.Mount(api)
	}
	api.Get("/matrix/storage/{product}/migration", f.handlers.MatrixStorageMigration)
	api.Post("/matrix/storage/{product}/migration", f.handlers.BeginMatrixStorageMigration)
	api.Post("/matrix/storage/{product}/migration/complete", f.handlers.CompleteMatrixStorageMigration)
	api.Get("/private-storage/migration", f.capabilities)
	api.Post("/private-storage/migration", f.begin)
	api.Get("/private-storage/migration/{epoch}/rows", f.rows)
	api.Get("/private-storage/migration/{epoch}/source/{kind}/{id}", f.open)
}

func (f *nativeMailSQLFixture) scope(w http.ResponseWriter, r *http.Request) bool {
	if err := storage.ValidateRequestScope(r.Header, f.state.Scope); err != nil {
		httpx.Fail(w, r, httpx.ErrConflict)
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	return true
}

func (f *nativeMailSQLFixture) capabilities(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scope(w, r) {
		return
	}
	httpx.JSON(w, r, http.StatusOK, f.state)
}

func (f *nativeMailSQLFixture) begin(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scope(w, r) {
		return
	}
	var input struct {
		Epoch string `json:"epoch"`
	}
	if httpx.DecodeJSONLimit(w, r, &input, 4096) != nil || !storage.MigrationEpoch(input.Epoch) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if f.state.Mode == "legacy" {
		now := time.Now().UTC()
		f.state.Mode = "frozen"
		f.state.Epoch = input.Epoch
		f.state.StartedAt = &now
		f.state.Rows = 1
	}
	if f.state.Epoch != input.Epoch {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	httpx.JSON(w, r, http.StatusOK, f.state.LegacyMigrationState)
}

func (f *nativeMailSQLFixture) rows(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scope(w, r) {
		return
	}
	if f.retired || chi.URLParam(r, "epoch") != f.state.Epoch {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	snapshot := f.snapshot
	snapshot.State = f.state
	page, err := snapshot.Page(after, 2)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	httpx.JSON(w, r, http.StatusOK, page)
}

func (f *nativeMailSQLFixture) open(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scope(w, r) {
		return
	}
	row, found := f.snapshot.Body(chi.URLParam(r, "kind"), chi.URLParam(r, "id"))
	if f.retired || chi.URLParam(r, "epoch") != f.state.Epoch || !found {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	f.opens++
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(row.Descriptor.Size, 10))
	w.Header().Set("X-Sytue-Source-SHA256", row.Descriptor.SHA256)
	_, _ = w.Write(row.Body)
}

func (f *nativeMailSQLFixture) extend(vector matrixprooftest.IngressDeviceVector) matrixprooftest.IngressDeviceVector {
	vector.LegacyUserID = f.legacyUser
	vector.LegacyRoomID = f.legacyRoom
	return vector
}
