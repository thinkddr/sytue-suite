// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// CLOUD-18: el `state` de la instalación de la GitHub App lo emite el servidor,
// vale una vez y solo para quien lo pidió.
func TestEstadoDeInstalacionDeGitHub(t *testing.T) {
	mr := miniredis.RunT(t)
	c, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := newTestService(&fakeStore{}, nil)
	s.UsarCache(c)
	ctx := tenantCtx(t)
	pid := uuid.New()

	state, err := s.StartGitHubInstall(ctx, pid, "ana")
	if err != nil || state == "" || state == pid.String() {
		t.Fatalf("state = %q, err = %v", state, err)
	}
	if _, err := s.ConsumeGitHubInstall(ctx, state, "mallory"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("otra persona canjea el state: %v", err)
	}
	// Se consumió aunque no valiera: ni la dueña puede reutilizarlo.
	if _, err := s.ConsumeGitHubInstall(ctx, state, "ana"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("state reutilizado: %v", err)
	}

	state, _ = s.StartGitHubInstall(ctx, pid, "ana")
	got, err := s.ConsumeGitHubInstall(ctx, state, "ana")
	if err != nil || got != pid {
		t.Fatalf("canje legítimo: %v %v", got, err)
	}
	if _, err := s.ConsumeGitHubInstall(ctx, pid.String(), "ana"); err == nil {
		t.Fatal("el id del proyecto como state no puede valer")
	}
}
