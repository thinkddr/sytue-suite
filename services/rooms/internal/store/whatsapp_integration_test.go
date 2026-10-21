// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Un número conectado guarda sus secretos cifrados, se encuentra por el token de la
// URL sin tenant, y cada cliente tiene UN canal —con el equipo dentro— que se reusa
// hasta que se archiva.
func TestWhatsAppAccountAndChats(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := comoAdmin(tenantCtx("acme"), "alice")
	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatal(err)
	}

	plain, hash, _ := domain.NewOpaqueToken()
	nueva := domain.WhatsAppAccount{
		TeamID: tm.ID, PhoneNumberID: "1234567890", DisplayPhone: "+34 910 00 00 00", VerifiedName: "Acme",
		AccessToken: "EAAG-secreto", AppSecret: "app-secreto", CreatedBy: "alice",
	}
	nuevaBob := nueva
	nuevaBob.CreatedBy = "bob"
	if _, err := st.CreateWhatsAppAccount(ctx, nuevaBob, hash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un miembro normal no conecta números: %v", err)
	}
	cuenta, err := st.CreateWhatsAppAccount(ctx, nueva, hash)
	if err != nil {
		t.Fatalf("CreateWhatsAppAccount: %v", err)
	}
	if _, err := st.CreateWhatsAppAccount(ctx, nueva, "otro-hash"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("el mismo número dos veces: %v", err)
	}
	var token, secreto string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT access_token, app_secret FROM whatsapp_accounts WHERE id=$1`, cuenta.ID).Scan(&token, &secreto)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "secreto") || strings.Contains(secreto, "secreto") || !strings.HasPrefix(token, "encv1:") {
		t.Fatalf("los secretos de WhatsApp están en claro: %q %q", token, secreto)
	}

	// El aviso de Meta llega sin tenant.
	halla, err := st.LookupWhatsAppAccountByTokenHash(context.Background(), domain.HashToken(plain))
	if err != nil || halla.Tenant != "acme" || halla.AppSecret != "app-secreto" {
		t.Fatalf("lookup = %+v, %v", halla, err)
	}

	hora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	chat, err := st.EnsureWhatsAppChat(ctx, halla, "34600111222", "Ana García", hora)
	if err != nil {
		t.Fatalf("EnsureWhatsAppChat: %v", err)
	}
	canal, err := st.GetChannel(ctx, chat.ChannelID, "bob")
	if err != nil {
		t.Fatalf("bob, del equipo, no ve el canal del cliente: %v", err)
	}
	if canal.Name != "WhatsApp · Ana García" || canal.Kind != domain.ChannelPublic {
		t.Fatalf("canal = %+v", canal)
	}
	miembros, err := st.ListChannelMembers(ctx, chat.ChannelID, "alice")
	if err != nil || len(miembros) != 2 {
		t.Fatalf("el equipo entero entra en el canal: %d, %v", len(miembros), err)
	}
	otra, err := st.EnsureWhatsAppChat(ctx, halla, "34600111222", "", hora.Add(time.Minute))
	if err != nil || otra.ChannelID != chat.ChannelID {
		t.Fatalf("el segundo mensaje reusa el canal: %+v, %v", otra, err)
	}

	// La salida: del canal a la cuenta, con el token en claro.
	c, a, err := st.WhatsAppChatForChannel(ctx, chat.ChannelID)
	if err != nil || c.WaID != "34600111222" || a.AccessToken != "EAAG-secreto" {
		t.Fatalf("WhatsAppChatForChannel = %+v %+v %v", c, a, err)
	}
	if _, _, err := st.WhatsAppChatForChannel(ctx, tm.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("un canal que no es de un cliente: %v", err)
	}

	// Archivado el canal, el cliente que vuelve a escribir abre uno nuevo.
	if _, err := st.SetChannelArchivedAsAdmin(ctx, chat.ChannelID, true); err != nil {
		t.Fatal(err)
	}
	reabierta, err := st.EnsureWhatsAppChat(ctx, halla, "34600111222", "Ana", hora.Add(time.Hour))
	if err != nil || reabierta.ChannelID == chat.ChannelID {
		t.Fatalf("tras archivar: %+v, %v", reabierta, err)
	}

	if visto, _ := st.WhatsAppSeen(ctx, "wamid.A"); visto {
		t.Fatal("nada visto aún")
	}
	if err := st.MarkWhatsAppSeen(ctx, "wamid.A"); err != nil {
		t.Fatal(err)
	}
	if visto, _ := st.WhatsAppSeen(ctx, "wamid.A"); !visto {
		t.Fatal("el mensaje anotado no consta como visto")
	}

	if err := st.DeleteWhatsAppAccount(ctx, cuenta.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un miembro normal no desconecta: %v", err)
	}
	if err := st.DeleteWhatsAppAccount(ctx, cuenta.ID, "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestWhatsAppPendingSetupLease(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := comoAdmin(tenantCtx("acme"), "alice")
	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	a := domain.WhatsAppAccount{
		TeamID: tm.ID, PhoneNumberID: "1234567890", InstanceID: "consent-instance",
		AccessToken: "token", AppSecret: "secret", CreatedBy: "alice",
	}
	first, err := st.PrepareWhatsAppAccount(ctx, a, "first-hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareWhatsAppAccount(ctx, a, "concurrent-hash"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("an in-flight setup must be fenced: %v", err)
	}
	if listed, err := st.ListWhatsAppAccounts(ctx, "alice"); err != nil || len(listed) != 0 {
		t.Fatalf("pending setup became visible: %+v, %v", listed, err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE whatsapp_accounts SET created_at=now()-interval '3 minutes' WHERE id=$1`, first.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retry, err := st.PrepareWhatsAppAccount(ctx, a, "retry-hash")
	if err != nil || retry.ID == first.ID {
		t.Fatalf("retry must own a new setup ID: %+v, %v", retry, err)
	}
	if err := st.AbortWhatsAppAccount(ctx, first.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinishWhatsAppAccount(ctx, first.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("late completion must not activate the retry: %v", err)
	}
	if current, err := st.LookupWhatsAppAccountByTokenHash(context.Background(), "retry-hash"); err != nil || current.ID != retry.ID || current.Ready {
		t.Fatalf("old abort removed or activated current setup: %+v, %v", current, err)
	}
	active, err := st.FinishWhatsAppAccount(ctx, retry.ID, "alice")
	if err != nil || !active.Ready {
		t.Fatalf("current completion failed: %+v, %v", active, err)
	}
	if _, err := st.PrepareWhatsAppAccount(ctx, a, "replacement-hash"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("an active number must not be replaced: %v", err)
	}
}
