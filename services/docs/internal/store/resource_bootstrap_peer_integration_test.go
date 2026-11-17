//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	docshttp "git.sytue.eu/sytue/one/services/docs/internal/http"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type bootstrapPeerAPISession struct{}

func (bootstrapPeerAPISession) AccessToken(context.Context) (string, error) {
	return "test-current-app-session", nil
}
func (bootstrapPeerAPISession) Invalidate(string) {}

type (
	peerSQLCounts struct{ Transactions, Queries, Execs, Source, ACL, Captures, Pages int }
	peerCountTx   struct {
		pgx.Tx
		counts *peerSQLCounts
	}
)

func (tx peerCountTx) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	tx.counts.Queries++
	if strings.Contains(q, "SELECT d.owner_sub,d.seq,") {
		tx.counts.Source++
	}
	if strings.Contains(q, "FROM resource_e2e_roster_captures") {
		tx.counts.Captures++
	}
	if strings.Contains(q, "FROM resource_e2e_roster_pages") {
		tx.counts.Pages++
	}
	return tx.Tx.QueryRow(ctx, q, args...)
}

func (tx peerCountTx) Query(ctx context.Context, q string, args ...any) (pgx.Rows, error) {
	tx.counts.Queries++
	if strings.Contains(q, "FROM doc_permissions p") {
		tx.counts.ACL++
	}
	return tx.Tx.Query(ctx, q, args...)
}

func (tx peerCountTx) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	tx.counts.Execs++
	return tx.Tx.Exec(ctx, q, args...)
}

