// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// COM-01. «Administrador del espacio» era ser admin de ALGÚN equipo, y un equipo lo
// crea cualquiera quedando como su admin. Con eso, un miembro sin rol daba de alta un
// webhook saliente sin canal y recibía los mensajes de todos los canales de la
// organización, privados y directos incluidos.
func TestCrearUnEquipoNoDaElEspacio(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	acme := tenantCtx("acme")

	// Mallory, miembro sin rol, se crea un equipo: es admin de ESE equipo y de nada más.
	propio, err := st.CreateTeam(acme, store.NewTeamRecord{Slug: "mio", Name: "Mío", CreatedBy: "mallory", CreatorName: "Mallory"})
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _ := domain.NewBotToken()
	sesiones := map[string]context.Context{
		"miembro sin rol, admin de su equipo": sinRol(acme, "mallory"),
		"sin sesión (un proceso, un bot)":     acme,
		"admin de OTRA organización":          authn.Into(acme, authn.Claims{Subject: "mallory", Tenant: "globex", Roles: []string{"owner"}}),
		"la sesión de otra persona con rol":   comoAdmin(acme, "olga"),
	}
	for nombre, ctx := range sesiones {
		t.Run(nombre, func(t *testing.T) {
			if _, err := st.CreateOutgoingWebhook(ctx, nil, "mallory", "https://atacante.example/h", "tok", "sytue", "c2VjcmV0bw==", []string{"hola"}); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("crea un saliente de toda la organización (err=%v)", err)
			}
			if _, err := st.CreateBot(ctx, "Espía", hash, "mallory"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("crea un bot (err=%v)", err)
			}
			if _, err := st.ListBots(ctx, "mallory"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("lista los bots (err=%v)", err)
			}
			if _, err := st.CreateSlashCommand(ctx, domain.SlashCommand{Name: "nominas", RequestURL: "https://atacante.example/c", Token: "t", SigningSecret: "c2VjcmV0bw=="}, "mallory"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("crea un comando (err=%v)", err)
			}
			if _, err := st.CreateWhatsAppAccount(ctx, domain.WhatsAppAccount{
				TeamID: propio.ID, PhoneNumberID: "123", AccessToken: "a", AppSecret: "s", CreatedBy: "mallory",
			}, "hash-wa"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("conecta un número de WhatsApp (err=%v)", err)
			}
			if admin, _ := st.IsWorkspaceAdmin(ctx, "mallory"); admin {
				t.Error("cuenta como administradora del espacio")
			}
		})
	}

	// Quien tiene el rol en la organización sí, sin ser admin de ningún equipo.
	olga := comoAdmin(acme, "olga")
	if _, err := st.CreateOutgoingWebhook(olga, nil, "olga", "https://bot.example/h", "tok", "sytue", "c2VjcmV0bw==", []string{"deploy"}); err != nil {
		t.Fatalf("la admin de la organización no puede crear un saliente sin canal: %v", err)
	}
	if _, err := st.CreateBot(olga, "Deployer", hash, "olga"); err != nil {
		t.Fatalf("la admin de la organización no puede crear un bot: %v", err)
	}

	// Y el saliente de toda la organización no lo ve en su lista quien no lo gestiona.
	if l, err := st.ListOutgoingWebhooks(sinRol(acme, "mallory"), "mallory"); err != nil || len(l) != 0 {
		t.Fatalf("mallory ve %d salientes de la organización (%v)", len(l), err)
	}
	if l, err := st.ListOutgoingWebhooks(olga, "olga"); err != nil || len(l) != 1 {
		t.Fatalf("olga ve %d salientes, quiero 1 (%v)", len(l), err)
	}
}

// Un saliente sin canal escucha los canales PÚBLICOS: un privado o un mensaje directo
// no salen de la organización porque alguien haya dado de alta un webhook «general».
func TestElSalienteSinCanalNoEscuchaPrivadosNiDirectos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	acme := tenantCtx("acme")
	tm, publico := seedTeamChannel(t, acme, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(acme, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatal(err)
	}
	privado, err := st.CreateChannel(acme, store.NewChannelRecord{TeamID: tm.ID, Slug: "direccion", Name: "Dirección", Kind: domain.ChannelPrivate, CreatedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	directo, err := st.OpenDirect(acme, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatal(err)
	}

	olga := comoAdmin(acme, "olga")
	general, err := st.CreateOutgoingWebhook(olga, nil, "olga", "https://bot.example/h", "tok", "sytue", "c2VjcmV0bw==", []string{"deploy"})
	if err != nil {
		t.Fatal(err)
	}
	// Uno atado al canal privado, dado de alta por quien administra ESE canal: ese sí.
	delPrivado, err := st.CreateOutgoingWebhook(acme, &privado.ID, "alice", "https://bot.example/p", "tok", "sytue", "c2VjcmV0bw==", []string{})
	if err != nil {
		t.Fatal(err)
	}

	ids := func(ws []domain.OutgoingWebhook) []string {
		out := make([]string, 0, len(ws))
		for _, w := range ws {
			out = append(out, w.ID.String())
		}
		return out
	}
	if c, err := st.OutgoingWebhooksForChannel(acme, publico.ID); err != nil || len(c) != 1 || c[0].ID != general.ID {
		t.Fatalf("canal público: candidatos %v (%v), quiero el general", ids(c), err)
	}
	if c, err := st.OutgoingWebhooksForChannel(acme, privado.ID); err != nil || len(c) != 1 || c[0].ID != delPrivado.ID {
		t.Fatalf("canal privado: candidatos %v (%v), quiero solo el suyo", ids(c), err)
	}
	if c, err := st.OutgoingWebhooksForChannel(acme, directo.ID); err != nil || len(c) != 0 {
		t.Fatalf("mensaje directo: candidatos %v (%v), quiero ninguno", ids(c), err)
	}
}
