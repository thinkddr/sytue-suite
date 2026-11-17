//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	chathttp "git.sytue.eu/sytue/one/services/rooms/internal/http"
	matrixDirectory "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestResourceSessionRegistryRealSynapseDirectoryAndDeviceRetirement(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_FIXTURE") != "1" {
		t.Skip("disposable remote Synapse required")
	}
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_URL"))
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" {
		t.Fatal("disposable loopback origin required")
	}
	admin := os.Getenv("SYTUE_MATRIX_RESOURCE_REGISTRY_ADMIN")
	if len(admin) < 32 {
		t.Fatal("disposable fixture admin required")
	}
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 2*time.Minute)
	defer cancel()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := store.New(h.Pool, testCipher(t))
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "docs"}
	sdk, err := mautrix.NewClient(target.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	password := uuid.NewString()
	registration := matrixprooftest.Register(ctx, t, sdk, "sytue_"+scope.Subject, password, "DOCS_REAL_A")
	_, server, err := registration.UserID.Parse()
	if err != nil {
		t.Fatal(err)
	}
	chain := matrixprooftest.NewUser(t, scope, server, registration.UserID, registration.DeviceID)
	matrixprooftest.PublishSigningChain(ctx, t, sdk, chain, password, true)
	directory := matrixDirectory.New(matrixDirectory.Config{InternalURL: target.String(), HomeserverURL: "https://" + server, ServerName: server, SSOProviderID: "sytue", AdminToken: admin})
	directory.SetIdentityGate(s)
	s.SetMatrixGate(directory)
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	consent, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := s.ResourceSessionChallenge(ctx, scope, consent.Grant, "sytue-docus", registryRecipient(t, chain, key))
	if err != nil {
		t.Fatal("actual SDK directory did not resolve product enrollment", err)
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", registryEnrollment(t, chain, key, challenge)); err != nil {
		t.Fatal(err)
	}
	provider, err := s.ResourceSessionProvider()
	if err != nil {
		t.Fatal(err)
	}
	a, err := provider.Resolve(ctx, scope, registration.DeviceID.String())
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("a", 64)
	cfg := config.Config{MatrixInternalURL: target.String(), MatrixHomeserverURL: "https://" + server, MatrixServerName: server, MatrixSSOProviderID: "sytue", MatrixAdminToken: admin, ResourceSessionDocsSecret: secret}
	svc := service.New(cfg, s, nil, nil)
	router := chathttp.NewRouter(chathttp.Deps{Cfg: cfg, Service: svc})
	readSession := func(q resourcepg.SessionOriginRequest, expected int) resourcepg.SessionOriginResponse {
		t.Helper()
		fresh, err := resourcepg.NewSessionOriginRequest(q.Scope, q.DeviceID)
		if err != nil {
			t.Fatal(err)
		}
		q.Nonce = fresh.Nonce
		raw, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/internal/resources/sessions/docs/current", bytes.NewReader(raw))
		request.Header.Set("Authorization", "Bearer "+secret)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if expected == 200 && response.Code != 200 || expected != 200 && response.Code == 200 {
			t.Fatal("original Registry/SDK session transport status", response.Code)
		}
		var value resourcepg.SessionOriginResponse
		if response.Code == 200 && json.Unmarshal(response.Body.Bytes(), &value) != nil {
			t.Fatal("public session response malformed")
		}
		if response.Code == 200 {
			if resourcepg.VerifySessionOriginResponse(secret, q, value) != nil {
				t.Fatal("fresh public session MAC rejected")
			}
			again, err := resourcepg.NewSessionOriginRequest(q.Scope, q.DeviceID)
			if err != nil {
				t.Fatal(err)
			}
			if again.Nonce == q.Nonce || resourcepg.VerifySessionOriginResponse(secret, again, value) == nil {
				t.Fatal("cached session response authorized a fresh request")
			}
			tampered := value
			tampered.Nonce = again.Nonce
			if resourcepg.VerifySessionOriginResponse(secret, again, tampered) == nil {
				t.Fatal("nonce substitution reused public session evidence")
			}
			tampered = value
			tampered.Record.ConfirmedAt = tampered.Record.ConfirmedAt.Add(time.Second)
			if resourcepg.VerifySessionOriginResponse(secret, q, tampered) == nil {
				t.Fatal("enrollment evidence changed under original MAC")
			}
		}
		return value
	}
	selection := resourcepg.SessionOriginRequest{Version: 1, Scope: scope, DeviceID: registration.DeviceID.String()}
	transportA := readSession(selection, 200)
	if transportA.Version != 1 || transportA.Record.Binding != a.Binding() || transportA.Origin.UserID != registration.UserID || transportA.Origin.DeviceID != registration.DeviceID {
		t.Fatal("public transport replaced original enrollment or SDK origin")
	}
	if strings.Contains(string(mustJSONPublicSession(t, transportA)), key.String()) {
		t.Fatal("public HTTP session included private leaf")
	}

	exerciseResourceSourceControlWithActualRegistry(t, ctx, s, cfg, a, chain)
	second, err := mautrix.NewClient(target.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: registration.UserID.String()}, Password: password, DeviceID: "DOCS_REAL_B", StoreCredentials: true}); err != nil {
		t.Fatal(err)
	}
	bChain := matrixprooftest.NewDevice(t, chain, second.DeviceID)
	matrixprooftest.PublishSigningChain(ctx, t, second, bChain, password, false)
	bKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bConsent, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	bChallenge, err := s.ResourceSessionChallenge(ctx, scope, bConsent.Grant, "sytue-docus", registryRecipient(t, bChain, bKey))
	if err != nil {
		t.Fatal(err)
	}
	if bChallenge.RequiresRootConfirmation {
		t.Fatal("fresh B reset confirmed root")
	}
	if _, err := resource.OpenSessionChallenge(ctx, bChallenge, key, time.Now()); err == nil {
		t.Fatal("A leaf opened B challenge")
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", registryEnrollment(t, bChain, bKey, bChallenge)); err != nil {
		t.Fatal(err)
	}
	b, err := provider.Resolve(ctx, scope, second.DeviceID.String())
	if err != nil {
		t.Fatal(err)
	}
	if b.Actor().RootKey != a.Actor().RootKey || b.Binding().ProductKeyID == a.Binding().ProductKeyID || b.Actor().DeviceID == a.Actor().DeviceID {
		t.Fatal("directory lost distinct SDK device leaf")
	}
	rotateRelay := prepareRegistryRelayFreshB(t, ctx, directory, provider, a, b, chain, bChain, key, bKey, func() error { _, err := second.Logout(ctx); return err })
	if _, err := sdk.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Revalidate(ctx); err == nil {
		t.Fatal("retired real Synapse device remained current")
	}
	if _, err := provider.Resolve(ctx, scope, registration.DeviceID.String()); err == nil {
		t.Fatal("retired real SDK device resolved")
	}
	if err := b.Revalidate(ctx); err != nil {
		t.Fatal("retiring A also retired independently enrolled B", err)
	}
	readSession(selection, 403)
	selection.DeviceID = second.DeviceID.String()
	transportB := readSession(selection, 200)
	if transportB.Record.Binding != b.Binding() {
		t.Fatal("current B HTTP transport changed leaf or binding")
	}
	selection.Scope.Product = "mail"
	readSession(selection, 400)
	mailScope := scope
	mailScope.Product = "mail"
	if _, err := provider.Resolve(ctx, mailScope, second.DeviceID.String()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("real Docs SDK device crossed into Mail", err)
	}
	if strings.Contains(string(b.PublicRecordBytes()), bKey.String()) {
		t.Fatal("public registry included private leaf")
	}
	rotateRelay()
	t.Log("real PostgreSQL Registry + configured Synapse directory + SDK cross-signatures + distinct hybrid leaves + actual SDK logout retirement verified")
}

func mustJSONPublicSession(t *testing.T, value resourcepg.SessionOriginResponse) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
