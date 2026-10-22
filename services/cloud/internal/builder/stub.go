// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// StubRunner es una implementación de Runner para dev/local que NO programa Jobs
// reales en Kubernetes (no hay clúster en dev local). Simula una build correcta
// tras un breve retardo y devuelve la referencia de imagen destino. El runner de
// producción (KubeRunner sobre Kapsule) lo sustituye vía configuración.
//
// Esto permite ejercitar el flujo completo (cola → build → deploy) en el entorno
// de desarrollo de un comando, sin depender de un clúster ni de red externa.
type StubRunner struct {
	cfg   config.Config
	delay time.Duration

	mu    sync.Mutex
	calls []KanikoRequest
}

// NewStubRunner construye un StubRunner.
func NewStubRunner(cfg config.Config) *StubRunner {
	return &StubRunner{cfg: cfg, delay: 500 * time.Millisecond}
}

// Run simula una build correcta. Implementa builder.Runner.
func (s *StubRunner) Run(ctx context.Context, req KanikoRequest) KanikoResult {
	log.From(ctx).Info("stub-build (dev): simulando build de Kaniko",
		"destination", req.Destination, "context", req.RepoURL)
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return KanikoResult{Err: ctx.Err()}
	case <-time.After(s.delay):
	}
	key := fmt.Sprintf("builds/%s/%s.log", req.Tenant, req.BuildID)
	return KanikoResult{ImageRef: req.Destination, LogsKey: key}
}

// Calls devuelve las solicitudes recibidas (para tests).
func (s *StubRunner) Calls() []KanikoRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]KanikoRequest, len(s.calls))
	copy(out, s.calls)
	return out
}
