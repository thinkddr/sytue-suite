// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

type identityCallRevoker struct {
	minimum  map[string]int64
	versions []int64
	err      error
}

func (f *identityCallRevoker) RevokeSubjectCalls(_ context.Context, actor, tenant string, version int64) error {
	if f.minimum == nil {
		f.minimum = map[string]int64{}
	}
	key := actor + "/" + tenant
	if version > f.minimum[key] {
		f.minimum[key] = version
	}
	f.versions = append(f.versions, version)
	return f.err
}

func (f *identityCallRevoker) MinimumSubjectVersion(_ context.Context, actor, tenant string) (int64, error) {
	return f.minimum[actor+"/"+tenant], nil
}

func TestCallIdentityRevokesBeforeGroupCommitAndRecoversRollback(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "current-group@acme.example")
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "current-groups", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	group, err := st.CreateGroup(ctx, nil, org.ID, "engineering", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupMember(ctx, nil, org.ID, group.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	revoker := &identityCallRevoker{}
	if err := st.ConfigureCallIdentity(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	revoker.err = errors.New("response lost after remote revocation")
	if err := st.RemoveGroupMember(ctx, nil, org.ID, group.ID, user.ID); err == nil {
		t.Fatal("group removal committed without remote acknowledgement")
	}
	failed := revoker.minimum[user.ID+"/"+org.Slug]
	groups, err := st.GroupsForUser(ctx, org.ID, user.ID)
	if err != nil || !slices.Contains(groups, "engineering") {
		t.Fatalf("failed removal changed ID: %v %v", groups, err)
	}
	revoker.err = nil
	proof, err := st.CallIdentity(ctx, user.ID, org.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Version <= failed || !proof.Active || !proof.Member || !proof.OrganizationAdmin || !slices.Contains(proof.Groups, "engineering") {
		t.Fatalf("rollback did not repair identity fence: %+v", proof)
	}
	if err := st.RemoveGroupMember(ctx, nil, org.ID, group.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	proof, err = st.CallIdentity(ctx, user.ID, org.Slug)
	if err != nil || len(proof.Groups) != 0 {
		t.Fatalf("removed group remained authoritative: %+v %v", proof, err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	proof, err = st.CallIdentity(ctx, user.ID, org.Slug)
	if err != nil || proof.OrganizationAdmin {
		t.Fatalf("stale administration role survived: %+v %v", proof, err)
	}
	if err := st.ConfigureCallIdentity(ctx, false, revoker); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupMember(ctx, nil, org.ID, group.ID, user.ID); err != nil {
		t.Fatalf("disabling calls broke guarded identity writes: %v", err)
	}
}

func TestCallIdentityCoversSCIMDynamicGroupsAndRoleWriters(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "dynamic-current@acme.example")
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "dynamic-calls", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	group, err := st.CreateGroup(ctx, nil, org.ID, "manual", "")
	if err != nil {
		t.Fatal(err)
	}
	revoker := &identityCallRevoker{}
	if err := st.ConfigureCallIdentity(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceGroupMembers(ctx, nil, org.ID, group.ID, []string{user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateGroupSCIM(ctx, nil, org.ID, group.ID, "renamed", "external-id"); err != nil {
		t.Fatal(err)
	}
	proof, err := st.CallIdentity(ctx, user.ID, org.Slug)
	if err != nil || !slices.Contains(proof.Groups, "renamed") {
		t.Fatalf("SCIM rename missing: %+v %v", proof, err)
	}
	if _, err := st.DeleteGroup(ctx, nil, org.ID, group.ID); err != nil {
		t.Fatal(err)
	}
	dynamic, err := st.CreateGroup(ctx, nil, org.ID, "dynamic", "")
	if err != nil {
		t.Fatal(err)
	}
	rule := domain.Rule{Match: "all", Conditions: []domain.Condition{{Attribute: domain.AttrDepartment, Op: domain.OpEquals, Value: "Sales"}}}
	if _, err := st.SetGroupRule(ctx, nil, org.ID, dynamic.ID, rule); err != nil {
		t.Fatal(err)
	}
	if err := st.InTx(ctx, func(q pg.Querier) error {
		if _, err := st.SetUserAttributes(ctx, q, user.ID, "Sales", "", ""); err != nil {
			return err
		}
		return st.ReevaluateUserDynamicGroups(ctx, q, org.ID, user.ID)
	}); err != nil {
		t.Fatal(err)
	}
	proof, err = st.CallIdentity(ctx, user.ID, org.Slug)
	if err != nil || !slices.Contains(proof.Groups, "dynamic") {
		t.Fatalf("dynamic addition missing: %+v %v", proof, err)
	}
	revoker.err = errors.New("calls unavailable")
	if err := st.InTx(ctx, func(q pg.Querier) error {
		if _, err := st.SetUserAttributes(ctx, q, user.ID, "Engineering", "", ""); err != nil {
			return err
		}
		return st.ReevaluateUserDynamicGroups(ctx, q, org.ID, user.ID)
	}); err == nil {
		t.Fatal("attribute update bypassed dynamic member revocation")
	}
	revoker.err = nil
	role, err := st.CreateCustomRole(ctx, nil, org.ID, "Support", []string{"users.manage"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AssignCustomRole(ctx, nil, org.ID, role.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateCustomRole(ctx, nil, org.ID, role.ID, "Support renamed", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UnassignCustomRole(ctx, nil, org.ID, role.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AssignCustomRole(ctx, nil, org.ID, role.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteCustomRole(ctx, org.ID, role.ID); err != nil {
		t.Fatal(err)
	}
	if len(revoker.versions) < 9 {
		t.Fatalf("identity writers did not revoke: %v", revoker.versions)
	}
}

func TestCallIdentitySQLImporterCannotBypassRevocation(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "import-current@acme.example")
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "import-calls", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	revoker := &identityCallRevoker{}
	if err := st.ConfigureCallIdentity(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(ctx, `UPDATE memberships SET role='admin' WHERE org_id=$1 AND user_id=$2`, org.ID, user.ID); err == nil {
		t.Fatal("raw SQL changed effective role without closing calls")
	}
	member, err := st.GetMembership(ctx, org.ID, user.ID)
	if err != nil || member.Role != domain.RoleMember {
		t.Fatalf("import did not roll back: %+v %v", member, err)
	}
}

func TestCallIdentityOrganizationMergePreservesBothScopeFences(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "merge-call-identity@acme.example")
	source, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "identity-source", Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "identity-target", Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: source.ID, UserID: user.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatal(err)
	}
	revoker := &identityCallRevoker{}
	if err := st.ConfigureCallIdentity(ctx, true, revoker); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MergeIdentity(ctx, source.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	if revoker.minimum[user.ID+"/"+source.Slug] <= 0 || revoker.minimum[user.ID+"/"+target.Slug] <= 0 {
		t.Fatalf("merge skipped an identity scope: %+v", revoker.minimum)
	}
	old, err := st.CallIdentity(ctx, user.ID, source.Slug)
	if err != nil || old.Member || old.Version == 0 {
		t.Fatalf("source tombstone disappeared: %+v %v", old, err)
	}
	current, err := st.CallIdentity(ctx, user.ID, target.Slug)
	if err != nil || !current.Member || !current.OrganizationAdmin || current.Version == 0 {
		t.Fatalf("destination identity missing: %+v %v", current, err)
	}
}
