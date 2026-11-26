// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// La congelación vale lo que vale su verja: si un push a producción la cruza, el
// interruptor de la pantalla es decorado. Se comprueba la verja, no el interruptor.
func TestCongelacionParaProduccionYDejaPasarPreviews(t *testing.T) {
	pid := uuid.New()
	fs := &fakeStore{
		memberCount: 1,
		project:     domain.Project{ID: pid, Slug: "web", ProductionRef: "main"},
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
		freeze:      &domain.DeployFreeze{ProjectID: pid, Reason: "cierre contable"},
	}
	pub := &capturePub{}
	s := newTestService(fs, pub)

	_, _, err := s.Deploy(tenantCtx(t), DeployInput{ProjectID: pid, Target: domain.TargetProduction, CreatedBy: "alice"})
	if !errors.Is(err, domain.ErrDeployFrozen) {
		t.Fatalf("producción congelada debe dar ErrDeployFrozen, got %v", err)
	}
	if fs.newBuild != nil || fs.newDep != nil || len(pub.types) != 0 {
		t.Fatal("un despliegue bloqueado no puede encolar build, crear deployment ni emitir eventos")
	}
	if !audito(fs, "deploy.blocked_frozen") {
		t.Error("el intento bloqueado tiene que quedar en la auditoría")
	}

	// Una preview no cambia lo que ve el público: pasa.
	pr := 7
	if _, _, err := s.Deploy(tenantCtx(t), DeployInput{ProjectID: pid, Target: domain.TargetPreview, PRNumber: &pr}); err != nil {
		t.Fatalf("la preview no debe congelarse: %v", err)
	}
}

func TestCongelacionParaPublicarPreviewPeroNoElRollback(t *testing.T) {
	pid := uuid.New()
	prev := domain.Deployment{
		ID: uuid.New(), ProjectID: pid, Target: domain.TargetPreview,
		Status: domain.DeployReady, ImageRef: "reg.sytue.eu/acme/web:abc",
	}
	fs := &fakeStore{
		project: domain.Project{ID: pid, Slug: "web"},
		dep:     prev,
		promote: domain.Deployment{ID: uuid.New()},
		freeze:  &domain.DeployFreeze{ProjectID: pid},
	}
	s := newTestService(fs, &capturePub{})
	if _, err := s.PromotePreview(tenantCtx(t), prev.ID); !errors.Is(err, domain.ErrDeployFrozen) {
		t.Fatalf("publicar una preview con producción congelada debe dar ErrDeployFrozen, got %v", err)
	}
	if fs.newDep != nil {
		t.Fatal("se creó un deployment de producción con la congelación puesta")
	}
	// El rollback es la salida de emergencia: congelar no puede dejar a nadie
	// atrapado con un despliegue roto.
	if _, err := s.Rollback(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("el rollback no debe congelarse: %v", err)
	}
}

// Un push a la rama de producción con la congelación puesta es trabajo HECHO para
// la forja: si se le devolviera error, reintentaría el mismo push sin parar.
func TestPushConProduccionCongeladaSeDaPorProcesado(t *testing.T) {
	pid := uuid.New()
	proj := domain.Project{ID: pid, Slug: "web", ProductionRef: "main"}
	fs := &fakeStore{
		memberCount: 1,
		resolved:    store.ResolvedRepo{Tenant: "acme", Project: proj},
		project:     proj,
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
		freeze:      &domain.DeployFreeze{ProjectID: pid},
	}
	s := newTestService(fs, &capturePub{})
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{
		Ref: "refs/heads/main", CommitSHA: "abc1234", Pusher: "alice",
	}, "d1", nil)
	if err != nil {
		t.Fatalf("el push congelado no es un error para la forja: %v", err)
	}
	if dep != nil || !ignored {
		t.Fatalf("esperaba ignorado sin deployment, got dep=%v ignored=%v", dep, ignored)
	}
}

func TestCongelarNormalizaElMotivoYDescongelarEsIdempotente(t *testing.T) {
	pid := uuid.New()
	fs := &fakeStore{project: domain.Project{ID: pid, Slug: "web"}}
	s := newTestService(fs, nil)

	f, err := s.FreezeDeploys(tenantCtx(t), pid, "  campaña\nde   navidad  ")
	if err != nil {
		t.Fatalf("FreezeDeploys: %v", err)
	}
	if f.Reason != "campaña de navidad" {
		t.Errorf("motivo = %q, quiero una sola línea sin espacios de sobra", f.Reason)
	}
	if !audito(fs, "deploy.freeze") {
		t.Error("congelar tiene que quedar en la auditoría")
	}

	if _, err := s.FreezeDeploys(tenantCtx(t), pid, strings.Repeat("x", domain.MaxFreezeReasonLen+1)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("un motivo demasiado largo debe dar ErrInvalidInput, got %v", err)
	}

	if err := s.UnfreezeDeploys(tenantCtx(t), pid); err != nil {
		t.Fatalf("UnfreezeDeploys: %v", err)
	}
	antes := len(fs.audited)
	if err := s.UnfreezeDeploys(tenantCtx(t), pid); err != nil {
		t.Fatalf("descongelar dos veces no es un error: %v", err)
	}
	if len(fs.audited) != antes {
		t.Error("descongelar lo que no estaba congelado no debe dejar entrada de auditoría")
	}
}

func audito(fs *fakeStore, accion string) bool {
	for _, a := range fs.audited {
		if a.Action == accion {
			return true
		}
	}
	return false
}
