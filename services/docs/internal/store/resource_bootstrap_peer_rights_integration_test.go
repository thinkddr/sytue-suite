//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func originalPeerMember(current resource.VerifiedSession, role string, actions []resource.Action) resource.Member {
	b := current.Binding()
	return resource.Member{Scope: b.Scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: role, Actions: actions, From: resource.Position{Seq: "0"}}
}

// These are normal-RLS ORIGINAL Docs rows. The peer's real enrollment cannot
// replace its role, promote a reader or supply collective ID membership.
func TestOfficeResourcePeerRightsUseOriginalRoleLiveACLAndNoCollectiveClaims(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	owner, ownerRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	peerScope := f.Scope
	peerScope.Subject = "original-peer-rights"
	peerChain := matrixprooftest.NewUser(t, peerScope, "matrix.test", "@sytue_original-peer-rights:matrix.test", "RIGHTS_B")
	peerLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	peer, peerRegistry := resourcetest.EnrolledSession(t, peerChain, peerLeaf)
	keys := map[string]resource.Resource{}
	for _, name := range []string{"viewer", "commenter", "editor", "delegated-owner", "group", "tenant", "public", "direct-plus-group", "link-live", "link-expired", "link-revoked", "link-blocked", "link-overridden", "deleted"} {
		doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: name, OwnerSub: f.Scope.Subject})
		if err != nil {
			t.Fatal(err)
		}
		keys[name] = resource.Resource{Tenant: f.Scope.Tenant, Product: f.Scope.Product, Kind: "document", ID: doc.ID}
		kind, id, role := domain.PrincipalUser, peerScope.Subject, domain.RoleViewer
		switch name {
		case "commenter":
			role = domain.RoleCommenter
		case "editor":
			role = domain.RoleEditor
		case "delegated-owner":
			role = domain.RoleOwner
		case "group":
			kind, role = domain.PrincipalGroup, domain.RoleOwner
		case "tenant":
			kind, id, role = domain.PrincipalTenant, f.Scope.Tenant, domain.RoleOwner
		case "public":
			kind, id, role = domain.PrincipalPublic, "", domain.RoleViewer
		}
		if strings.HasPrefix(name, "link-") {
			expires := time.Now().Add(time.Hour)
			if name == "link-expired" {
				expires = time.Now().Add(-time.Hour)
			}
			link, err := st.CreateShareLink(ctx, doc.ID, domain.HashToken(name), "", f.Scope.Subject, domain.RoleViewer, &expires)
			if err != nil {
				t.Fatal(err)
			}
			// Canjear receives a resolution already obtained from the token port.
			// Keeping an expired row here exercises the original live-row filter.
			if written, err := st.Canjear(ctx, &store.EnlaceCanjeable{LinkID: link.ID, ShareResolution: domain.ShareResolution{DocumentID: doc.ID, Tenant: f.Scope.Tenant, Role: domain.RoleViewer}}, peerScope.Subject, false); err != nil || !written {
				t.Fatal("original redeemed ACL fixture", written, err)
			}
			if name == "link-revoked" || name == "link-overridden" {
				if err := st.RevokeShareLink(ctx, doc.ID, link.ID); err != nil {
					t.Fatal(err)
				}
			}
			if name == "link-blocked" {
				if err := st.RevokePermission(ctx, doc.ID, domain.PrincipalUser, peerScope.Subject); err != nil {
					t.Fatal(err)
				}
			}
			if name != "link-overridden" {
				continue
			}
		}
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: kind, PrincipalID: id, Role: role, GrantedBy: f.Scope.Subject}); err != nil {
			t.Fatal(err)
		}
		if name == "deleted" {
			if err := st.SoftDeleteDocument(ctx, doc.ID); err != nil {
				t.Fatal(err)
			}
		}
		if name == "direct-plus-group" {
			if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalGroup, PrincipalID: peerScope.Subject, Role: domain.RoleOwner, GrantedBy: f.Scope.Subject}); err != nil {
				t.Fatal(err)
			}
		}
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
	if _, err := source.BootstrapSource(ctx, keys["deleted"]); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("deleted original document authorized bootstrap source", err)
	}
	for _, tc := range []struct {
		name, doc, role string
		actions         []resource.Action
		owner, allowed  bool
	}{
		{"viewer", "viewer", "viewer", []resource.Action{resource.Read}, false, true},
		{"viewer-role-promotion", "viewer", "owner", []resource.Action{resource.Read}, false, false},
		{"viewer-action-promotion", "viewer", "viewer", []resource.Action{resource.Publish, resource.Read}, false, false},
		{"viewer-comment", "viewer", "viewer", []resource.Action{resource.Comment, resource.Read}, false, false},
		{"admin-is-not-owner", "viewer", "admin", []resource.Action{resource.Manage, resource.Read}, false, false},
		{"commenter", "commenter", "commenter", []resource.Action{resource.Comment, resource.Read}, false, true},
		{"commenter-publish", "commenter", "commenter", []resource.Action{resource.Publish, resource.Read}, false, false},
		{"editor", "editor", "editor", []resource.Action{resource.Comment, resource.Publish, resource.Read}, false, true},
		{"editor-cannot-validate", "editor", "editor", []resource.Action{resource.Read, resource.Validate}, false, false},
		{"editor-cannot-manage", "editor", "editor", []resource.Action{resource.Manage, resource.Read}, false, false},
		{"owner-full", "viewer", "owner", []resource.Action{resource.Comment, resource.Manage, resource.Publish, resource.Read, resource.Validate}, true, true},
		{"owner-subset", "viewer", "owner", []resource.Action{resource.Read}, true, true},
		{"owner-role-mismatch", "viewer", "viewer", []resource.Action{resource.Read}, true, false},
		{"delegated-original-owner", "delegated-owner", "owner", []resource.Action{resource.Manage, resource.Read, resource.Validate}, false, true},
		{"group-no-ID-proof", "group", "owner", []resource.Action{resource.Read}, false, false},
		{"tenant-no-ID-proof", "tenant", "owner", []resource.Action{resource.Read}, false, false},
		{"public-no-token", "public", "viewer", []resource.Action{resource.Read}, false, false},
		{"collective-cannot-promote-direct", "direct-plus-group", "owner", []resource.Action{resource.Read}, false, false},
		{"direct-survives-unknown-group", "direct-plus-group", "viewer", []resource.Action{resource.Read}, false, true},
		{"redeemed-live-link", "link-live", "viewer", []resource.Action{resource.Read}, false, true},
		{"redeemed-expired-link", "link-expired", "viewer", []resource.Action{resource.Read}, false, false},
		{"redeemed-revoked-link", "link-revoked", "viewer", []resource.Action{resource.Read}, false, false},
		{"redeemed-blocked-link", "link-blocked", "viewer", []resource.Action{resource.Read}, false, false},
		{"direct-clears-link-dependency", "link-overridden", "viewer", []resource.Action{resource.Read}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := keys[tc.doc]
			snapshot, err := source.BootstrapSource(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			current, registry := peer, peerRegistry
			if tc.owner {
				current, registry = owner, ownerRegistry
			}
			input := resource.BootstrapRosterInput{Member: originalPeerMember(current, tc.role, tc.actions), Recipient: registry.Record.Recipient}
			err = source.CheckBootstrapPeer(ctx, snapshot, input)
			if tc.allowed && err != nil {
				t.Fatal("original ACL rejected matching signed rights", err)
			}
			if !tc.allowed && !errors.Is(err, resource.ErrRevoked) {
				t.Fatal("original ACL accepted signed rights outside domain", err)
			}
		})
	}
}

