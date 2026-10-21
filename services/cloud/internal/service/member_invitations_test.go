// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/identity"
)

type fakeIdentityDirectory struct {
	member      identity.Member
	found       bool
	expiresAt   time.Time
	resolveErr  error
	inviteErr   error
	revokeErr   error
	invitedMail string
	invitedRole string
	revokedMail string
}

func (f *fakeIdentityDirectory) ResolveMember(context.Context, string, string) (identity.Member, bool, error) {
	return f.member, f.found, f.resolveErr
}

func (f *fakeIdentityDirectory) Invite(_ context.Context, _, email, role string) (time.Time, error) {
	f.invitedMail, f.invitedRole = email, role
	return f.expiresAt, f.inviteErr
}

func (f *fakeIdentityDirectory) RevokeInvitation(_ context.Context, _, email string) error {
	f.revokedMail = email
	return f.revokeErr
}

func invitationTestContext(t *testing.T) context.Context {
	t.Helper()
	tenant, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	return tenancy.Into(context.Background(), tenant)
}

func TestInviteMemberExistingIdentityMemberMaterializesSeat(t *testing.T) {
	fs := &fakeStore{membership: domain.Membership{Sub: "user-1", Role: domain.RoleMember}}
	id := &fakeIdentityDirectory{
		member: identity.Member{Sub: "user-1", Role: "member"}, found: true,
	}
	svc := &Service{store: fs, identity: id}

	got, err := svc.InviteMember(invitationTestContext(t), "jwt", " Ada@Example.eu ", domain.RoleAdmin, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Member == nil || got.Invitation != nil || fs.upsertSub != "user-1" || fs.upsertRole != domain.RoleAdmin {
		t.Fatalf("resultado=%+v sub=%q role=%q", got, fs.upsertSub, fs.upsertRole)
	}
	if id.invitedMail != "" {
		t.Fatal("un miembro existente no debe recibir otra invitación")
	}
}

func TestInviteMemberPendingDoesNotMaterializeSeat(t *testing.T) {
	expires := time.Now().Add(48 * time.Hour).UTC()
	inv := domain.MemberInvitation{ID: uuid.New(), Email: "new@example.eu", Role: domain.RoleMember, ExpiresAt: expires}
	fs := &fakeStore{invitation: inv}
	id := &fakeIdentityDirectory{expiresAt: expires}
	svc := &Service{store: fs, identity: id}

	got, err := svc.InviteMember(invitationTestContext(t), "jwt", "New@Example.eu", domain.RoleMember, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Invitation == nil || got.Member != nil {
		t.Fatalf("resultado=%+v", got)
	}
	if fs.upsertSub != "" {
		t.Fatalf("una invitación pendiente creó puesto para %q", fs.upsertSub)
	}
	if id.invitedMail != "new@example.eu" || id.invitedRole != "member" {
		t.Fatalf("invite=(%q,%q)", id.invitedMail, id.invitedRole)
	}
}

func TestInviteMemberIdentityFailureIsUnavailable(t *testing.T) {
	fs := &fakeStore{}
	id := &fakeIdentityDirectory{resolveErr: errors.New("network down")}
	svc := &Service{store: fs, identity: id}
	_, err := svc.InviteMember(invitationTestContext(t), "jwt", "new@example.eu", domain.RoleMember, "owner-1")
	if !errors.Is(err, domain.ErrIdentityUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestAcceptPendingMemberMaterializesOnce(t *testing.T) {
	fs := &fakeStore{
		accepted:   true,
		membership: domain.Membership{Sub: "user-1", Role: domain.RoleMember},
	}
	svc := &Service{store: fs}
	accepted, err := svc.AcceptPendingMember(invitationTestContext(t), "user-1", "new@example.eu")
	if err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v", accepted, err)
	}
}
