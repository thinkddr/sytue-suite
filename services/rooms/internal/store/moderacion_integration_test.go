// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Denuncias y bloqueos (ADR 0073) contra PostgreSQL real. Lo que se fija son las
// verjas: el bloqueo se aplica en el servidor y entre organizaciones; nadie
// bloquea ni denuncia lo que no comparte; y una denuncia solo la ve y la resuelve
// quien modera ESA conversación, nunca el denunciado ni otra organización.
package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// equipoDeTres deja un equipo con alice (admin), bob y carla, todos en un canal
// público.
func equipoDeTres(ctx context.Context, t *testing.T, st *store.Store) (domain.Team, domain.Channel) {
	t.Helper()
	tm, canal := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	for _, p := range []string{"bob", "carla"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", p, p, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember(%s): %v", p, err)
		}
		if _, err := st.JoinChannel(ctx, canal.ID, p, p); err != nil {
			t.Fatalf("JoinChannel(%s): %v", p, err)
		}
	}
	return tm, canal
}

func escribir(ctx context.Context, st *store.Store, canal uuid.UUID, autor, cuerpo string, menciones ...string) (domain.Message, error) {
	return st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: canal, AuthorSub: autor, AuthorName: autor, Body: cuerpo, Mentions: menciones,
	})
}

