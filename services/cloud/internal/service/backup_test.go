// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func TestValidateBackupName(t *testing.T) {
	valid := []string{
		"20260720T030000Z.sql.gz",
		"backup.sql.gz",
		"a-b_c.1.sql.gz",
	}
	for _, n := range valid {
		if err := validateBackupName(n); err != nil {
			t.Errorf("validateBackupName(%q) = %v, se esperaba nil", n, err)
		}
	}
	// Frontera de confianza: cualquier ruta/traversal/nombre fuera de patrón se rechaza.
	invalid := []string{
		"",                        // vacío
		"../otro-tenant/x.sql.gz", // traversal cross-tenant
		"/etc/passwd",             // ruta absoluta
		"a/b.sql.gz",              // separador
		`a\b.sql.gz`,              // separador windows
		"..sql.gz",                // contiene ..
		"foo.txt",                 // extensión incorrecta
		"foo.sql.gz.evil",         // no termina en .sql.gz
		"foo.sql.gz/",             // separador final
	}
	for _, n := range invalid {
		if err := validateBackupName(n); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("validateBackupName(%q) = %v, se esperaba ErrInvalidInput", n, err)
		}
	}
}

// La descarga es la MISMA frontera que el restore: el nombre se valida y el kind
// se exige ANTES de llegar a Object Storage. Aquí no hay S3 configurado, así que
// si alguno de los dos cortes se cayera, el test lo vería como otro error —o como
// una URL— en vez del rechazo que toca.
func TestDownloadServiceBackupURLValidaAntesDeS3(t *testing.T) {
	postgres := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServicePostgres}
	s := newTestService(&fakeStore{svc: postgres}, nil)

	if _, err := s.DownloadServiceBackupURL(tenantCtx(t), postgres.ID, "../otro/x.sql.gz"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("un nombre con ruta debe dar ErrInvalidInput, got %v", err)
	}

	// Un bucket no es un Postgres: no tiene copias que bajarse.
	bucket := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceStorage}
	s2 := newTestService(&fakeStore{svc: bucket}, nil)
	if _, err := s2.DownloadServiceBackupURL(tenantCtx(t), bucket.ID, "x.sql.gz"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("un servicio que no es postgres debe dar ErrConflict, got %v", err)
	}
}

func TestBackupObjectKeyComposedServerSide(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	svc := domain.ManagedService{ID: id, Tenant: "acme", Kind: domain.ServicePostgres}
	short := shortUUID(id)

	wantPrefix := "backups/acme/" + short + "/"
	if got := backupPrefix(svc); got != wantPrefix {
		t.Fatalf("backupPrefix = %q, se esperaba %q", got, wantPrefix)
	}
	// El objeto SIEMPRE = prefijo propio del servicio + nombre; tenant/shortID salen
	// del svc (cargado bajo RLS), no del cliente.
	if got := backupObjectKey(svc, "x.sql.gz"); got != wantPrefix+"x.sql.gz" {
		t.Fatalf("backupObjectKey = %q, se esperaba %q", got, wantPrefix+"x.sql.gz")
	}
	// Cross-tenant: dos servicios de tenants distintos NUNCA comparten prefijo.
	other := domain.ManagedService{ID: id, Tenant: "evil", Kind: domain.ServicePostgres}
	if backupPrefix(other) == backupPrefix(svc) {
		t.Fatal("prefijos de tenants distintos coinciden: fuga cross-tenant")
	}
}

func TestRestoreServiceBackupRejectsNonPostgres(t *testing.T) {
	fs := &fakeStore{svc: domain.ManagedService{Kind: domain.ServiceStorage, Tenant: "acme"}}
	s := newTestService(fs, &capturePub{})

	// Servicio no-postgres → ErrConflict ANTES de tocar S3.
	if err := s.RestoreServiceBackup(tenantCtx(t), uuid.New(), "x.sql.gz"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("RestoreServiceBackup no-postgres = %v, se esperaba ErrConflict", err)
	}
	if _, err := s.ListServiceBackups(tenantCtx(t), uuid.New()); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ListServiceBackups no-postgres = %v, se esperaba ErrConflict", err)
	}
}

func TestRestoreServiceBackupRejectsBadNameBeforeS3(t *testing.T) {
	fs := &fakeStore{svc: domain.ManagedService{Kind: domain.ServicePostgres, Tenant: "acme", ID: uuid.New()}}
	s := newTestService(fs, &capturePub{})

	// Un servicio postgres válido pero un nombre con traversal → ErrInvalidInput. La
	// validación precede a cualquier acceso a S3 (sin red en el test).
	for _, bad := range []string{"../otro/x.sql.gz", "/etc/passwd", "a/b.sql.gz", ""} {
		if err := s.RestoreServiceBackup(tenantCtx(t), uuid.New(), bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("RestoreServiceBackup(%q) = %v, se esperaba ErrInvalidInput", bad, err)
		}
	}
}