// A correctly signed COMPLETE manifest with a live viewer still cannot replace
// the original role/actions. PRE denial must happen before B's registry lookup.
func TestOfficeResourcePeerOperationRejectsSignedRoleAndActionPromotion(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	owner, ownerRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	peerScope := f.Scope
	peerScope.Subject = "signed-rights-peer"
	chain := matrixprooftest.NewUser(t, peerScope, "matrix.test", "@sytue_signed-rights-peer:matrix.test", "SIGNED_RIGHTS_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	peer, peerRegistry := resourcetest.EnrolledSession(t, chain, leaf)
	cases := []struct {
		name, role string
		actions    []resource.Action
		allowed    bool
	}{
		{"valid-reader", "viewer", []resource.Action{resource.Read}, true},
		{"signed-owner-promotion", "owner", []resource.Action{resource.Read}, false},
		{"signed-admin-promotion", "admin", []resource.Action{resource.Read}, false},
		{"signed-publish-promotion", "viewer", []resource.Action{resource.Publish, resource.Read}, false},
		{"signed-validator-promotion", "viewer", []resource.Action{resource.Read, resource.Validate}, false},
	}
	keys := map[string]resource.Resource{}
	for _, tc := range cases {
		doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: tc.name, OwnerSub: f.Scope.Subject})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: peerScope.Subject, Role: domain.RoleViewer, GrantedBy: f.Scope.Subject}); err != nil {
			t.Fatal(err)
		}
		keys[tc.name] = resource.Resource{Tenant: f.Scope.Tenant, Product: f.Scope.Product, Kind: "document", ID: doc.ID}
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
	capture, err := store.NewResourceBootstrapCaptureStore(st, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("a", 64)
	var peerReads atomic.Int64
	remote := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q resourcepg.SessionOriginRequest
		if httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		registry := ownerRegistry
		if q.Scope == peerScope {
			registry = peerRegistry
			peerReads.Add(1)
		}
		record, err := registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		response, err := resourcepg.SignSessionOriginResponse(secret, q, resourcepg.SessionOriginResponse{Version: 1, Record: record, Origin: registry.Chain.Origin})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		httpx.JSON(w, r, http.StatusOK, response)
	})))
	defer remote.Close()
	authority, err := resourcepg.NewSessionAuthorityClient("docs", remote.URL, secret, remote.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := keys[tc.name]
			bootstrap := resourcetest.NewBootstrap(t, ctx, f, owner, source, key)
			plan := bootstrap.Signed.BootstrapPlan
			plan.ID = uuid.NewString()
			member := originalPeerMember(peer, tc.role, tc.actions)
			plan.MembershipSHA256, err = resource.MembershipDigest(key, []resource.Member{bootstrap.Roster.Members()[0], member})
			if err != nil {
				t.Fatal(err)
			}
			manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: bootstrap.Roster.Members()[0], Recipient: ownerRegistry.Record.Recipient}, {Member: member, Recipient: peerRegistry.Record.Recipient}})
			if err != nil {
				t.Fatal(err)
			}
			signed := resource.SignedBootstrapPlan{BootstrapPlan: plan, Signatures: bootstrap.Sign(plan)}
			signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: bootstrap.Sign(manifest)}
			pending, err := resource.VerifyBootstrapRosterManifest(ctx, signed, signedManifest, owner, source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := capture.Begin(ctx, pending); err != nil {
				t.Fatal(err)
			}
			if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: inputs}); err != nil {
				t.Fatal(err)
			}
			before := peerReads.Load()
			query := resource.BootstrapPeerQuery{Plan: signed, Manifest: signedManifest, Scope: peerScope, DeviceID: peer.Actor().DeviceID}
			_, err = capture.CurrentPeerEvidenceOperation(ctx, query, source, authority)
			if tc.allowed && (err != nil || peerReads.Load()-before != 2) {
				t.Fatal("matching original viewer rights unavailable", err, peerReads.Load()-before)
			}
			if !tc.allowed && (!errors.Is(err, resource.ErrRevoked) || peerReads.Load() != before) {
				t.Fatal("signed roster promoted original viewer before CURRENT lookup", err, peerReads.Load()-before)
			}
		})
	}
}