func TestBloqueo_DirectoAvisosYEntreOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme := tenantCtx("acme")
	tm, canal := equipoDeTres(acme, t, st)

	dm, err := st.OpenDirect(acme, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if _, err := escribir(acme, st, dm.ID, "alice", "antes del bloqueo"); err != nil {
		t.Fatalf("mensaje directo sin bloqueo: %v", err)
	}
	avisados := func(autor string, menciones ...string) int {
		t.Helper()
		rs, err := st.NotificationRecipientsForMessage(acme, canal.ID, autor, menciones)
		if err != nil {
			t.Fatalf("NotificationRecipientsForMessage: %v", err)
		}
		return len(rs)
	}
	if n := avisados("alice", "bob"); n != 1 {
		t.Fatalf("sin bloqueo, la mención a bob avisa a %d, quiero 1", n)
	}

	// No se bloquea a quien no se comparte: ni desde un canal que no se ve, ni a
	// alguien que no está en él, ni a uno mismo.
	if _, err := st.BlockMember(acme, dm.ID, "carla", "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("carla bloquea desde un directo que no es suyo: err=%v", err)
	}
	if _, err := st.BlockMember(acme, dm.ID, "bob", "carla"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob bloquea a alguien que no está en el canal: err=%v", err)
	}
	if _, err := st.BlockMember(acme, dm.ID, "bob", "bob"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("bob se bloquea a sí mismo: err=%v", err)
	}

	// Bob bloquea a Alice. Dos veces: es idempotente.
	for range 2 {
		if b, err := st.BlockMember(acme, dm.ID, "bob", "alice"); err != nil || b.Subject != "alice" {
			t.Fatalf("BlockMember: %+v (%v)", b, err)
		}
	}
	// El directo queda cerrado EN EL SERVIDOR, para los dos y con el mismo error.
	for _, autor := range []string{"alice", "bob"} {
		if _, err := escribir(acme, st, dm.ID, autor, "tras el bloqueo"); !errors.Is(err, domain.ErrBlocked) {
			t.Fatalf("%s escribe en un directo bloqueado: err=%v", autor, err)
		}
	}
	// Bloquear no borra nada: lo anterior sigue ahí.
	if ms, err := st.ListMessages(acme, dm.ID, "bob", store.MessagePage{}); err != nil || len(ms) != 1 {
		t.Fatalf("tras bloquear quedan %d mensajes (%v), quiero 1", len(ms), err)
	}
	// En el canal compartido Alice sigue escribiendo, pero ya no AVISA a Bob…
	if _, err := escribir(acme, st, canal.ID, "alice", "hola @bob", "bob"); err != nil {
		t.Fatalf("alice escribe en el canal compartido: %v", err)
	}
	if n := avisados("alice", "bob"); n != 0 {
		t.Fatalf("la mención de una bloqueada avisa a %d, quiero 0", n)
	}
	// …y el bloqueo es de un sentido para los avisos: Bob sí avisa a Alice.
	if n := avisados("bob", "alice"); n != 1 {
		t.Fatalf("la mención de bob a alice avisa a %d, quiero 1", n)
	}

	// La lista es de quien bloquea; el bloqueado no la ve ni la puede deshacer.
	if bs, _ := st.ListBlocks(acme, "bob"); len(bs) != 1 || bs[0].Subject != "alice" {
		t.Fatalf("ListBlocks(bob) = %+v", bs)
	}
	if bs, _ := st.ListBlocks(acme, "alice"); len(bs) != 0 {
		t.Fatalf("la bloqueada ve bloqueos que no son suyos: %+v", bs)
	}
	if err := st.Unblock(acme, "alice", "bob"); err != nil {
		t.Fatalf("Unblock ajeno: %v", err)
	}
	if _, err := escribir(acme, st, dm.ID, "alice", "x"); !errors.Is(err, domain.ErrBlocked) {
		t.Fatalf("la bloqueada se ha desbloqueado sola: err=%v", err)
	}

	// ENTRE ORGANIZACIONES: el mismo par, en un directo que aloja «otra», sigue
	// bloqueado; y desde «otra» Bob ve su bloqueo y lo quita.
	otra := tenantCtx("otra")
	tmOtra, err := st.CreateTeam(otra, store.NewTeamRecord{Slug: "eq", Name: "Equipo", CreatedBy: "bob", CreatorName: "Bob"})
	if err != nil {
		t.Fatalf("CreateTeam(otra): %v", err)
	}
	dmOtra, err := st.OpenGroupDirect(otra, tmOtra.ID, "bob", []string{"alice"}, map[string]string{"alice": "Alice"})
	if err != nil {
		t.Fatalf("OpenGroupDirect(otra): %v", err)
	}
	// Desde la 0041 alice entra al aceptar la invitación.
	if _, err := st.AceptarInvitacion(context.Background(), "alice", "acme", dmOtra.ID); err != nil {
		t.Fatalf("aceptar el directo: %v", err)
	}
	if _, err := escribir(otra, st, dmOtra.ID, "alice", "por la otra puerta"); !errors.Is(err, domain.ErrBlocked) {
		t.Fatalf("el bloqueo no cruza de organización: err=%v", err)
	}
	if bs, _ := st.ListBlocks(otra, "bob"); len(bs) != 1 {
		t.Fatalf("desde otra organización bob no ve su bloqueo: %+v", bs)
	}
	if err := st.Unblock(otra, "bob", "alice"); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if _, err := escribir(acme, st, dm.ID, "alice", "desbloqueada"); err != nil {
		t.Fatalf("tras desbloquear alice no escribe en acme: %v", err)
	}
	if _, err := escribir(otra, st, dmOtra.ID, "alice", "desbloqueada"); err != nil {
		t.Fatalf("tras desbloquear alice no escribe en otra: %v", err)
	}
}

