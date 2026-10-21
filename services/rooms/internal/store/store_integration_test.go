// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de Sytue Rooms contra un PostgreSQL real (efimero, via
// platform/pkg/pgtest sobre testcontainers-go). Detras del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - que las migraciones aplican (esquema + RLS),
//   - flujo team/canal/mensaje/hilo/reaccion,
//   - AUTORIZACION por membresia de canal privado,
//   - PAGINACION KEYSET de mensajes (ListMessages before),
//   - AISLAMIENTO por tenant (RLS).
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// comoAdmin es la sesión de `sub` con el rol `admin` de la organización del contexto
// en su token: lo que decide quién administra el espacio (bots, comandos, WhatsApp,
// salientes sin canal). Ser admin de un equipo no basta.
func comoAdmin(ctx context.Context, sub string) context.Context {
	return authn.Into(ctx, authn.Claims{Subject: sub, Tenant: tenancy.MustFrom(ctx).Slug, Roles: []string{"admin"}})
}

// sinRol es la sesión de `sub` sin rol en la organización: un miembro cualquiera, o
// alguien que llega de otra organización (EnAnfitrion le quita los roles).
func sinRol(ctx context.Context, sub string) context.Context {
	return authn.Into(ctx, authn.Claims{Subject: sub, Tenant: tenancy.MustFrom(ctx).Slug})
}

// seedTeamChannel crea un team y un canal, con el creador como admin. El slug es
// "canal" y no "general" a propósito: CreateTeam siembra ya los canales por defecto
// del team (#general y #aleatorio), así que reusar su slug daría ErrConflict.
func seedTeamChannel(t *testing.T, ctx context.Context, st *store.Store, creator string, kind domain.ChannelKind) (domain.Team, domain.Channel) {
	t.Helper()
	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: creator, CreatorName: "Creador",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "canal", Name: "Canal", Kind: kind, CreatedBy: creator,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	return tm, ch
}

func TestChatFlowAndThread(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	root, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "hola equipo",
	})
	if err != nil {
		t.Fatalf("CreateMessage raiz: %v", err)
	}
	// Respuesta en hilo.
	reply, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "respuesta",
	})
	if err != nil {
		t.Fatalf("CreateMessage respuesta: %v", err)
	}

	// Reaccion + agregacion al leer el hilo.
	if _, err := st.AddReaction(ctx, root.ID, "alice", ":tada:"); err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	gotRoot, replies, err := st.ListThread(ctx, root.ID, "alice")
	if err != nil {
		t.Fatalf("ListThread: %v", err)
	}
	if gotRoot.ReplyCount != 1 {
		t.Fatalf("reply_count=%d, quiero 1", gotRoot.ReplyCount)
	}
	if len(replies) != 1 || replies[0].ID != reply.ID {
		t.Fatalf("respuestas inesperadas: %+v", replies)
	}
	if len(gotRoot.Reactions) != 1 || gotRoot.Reactions[0].Emoji != ":tada:" || gotRoot.Reactions[0].Count != 1 {
		t.Fatalf("reacciones inesperadas: %+v", gotRoot.Reactions)
	}

	// Feed de canal (solo raices).
	msgs, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != root.ID {
		t.Fatalf("el feed debe listar solo raices: %+v", msgs)
	}
}

// TestPrivateChannelAuthorization comprueba que un no-miembro no puede leer ni
// escribir en un canal privado (ErrForbidden), y que un miembro sembrado si.
func TestPrivateChannelAuthorization(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{Slug: "eq", Name: "Equipo", CreatedBy: "owner", CreatorName: "Owner"})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	// bob es miembro del team pero no del canal privado.
	if err := st.AddTeamMember(ctx, tm.ID, "owner", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate, CreatedBy: "owner",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	if _, err := st.GetChannel(ctx, ch.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob no-miembro debe recibir ErrForbidden, got %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "bob", Body: "intruso"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob no-miembro no debe escribir, got %v", err)
	}
	// El owner (miembro admin del canal) si.
	if _, err := st.GetChannel(ctx, ch.ID, "owner"); err != nil {
		t.Fatalf("owner debe acceder: %v", err)
	}
}

