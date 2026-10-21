// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de las UNIDADES DE EQUIPO contra un PostgreSQL real (pgtest
// sobre testcontainers, tras el build-tag `integration`). Prueban la enforcement
// REAL de extremo a extremo por el servicio: AISLAMIENTO deny-by-default (un
// no-miembro no ve ni accede — ErrNotFound), la verja por ROL (viewer lee pero no
// escribe/gestiona; editor escribe pero no gestiona; manager gestiona), el
// aislamiento del CONTENIDO entre unidades (RLS por ámbito) y la guarda del último
// gestor. Se construye un Service real SIN S3 (blobStat con cliente nil basta para
// las rutas que no tocan blobs: unidades, miembros, carpetas). Sin Docker no corren.
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

func newTeamService(t *testing.T) (*service.Service, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := service.NewStore(h.Pool)
	// blobs nil: las operaciones probadas (unidades/miembros/carpetas) no tocan S3.
	// chat nil: la verja de adjuntos de canal no interviene en las unidades de equipo.
	svc := service.New(config.Config{DefaultQuotaBytes: 1 << 30, MaxUploadBytes: 1 << 20}, st, nil, nil, nil)
	return svc, tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
}

// TestTeamDriveIsolationAndRoles cubre el aislamiento por membresía y la verja por
// rol de extremo a extremo (el requisito central: no-miembro rechazado, roles
// insuficientes rechazados).
func TestTeamDriveIsolationAndRoles(t *testing.T) {
	svc, ctx := newTeamService(t)
	const (
		alice = "alice" // creadora → manager
		bob   = "bob"   // ajeno
		carol = "carol" // se le asignan roles
	)

	d, err := svc.CreateTeamDrive(ctx, "Marketing", alice)
	if err != nil {
		t.Fatalf("CreateTeamDrive: %v", err)
	}

	// --- AISLAMIENTO: bob no es miembro; no ve ni accede (ErrNotFound, sin fuga).
	if _, _, err := svc.GetTeamDrive(ctx, d.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro Get => %v, quiero ErrNotFound", err)
	}
	if drives, _ := svc.ListTeamDrives(ctx, bob); len(drives) != 0 {
		t.Fatalf("no-miembro ve %d unidades, quiero 0", len(drives))
	}
	if _, err := svc.TeamListChildren(ctx, d.ID, bob, uuid.Nil, "", 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro lista contenido => %v, quiero ErrNotFound", err)
	}
	if _, err := svc.TeamCreateFolder(ctx, d.ID, bob, uuid.Nil, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro crea carpeta => %v, quiero ErrNotFound", err)
	}

	// alice se ve como manager.
	drives, err := svc.ListTeamDrives(ctx, alice)
	if err != nil || len(drives) != 1 || drives[0].Role != domain.TeamManager {
		t.Fatalf("ListTeamDrives(alice) = %+v, %v", drives, err)
	}

	// --- ROL viewer: lee, pero no escribe ni gestiona.
	if _, err := svc.PutTeamDriveMember(ctx, d.ID, alice, carol, domain.TeamViewer); err != nil {
		t.Fatalf("añadir viewer: %v", err)
	}
	if _, err := svc.TeamListChildren(ctx, d.ID, carol, uuid.Nil, "", 0); err != nil {
		t.Fatalf("viewer lee => %v, quiero ok", err)
	}
	if _, err := svc.TeamCreateFolder(ctx, d.ID, carol, uuid.Nil, "docs"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer escribe => %v, quiero ErrPermission", err)
	}
	if _, err := svc.RenameTeamDrive(ctx, d.ID, carol, "x"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer renombra => %v, quiero ErrPermission", err)
	}
	if _, err := svc.ListTeamDriveMembers(ctx, d.ID, carol); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer lista miembros => %v, quiero ErrPermission", err)
	}

	// --- ROL editor: escribe, pero sigue sin gestionar.
	if _, err := svc.PutTeamDriveMember(ctx, d.ID, alice, carol, domain.TeamEditor); err != nil {
		t.Fatalf("subir a editor: %v", err)
	}
	if _, err := svc.TeamCreateFolder(ctx, d.ID, carol, uuid.Nil, "docs"); err != nil {
		t.Fatalf("editor crea carpeta => %v, quiero ok", err)
	}
	if _, err := svc.RenameTeamDrive(ctx, d.ID, carol, "x"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("editor gestiona => %v, quiero ErrPermission", err)
	}

	// --- ROL manager: gestiona.
	if _, err := svc.RenameTeamDrive(ctx, d.ID, alice, "Growth"); err != nil {
		t.Fatalf("manager renombra => %v", err)
	}
	members, err := svc.ListTeamDriveMembers(ctx, d.ID, alice)
	if err != nil || len(members) != 2 {
		t.Fatalf("ListTeamDriveMembers = %+v, %v (quiero 2)", members, err)
	}
}

// TestTeamDriveContentScoping comprueba que el contenido de una unidad está AISLADO
// de las demás (RLS por ámbito app.team_drive): una carpeta creada en A no aparece
// al listar B, aunque el mismo usuario sea manager de ambas.
func TestTeamDriveContentScoping(t *testing.T) {
	svc, ctx := newTeamService(t)
	a, err := svc.CreateTeamDrive(ctx, "A", "alice")
	if err != nil {
		t.Fatalf("crear A: %v", err)
	}
	b, err := svc.CreateTeamDrive(ctx, "B", "alice")
	if err != nil {
		t.Fatalf("crear B: %v", err)
	}
	if _, err := svc.TeamCreateFolder(ctx, a.ID, "alice", uuid.Nil, "solo-A"); err != nil {
		t.Fatalf("crear carpeta en A: %v", err)
	}
	la, err := svc.TeamListChildren(ctx, a.ID, "alice", uuid.Nil, "", 0)
	if err != nil || len(la.Subfolders) != 1 {
		t.Fatalf("A debería ver 1 subcarpeta: %+v, %v", la.Subfolders, err)
	}
	lb, err := svc.TeamListChildren(ctx, b.ID, "alice", uuid.Nil, "", 0)
	if err != nil || len(lb.Subfolders) != 0 {
		t.Fatalf("B NO debería ver el contenido de A: %+v, %v", lb.Subfolders, err)
	}
}

// TestTeamDriveLastManagerGuard: la unidad nunca queda sin gestor.
func TestTeamDriveLastManagerGuard(t *testing.T) {
	svc, ctx := newTeamService(t)
	d, err := svc.CreateTeamDrive(ctx, "Ops", "alice")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// alice es la única manager: ni quitarla ni degradarla.
	if err := svc.RemoveTeamDriveMember(ctx, d.ID, "alice", "alice"); !errors.Is(err, domain.ErrLastManager) {
		t.Fatalf("quitar último gestor => %v, quiero ErrLastManager", err)
	}
	if _, err := svc.PutTeamDriveMember(ctx, d.ID, "alice", "alice", domain.TeamViewer); !errors.Is(err, domain.ErrLastManager) {
		t.Fatalf("degradar último gestor => %v, quiero ErrLastManager", err)
	}
	// Con un segundo manager, alice ya puede salir.
	if _, err := svc.PutTeamDriveMember(ctx, d.ID, "alice", "bob", domain.TeamManager); err != nil {
		t.Fatalf("añadir 2º manager: %v", err)
	}
	if err := svc.RemoveTeamDriveMember(ctx, d.ID, "alice", "alice"); err != nil {
		t.Fatalf("quitar con otro gestor presente => %v, quiero ok", err)
	}
}
