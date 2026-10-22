// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// fakeGetter simula el store: registra bajo qué tenant se resolvió el servicio
// (para probar el aislamiento) y devuelve el servicio/err configurados.
type fakeGetter struct {
	svc       domain.ManagedService
	err       error
	gotTenant string
	gotID     uuid.UUID
}

func (f *fakeGetter) GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error) {
	f.gotTenant = tenancy.MustFrom(ctx).Slug
	f.gotID = id
	return f.svc, f.err
}

// restoreRecorder es un ServiceProvisioner fake que graba los argumentos con los
// que se invocó RestorePostgres (o BackupPostgresNow).
type restoreRecorder struct {
	called       bool
	svc          domain.ManagedService
	backup       string
	backupCalled bool
	runCalled    bool
}

func (r *restoreRecorder) Provision(context.Context, domain.ManagedService) (ProvisionResult, error) {
	return ProvisionResult{}, nil
}
func (r *restoreRecorder) Deprovision(context.Context, domain.ManagedService) error { return nil }

func (r *restoreRecorder) RestorePostgres(_ context.Context, svc domain.ManagedService, backup string) error {
	r.called = true
	r.svc = svc
	r.backup = backup
	return nil
}

func (r *restoreRecorder) RunServiceNow(_ context.Context, svc domain.ManagedService) error {
	r.runCalled = true
	r.svc = svc
	return nil
}

func (r *restoreRecorder) BackupPostgresNow(_ context.Context, svc domain.ManagedService) error {
	r.backupCalled = true
	r.svc = svc
	return nil
}

// Propiedad crítica: el evento dispara RestorePostgres con el servicio/tenant
// correctos (el mismo que resolvió el store bajo el tenant del evento).
func TestDispatchServiceRestore(t *testing.T) {
	id := uuid.New()
	getter := &fakeGetter{svc: domain.ManagedService{ID: id, Tenant: "acme", Kind: domain.ServicePostgres}}
	rec := &restoreRecorder{}

	const backup = "20260720T030000Z.sql.gz"
	if err := DispatchServiceRestore(context.Background(), getter, rec, "acme", id, backup); err != nil {
		t.Fatalf("DispatchServiceRestore: %v", err)
	}
	if !rec.called {
		t.Fatal("RestorePostgres no fue invocado por el evento de restore")
	}
	if rec.svc.ID != id || rec.svc.Tenant != "acme" {
		t.Fatalf("RestorePostgres recibió svc=%v/%q, se esperaba %v/acme", rec.svc.ID, rec.svc.Tenant, id)
	}
	if rec.backup != backup {
		t.Fatalf("RestorePostgres recibió backup=%q, se esperaba %q", rec.backup, backup)
	}
	// Aislamiento: el servicio se resolvió bajo el tenant del evento (RLS), no otro.
	if getter.gotTenant != "acme" || getter.gotID != id {
		t.Fatalf("GetService corrió con tenant=%q id=%v, se esperaba acme/%v", getter.gotTenant, getter.gotID, id)
	}
}

func TestDispatchServiceRestoreRejectsNonPostgres(t *testing.T) {
	id := uuid.New()
	getter := &fakeGetter{svc: domain.ManagedService{ID: id, Tenant: "acme", Kind: domain.ServiceStorage}}
	rec := &restoreRecorder{}
	if err := DispatchServiceRestore(context.Background(), getter, rec, "acme", id, "x.sql.gz"); err == nil {
		t.Fatal("se esperaba rechazo de restore sobre un servicio no-postgres")
	}
	if rec.called {
		t.Fatal("RestorePostgres no debe invocarse para un servicio no-postgres")
	}
}

// Un id de otro tenant ⇒ el store (RLS real) devuelve ErrNotFound; el restore no
// se dispara sobre un servicio equivocado.
func TestDispatchServiceRestoreNotFound(t *testing.T) {
	getter := &fakeGetter{err: domain.ErrNotFound}
	rec := &restoreRecorder{}
	if err := DispatchServiceRestore(context.Background(), getter, rec, "acme", uuid.New(), "x.sql.gz"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, se esperaba ErrNotFound", err)
	}
	if rec.called {
		t.Fatal("RestorePostgres no debe invocarse si el servicio no existe en el tenant")
	}
}

// La copia a demanda comparte camino con el restore, así que comparte también sus
// dos propiedades críticas: se resuelve bajo el tenant DEL EVENTO —nunca sobre el
// servicio de otra organización— y solo se dispara para postgres.
func TestDispatchServiceBackup(t *testing.T) {
	id := uuid.New()
	getter := &fakeGetter{svc: domain.ManagedService{ID: id, Tenant: "acme", Kind: domain.ServicePostgres}}
	rec := &restoreRecorder{}

	if err := DispatchServiceBackup(context.Background(), getter, rec, "acme", id); err != nil {
		t.Fatalf("DispatchServiceBackup: %v", err)
	}
	if getter.gotTenant != "acme" || getter.gotID != id {
		t.Errorf("se resolvió con tenant=%q id=%v; se esperaba acme/%v", getter.gotTenant, getter.gotID, id)
	}
	if !rec.backupCalled || rec.svc.ID != id {
		t.Errorf("no se disparó la copia sobre el servicio esperado: %+v", rec)
	}

	// Un servicio que no es una base de datos NO puede acabar en un pg_dump.
	otro := &fakeGetter{svc: domain.ManagedService{ID: id, Tenant: "acme", Kind: domain.ServiceStorage}}
	rec2 := &restoreRecorder{}
	if err := DispatchServiceBackup(context.Background(), otro, rec2, "acme", id); err == nil {
		t.Error("un servicio de almacenamiento no puede aceptar una copia de postgres")
	}
	if rec2.backupCalled {
		t.Error("se disparó la copia sobre un servicio que no es postgres")
	}
}
