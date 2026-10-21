// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func TestDeploymentURL(t *testing.T) {
	s := &Service{cfg: config.Config{
		AppsDomain:    "apps.sytue.eu",
		PreviewDomain: "preview.sytue.eu",
	}}
	proj := domain.Project{Slug: "mi-app"}

	if got := s.DeploymentURL(proj, domain.TargetProduction, nil); got != "https://mi-app.apps.sytue.eu" {
		t.Fatalf("URL producción: %q", got)
	}
	pr := 12
	if got := s.DeploymentURL(proj, domain.TargetPreview, &pr); got != "https://mi-app-pr-12.preview.sytue.eu" {
		t.Fatalf("URL preview: %q", got)
	}
	// Preview sin número de PR cae a la URL de producción (defensa).
	if got := s.DeploymentURL(proj, domain.TargetPreview, nil); got != "https://mi-app.apps.sytue.eu" {
		t.Fatalf("URL preview sin PR: %q", got)
	}
}

// TestCancelarDespliegueParaSuCompilacion: los minutos de compilación se facturan, así
// que cancelar un despliegue atascado tenía que dejar de costar dinero. Antes solo
// marcaba el despliegue y el trabajo de compilación seguía corriendo en el clúster.
func TestCancelarDespliegueParaSuCompilacion(t *testing.T) {
	buildID := uuid.New()
	st := &fakeStore{dep: domain.Deployment{ID: uuid.New(), BuildID: &buildID}}
	s := &Service{store: st}

	if err := s.CancelDeployment(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), st.dep.ID); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if st.canceledBuild == nil || *st.canceledBuild != buildID {
		t.Fatalf("no se paró la compilación: %v", st.canceledBuild)
	}
}

// TestCancelarDespliegueSinCompilacionViva: un despliegue promocionado (rollback) no
// tiene compilación propia. No hay nada que parar y tampoco un error que dar.
func TestCancelarDespliegueSinCompilacionViva(t *testing.T) {
	st := &fakeStore{dep: domain.Deployment{ID: uuid.New()}}
	s := &Service{store: st}

	if err := s.CancelDeployment(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), st.dep.ID); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if st.canceledBuild != nil {
		t.Fatalf("se canceló una compilación que no existía: %v", st.canceledBuild)
	}
}

// TestCancelarDespliegueAunqueLaCompilacionYaAcabase: el caso normal —la compilación ya
// terminó— llega como ErrInvalidState y NO puede hacer que la interfaz diga que no se
// pudo cancelar, porque el despliegue sí se canceló.
func TestCancelarDespliegueAunqueLaCompilacionYaAcabase(t *testing.T) {
	buildID := uuid.New()
	st := &fakeStore{
		dep:            domain.Deployment{ID: uuid.New(), BuildID: &buildID},
		cancelBuildErr: domain.ErrInvalidState,
	}
	s := &Service{store: st}

	if err := s.CancelDeployment(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), st.dep.ID); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
}
