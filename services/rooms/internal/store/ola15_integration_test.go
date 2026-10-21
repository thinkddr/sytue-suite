// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 15 (Integraciones) contra PostgreSQL real (pgtest):
// webhooks entrantes (alta, lookup cross-tenant por hash, publicación sin membresía),
// salientes (CRUD + candidatos por canal) y bots (CRUD, lookup por hash, membresía de
// canal). Verifica que el token se guarda HASHEADO (nunca en claro) y el aislamiento
// por tenant. Detrás del build-tag `integration`. Sigue el patrón de ola14.
package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestIncomingWebhookLifecycleAndTokenLookup(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	plain, hash, _ := domain.NewOpaqueToken()
	w, err := st.CreateIncomingWebhook(ctx, ch.ID, "alice", "CI", "", hash)
	if err != nil {
		t.Fatalf("CreateIncomingWebhook: %v", err)
	}
	if w.ChannelID != ch.ID || w.Name != "CI" {
		t.Fatalf("webhook inesperado: %+v", w)
	}

	// El token NO se guarda en claro: la columna es el hash.
	assertTokenHashed(t, h, "incoming_webhooks", plain, hash)

	// Lookup por hash del token VÁLIDO, CROSS-TENANT (sin tenant en el contexto).
	got, err := st.LookupIncomingWebhookByTokenHash(context.Background(), domain.HashToken(plain))
	if err != nil {
		t.Fatalf("LookupIncomingWebhookByTokenHash (válido): %v", err)
	}
	if got.ID != w.ID || got.Tenant != "acme" || got.ChannelID != ch.ID {
		t.Fatalf("lookup devolvió %+v", got)
	}

	// Token INVÁLIDO ⇒ ErrNotFound.
	if _, err := st.LookupIncomingWebhookByTokenHash(context.Background(), domain.HashToken("no-existe")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token inválido debe dar ErrNotFound, got %v", err)
	}

	// Publicar SIN membresía (el webhook es la autorización), fijando el tenant.
	tctx := tenantCtx(got.Tenant)
	msg, err := st.CreateHookMessage(tctx, got.ChannelID, domain.HookAuthorSub(got.ID), got.Name, "deploy ok")
	if err != nil {
		t.Fatalf("CreateHookMessage: %v", err)
	}
	if !msg.IsBot {
		t.Fatal("el mensaje del webhook debe marcarse IsBot (badge BOT)")
	}
	if msg.AuthorSub != domain.HookAuthorSub(got.ID) {
		t.Fatalf("autor sintético inesperado: %q", msg.AuthorSub)
	}

	// Rotar cambia el hash: el token viejo deja de valer.
	_, newHash, _ := domain.NewOpaqueToken()
	if _, err := st.RotateIncomingWebhook(ctx, w.ID, "alice", newHash); err != nil {
		t.Fatalf("RotateIncomingWebhook: %v", err)
	}
	if _, err := st.LookupIncomingWebhookByTokenHash(context.Background(), domain.HashToken(plain)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras rotar, el token viejo debe dar ErrNotFound, got %v", err)
	}

	// Borrado (admin del canal).
	if err := st.DeleteIncomingWebhook(ctx, w.ID, "alice"); err != nil {
		t.Fatalf("DeleteIncomingWebhook: %v", err)
	}
}

func TestIncomingWebhookAdminOnly(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	// bob es miembro normal del team (no admin del canal).
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	_, hash, _ := domain.NewOpaqueToken()
	if _, err := st.CreateIncomingWebhook(ctx, ch.ID, "bob", "x", "", hash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un no-admin no debe poder crear webhooks, got %v", err)
	}
}

