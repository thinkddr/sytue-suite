// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// CLOUD-14: lo que cambia qué se ejecuta o quién entra deja rastro en la
// auditoría, sin el valor de los secretos.
func TestAccionesQueDejanRastro(t *testing.T) {
	fs := &fakeStore{}
	s := newTestService(fs, nil)
	ctx := tenantCtx(t)
	pid := uuid.New()

	if _, err := s.SetEnv(ctx, SetEnvInput{ProjectID: pid, Key: "API_KEY", Value: "muy-secreto", IsSecret: true}); err != nil {
		t.Fatalf("SetEnv: %v", err)
	}
	if err := s.DeleteEnv(ctx, uuid.New()); err != nil {
		t.Fatalf("DeleteEnv: %v", err)
	}
	cmd := "make release"
	if _, err := s.UpdateProject(ctx, pid, store.UpdateProjectRecord{ReleaseCommand: &cmd}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if _, err := s.AddDomain(ctx, pid, "tienda.example.eu"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	s.AuditExec(ctx, pid, uuid.New(), -1)
	s.AuditExec(ctx, pid, uuid.New(), 90*time.Second)

	acciones := make([]string, 0, len(fs.audited))
	for _, a := range fs.audited {
		acciones = append(acciones, a.Action)
		if strings.Contains(fmt.Sprint(a.Detail), "muy-secreto") {
			t.Fatalf("el valor del secreto llegó a la auditoría: %+v", a)
		}
	}
	got := strings.Join(acciones, ",")
	for _, want := range []string{"env.set", "env.delete", "project.update", "domain.add", "exec.open", "exec.close"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en la auditoría: %s", want, got)
		}
	}
}
