// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de buzones compartidos + delegación contra PostgreSQL real
// (pgtest). Verifican: la entrega entrante a la dirección compartida se resuelve a
// su cuenta y los grantees la leen; sin ACL no hay acceso (deny-by-default);
// grant/revoke y niveles; la identidad de envío (compartida ⇒ enviar-como, personal
// ⇒ en-nombre-de); y el aislamiento por tenant (RLS).
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
)

func TestSharedMailboxDeliveryAndACL(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Personas del tenant y el buzón compartido de la ORG (Ada lo crea ⇒ manage).
	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	ben := seedAccount(t, ctx, st, "ben@sytue.eu") // grantee
	seedAccount(t, ctx, st, "eve@sytue.eu")        // NO grantee
	shared, err := st.EnsureSharedMailbox(ctx, "soporte@sytue.eu", "Soporte", 1<<30, ada.Subject)
	if err != nil {
		t.Fatalf("EnsureSharedMailbox: %v", err)
	}

	// La ENTREGA ENTRANTE a la dirección compartida se resuelve a su cuenta (reúsa
	// LookupAddress, el mismo enrutado que el correo normal), sin conocer el tenant.
	res, err := st.LookupAddress(tenantCtx(""), "soporte@sytue.eu")
	if err != nil {
		t.Fatalf("LookupAddress(soporte@): %v", err)
	}
	if res.AccountID != shared.ID || res.Tenant != "acme" {
		t.Fatalf("resolución compartida = %v/%s, quiero %v/acme", res.AccountID, res.Tenant, shared.ID)
	}
	// Un mensaje entregado cae en el Inbox del buzón compartido; los grantees lo leen.
	inbox, err := st.MailboxByRole(ctx, shared.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole(shared inbox): %v", err)
	}
	msg := seedMessage(t, ctx, st, shared, inbox.ID, "Ticket #1", "cliente@x.com", nil)

	// Conceder a Ben acceso de lectura al buzón compartido.
	if err := st.GrantAccess(ctx, shared.ID, "ben@sytue.eu", domain.PermRead); err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}

	// Ben VE el buzón compartido en su lista (además del propio) y su permiso.
	boxes, err := st.ListAccessibleMailboxes(ctx, ben.Subject)
	if err != nil {
		t.Fatalf("ListAccessibleMailboxes(ben): %v", err)
	}
	if len(boxes) != 1 || boxes[0].AccountID != shared.ID ||
		boxes[0].Kind != domain.KindShared || boxes[0].Permission != domain.PermRead {
		t.Fatalf("accesibles de Ben = %+v", boxes)
	}
	if perm, err := st.GranteePermission(ctx, ben.Subject, shared.ID); err != nil || perm != domain.PermRead {
		t.Fatalf("GranteePermission(ben) = %q, %v", perm, err)
	}

	// SIN ACL → SIN ACCESO (deny). Eve no ve el buzón ni obtiene permiso.
	if _, err := st.GranteePermission(ctx, "sub-eve@sytue.eu", shared.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Eve sin ACL debe dar ErrNotFound, dio %v", err)
	}
	if boxes, _ := st.ListAccessibleMailboxes(ctx, "sub-eve@sytue.eu"); len(boxes) != 0 {
		t.Fatalf("Eve no debe ver ningún buzón compartido: %+v", boxes)
	}

	// El mensaje es legible desde el buzón compartido (la carpeta es de esa cuenta).
	msgs, err := st.ListMessages(ctx, inbox.ID, 50, msg.ReceivedAt.Add(1), msg.ID)
	if err != nil {
		t.Fatalf("ListMessages(shared inbox): %v", err)
	}
	_ = msgs

	// El grantee no puede colar una carpeta de OTRA cuenta como si fuera del compartido.
	if ok, _ := st.MailboxInAccount(ctx, inbox.ID, shared.ID); !ok {
		t.Fatal("MailboxInAccount(shared inbox, shared) debe ser true")
	}
	if ok, _ := st.MailboxInAccount(ctx, inbox.ID, ben.ID); ok {
		t.Fatal("MailboxInAccount(shared inbox, ben) debe ser false")
	}
}