func TestOutgoingWebhookCRUDAndCandidates(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	w, err := st.CreateOutgoingWebhook(ctx, &ch.ID, "alice", "https://example.com/hook", "vtok", "sytue", "c2VjcmV0bw==", []string{"deploy"})
	if err != nil {
		t.Fatalf("CreateOutgoingWebhook: %v", err)
	}
	if w.CallbackURL != "https://example.com/hook" || w.Token != "vtok" {
		t.Fatalf("saliente inesperado: %+v", w)
	}
	if w.Dialect != "sytue" {
		t.Fatalf("dialecto por defecto = %q, quiero sytue (mig. 0017)", w.Dialect)
	}
	// Sin secreto persistido no habría con qué firmar y el bot receptor rechazaría
	// cada entrega (mig. 0022).
	if w.SigningSecret != "c2VjcmV0bw==" {
		t.Fatalf("secreto de firma = %q", w.SigningSecret)
	}
	// Y en la BASE no está en claro: quien la lea no puede firmar en nuestro
	// nombre. Se mira la columna directamente, sin pasar por el store.
	// Dentro de una tx con tenant: `outgoing_webhooks` está bajo RLS y una lectura
	// suelta no ve ninguna fila (que es justo lo que debe pasar).
	var enBase string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT signing_secret FROM outgoing_webhooks WHERE id = $1`, w.ID).Scan(&enBase)
	}); err != nil {
		t.Fatalf("leer signing_secret: %v", err)
	}
	if enBase == "c2VjcmV0bw==" {
		t.Fatal("el secreto de firma está EN CLARO en la base")
	}
	if !strings.HasPrefix(enBase, "encv1:") {
		t.Fatalf("el secreto de firma no está sellado con keycrypto: %q", enBase)
	}

	list, err := st.ListOutgoingWebhooks(ctx, "alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListOutgoingWebhooks = %+v err=%v", list, err)
	}

	// Candidatos para el canal (los consume el dispatcher; filtra por palabra en Go).
	cands, err := st.OutgoingWebhooksForChannel(ctx, ch.ID)
	if err != nil || len(cands) != 1 {
		t.Fatalf("OutgoingWebhooksForChannel = %+v err=%v", cands, err)
	}
	other := uuid.New()
	if cands, _ := st.OutgoingWebhooksForChannel(ctx, other); len(cands) != 0 {
		t.Fatalf("un canal sin salientes no debe traer candidatos: %+v", cands)
	}

	if err := st.DeleteOutgoingWebhook(ctx, w.ID, "alice"); err != nil {
		t.Fatalf("DeleteOutgoingWebhook: %v", err)
	}
}

func TestBotLifecycleTokenAndChannelMembership(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := comoAdmin(tenantCtx("acme"), "alice") // alice: admin de la organización
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	plain, hash, _ := domain.NewBotToken()
	bot, err := st.CreateBot(ctx, "Deployer", hash, "alice")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	assertTokenHashed(t, h, "bots", plain, hash)

	// Lookup por hash del token, cross-tenant (auth de bot, sin JWT).
	got, err := st.LookupBotByTokenHash(context.Background(), domain.HashToken(plain))
	if err != nil || got.ID != bot.ID || got.Tenant != "acme" {
		t.Fatalf("LookupBotByTokenHash = %+v err=%v", got, err)
	}

	botSub := domain.BotAuthorSub(bot.ID)

	// Antes de añadirlo al canal, el bot NO puede publicar (sin membresía).
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: botSub, AuthorName: "Deployer", Body: "hola",
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un bot sin membresía no debe poder publicar, got %v", err)
	}

	// El admin lo añade al canal; ahora sí publica, y el mensaje se marca IsBot.
	if err := st.AddBotToChannel(ctx, bot.ID, ch.ID, "alice"); err != nil {
		t.Fatalf("AddBotToChannel: %v", err)
	}
	msg, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: botSub, AuthorName: "Deployer", Body: "desplegado",
	})
	if err != nil {
		t.Fatalf("bot CreateMessage tras añadirlo: %v", err)
	}
	if !msg.IsBot {
		t.Fatal("el mensaje del bot debe marcarse IsBot")
	}

	// Borrar el bot limpia su membresía y su token deja de valer.
	if err := st.DeleteBot(ctx, bot.ID, "alice"); err != nil {
		t.Fatalf("DeleteBot: %v", err)
	}
	if _, err := st.LookupBotByTokenHash(context.Background(), domain.HashToken(plain)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras borrar, el token del bot debe dar ErrNotFound, got %v", err)
	}
}

func TestIntegrationsTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme := tenantCtx("acme")
	globex := tenantCtx("globex")
	_, chA := seedTeamChannel(t, acme, st, "alice", domain.ChannelPublic)
	seedTeamChannel(t, globex, st, "gwen", domain.ChannelPublic)

	_, hash, _ := domain.NewOpaqueToken()
	if _, err := st.CreateIncomingWebhook(acme, chA.ID, "alice", "CI", "", hash); err != nil {
		t.Fatalf("CreateIncomingWebhook: %v", err)
	}
	// globex no ve los webhooks de acme (RLS por tenant).
	if list, _ := st.ListIncomingWebhooks(globex, chA.ID, "gwen"); len(list) != 0 {
		t.Fatalf("cross-tenant: globex no debe ver webhooks de acme, got %+v", list)
	}
}

// assertTokenHashed comprueba directamente en la BD (con maintenance) que la columna
// token_hash guarda el HASH y NUNCA el token en claro. El nombre de tabla es una
// constante del test (no input externo), así que interpolarlo es seguro.
func assertTokenHashed(t *testing.T, h *pgtest.Harness, table, plain, hash string) {
	t.Helper()
	ctx := context.Background()
	err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var byHash, byPlain int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE token_hash = $1`, hash).Scan(&byHash); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE token_hash = $1`, plain).Scan(&byPlain); err != nil {
			return err
		}
		if byHash == 0 {
			t.Fatalf("%s: no se encontró la fila por su hash", table)
		}
		if byPlain != 0 {
			t.Fatalf("%s: el token en CLARO no debe aparecer en token_hash", table)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("assertTokenHashed(%s): %v", table, err)
	}
}
