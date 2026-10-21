// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// CLOUD-06: una organización no llena la cola de builds de todas ni agota con
// previews el cupo de certificados del dominio.
func TestTopesDeCola(t *testing.T) {
	pid := uuid.New()
	nuevo := func(builds, previews int) *Service {
		return newTestService(&fakeStore{
			memberCount:  1,
			project:      domain.Project{ID: pid, Slug: "web", ProductionRef: "main"},
			build:        domain.Build{ID: uuid.New()},
			dep:          domain.Deployment{ID: uuid.New()},
			activeBuilds: builds, activePreviews: previews,
		}, &capturePub{})
	}
	ctx := tenantCtx(t)
	pr := 7
	prod := DeployInput{ProjectID: pid, Target: domain.TargetProduction, GitRef: "main"}
	prev := DeployInput{ProjectID: pid, Target: domain.TargetPreview, GitRef: "parche", PRNumber: &pr}

	if _, _, err := nuevo(domain.MaxActiveBuildsPerOrg, 0).Deploy(ctx, prod); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("cola de builds llena: %v", err)
	}
	if _, _, err := nuevo(0, domain.MaxLivePreviewsPerProject).Deploy(ctx, prev); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("previews al tope: %v", err)
	}
	if _, _, err := nuevo(domain.MaxActiveBuildsPerOrg-1, domain.MaxLivePreviewsPerProject-1).Deploy(ctx, prev); err != nil {
		t.Fatalf("por debajo de los topes: %v", err)
	}
}
