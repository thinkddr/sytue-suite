// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del RECOLECTOR de logs (WS4/O1) contra un PostgreSQL real.
// Cierra el circuito entero —reclamar el deployment, leer del runtime, guardar y
// poder buscarlo— que es lo único que demuestra que la búsqueda no consulta una
// tabla que nadie llena.
package service

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// fakeCollector hace de runtime: registra con qué corte se le pidieron los logs
// y devuelve lo que se le programe (el pod real no cabe en un test).
type fakeCollector struct {
	sinces []time.Time
	lines  []domain.LogLine
	err    error
}

func (f *fakeCollector) StreamDeploymentLogs(context.Context, domain.Deployment, string, io.Writer) error {
	return nil
}

func (f *fakeCollector) CollectDeploymentLogs(_ context.Context, _ string, _ uuid.UUID, since time.Time, _ int) ([]domain.LogLine, error) {
	f.sinces = append(f.sinces, since)
	return f.lines, f.err
}

func TestCollectLogsOnceGuardaYAvanzaElCorte(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})

	p, err := st.CreateProject(ctx, store.NewProjectRecord{
		Slug: "web", Name: "web", GitProvider: domain.GitGeneric,
		ProductionRef: "main", Framework: domain.FrameworkDetect,
	}, 0)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	d, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID: p.ID, Target: domain.TargetProduction, GitRef: "main",
		URL: "https://web.apps.sytue.eu",
	})
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if err := st.SetDeploymentStatus(ctx, "acme", d.ID, domain.DeployReady, d.URL, ""); err != nil {
		t.Fatalf("SetDeploymentStatus: %v", err)
	}

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	lastTS := now.Add(-30 * time.Second)
	rt := &fakeCollector{lines: []domain.LogLine{
		{TS: now.Add(-time.Minute), Text: "GET /health 200"},
		{TS: lastTS, Text: "ERROR timeout conectando con db"},
	}}
	svc := &Service{store: st, pstore: st, logStreamer: rt}

	if err := svc.collectLogsOnce(context.Background(), now); err != nil {
		t.Fatalf("collectLogsOnce: %v", err)
	}
	if len(rt.sinces) != 1 {
		t.Fatalf("se pidieron logs %d veces; quiero 1", len(rt.sinces))
	}
	// Primera recolección: mira hacia atrás la ventana de backfill, no el
	// historial entero del pod.
	if want := now.Add(-logCollectBackfill); !rt.sinces[0].Equal(want) {
		t.Errorf("primer corte = %v; quiero %v", rt.sinces[0], want)
	}
	// Y lo leído YA SE BUSCA.
	page, err := svc.SearchLogs(ctx, p.ID, store.RuntimeLogFilter{Query: "timeout"}, "", 10)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if len(page.Lines) != 1 || page.Lines[0].Text != "ERROR timeout conectando con db" {
		t.Fatalf("la búsqueda devolvió %+v", page.Lines)
	}
	if page.Lines[0].DeploymentID != d.ID {
		t.Errorf("línea sin su deployment: %s", page.Lines[0].DeploymentID)
	}

	// Segunda pasada, pasada la ventana de arriendo: el corte es el ts de la
	// ÚLTIMA línea guardada. Si fuera el reloj de la reclamación anterior, lo
	// escrito mientras se leía el pod se guardaría dos veces.
	rt.lines = nil
	if err := svc.collectLogsOnce(context.Background(), now.Add(5*time.Minute)); err != nil {
		t.Fatalf("collectLogsOnce (2ª): %v", err)
	}
	if len(rt.sinces) != 2 {
		t.Fatalf("segunda pasada no reclamó (sinces=%v)", rt.sinces)
	}
	if !rt.sinces[1].Equal(lastTS) {
		t.Errorf("segundo corte = %v; quiero %v (última línea guardada)", rt.sinces[1], lastTS)
	}

	// Un runtime que falla (pod reiniciándose) no rompe la pasada ni pierde lo ya
	// guardado: se salta ese deployment y se sigue.
	rt.err = context.DeadlineExceeded
	if err := svc.collectLogsOnce(context.Background(), now.Add(10*time.Minute)); err != nil {
		t.Fatalf("collectLogsOnce con runtime caído: %v", err)
	}
	page, err = svc.SearchLogs(ctx, p.ID, store.RuntimeLogFilter{}, "", 10)
	if err != nil || len(page.Lines) != 2 {
		t.Fatalf("tras el fallo hay %d líneas (err=%v); quiero 2", len(page.Lines), err)
	}
}