func TestDenuncia_VerjasColaYResolucion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")
	_, canal := equipoDeTres(acme, t, st)

	m, err := escribir(acme, st, canal.ID, "bob", "mensaje denunciable")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	denuncia := store.NewReport{Reporter: "carla", MessageID: &m.ID, Reason: "harassment", Note: "  me insulta  "}

	// Quien no ve el canal no lo denuncia; desde otra organización ni existe.
	fuera := denuncia
	fuera.Reporter = "zoe"
	if _, err := st.CreateReport(acme, fuera); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("denuncia de quien no ve el canal: err=%v", err)
	}
	if _, err := st.CreateReport(otra, denuncia); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("denuncia desde otra organización: err=%v", err)
	}
	// Ni a uno mismo, ni con un motivo de fuera de la lista.
	propia := denuncia
	propia.Reporter = "bob"
	if _, err := st.CreateReport(acme, propia); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("bob se denuncia a sí mismo: err=%v", err)
	}
	mala := denuncia
	mala.Reason = "porque sí"
	if _, err := st.CreateReport(acme, mala); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("motivo fuera de la lista: err=%v", err)
	}

	// La buena: nueva, y le llega a la administradora del equipo (alice), no a
	// quien denuncia ni al denunciado.
	r1, err := st.CreateReport(acme, denuncia)
	if err != nil || !r1.Nueva || len(r1.Moderadores) != 1 || r1.Moderadores[0] != "alice" {
		t.Fatalf("CreateReport = %+v (%v)", r1, err)
	}
	// Repetida: la misma, sin avisar otra vez.
	r2, err := st.CreateReport(acme, denuncia)
	if err != nil || r2.Nueva || r2.ID != r1.ID || len(r2.Moderadores) != 0 {
		t.Fatalf("denuncia repetida = %+v (%v), quiero la misma sin avisos", r2, err)
	}

	cola := func(ctx context.Context, actor string, orgAdmin bool) []domain.Report {
		t.Helper()
		rs, err := st.ListReports(ctx, actor, orgAdmin, true)
		if err != nil {
			t.Fatalf("ListReports(%s): %v", actor, err)
		}
		return rs
	}
	// La ve la admin del equipo, con el texto en vivo y la nota recortada.
	if rs := cola(acme, "alice", false); len(rs) != 1 || rs[0].MessageBody != "mensaje denunciable" ||
		rs[0].Note != "me insulta" || rs[0].TargetSub != "bob" {
		t.Fatalf("cola de alice = %+v", rs)
	}
	// Y un admin de la ORGANIZACIÓN que no está en el equipo.
	if rs := cola(acme, "zoe", true); len(rs) != 1 {
		t.Fatalf("cola del admin de organización = %+v", rs)
	}
	// No la ve: quien denuncia (no modera), el DENUNCIADO aunque administre la
	// organización, ni el admin de otra organización.
	for _, c := range []struct {
		ctx      context.Context
		actor    string
		orgAdmin bool
	}{{acme, "carla", false}, {acme, "bob", true}, {otra, "zoe", true}} {
		if rs := cola(c.ctx, c.actor, c.orgAdmin); len(rs) != 0 {
			t.Fatalf("%s (orgAdmin=%v) ve denuncias que no le tocan: %+v", c.actor, c.orgAdmin, rs)
		}
		if _, err := st.ResolveReport(c.ctx, r1.ID, c.actor, c.orgAdmin, domain.ReportDismissed); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s (orgAdmin=%v) resuelve una denuncia que no le toca: err=%v", c.actor, c.orgAdmin, err)
		}
	}

	// Acción inventada: no.
	if _, err := st.ResolveReport(acme, r1.ID, "alice", false, "open"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("acción inválida: err=%v", err)
	}
	// Borrar el mensaje: lo hace el admin de organización SIN ser del canal.
	res, err := st.ResolveReport(acme, r1.ID, "zoe", true, domain.ReportMessageDeleted)
	if err != nil || res.Status != domain.ReportMessageDeleted || res.ResolvedBy != "zoe" || res.ResolvedAt == nil {
		t.Fatalf("ResolveReport = %+v (%v)", res, err)
	}
	if ms, _ := st.ListMessages(acme, canal.ID, "alice", store.MessagePage{}); len(ms) != 0 {
		t.Fatalf("el mensaje denunciado sigue a la vista: %+v", ms)
	}
	// Una denuncia se resuelve una vez.
	if _, err := st.ResolveReport(acme, r1.ID, "alice", false, domain.ReportDismissed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("segunda resolución: err=%v", err)
	}
	// Ya no está en la cola de abiertas, pero la traza queda, sin el texto.
	if rs := cola(acme, "alice", false); len(rs) != 0 {
		t.Fatalf("la resuelta sigue abierta: %+v", rs)
	}
	if rs, _ := st.ListReports(acme, "alice", false, false); len(rs) != 1 || rs[0].MessageBody != "" {
		t.Fatalf("traza de la resuelta = %+v", rs)
	}

	// DENUNCIA A LA PERSONA, y expulsión del canal.
	persona := store.NewReport{Reporter: "carla", ChannelID: canal.ID, TargetSub: "bob", Reason: "spam"}
	nadie := persona
	nadie.TargetSub = "nadie"
	if _, err := st.CreateReport(acme, nadie); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("denuncia a alguien que no está en el canal: err=%v", err)
	}
	p1, err := st.CreateReport(acme, persona)
	if err != nil || !p1.Nueva {
		t.Fatalf("denuncia a la persona = %+v (%v)", p1, err)
	}
	if p2, err := st.CreateReport(acme, persona); err != nil || p2.Nueva || p2.ID != p1.ID {
		t.Fatalf("denuncia a la persona repetida = %+v (%v)", p2, err)
	}
	if _, err := st.ResolveReport(acme, p1.ID, "alice", false, domain.ReportMessageDeleted); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("borrar el mensaje de una denuncia sin mensaje: err=%v", err)
	}
	if _, err := st.ResolveReport(acme, p1.ID, "alice", false, domain.ReportMemberRemoved); err != nil {
		t.Fatalf("expulsar: %v", err)
	}
	miembros, err := st.ListChannelMembers(acme, canal.ID, "alice")
	if err != nil {
		t.Fatalf("ListChannelMembers: %v", err)
	}
	for _, mb := range miembros {
		if mb.Subject == "bob" {
			t.Fatal("el expulsado sigue en el canal")
		}
	}
}

