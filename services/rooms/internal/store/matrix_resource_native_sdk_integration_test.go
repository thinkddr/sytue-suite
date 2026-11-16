//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	native "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	directory "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type nativeResourceTestSecrets struct {
	*authtest.Secrets
	missingLeaf atomic.Bool
	leafWrites  atomic.Int64
}

func (s *nativeResourceTestSecrets) Read(key string) ([]byte, error) {
	if s.missingLeaf.Load() && strings.HasPrefix(key, "matrix-resource-device-leaf-") {
		return nil, nil
	}
	return s.Secrets.Read(key)
}

func (s *nativeResourceTestSecrets) Write(key string, value []byte) error {
	if strings.HasPrefix(key, "matrix-resource-device-leaf-") {
		s.leafWrites.Add(1)
	}
	return s.Secrets.Write(key, value)
}

type nativeResourceActorWitness struct{}

func (nativeResourceActorWitness) AccessToken(context.Context) (string, error) {
	return "synthetic-native-resource-actor-witness", nil
}
func (nativeResourceActorWitness) Invalidate(string) {}

// The fixture supplies only the authenticated Sytue actor/audience and browser
// approval witness. Purpose/PKCE consumption, Registry/RLS, SDK login/device,
// SQLite, SSSS, backup, cross-signatures, age and persistent lease checks are real.
// LoginProxy bridges the ONE synthetic SSO token to the maintained SDK password
// login on this disposable Synapse; it never fakes decryption or device trust.
func TestNativeResourceConsentSDKProviderAndUpdatePersistence(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_FIXTURE") != "1" {
		t.Skip("disposable remote Synapse required")
	}
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_URL"))
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" {
		t.Fatal("disposable origin required")
	}
	admin := os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_ADMIN")
	if len(admin) < 32 {
		t.Fatal("fixture controller required")
	}
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 5*time.Minute)
	defer cancel()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := store.New(h.Pool, testCipher(t))
	actor := uuid.NewString()
	password := uuid.NewString()
	raw, err := mautrix.NewClient(target.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	registered := matrixprooftest.Register(ctx, t, raw, "sytue_"+actor, password, "ACCOUNT_SETUP")
	_, serverName, err := registered.UserID.Parse()
	if err != nil {
		t.Fatal(err)
	}
	gate := directory.New(directory.Config{InternalURL: target.String(), HomeserverURL: "https://" + serverName, ServerName: serverName, SSOProviderID: "sytue", AdminToken: admin})
	gate.SetIdentityGate(s)
	s.SetMatrixGate(gate)
	proxy, err := matrixprooftest.NewLoginProxy(t, matrixprooftest.LoginProxyConfig{SynapseURL: target.String(), UserID: registered.UserID.String(), Password: password})
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: actor, Product: "docs"}
	fail := func(w http.ResponseWriter) { http.Error(w, "fixture authorization rejected", http.StatusForbidden) }
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-native-resource-actor-witness" {
			fail(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/api/v1/matrix/resources/native/start":
			var input struct {
				Challenge string `json:"code_challenge"`
				Method    string `json:"code_challenge_method"`
				Product   string `json:"product"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Method != "S256" || input.Product != "docs" {
				fail(w)
				return
			}
			state, err := s.CreateMatrixResourceNativeConsent(ctx, actor, input.Challenge, "docs", "sytue-desktop")
			if err != nil {
				fail(w)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"purpose": "resources:docs", "resource_approved": false, "product": "docs", "tenant": "acme", "state": state, "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=" + state})
		case "/api/v1/matrix/resources/native/finish":
			var input struct {
				State    string `json:"state"`
				Ticket   string `json:"ticket"`
				Verifier string `json:"code_verifier"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				fail(w)
				return
			}
			hash := sha256.Sum256([]byte(input.Verifier))
			grant, err := s.FinishMatrixResourceNativeConsent(ctx, actor, input.State, input.Ticket, base64.RawURLEncoding.EncodeToString(hash[:]), "sytue-desktop")
			if err != nil {
				fail(w)
				return
			}
			_ = json.NewEncoder(w).Encode(grant)
		case "/api/v1/matrix/resources/docs/session/challenge":
			var input struct {
				Grant     string                   `json:"grant"`
				Recipient resource.SignedRecipient `json:"recipient"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				fail(w)
				return
			}
			challenge, err := s.ResourceSessionChallenge(ctx, scope, input.Grant, "sytue-desktop", input.Recipient)
			if err != nil {
				fail(w)
				return
			}
			_ = json.NewEncoder(w).Encode(challenge)
		case "/api/v1/matrix/resources/docs/session/confirm":
			var input resource.SignedSessionEnrollment
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				fail(w)
				return
			}
			record, err := s.ConfirmResourceSession(ctx, scope, "sytue-desktop", input)
			if err != nil {
				fail(w)
				return
			}
			_ = json.NewEncoder(w).Encode(record)
		case "/api/v1/matrix/resources/docs/session":
			provider, err := s.ResourceSessionProvider()
			if err != nil {
				fail(w)
				return
			}
			session, err := provider.Resolve(ctx, scope, r.URL.Query().Get("device"))
			if err != nil {
				fail(w)
				return
			}
			_, _ = w.Write(session.PublicRecordBytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()
	rooms := api.NewRooms(broker.URL, nativeResourceActorWitness{}, broker.Client())
	secrets := &nativeResourceTestSecrets{Secrets: &authtest.Secrets{}}
	cfg := native.Config{ResourceProduct: "docs", Bootstrap: native.Bootstrap{Actor: actor, Tenant: scope.Tenant, HomeserverURL: proxy.Server.URL, MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:" + serverName, Encryption: string(id.AlgorithmMegolmV1), SSOProviderID: "sytue"}, SytueApex: "sytue.eu", Subject: actor, Tenant: scope.Tenant, StoreDir: t.TempDir(), DeviceName: "Sytue Docus", Secrets: secrets, HTTPClient: proxy.Server.Client()}
	await := func(ctx context.Context, a api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error) {
		if !a.ForResources("sytue.eu", "docs") || a.Tenant() != scope.Tenant {
			return api.MatrixNativeCallback{}, api.ErrMatrixSSO
		}
		if err := s.ApproveMatrixResourceNativeConsent(ctx, actor, a.State); err != nil {
			return api.MatrixNativeCallback{}, err
		}
		ticket, _, err := s.AcceptMatrixNativeConsentWithDestination(ctx, actor, a.State, proxy.ArtificialNativeLoginToken())
		if err != nil {
			return api.MatrixNativeCallback{}, err
		}
		callback := &url.URL{Scheme: a.ReturnScheme(), Host: "matrix", Path: "/callback", RawQuery: url.Values{"state": {a.State}, "ticket": {ticket}}.Encode()}
		value, ok := api.ParseMatrixNativeCallbackFor(callback, "docs")
		if !ok {
			return value, api.ErrMatrixSSO
		}
		return value, nil
	}
	client, err := native.LoginNative(ctx, cfg, rooms, await)
	if err != nil {
		t.Fatal("native product PKCE/SDK login", err)
	}
	defer func() { _ = client.Close() }()
	recovery, err := client.BootstrapRecovery(ctx, func(uia *mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": registered.UserID.String()}, "password": password, "session": uia.Session}
	})
	if err != nil {
		t.Fatal(err)
	}
	if recovery == "" {
		t.Fatal("actual SDK recovery not established")
	}
	go func() { _ = client.Sync(ctx, func(native.DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	var active atomic.Int32
	var leases atomic.Int64
	lease := func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if active.Add(1) != 1 {
			active.Add(-1)
			return nil, errors.New("nested resource work lease")
		}
		leases.Add(1)
		return func() { active.Add(-1) }, nil
	}
	if _, err := client.EnrollResourceSession(ctx, rooms, lease, nil); !errors.Is(err, native.ErrUnverified) {
		t.Fatal("first root silently confirmed", err)
	}
	confirmations := 0
	session, err := client.EnrollResourceSession(ctx, rooms, lease, func(ctx context.Context, b resource.SessionBinding) (bool, error) {
		confirmations++
		if b.Scope != scope || b.DeviceID != client.DeviceID() || b.Namespace != "resources:docs" {
			return false, resource.ErrInvalid
		}
		return true, ctx.Err()
	})
	if err != nil {
		t.Fatal("native SDK product enrollment", err)
	}
	if confirmations != 1 || session.Actor().DeviceID != client.DeviceID() {
		t.Fatal("native consent changed actor or prompted twice")
	}
	signing, err := client.ResourcePublicationSession(ctx, session, lease)
	if err != nil || signing.Namespace() != "resources:docs" || signing.Scope() != scope || signing.Generation() != 1 {
		t.Fatal("native publication capability lost registered scope", err)
	}
	if active.Load() != 0 || leases.Load() < 5 {
		t.Fatal("session lease leaked or provider did not acquire it")
	}
	originalDevice, originalLeaf := client.DeviceID(), session.Binding().ProductKeyID
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = native.Open(ctx, cfg)
	if err != nil {
		t.Fatal("app update failed to restore encrypted SDK device", err)
	}
	go func() { _ = client.Sync(ctx, func(native.DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := client.EnrollResourceSession(ctx, rooms, lease, nil)
	if err != nil || client.DeviceID() != originalDevice || restored.Binding().ProductKeyID != originalLeaf {
		t.Fatal("update replaced resource device, leaf or required new consent", err)
	}
	if confirmations != 1 {
		t.Fatal("update repeated root consent")
	}
	secrets.missingLeaf.Store(true)
	writesBefore := secrets.leafWrites.Load()
	if _, err := client.EnrollResourceSession(ctx, rooms, lease, nil); !errors.Is(err, native.ErrRecoveryRequired) || secrets.leafWrites.Load() != writesBefore {
		t.Fatal("missing existing leaf was silently accepted or replaced", err)
	}
	secrets.missingLeaf.Store(false)
	if recovered, err := client.EnrollResourceSession(ctx, rooms, lease, nil); err != nil || recovered.Binding().ProductKeyID != originalLeaf {
		t.Fatal("temporary keychain failure destroyed original resource identity", err)
	}
	other := cfg
	other.ResourceProduct = "mail"
	if _, err := native.Open(ctx, other); !errors.Is(err, native.ErrLoginRequired) {
		t.Fatal("Mail reused Docs SDK credentials", err)
	}
	if err := client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := native.Open(ctx, cfg); !errors.Is(err, native.ErrLoginRequired) {
		t.Fatal("logout reopened deleted resource credentials", err)
	}
	backendProvider, err := s.ResourceSessionProvider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backendProvider.Resolve(ctx, scope, originalDevice); err == nil || errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("actual SDK logout retained Registry current authority", err)
	}
	t.Log("native purpose PKCE + actual SDK/SSSS/backup + real Registry and independent hybrid device leaf; app reopen survives update without repeat consent; exact work leases and remote logout verified")
}
