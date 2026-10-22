// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la monitorización de uptime + alertas (Wave 2) contra
// un PostgreSQL real (build-tag `integration`). Verifican la parte que vive en
// SQL y no en funciones puras (esas van en los *_test.go sin tag):
//   - ClaimDueChecks: reclama solo los vencidos+habilitados y los ARRIENDA
//     (bump de next_run_at) de modo que una segunda reclamación no los repite;
//   - RecordCheckResult: emite al outbox SOLO en la transición up↔down;
//   - MatchAlertPolicies: enruta por (evento, proyecto|org) y aísla por tenant;
//   - ClaimAlertDelivery: dedupe idempotente por (policy_id, event_id).
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestClaimDueChecksLease(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")

	// Dos checks vencidos y habilitados. Se retrodatan con el reloj de la BD
	// (now() - 1h) para no depender del posible desfase de reloj entre el proceso
	// de test y el contenedor de Postgres (en prod app y BD van sincronizados NTP).
	due1 := seedCheck(t, ctx, st, p.ID, "https://a.example.com")
	due2 := seedCheck(t, ctx, st, p.ID, "https://b.example.com")
	maintExec(t, h.Pool, `UPDATE uptime_checks SET next_run_at = now() - interval '1 hour' WHERE id IN ($1,$2)`, due1.ID, due2.ID)
	// Uno deshabilitado (no debe reclamarse aunque esté vencido).
	off := seedCheck(t, ctx, st, p.ID, "https://c.example.com")
	no := false
	if _, err := st.UpdateUptimeCheck(ctx, off.ID, store.UpdateUptimeCheckRecord{Enabled: &no}); err != nil {
		t.Fatalf("deshabilitar check: %v", err)
	}
	// Uno futuro (aún no vencido).
	future := seedCheck(t, ctx, st, p.ID, "https://d.example.com")
	maintExec(t, h.Pool, `UPDATE uptime_checks SET next_run_at = now() + interval '1 hour' WHERE id=$1`, future.ID)

	claimed, err := st.ClaimDueChecks(context.Background(), time.Now(), 100)
	if err != nil {
		t.Fatalf("ClaimDueChecks: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, c := range claimed {
		got[c.ID] = true
	}
	if !got[due1.ID] || !got[due2.ID] {
		t.Errorf("faltan checks vencidos en la reclamación: %+v", got)
	}
	if got[off.ID] {
		t.Error("se reclamó un check deshabilitado")
	}
	if got[future.ID] {
		t.Error("se reclamó un check aún no vencido")
	}

	// Arriendo: una segunda reclamación inmediata NO debe devolver nada (next_run_at
	// se bumpeó al reclamar), evitando doble sondeo entre ticks/réplicas.
	again, err := st.ClaimDueChecks(context.Background(), time.Now(), 100)
	if err != nil {
		t.Fatalf("ClaimDueChecks (2ª): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("segunda reclamación devolvió %d checks, quiero 0 (arriendo por next_run_at)", len(again))
	}
}

func TestRecordCheckResultEmitsOnTransition(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	c := seedCheck(t, ctx, st, p.ID, "https://a.example.com")

	base := store.ClaimedCheck{
		ID: c.ID, Tenant: "acme", ProjectID: p.ID, Name: c.Name,
		TargetURL: c.TargetURL, LastStatus: domain.UptimeStatusUnknown,
	}
	// Baseline (desconocido → up): NO emite.
	if err := st.RecordCheckResult(context.Background(), base, domain.UptimeStatusUp, 12, time.Now()); err != nil {
		t.Fatalf("RecordCheckResult baseline: %v", err)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM events_outbox WHERE type IN ($1,$2)`,
		store.EventUptimeDown, store.EventUptimeUp); n != 0 {
		t.Fatalf("baseline emitió %d eventos, quiero 0", n)
	}

	// up → down: emite cloud.uptime.down.
	up := base
	up.LastStatus = domain.UptimeStatusUp
	if err := st.RecordCheckResult(context.Background(), up, domain.UptimeStatusDown, 0, time.Now()); err != nil {
		t.Fatalf("RecordCheckResult down: %v", err)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM events_outbox WHERE type=$1`, store.EventUptimeDown); n != 1 {
		t.Fatalf("transición down emitió %d eventos, quiero 1", n)
	}

	// down → down: sin cambio, NO emite otro.
	down := base
	down.LastStatus = domain.UptimeStatusDown
	if err := st.RecordCheckResult(context.Background(), down, domain.UptimeStatusDown, 0, time.Now()); err != nil {
		t.Fatalf("RecordCheckResult sin cambio: %v", err)
	}
	if n := maintCount(t, h.Pool, `SELECT count(*) FROM events_outbox WHERE type=$1`, store.EventUptimeDown); n != 1 {
		t.Fatalf("sin cambio emitió de más: %d eventos down, quiero 1", n)
	}

	// El último estado quedó persistido en la fila (no hay tabla de histórico).
	got, err := st.GetUptimeCheck(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetUptimeCheck: %v", err)
	}
	if got.LastStatus != domain.UptimeStatusDown {
		t.Errorf("last_status = %q, quiero down", got.LastStatus)
	}
}

func TestMatchAlertPoliciesRouting(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	other := seedProject(t, ctx, st, "api")

	// Casan deploy.failed para el proyecto p.
	proj := mustPolicy(t, ctx, st, &p.ID, domain.AlertEventDeployFailed, true) // proyecto
	org := mustPolicy(t, ctx, st, nil, domain.AlertEventDeployFailed, true)    // org-wide
	mustPolicy(t, ctx, st, &other.ID, domain.AlertEventDeployFailed, true)     // otro proyecto: NO
	mustPolicy(t, ctx, st, &p.ID, domain.AlertEventUptimeDown, true)           // otro evento: NO
	_ = mustPolicy(t, ctx, st, &p.ID, domain.AlertEventDeployFailed, false)    // deshabilitada: NO
	tokenHash := "hash-confirmacion-mailto"
	mailtoUnconfirmed, err := st.CreateAlertPolicy(ctx, store.NewAlertPolicyRecord{
		ProjectID:        &p.ID,
		Event:            domain.AlertEventDeployFailed,
		ChannelURL:       "mailto:ops@acme.eu",
		Enabled:          true,
		Confirmed:        false,
		ConfirmTokenHash: &tokenHash,
	})
	if err != nil {
		t.Fatalf("CreateAlertPolicy mailto: %v", err)
	}
	if mailtoUnconfirmed.Confirmed {
		t.Fatalf("mailto sin confirmar salió Confirmed=true")
	}

	// Policy de OTRO tenant para el mismo evento: aislamiento (NO debe casar).
	// Slug distinto: projects_unique_global_slug hace el slug único global (subdominio).
	octx := tenantCtx("globex")
	gp := seedProject(t, octx, st, "globexweb")
	mustPolicy(t, octx, st, &gp.ID, domain.AlertEventDeployFailed, true)

	matched, err := st.MatchAlertPolicies(context.Background(), "acme", domain.AlertEventDeployFailed, p.ID)
	if err != nil {
		t.Fatalf("MatchAlertPolicies: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, m := range matched {
		got[m.ID] = true
	}
	if len(matched) != 2 || !got[proj.ID] || !got[org.ID] {
		t.Fatalf("casaron %d policies (%v), quiero exactamente {proyecto, org}", len(matched), got)
	}

	// Al confirmar el token del mailto, empieza a casar.
	confirmedPolicy, err := st.ConfirmAlertPolicyByToken(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("ConfirmAlertPolicyByToken: %v", err)
	}
	if !confirmedPolicy.Confirmed || confirmedPolicy.ID != mailtoUnconfirmed.ID {
		t.Fatalf("ConfirmAlertPolicyByToken = %+v", confirmedPolicy)
	}
	matchedAfter, err := st.MatchAlertPolicies(context.Background(), "acme", domain.AlertEventDeployFailed, p.ID)
	if err != nil {
		t.Fatalf("MatchAlertPolicies tras confirmar: %v", err)
	}
	if len(matchedAfter) != 3 {
		t.Fatalf("casaron %d policies tras confirmar mailto, quiero 3", len(matchedAfter))
	}
}

func TestClaimAlertDeliveryDedupe(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	p := seedProject(t, ctx, st, "web")
	policy := mustPolicy(t, ctx, st, &p.ID, domain.AlertEventDeployReady, true)
	eventID := uuid.New()

	first, err := st.ClaimAlertDelivery(context.Background(), "acme", policy.ID, eventID)
	if err != nil {
		t.Fatalf("ClaimAlertDelivery 1ª: %v", err)
	}
	if !first {
		t.Fatal("primera reclamación devolvió false, quiero true")
	}
	second, err := st.ClaimAlertDelivery(context.Background(), "acme", policy.ID, eventID)
	if err != nil {
		t.Fatalf("ClaimAlertDelivery 2ª: %v", err)
	}
	if second {
		t.Fatal("segunda reclamación devolvió true, quiero false (dedupe)")
	}

	// Tras liberar, se puede volver a reclamar (reintento de envío).
	if err := st.ReleaseAlertDelivery(context.Background(), "acme", policy.ID, eventID); err != nil {
		t.Fatalf("ReleaseAlertDelivery: %v", err)
	}
	again, err := st.ClaimAlertDelivery(context.Background(), "acme", policy.ID, eventID)
	if err != nil {
		t.Fatalf("ClaimAlertDelivery tras release: %v", err)
	}
	if !again {
		t.Fatal("reclamación tras release devolvió false, quiero true")
	}
}

func seedCheck(t *testing.T, ctx context.Context, st *store.Store, projectID uuid.UUID, target string) domain.UptimeCheck {
	t.Helper()
	c, err := st.CreateUptimeCheck(ctx, store.NewUptimeCheckRecord{
		ProjectID: projectID, Name: "check", TargetURL: target, IntervalSeconds: 60, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateUptimeCheck(%q): %v", target, err)
	}
	return c
}

func mustPolicy(t *testing.T, ctx context.Context, st *store.Store, projectID *uuid.UUID, event string, enabled bool) domain.AlertPolicy {
	t.Helper()
	p, err := st.CreateAlertPolicy(ctx, store.NewAlertPolicyRecord{
		ProjectID: projectID, Event: event, ChannelURL: "https://hooks.example.com/x", Enabled: enabled,
	})
	if err != nil {
		t.Fatalf("CreateAlertPolicy(%s): %v", event, err)
	}
	return p
}