// El tope diario: con la deduplicación, lo que impide usar la denuncia para acosar.
func TestDenuncia_TopeDiario(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme := tenantCtx("acme")
	_, canal := equipoDeTres(acme, t, st)

	hechas := 0
	var err error
	for i := range 25 {
		var m domain.Message
		if m, err = escribir(acme, st, canal.ID, "bob", fmt.Sprintf("mensaje %d", i)); err != nil {
			t.Fatalf("CreateMessage: %v", err)
		}
		if _, err = st.CreateReport(acme, store.NewReport{Reporter: "carla", MessageID: &m.ID, Reason: "other"}); err != nil {
			break
		}
		hechas++
	}
	if hechas != 20 || !errors.Is(err, domain.ErrTooManyLookups) {
		t.Fatalf("entraron %d denuncias y la siguiente dio %v; el tope es 20 y después 429", hechas, err)
	}
	// El tope es por persona: Alice sigue pudiendo denunciar.
	m, _ := escribir(acme, st, canal.ID, "bob", "otro más")
	if _, err := st.CreateReport(acme, store.NewReport{Reporter: "alice", MessageID: &m.ID, Reason: "other"}); err != nil {
		t.Fatalf("el tope de carla frena a alice: %v", err)
	}
}

// La cola del operador: todas las organizaciones —también la denuncia contra
// quien administra el equipo—, el mensaje acotado, y resolver desde ella cierra
// la denuncia en la cola de la organización con la marca de la plataforma.
func TestDenuncia_ColaDelOperador(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	_, canal := equipoDeTres(acme, t, st)
	_, canalG := equipoDeTres(globex, t, st)

	largo := strings.Repeat("palabra ", 100)
	contraAlice, err := escribir(acme, st, canal.ID, "alice", largo) // alice administra el equipo
	if err != nil {
		t.Fatal(err)
	}
	mG, err := escribir(globex, st, canalG.ID, "bob", "en globex")
	if err != nil {
		t.Fatal(err)
	}
	ra, err := st.CreateReport(acme, store.NewReport{Reporter: "carla", MessageID: &contraAlice.ID, Reason: "harassment"})
	if err != nil {
		t.Fatal(err)
	}
	rg, err := st.CreateReport(globex, store.NewReport{Reporter: "carla", MessageID: &mG.ID, Reason: "spam"})
	if err != nil {
		t.Fatal(err)
	}
	// Alice no la ve; la operadora, sí, con las dos organizaciones.
	if rs, _ := st.ListReports(acme, "alice", true, true); len(rs) != 0 {
		t.Fatalf("alice ve la denuncia contra ella: %+v", rs)
	}
	rs, err := st.ListReportsOperador(context.Background(), "operadora", true)
	if err != nil || len(rs) != 2 || rs[0].ID != rg.ID || rs[0].Tenant != "globex" || rs[1].Tenant != "acme" {
		t.Fatalf("cola del operador = %+v (%v)", rs, err)
	}
	if n := utf8.RuneCountInString(rs[1].MessageBody); n != 300 || rs[1].ChannelName != canal.Name {
		t.Fatalf("el operador ve %d caracteres del mensaje (quiero 300) del canal %q", n, rs[1].ChannelName)
	}
	if org, err := st.OrgDeDenuncia(context.Background(), ra.ID); err != nil || org != "acme" {
		t.Fatalf("OrgDeDenuncia = %q (%v)", org, err)
	}
	// En otra organización, la denuncia no existe.
	if _, err := st.ResolveReportOperador(globex, ra.ID, "operadora", domain.ReportDismissed); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("resolver en otra organización: err=%v", err)
	}
	res, err := st.ResolveReportOperador(acme, ra.ID, "operadora", domain.ReportMessageDeleted)
	if err != nil || res.ResolvedFrom != "platform" || res.ResolvedBy != "operadora" {
		t.Fatalf("ResolveReportOperador = %+v (%v)", res, err)
	}
	// Cerrada en las dos colas; la organización ve desde dónde.
	if rs, _ := st.ListReportsOperador(context.Background(), "operadora", true); len(rs) != 1 || rs[0].ID != rg.ID {
		t.Fatalf("abiertas del operador = %+v", rs)
	}
	if rs, _ := st.ListReports(acme, "zoe", true, false); len(rs) != 1 || rs[0].ResolvedFrom != "platform" {
		t.Fatalf("la organización no ve que lo resolvió la plataforma: %+v", rs)
	}
	// Y al revés: lo que resuelve la organización sale como suyo en la del operador.
	if _, err := st.ResolveReport(globex, rg.ID, "alice", false, domain.ReportDismissed); err != nil {
		t.Fatal(err)
	}
	if rs, _ := st.ListReportsOperador(context.Background(), "operadora", false); len(rs) != 2 || rs[0].ResolvedFrom != "org" {
		t.Fatalf("resueltas del operador = %+v", rs)
	}
}