// Actual ORIGINAL Docs freeze/RLS/ACL and immutable capture + real HTTP client,
// nonce/MAC and maintained SDK/SessionProvider. Remote Rooms registry/directory
// is a public test witness here, not proof of real PKCE or Synapse composition.
func TestOfficeResourceBootstrapPeerEvidenceRequiresOriginalACLManifestAndCurrentB(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	current, ownerRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	readerScope := f.Scope
	readerScope.Subject = "public-reader"
	readerChain := matrixprooftest.NewUser(t, readerScope, "matrix.test", "@sytue_public-reader:matrix.test", "PEER_CURRENT_B")
	readerLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	reader, readerRegistry := resourcetest.EnrolledSession(t, readerChain, readerLeaf)
	strangerScope := f.Scope
	strangerScope.Subject = "not-in-original-acl"
	strangerChain := matrixprooftest.NewUser(t, strangerScope, "matrix.test", "@sytue_not-in-original-acl:matrix.test", "STRANGER_B")
	strangerLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	stranger, strangerRegistry := resourcetest.EnrolledSession(t, strangerChain, strangerLeaf)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "original shared source", OwnerSub: f.Scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: "user", PrincipalID: readerScope.Subject, Role: domain.RoleViewer, GrantedBy: f.Scope.Subject}); err != nil {
		t.Fatal(err)
	}
	original, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.Begin(ctx, f.Scope, uuid.New()); err != nil {
		t.Fatal(err)
	}
	source, err := store.NewResourceBootstrapSource(st, f.Scope)
	if err != nil {
		t.Fatal(err)
	}
	key := resource.Resource{Tenant: f.Scope.Tenant, Product: f.Scope.Product, Kind: "document", ID: doc.ID}
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	member := func(s resource.VerifiedSession) resource.Member {
		b := s.Binding()
		return resource.Member{Scope: b.Scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: "viewer", Actions: []resource.Action{resource.Read}, From: resource.Position{Seq: "0"}}
	}
	owner := bootstrap.Roster.Members()[0]
	plan := bootstrap.Signed.BootstrapPlan
	plan.MembershipSHA256, err = resource.MembershipDigest(key, []resource.Member{owner, member(reader), member(stranger)})
	if err != nil {
		t.Fatal(err)
	}
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: owner, Recipient: ownerRegistry.Record.Recipient}, {Member: member(reader), Recipient: readerRegistry.Record.Recipient}, {Member: member(stranger), Recipient: strangerRegistry.Record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapPlan{BootstrapPlan: plan, Signatures: bootstrap.Sign(plan)}
	signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: bootstrap.Sign(manifest)}
	pending, err := resource.VerifyBootstrapRosterManifest(ctx, signed, signedManifest, current, source)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.NewResourceBootstrapCaptureStore(st, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Begin(ctx, pending); err != nil {
		t.Fatal(err)
	}
	readerResetChain := matrixprooftest.NewUser(t, readerScope, "matrix.test", readerChain.Origin.UserID, readerChain.Origin.DeviceID)
	readerResetLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_, readerResetRegistry := resourcetest.EnrolledSession(t, readerResetChain, readerResetLeaf)
	ownerResetChain := matrixprooftest.NewUser(t, f.Scope, "matrix.test", f.Chain.Origin.UserID, f.Chain.Origin.DeviceID)
	ownerResetLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_, ownerResetRegistry := resourcetest.EnrolledSession(t, ownerResetChain, ownerResetLeaf)
	readerRekeyLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_, readerRekeyRegistry := resourcetest.EnrolledSession(t, readerChain, readerRekeyLeaf)
	secret := strings.Repeat("a", 64)
	var mu sync.Mutex
	peerReads := 0
	ownerReads := 0
	scopeReads := map[string]int{}
	nonces := map[string]bool{}
	mode := "valid"
	var previous resourcepg.SessionOriginResponse
	remote := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var q resourcepg.SessionOriginRequest
		if r.URL.Path != "/internal/resources/sessions/docs/current" || httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		if nonces[q.Nonce] {
			t.Error("operation reused authority nonce")
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		nonces[q.Nonce] = true
		scopeReads[q.Scope.Subject]++
		phase := scopeReads[q.Scope.Subject]
		if (mode == "owner-retired-post" && q.Scope == f.Scope && phase == 2) || (mode == "peer-retired-post" && q.Scope == readerScope && phase == 2) {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		registry := ownerRegistry
		if q.Scope == readerScope {
			registry = readerRegistry
			peerReads++
		} else if q.Scope == strangerScope {
			registry = strangerRegistry
			peerReads++
		} else {
			ownerReads++
		}
		if mode == "peer-root-post" && q.Scope == readerScope && phase == 2 {
			registry = readerResetRegistry
		}
		if mode == "owner-root-post" && q.Scope == f.Scope && phase == 2 {
			registry = ownerResetRegistry
		}
		if mode == "peer-leaf-post" && q.Scope == readerScope && phase == 2 {
			registry = readerRekeyRegistry
		}
		record, err := registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		if mode == "replay" && q.Scope == readerScope {
			httpx.JSON(w, r, http.StatusOK, previous)
			return
		}
		origin := registry.Chain.Origin
		if mode == "forged-sdk" && q.Scope == readerScope {
			origin.Keys.DeviceKeys = nil
		}
		response, err := resourcepg.SignSessionOriginResponse(secret, q, resourcepg.SessionOriginResponse{Version: 1, Record: record, Origin: origin})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		if q.Scope == readerScope {
			previous = response
		}
		httpx.JSON(w, r, http.StatusOK, response)
	})))
	defer remote.Close()
	authority, err := resourcepg.NewSessionAuthorityClient("docs", remote.URL, secret, remote.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	peerHandler, err := docshttp.NewResourceBootstrapPeerHandler(st, authority)
	if err != nil {
		t.Fatal(err)
	}
	claims := authn.Claims{Subject: f.Scope.Subject, Tenant: f.Scope.Tenant, Audience: []string{"sytue-docus"}}
	request := func(q resource.BootstrapPeerQuery, c authn.Claims) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, docshttp.ResourceBootstrapPeerPath, bytes.NewReader(raw)).WithContext(authn.Into(ctx, c))
		w := httptest.NewRecorder()
		peerHandler.ServeHTTP(w, r)
		return w
	}
	query := resource.BootstrapPeerQuery{Plan: signed, Manifest: signedManifest, Scope: readerScope, DeviceID: reader.Actor().DeviceID}
	if got := request(query, claims); got.Code != http.StatusConflict {
		t.Fatal("partial captured roster disclosed peer", got.Code)
	}
	mu.Lock()
	readCount := peerReads
	mu.Unlock()
	if readCount != 0 {
		t.Fatal("partial manifest queried private peer registry")
	}
	for i, input := range inputs {
		offset, _ := resource.Sequence(int64(i))
		if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: offset, Inputs: []resource.BootstrapRosterInput{input}}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	ownerBefore, peerBefore := ownerReads, peerReads
	mu.Unlock()
	got := request(query, claims)
	mu.Lock()
	ownerDelta, peerDelta := ownerReads-ownerBefore, peerReads-peerBefore
	mu.Unlock()
	if ownerDelta != 2 || peerDelta != 2 {
		t.Fatal("peer operation repeated CURRENT lookups", ownerDelta, peerDelta)
	}
	t.Log("PEER_PUBLIC_HTTP_AUTHORITY_COUNTS", ownerDelta, peerDelta)
	if got.Code != http.StatusOK || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authorized current peer unavailable", got.Code, got.Body.String())
	}
	var evidence resource.SessionEvidence
	if json.Unmarshal(got.Body.Bytes(), &evidence) != nil || evidence.Record.Binding != reader.Binding() || bytes.Contains(got.Body.Bytes(), []byte(secret)) || bytes.Contains(got.Body.Bytes(), []byte(`"mac"`)) {
		t.Fatal("public evidence changed binding or leaked service proof")
	}
	counts := &peerSQLCounts{}
	counted, err := resourcepg.NewBootstrapCaptureStore(resourcepg.Dependencies{
		TenantTx: func(ctx context.Context, fn func(pgx.Tx) error) error {
			counts.Transactions++
			return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { return fn(peerCountTx{Tx: tx, counts: counts}) })
		},
		ValidateResource: func(_ context.Context, value resource.Resource) error {
			if value != key {
				return resource.ErrInvalid
			}
			return nil
		},
		CheckBootstrapSource: source.CheckBootstrapSourceTx,
	}, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counted.CurrentPeerEvidenceOperation(ctx, query, source, authority); err != nil {
		t.Fatal("counted actual original SQL operation", err)
	}
	if counts.Transactions != 2 || counts.Source != 2 || counts.ACL != 2 || counts.Captures != 2 || counts.Pages != len(inputs) {
		t.Fatal("immutable pages repeated CURRENT/source checks", counts)
	}
	t.Log("PEER_ORIGINAL_SQL_COUNTS", *counts)
	mu.Lock()
	before := peerReads
	mu.Unlock()
	unlisted := query
	unlisted.Scope = strangerScope
	unlisted.DeviceID = stranger.Actor().DeviceID
	if got := request(unlisted, claims); got.Code != http.StatusForbidden {
		t.Fatal("signed roster replaced original SQL ACL", got.Code)
	}
	mu.Lock()
	after := peerReads
	mu.Unlock()
	if after != before {
		t.Fatal("unauthorized original ACL queried peer registry")
	}
	for _, extra := range []string{`"caller_root":"untrusted"`, `"pinned_root":"untrusted"`} {
		raw, _ := json.Marshal(query)
		raw = append(raw[:len(raw)-1], []byte(","+extra+"}")...)
		r := httptest.NewRequest(http.MethodPost, docshttp.ResourceBootstrapPeerPath, bytes.NewReader(raw)).WithContext(authn.Into(ctx, claims))
		w := httptest.NewRecorder()
		peerHandler.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatal("caller supplied peer root/pin", w.Code)
		}
	}
	foreign := claims
	foreign.Subject = "not-the-owner"
	if got := request(query, foreign); got.Code != http.StatusForbidden {
		t.Fatal("caller selected another owner", got.Code)
	}
	legacy := claims
	legacy.Audience = []string{"sytue-docs"}
	if got := request(query, legacy); got.Code != http.StatusForbidden {
		t.Fatal("legacy audience authorized peer", got.Code)
	}
	signatureless := query
	signatureless.Manifest.Signatures = nil
	if got := request(signatureless, claims); got.Code == http.StatusOK {
		t.Fatal("unsigned manifest disclosed peer")
	}
	wrongDevice := query
	wrongDevice.DeviceID = "not-in-manifest"
	if got := request(wrongDevice, claims); got.Code != http.StatusForbidden {
		t.Fatal("unlisted device disclosed peer", got.Code)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != docshttp.ResourceBootstrapPeerPath || r.Header.Get("Authorization") != "Bearer test-current-app-session" {
			http.NotFound(w, r)
			return
		}
		peerHandler.ServeHTTP(w, r.WithContext(authn.Into(ctx, claims)))
	}))
	defer server.Close()
	// The production API transport is exercised below through its own session.
	client := api.NewDocs(server.URL, bootstrapPeerAPISession{}, server.Client())
	provider, err := client.BootstrapPeerSessionProvider(signed, signedManifest, "matrix.test")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	ownerBefore, peerBefore = ownerReads, peerReads
	mu.Unlock()
	verified, err := provider.Resolve(context.Background(), readerScope, reader.Actor().DeviceID)
	if err != nil || verified.Binding() != reader.Binding() {
		t.Fatal("API public evidence did not verify through same provider", err)
	}
	mu.Lock()
	ownerDelta, peerDelta = ownerReads-ownerBefore, peerReads-peerBefore
	mu.Unlock()
	if ownerDelta != 2 || peerDelta != 2 {
		t.Fatal("consumer Resolve fetched peer operation repeatedly", ownerDelta, peerDelta)
	}
	t.Log("PEER_CONSUMER_RESOLVE_AUTHORITY_COUNTS", ownerDelta, peerDelta)
	mu.Lock()
	ownerBefore, peerBefore = ownerReads, peerReads
	mu.Unlock()
	if err := verified.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	ownerDelta, peerDelta = ownerReads-ownerBefore, peerReads-peerBefore
	mu.Unlock()
	if ownerDelta != 6 || peerDelta != 6 {
		t.Fatal("future consumer Revalidate cached request witness", ownerDelta, peerDelta)
	}
	t.Log("PEER_CONSUMER_REVALIDATE_AUTHORITY_COUNTS", ownerDelta, peerDelta)
	for _, interleave := range []string{"peer-retired-post", "owner-retired-post", "peer-root-post", "owner-root-post", "peer-leaf-post"} {
		mu.Lock()
		mode = interleave
		scopeReads = map[string]int{}
		mu.Unlock()
		if got := request(query, claims); got.Code != http.StatusForbidden {
			t.Fatal("PRE witness replaced fresh POST lifecycle/root/binding", interleave, got.Code)
		}
		faultSubject := readerScope.Subject
		if strings.HasPrefix(interleave, "owner-") {
			faultSubject = f.Scope.Subject
		}
		mu.Lock()
		reached := scopeReads[faultSubject]
		mu.Unlock()
		if reached != 2 {
			t.Fatal("POST interleaving was not reached", interleave, reached)
		}
	}
	mu.Lock()
	mode = "replay"
	mu.Unlock()
	if got := request(query, claims); got.Code == http.StatusOK {
		t.Fatal("replayed peer nonce revived stale authority")
	}
	mu.Lock()
	mode = "forged-sdk"
	mu.Unlock()
	if got := request(query, claims); got.Code == http.StatusOK {
		t.Fatal("metadata MAC replaced peer SDK verification")
	}
	mu.Lock()
	mode = "valid"
	mu.Unlock()
	readerRegistry.Retire()
	if err := verified.Revalidate(ctx); err == nil {
		t.Fatal("retired B remained authorized through client provider")
	}
	if got := request(query, claims); got.Code == http.StatusOK {
		t.Fatal("retired B disclosed cached evidence")
	}
	if err := source.CheckBootstrapSource(ctx, pending.Plan().Source); err != nil {
		t.Fatal("public evidence retired original source", err)
	}
}
