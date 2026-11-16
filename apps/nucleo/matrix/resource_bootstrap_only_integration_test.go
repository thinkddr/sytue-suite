//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// Real SDK stores, cross-signing, two SSSS identities, device retirement and
// BOTH Megolm+hybrid descriptor readbacks. Source/Registry lifecycle are the
// explicit native test witnesses; no SourceSQL/PKCE/Origin/physical OS claim.
func TestNativeBootstrapOnlyPolicyRecoversOnFreshBWithoutNormalOperations(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE") != "1" {
		t.Skip("isolated remote Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("isolated fixture URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	actor := uuid.NewString()
	scope := storage.Scope{Subject: actor, Tenant: "resource-bootstrap-only", Product: "docs"}
	raw, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw.Client = proxy.Client()
	password := uuid.NewString()
	registered := matrixprooftest.Register(ctx, t, raw, "sytue_"+actor, password, "RESOURCE_BOOTSTRAP_ONLY_A")
	roomID := newPrivateStorageTestRoom(ctx, t, raw)
	_, server, _ := registered.UserID.Parse()
	cfg := Config{ResourceProduct: "docs", Bootstrap: Bootstrap{Actor: actor, Tenant: scope.Tenant, HomeserverURL: proxy.URL, MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:" + server, Encryption: string(id.AlgorithmMegolmV1)}, Subject: actor, Tenant: scope.Tenant, StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: proxy.Client()}
	c := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: proxy.URL, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: actor, Tenant: scope.Tenant, ResourceProduct: "docs"})
	defer func() { _ = c.Close() }()
	recovery, err := c.BootstrapRecovery(ctx, storageFixtureUIA(registered.UserID, password))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	roots, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || roots == nil {
		t.Fatal(err)
	}
	current, registry := nativePolicySession(ctx, t, c, scope, roots.MasterKey.String(), "1")
	enrolled, err := resource.VerifyAccountRecipient(ctx, registry.record.Recipient, current.Device())
	if err != nil {
		t.Fatal(err)
	}
	b := current.Binding()
	key := resource.Resource{Tenant: scope.Tenant, Product: scope.Product, Kind: "document", ID: uuid.NewString()}
	member := resource.Member{Scope: scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: "owner", Actions: []resource.Action{resource.Comment, resource.Manage, resource.Publish, resource.Read, resource.Validate}, From: resource.Position{Seq: "0"}}
	roster, err := resource.VerifyRoster(key, []resource.Member{member}, []resource.VerifiedRecipient{enrolled})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resource.NewSessionProvider(registry, registry)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	var work atomic.Int64
	lease := func(ctx context.Context) (func(), error) {
		if work.Add(1) != 1 {
			work.Add(-1)
			return nil, errors.New("nested bootstrap-only original work lease")
		}
		return func() { work.Add(-1) }, ctx.Err()
	}
	input := nativeBootstrapInput{key: key, room: roomID.String(), validation: resource.ValidationPolicy{RulesVersion: strings.Repeat("2", 64)}, roster: roster, enrolled: enrolled}
	saved := exerciseNativeBootstrapInput(ctx, t, c, current, input, provider, lease, journal)
	owner, err := scope.JournalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if count, err := journal.CountOwner(owner, storage.ResourcePublicationNamespace); err != nil || count != 0 {
		t.Fatal("bootstrap-only test accidentally prepared a normal operation", count, err)
	}
	// Explicitly discard the only pre-epoch typed plan before new-B startup.
	// Source records contain no signed owner capability or normal operation.
	saved.plan = resource.VerifiedBootstrapPlan{}
	registry.active.Store(false)
	exerciseNativeFreshBootstrapPolicy(ctx, t, c, cfg, password, recovery, saved.vault, saved.policy, lease, journal)
	if work.Load() != 0 {
		t.Fatal("bootstrap-only recovery leaked the original work lease")
	}
	t.Log("BOOTSTRAP_ONLY_ZERO_NORMAL_OPERATIONS_SIGNED_OWNER_CAPSULE_RETIRED_A_FRESH_B_GREEN_NO_ACK")
}