// La purga: las resueltas hace más de un año se borran; las abiertas se quedan
// aunque sean viejas, y las resueltas hace menos también. De todas las
// organizaciones.
func TestDenuncia_Purga(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ids := map[string]uuid.UUID{}
	for _, org := range []string{"acme", "globex"} {
		ctx := tenantCtx(org)
		_, canal := equipoDeTres(ctx, t, st)
		for _, k := range []string{"abierta-vieja", "resuelta-vieja", "resuelta-reciente"} {
			m, err := escribir(ctx, st, canal.ID, "bob", k)
			if err != nil {
				t.Fatal(err)
			}
			r, err := st.CreateReport(ctx, store.NewReport{Reporter: "carla", MessageID: &m.ID, Reason: "spam"})
			if err != nil {
				t.Fatal(err)
			}
			ids[org+"/"+k] = r.ID
			if k != "abierta-vieja" {
				if _, err := st.ResolveReport(ctx, r.ID, "alice", false, domain.ReportDismissed); err != nil {
					t.Fatal(err)
				}
			}
			hace := map[string]string{"abierta-vieja": "2 years", "resuelta-vieja": "13 months", "resuelta-reciente": "11 months"}[k]
			if err := h.Pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), `UPDATE message_reports
					SET created_at = now() - $2::interval, resolved_at = CASE WHEN status = 'open' THEN NULL ELSE now() - $2::interval END
					WHERE id = $1`, r.ID, hace)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	n, err := st.PurgarDenuncias(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("purgadas %d (%v), quiero las 2 resueltas hace más de un año", n, err)
	}
	quedan, err := st.ListReportsOperador(context.Background(), "operadora", false)
	if err != nil {
		t.Fatal(err)
	}
	vivas := map[uuid.UUID]bool{}
	for _, r := range quedan {
		vivas[r.ID] = true
	}
	for k, id := range ids {
		if quiero := !strings.HasSuffix(k, "resuelta-vieja"); vivas[id] != quiero {
			t.Errorf("%s: queda=%v, quiero %v", k, vivas[id], quiero)
		}
	}
}