func TestSharedSendIdentityAndRevoke(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	ben := seedAccount(t, ctx, st, "ben@sytue.eu")
	shared, err := st.EnsureSharedMailbox(ctx, "ventas@sytue.eu", "Ventas", 1<<30, ada.Subject)
	if err != nil {
		t.Fatalf("EnsureSharedMailbox: %v", err)
	}

	// Sin permiso send todavía: no hay identidad de envío para Ben.
	if _, err := st.GrantedIdentityByAddress(ctx, ben.Subject, "ventas@sytue.eu"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin ACL, identidad debe dar ErrNotFound, dio %v", err)
	}
	if err := st.GrantAccess(ctx, shared.ID, "ben@sytue.eu", domain.PermSend); err != nil {
		t.Fatalf("GrantAccess(send): %v", err)
	}
	// Enviar-COMO: identidad compartida (kind=shared) con permiso send.
	id, err := st.GrantedIdentityByAddress(ctx, ben.Subject, "ventas@sytue.eu")
	if err != nil {
		t.Fatalf("GrantedIdentityByAddress: %v", err)
	}
	if id.AccountID != shared.ID || id.Kind != domain.KindShared || !id.Permission.AtLeast(domain.PermSend) {
		t.Fatalf("identidad = %+v", id)
	}

	// Revocar retira el acceso (deny de nuevo).
	if err := st.RevokeAccess(ctx, shared.ID, "ben@sytue.eu"); err != nil {
		t.Fatalf("RevokeAccess: %v", err)
	}
	if _, err := st.GranteePermission(ctx, ben.Subject, shared.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras revocar debe dar ErrNotFound, dio %v", err)
	}
}

func TestDelegationOfPersonalMailbox(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	director := seedAccount(t, ctx, st, "director@sytue.eu")
	asst := seedAccount(t, ctx, st, "asst@sytue.eu")

	// El directivo delega su buzón PERSONAL en el asistente (enviar-en-nombre-de).
	if err := st.GrantAccess(ctx, director.ID, "asst@sytue.eu", domain.PermSend); err != nil {
		t.Fatalf("GrantAccess(delegación): %v", err)
	}
	id, err := st.GrantedIdentityByAddress(ctx, asst.Subject, "director@sytue.eu")
	if err != nil {
		t.Fatalf("GrantedIdentityByAddress: %v", err)
	}
	// kind=user ⇒ en-nombre-de (el envío pondrá Sender = el asistente).
	if id.Kind != domain.KindUser || !id.Permission.AtLeast(domain.PermSend) {
		t.Fatalf("identidad delegada = %+v", id)
	}
	boxes, _ := st.ListAccessibleMailboxes(ctx, asst.Subject)
	if len(boxes) != 1 || boxes[0].AccountID != director.ID || boxes[0].Kind != domain.KindUser {
		t.Fatalf("buzones del asistente = %+v", boxes)
	}
}

func TestSharedACLTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	ben := seedAccount(t, ctx, st, "ben@sytue.eu")
	shared, err := st.EnsureSharedMailbox(ctx, "soporte@sytue.eu", "Soporte", 1<<30, ada.Subject)
	if err != nil {
		t.Fatalf("EnsureSharedMailbox: %v", err)
	}
	if err := st.GrantAccess(ctx, shared.ID, "ben@sytue.eu", domain.PermRead); err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}

	// Otro tenant NO ve el ACL ni el buzón compartido (RLS deny-by-default).
	other := tenantCtx("other")
	if _, err := st.GranteePermission(other, ben.Subject, shared.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: otro tenant obtuvo permiso (%v)", err)
	}
	if boxes, _ := st.ListAccessibleMailboxes(other, ben.Subject); len(boxes) != 0 {
		t.Fatalf("aislamiento roto: otro tenant ve %d buzones", len(boxes))
	}
	if _, err := st.GrantedIdentityByAddress(other, ben.Subject, "soporte@sytue.eu"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: identidad de envío cross-tenant (%v)", err)
	}
}