// TestMessageKeysetPagination verifica que la paginacion con `before` recorre el
// feed completo sin repetir mensajes.
func TestMessageKeysetPagination(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	const total = 12
	for i := 0; i < total; i++ {
		if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
			ChannelID: ch.ID, AuthorSub: "alice", Body: "m",
		}); err != nil {
			t.Fatalf("CreateMessage %d: %v", i, err)
		}
	}

	seen := map[string]bool{}
	page := store.MessagePage{Limit: 5}
	iters := 0
	for {
		msgs, err := st.ListMessages(ctx, ch.ID, "alice", page)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			if seen[m.ID.String()] {
				t.Fatalf("mensaje repetido en paginacion: %s", m.ID)
			}
			seen[m.ID.String()] = true
		}
		last := msgs[len(msgs)-1].CreatedAt
		page.Before = &last
		iters++
		if iters > 50 {
			t.Fatal("paginacion no termina")
		}
	}
	if len(seen) != total {
		t.Fatalf("recorridos %d mensajes, quiero %d", len(seen), total)
	}
}

// TestListOwnMessages: la exportación de datos recorre, por páginas y en orden,
// SOLO lo que escribió la persona —respuestas de hilo incluidas— y nada de los demás.
func TestListOwnMessages(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	if _, err := st.AddChannelMember(ctx, ch.ID, "alice", "bob", "Bob"); err != nil {
		t.Fatalf("AddChannelMember: %v", err)
	}
	raiz, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "raíz"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "m"}); err != nil {
			t.Fatalf("CreateMessage %d: %v", i, err)
		}
		if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "bob", Body: "ajeno"}); err != nil {
			t.Fatalf("CreateMessage bob %d: %v", i, err)
		}
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, ParentID: &raiz.ID, AuthorSub: "alice", Body: "en el hilo",
	}); err != nil {
		t.Fatalf("respuesta: %v", err)
	}

	var (
		vistos  []domain.Message
		desde   time.Time
		desdeID uuid.UUID
	)
	for vueltas := 0; ; vueltas++ {
		pagina, err := st.ListOwnMessages(ctx, "alice", desde, desdeID, 2)
		if err != nil {
			t.Fatalf("ListOwnMessages: %v", err)
		}
		vistos = append(vistos, pagina...)
		if len(pagina) < 2 || vueltas > 20 {
			break
		}
		ultimo := pagina[len(pagina)-1]
		desde, desdeID = ultimo.CreatedAt, ultimo.ID
	}
	if len(vistos) != 6 {
		t.Fatalf("mensajes propios = %d, quiero 6 (5 del canal y la respuesta)", len(vistos))
	}
	for i, m := range vistos {
		if m.AuthorSub != "alice" {
			t.Fatalf("se coló un mensaje de %s", m.AuthorSub)
		}
		if i > 0 && m.CreatedAt.Before(vistos[i-1].CreatedAt) {
			t.Fatal("las páginas deben ir en orden cronológico")
		}
	}
}

func TestChatTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	_, chA := seedTeamChannel(t, ctxA, st, "alice", domain.ChannelPublic)
	msgA, err := st.CreateMessage(ctxA, store.NewMessageRecord{ChannelID: chA.ID, AuthorSub: "alice", Body: "secreto de A"})
	if err != nil {
		t.Fatalf("A CreateMessage: %v", err)
	}

	// Tenant B no ve el canal de A por su ID (RLS -> ErrNotFound).
	if _, err := st.GetChannel(ctxB, chA.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el canal de A (err=%v)", err)
	}
	// Tenant B no ve el mensaje de A (ChannelOfMessage -> ErrNotFound).
	if _, err := st.ChannelOfMessage(ctxB, msgA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el mensaje de A (err=%v)", err)
	}
}