// Expiry needs no mutation/version bump. An actual redeemed link can become
// invalid across the HTTP boundary while SourceSeq/ACLVersion remain unchanged.
func TestOfficeResourcePeerOperationRechecksRedeemedLinkExpiryAtPost(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	owner, ownerRegistry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	peerScope := f.Scope
	peerScope.Subject = "expiring-link-peer"
	chain := matrixprooftest.NewUser(t, peerScope, "matrix.test", "@sytue_expiring-link-peer:matrix.test", "EXPIRING_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	peer, peerRegistry := resourcetest.EnrolledSession(t, chain, leaf)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "expiring original link", OwnerSub: f.Scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(12 * time.Second)
	link, err := st.CreateShareLink(ctx, doc.ID, domain.HashToken("expiring-original-link"), "", f.Scope.Subject, domain.RoleViewer, &expires)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := st.Canjear(ctx, &store.EnlaceCanjeable{LinkID: link.ID, ShareResolution: domain.ShareResolution{DocumentID: doc.ID, Tenant: f.Scope.Tenant, Role: domain.RoleViewer}}, peerScope.Subject, false); err != nil || !written {
		t.Fatal("actual original redeemed link", written, err)
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
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, owner, source, key)
	member := originalPeerMember(peer, "viewer", []resource.Action{resource.Read})
	plan := bootstrap.Signed.BootstrapPlan
	plan.MembershipSHA256, err = resource.MembershipDigest(key, []resource.Member{bootstrap.Roster.Members()[0], member})
	if err != nil {
		t.Fatal(err)
	}
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(plan, []resource.BootstrapRosterInput{{Member: bootstrap.Roster.Members()[0], Recipient: ownerRegistry.Record.Recipient}, {Member: member, Recipient: peerRegistry.Record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapPlan{BootstrapPlan: plan, Signatures: bootstrap.Sign(plan)}
	signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: bootstrap.Sign(manifest)}
	pending, err := resource.VerifyBootstrapRosterManifest(ctx, signed, signedManifest, owner, source)
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
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: inputs}); err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("a", 64)
	var peerReads atomic.Int64
	remote := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q resourcepg.SessionOriginRequest
		if httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		registry := ownerRegistry
		if q.Scope == peerScope {
			registry = peerRegistry
			peerReads.Add(1)
			if !time.Now().Before(expires) {
				t.Error("expiry fixture missed the live PRE boundary")
			}
			timer := time.NewTimer(time.Until(expires) + 30*time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		record, err := registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		response, err := resourcepg.SignSessionOriginResponse(secret, q, resourcepg.SessionOriginResponse{Version: 1, Record: record, Origin: registry.Chain.Origin})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		httpx.JSON(w, r, http.StatusOK, response)
	})))
	defer remote.Close()
	authority, err := resourcepg.NewSessionAuthorityClient("docs", remote.URL, secret, remote.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	query := resource.BootstrapPeerQuery{Plan: signed, Manifest: signedManifest, Scope: peerScope, DeviceID: peer.Actor().DeviceID}
	_, err = capture.CurrentPeerEvidenceOperation(ctx, query, source, authority)
	if !errors.Is(err, resource.ErrRevoked) || peerReads.Load() != 1 {
		t.Fatal("expired redeemed link survived ORIGINAL POST rights check", err, peerReads.Load())
	}
	current, err := source.BootstrapSource(ctx, key)
	if err != nil || current != plan.Source {
		t.Fatal("expiry changed original source/version instead of exercising live ACL", err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var persisted bool
		if err := tx.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM doc_permissions WHERE document_id=$1 AND principal_id=$2 AND via_link=$3 AND bloqueado_desde IS NULL)`, doc.ID, peerScope.Subject, link.ID).Scan(&persisted); err != nil {
			return err
		}
		if !persisted {
			t.Fatal("expired via_link was removed instead of filtered live")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
