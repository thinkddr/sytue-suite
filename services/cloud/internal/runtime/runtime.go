// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package runtime abstrae operaciones en vivo sobre pods de despliegues (logs SSE,
// exec WS). En dev usa stubs; en prod client-go contra Kapsule.
package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// LogStreamer envía líneas de log de un deployment en vivo y, además, las
// RECOGE para que puedan guardarse y buscarse después (WS4/O1: el tail en vivo
// solo sirve a quien está mirando en ese momento).
type LogStreamer interface {
	StreamDeploymentLogs(ctx context.Context, dep domain.Deployment, projectSlug string, w io.Writer) error
	// CollectDeploymentLogs devuelve hasta `limit` líneas escritas después de
	// `since`, fechadas por el runtime. Lectura puntual, sin seguir el fichero.
	CollectDeploymentLogs(ctx context.Context, tenant string, deploymentID uuid.UUID, since time.Time, limit int) ([]domain.LogLine, error)
}

// ExecSession ejecuta comandos interactivos en un pod del deployment.
type ExecSession interface {
	Run(ctx context.Context, dep domain.Deployment, projectSlug string, stdin io.Reader, stdout, stderr io.Writer) error
}

// NewLogStreamer elige implementación según configuración.
func NewLogStreamer(cfg config.Config) (LogStreamer, error) {
	if cfg.UseKubeRunner() {
		return NewKubeLogStreamer(cfg)
	}
	return StubLogStreamer{}, nil
}

// NewExecSession elige implementación según configuración.
func NewExecSession(cfg config.Config) (ExecSession, error) {
	if cfg.UseKubeRunner() {
		return NewKubeExecSession(cfg)
	}
	return StubExecSession{}, nil
}

// StubLogStreamer emite mensajes simulados (dev sin clúster).
type StubLogStreamer struct{}

func (StubLogStreamer) StreamDeploymentLogs(ctx context.Context, dep domain.Deployment, _ string, w io.Writer) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for i := 0; i < 8; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			line := fmt.Sprintf("[%s] deployment %s status=%s\n", time.Now().UTC().Format(time.RFC3339), dep.ID, dep.Status)
			if _, err := io.WriteString(w, line); err != nil {
				return err
			}
		}
	}
	return nil
}

// CollectDeploymentLogs no recoge nada en dev. El stub SÍ inventa líneas para el
// tail en vivo (ahí son visiblemente falsas y se las lleva el cierre de la
// conexión), pero guardarlas ensuciaría la búsqueda con logs que nadie escribió.
func (StubLogStreamer) CollectDeploymentLogs(context.Context, string, uuid.UUID, time.Time, int) ([]domain.LogLine, error) {
	return nil, nil
}

// StubExecSession eco del stdin al stdout (dev).
type StubExecSession struct{}

func (StubExecSession) Run(ctx context.Context, _ domain.Deployment, _ string, stdin io.Reader, stdout, _ io.Writer) error {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := stdin.Read(buf)
		if n > 0 {
			if _, werr := stdout.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