// TestUnreadMentions verifica que UnreadByChannel separa el contador de menciones
// (@sub) del de no leídos, y que MarkRead pone ambos a cero para el usuario.
func TestUnreadMentions(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	// alice publica un mensaje normal y otro que menciona a bob.
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "hola a todos"}); err != nil {
		t.Fatalf("CreateMessage normal: %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", Body: "@bob revisa esto", Mentions: []string{"bob"},
	}); err != nil {
		t.Fatalf("CreateMessage mención: %v", err)
	}
	// @channel: menciona a todo el canal vía flag (sin fila por miembro). Cuenta
	// como mención para bob igual que @usuario.
	mention, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", Body: "@channel reunión ya", MentionsAll: true,
	})
	if err != nil {
		t.Fatalf("CreateMessage @channel: %v", err)
	}

	states, err := st.UnreadByChannel(ctx, tm.ID, "bob")
	if err != nil {
		t.Fatalf("UnreadByChannel: %v", err)
	}
	rs := findState(t, states, ch.ID)
	if rs.Unread != 3 || rs.Mentions != 2 {
		t.Fatalf("bob: unread=%d mentions=%d, quiero 3 y 2 (@bob + @channel)", rs.Unread, rs.Mentions)
	}

	// La mención NO cuenta para alice (autora): se excluye author_sub y su @.
	aliceStates, err := st.UnreadByChannel(ctx, tm.ID, "alice")
	if err != nil {
		t.Fatalf("UnreadByChannel alice: %v", err)
	}
	if a := findState(t, aliceStates, ch.ID); a.Unread != 0 || a.Mentions != 0 {
		t.Fatalf("alice (autora): unread=%d mentions=%d, quiero 0 y 0", a.Unread, a.Mentions)
	}

	// Tras marcar leído hasta el último mensaje, ambos contadores caen a cero.
	if err := st.MarkRead(ctx, ch.ID, "bob", &mention.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	states, err = st.UnreadByChannel(ctx, tm.ID, "bob")
	if err != nil {
		t.Fatalf("UnreadByChannel tras MarkRead: %v", err)
	}
	if rs := findState(t, states, ch.ID); rs.Unread != 0 || rs.Mentions != 0 {
		t.Fatalf("tras MarkRead: unread=%d mentions=%d, quiero 0 y 0", rs.Unread, rs.Mentions)
	}
}

// TestListTeamsDevuelveElRolDeQuienPregunta fija el contrato del que depende la interfaz
// para decidir qué le enseña a cada cual: `role` NO es una propiedad del team, es el rol
// del SUJETO QUE CONSULTA. Con dos miembros de roles distintos en el mismo team, un
// SELECT que perdiera el filtro por sujeto devolvería el mismo rol a los dos y el menú de
// administración le saldría a quien no puede usarlo. Los tests de la capa HTTP no pueden
// cazarlo: usan un fakeService que devuelve lo que se le diga.
func TestListTeamsDevuelveElRolDeQuienPregunta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	// CreateTeam mete al creador como admin; bob entra como miembro raso.
	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alice",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	for _, c := range []struct{ subject, quiere string }{
		{"alice", string(domain.RoleAdmin)},
		{"bob", string(domain.RoleMember)},
	} {
		teams, err := st.ListTeams(ctx, c.subject)
		if err != nil {
			t.Fatalf("ListTeams(%q): %v", c.subject, err)
		}
		if len(teams) != 1 {
			t.Fatalf("%q ve %d teams, quiero 1", c.subject, len(teams))
		}
		if teams[0].Role != c.quiere {
			t.Errorf("%q ve role=%q, quiero %q", c.subject, teams[0].Role, c.quiere)
		}
	}
}

// findState localiza el ReadState de un canal en la lista o falla el test.
func findState(t *testing.T, states []domain.ReadState, channelID uuid.UUID) domain.ReadState {
	t.Helper()
	for _, s := range states {
		if s.ChannelID == channelID {
			return s
		}
	}
	t.Fatalf("no hay ReadState para el canal %s", channelID)
	return domain.ReadState{}
}
