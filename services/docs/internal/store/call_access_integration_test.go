// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type currentCallIdentity struct {
	groups  []string
	member  bool
	version int64
	admin   bool
}

func (f *currentCallIdentity) ReadCallIdentity(_ context.Context, actor, tenant string) (callaccess.IdentityProof, error) {
	return callaccess.IdentityProof{Subject: actor, Tenant: tenant, Active: true, Member: f.member, Groups: f.groups, Version: f.version, OrganizationAdmin: f.admin}, nil
}

type documentRevoker struct {
	err      error
	minimum  map[string]int64
	versions []int64
	before   func()
}

func (f *documentRevoker) RevokeDocumentCalls(_ context.Context, id, _ string, version int64) error {
	if f.minimum == nil {
		f.minimum = map[string]int64{}
	}
	if version > f.minimum[id] {
		f.minimum[id] = version
	}
	f.versions = append(f.versions, version)
	if f.before != nil {
		f.before()
	}
	return f.err
}

func (f *documentRevoker) MinimumDocumentVersion(_ context.Context, id string) (int64, error) {
	return f.minimum[id], nil
}

func TestDocumentAccessRollbackRecoveryUsesFreshFence(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	st.SetCallIdentitySource(&currentCallIdentity{member: true})
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Private", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	revoker := &documentRevoker{}
	if err := st.ConfigureCallAccess(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "alice", Role: domain.RoleViewer, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	initial := revoker.minimum[doc.ID]
	revoker.err = errors.New("response lost after Rooms fenced the call")
	if err := st.RevokePermission(ctx, doc.ID, domain.PrincipalUser, "alice"); err == nil {
		t.Fatal("Docs committed without a revocation acknowledgement")
	}
	failed := revoker.minimum[doc.ID]
	if failed <= initial {
		t.Fatal("fence sequence was reused")
	}
	permissions, err := st.ListPermissions(ctx, doc.ID)
	if err != nil || len(permissions) != 1 {
		t.Fatalf("failed revocation changed ACL: %+v %v", permissions, err)
	}
	revoker.err = nil
	proof, err := st.DocumentCallAccess(ctx, doc.ID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if proof.Version <= failed || proof.Actor != "alice" || proof.Tenant != "acme" {
		t.Fatalf("rollback overblocking did not recover: %+v", proof)
	}
	if err := st.ConfigureCallAccess(ctx, false, revoker); err != nil {
		t.Fatalf("disabling calls broke ACL mutation guard: %v", err)
	}
	if _, err := st.DocumentCallAccess(ctx, doc.ID, "owner"); !errors.Is(err, domain.ErrCallAccessUnavailable) {
		t.Fatalf("disabled call proof still available: %v", err)
	}
	if err := st.RevokePermission(ctx, doc.ID, domain.PrincipalUser, "alice"); err != nil {
		t.Fatalf("protected ACL mutation stopped working with calls disabled: %v", err)
	}
}

func TestDocumentSQLImporterCannotBypassCallRevocation(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	st.SetCallIdentitySource(&currentCallIdentity{member: true})
	ctx := tenantCtx("acme")
	revoker := &documentRevoker{}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Private", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfigureCallAccess(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	mutate := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE documents SET owner_sub='new-owner' WHERE id=$1`, doc.ID)
		return err
	}
	if err := h.Pool.InTenantTx(ctx, mutate); err == nil {
		t.Fatal("direct SQL import bypassed deferred revocation guard")
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := mutate(tx); err != nil {
			return err
		}
		return callaccess.Flush(ctx, tx, "acme", revoker)
	}); err != nil {
		t.Fatal(err)
	}
	if len(revoker.versions) != 1 {
		t.Fatalf("importer did not revoke before commit: %v", revoker.versions)
	}
}

func TestDocumentCallsUseIndividualACLAndActualLinkExpiry(t *testing.T) {
	st := newStore(t)
	identity := &currentCallIdentity{member: true}
	st.SetCallIdentitySource(identity)
	ctx := tenantCtx("acme")
	revoker := &documentRevoker{}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Private", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfigureCallAccess(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalGroup, PrincipalID: "editors", Role: domain.RoleEditor, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := domain.NewService(st).Authorize(ctx, doc.ID, domain.Subject{Sub: "group-user", Groups: []string{"editors"}}, domain.RoleViewer); err != nil {
		t.Fatalf("normal document authorization changed: %v", err)
	}
	if _, err := st.DocumentCallAccess(ctx, doc.ID, "group-user"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stale JWT group accepted for call: %v", err)
	}
	identity.groups = []string{"editors"}
	identity.version = 42
	groupProof, err := st.DocumentCallAccess(ctx, doc.ID, "group-user")
	if err != nil || groupProof.IdentityVersion != 42 {
		t.Fatalf("live group identity denied: %+v %v", groupProof, err)
	}
	identity.groups = nil
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalTenant, PrincipalID: "acme", Role: domain.RoleViewer, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DocumentCallAccess(ctx, doc.ID, "org-member"); err != nil {
		t.Fatalf("live organization membership denied: %v", err)
	}
	identity.member = false
	if _, err := st.DocumentCallAccess(ctx, doc.ID, "org-member"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stale tenant membership accepted: %v", err)
	}
	expiry := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	if _, err := st.CreateShareLink(ctx, doc.ID, domain.HashToken("link"), "", "owner", domain.RoleEditor, &expiry); err != nil {
		t.Fatal(err)
	}
	link, err := st.EnlaceParaCanjear(ctx, domain.HashToken("link"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Canjear(ctx, link, "guest", true); err != nil {
		t.Fatal(err)
	}
	proof, err := st.DocumentCallAccess(ctx, doc.ID, "guest")
	if err != nil || proof.ExpiresAtMS != expiry.UnixMilli() {
		t.Fatalf("link expiry was extended: %+v %v", proof, err)
	}
	owner, err := st.DocumentCallAccess(ctx, doc.ID, "owner")
	if err != nil || owner.ExpiresAtMS != 0 {
		t.Fatalf("owner incorrectly tied to link: %+v %v", owner, err)
	}
	if err := st.SoftDeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DocumentCallAccess(ctx, doc.ID, "owner"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("deleted document authorized call: %v", err)
	}
}
