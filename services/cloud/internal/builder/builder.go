// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package builder implementa el ciclo de build de Sytue Cloud: toma builds
// 'queued' de la cola (PostgreSQL, FOR UPDATE SKIP LOCKED), clona el repo,
// detecta el framework, construye la imagen OCI con Kaniko (Job de Kapsule,
// rootless), la empuja al Container Registry de Scaleway y cachea capas en S3.
// Al terminar, actualiza el estado de la build y emite un evento que el
// controller reconcilia para desplegar.
//
// El Runner de Kaniko es una interfaz: la implementación por defecto programa un
// Job de Kubernetes; en dev puede sustituirse por un stub. Mantener el orquestador
// desacoplado del cliente de k8s permite tests y evolución sin tocar el bucle.
package builder

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// KanikoRequest describe una construcción a ejecutar con Kaniko.
type KanikoRequest struct {
	BuildID    uuid.UUID
	Tenant     string
	RepoURL    string
	GitRef     string
	CommitSHA  string
	ContextSub string // subdirectorio del monorepo (root_dir)
	Dockerfile string // ruta del Dockerfile (generado o del repo)
	// DockerfileContent es el Dockerfile generado a materializar en /workspace
	// (frameworks gestionados). Vacío => usar el Dockerfile del repo ('docker').
	DockerfileContent string
	// Buildpacks selecciona la estrategia Cloud Native Buildpacks (Job con el
	// lifecycle de CNB / Paketo) en vez de Dockerfile+Kaniko. Con Buildpacks no se
	// usa Dockerfile/DockerfileContent: el buildpack auto-detecta el lenguaje.
	Buildpacks  bool
	Destination string // registry/imagen:tag
	BuildArgs   map[string]string
	// GitSSHKey es la deploy key privada (OpenSSH) para repos privados.
	GitSSHKey string
	// GitHTTPSToken es un installation token efímero de la GitHub App para clonar
	// el repo privado por HTTPS (basic auth x-access-token:<token> vía env desde un
	// Secret montado). Tiene prioridad sobre GitSSHKey si ambos están.
	GitHTTPSToken string
	// OnJobStarted se invoca tras crear el Job (para persistir kaniko_job_name).
	OnJobStarted func(jobName string)
	// IsCanceled consulta si la build fue cancelada (poll durante Wait).
	IsCanceled func() bool
}

// KanikoResult es el desenlace de un Job de Kaniko.
type KanikoResult struct {
	ImageRef string // registry/imagen@sha256:digest
	LogsKey  string // clave S3 del log de build
	JobName  string
	Err      error
}

// Runner ejecuta una build de Kaniko (Job Kapsule rootless) y bloquea hasta su
// fin. La implementación por defecto vive en kaniko.go.
type Runner interface {
	Run(ctx context.Context, req KanikoRequest) KanikoResult
}

// Builder es el worker que drena la cola de builds.
type Builder struct {
	cfg    config.Config
	svc    *service.Service
	runner Runner
	worker string
}

// New construye el Builder.
func New(cfg config.Config, svc *service.Service, runner Runner) *Builder {
	return &Builder{cfg: cfg, svc: svc, runner: runner, worker: "cloud-builder/" + uuid.NewString()[:8]}
}

// Run drena la cola hasta que se cancele el contexto. Hace poll con un breve
// backoff cuando no hay trabajo (la cola vive en PostgreSQL; sin Redis).
func (b *Builder) Run(ctx context.Context) error {
	l := log.From(ctx).With("component", "cloud-builder", "worker", b.worker)
	l.Info("builder arrancado")
	idle := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		processed, err := b.processOne(ctx)
		if err != nil {
			l.Warn("error procesando build", "error", err)
		}
		if processed {
			continue // drena agresivamente mientras haya trabajo
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(idle):
		}
	}
}

// processOne toma y ejecuta una build. Devuelve processed=false si la cola está
// vacía.
func (b *Builder) processOne(ctx context.Context) (bool, error) {
	st := b.svc.Store()
	claimed, ok, err := st.ClaimNextBuild(ctx, b.worker)
	if err != nil {
		return false, fmt.Errorf("reclamar build: %w", err)
	}
	if !ok {
		return false, nil
	}

	l := log.From(ctx).With("build_id", claimed.Build.ID, "tenant", claimed.Build.Tenant,
		"project", claimed.Project.Slug)
	l.Info("build reclamada", "commit", claimed.Build.CommitSHA, "ref", claimed.Build.GitRef)

	res := b.build(ctx, claimed)
	if res.Status == domain.BuildSucceeded {
		l.Info("build correcta", "image", res.ImageRef)
	} else {
		l.Warn("build fallida", "error", res.ErrorMsg)
	}

	if err := st.FinishBuild(ctx, claimed.Build.Tenant, claimed.Build.ID, res); err != nil {
		return true, fmt.Errorf("finalizar build: %w", err)
	}
	b.recordBuildUsage(ctx, claimed)
	return true, nil
}

// build ejecuta una build completa y devuelve su resultado de store.
func (b *Builder) build(ctx context.Context, c store.ClaimedBuild) store.BuildResult {
	bctx, cancel := context.WithTimeout(ctx, b.cfg.BuildTimeout)
	defer cancel()
	st := b.svc.Store()

	fw := DetectFramework(c.Project)
	strategy := ResolveBuildStrategy(c.Project, fw)

	dest := b.imageDestination(c.Project, c.Build)
	gitSSHKey := ""
	if len(c.RepoCredsEnc) > 0 {
		if key, err := b.svc.Sealer().OpenString(c.RepoCredsEnc); err == nil {
			gitSSHKey = key
		}
	}
	// GitHub App: acuña un installation token EFÍMERO para clonar el repo privado
	// por HTTPS. No se persiste (vive lo que el Job). Tiene prioridad sobre la
	// deploy-key SSH; si falla, degrada a la deploy-key / clone público.
	gitToken := ""
	if c.InstallationID != 0 {
		if gh := b.svc.GitHub(); gh != nil {
			if tok, _, err := gh.InstallationToken(bctx, c.InstallationID); err != nil {
				log.From(ctx).Warn("no se pudo acuñar installation token de GitHub App; se usa deploy-key/clone público",
					"error", err, "project", c.Project.Slug)
			} else {
				gitToken = tok
				gitSSHKey = "" // el token HTTPS efímero manda: no montar también la SSH key
			}
		}
	}
	req := KanikoRequest{
		BuildID:       c.Build.ID,
		Tenant:        c.Build.Tenant,
		RepoURL:       c.Project.RepoURL,
		GitRef:        c.Build.GitRef,
		CommitSHA:     c.Build.CommitSHA,
		ContextSub:    c.Project.RootDir,
		Destination:   dest,
		BuildArgs:     map[string]string{"SYTUE_FRAMEWORK": string(fw)},
		GitSSHKey:     gitSSHKey,
		GitHTTPSToken: gitToken,
		OnJobStarted: func(jobName string) {
			_ = st.SetBuildJobName(ctx, c.Build.Tenant, c.Build.ID, jobName)
		},
		IsCanceled: func() bool {
			status, err := st.BuildStatus(ctx, c.Build.Tenant, c.Build.ID)
			return err == nil && status == domain.BuildCanceled
		},
	}
	if strategy == domain.BuildStrategyBuildpacks {
		// CNB: el buildpack auto-detecta el lenguaje; no hay Dockerfile.
		req.Buildpacks = true
	} else {
		// Dockerfile propio ('docker') o generado (frameworks gestionados
		// astro/node/go/static). 'auto' no llega aquí por defecto (resuelve a
		// buildpacks); si el usuario fuerza build_strategy='dockerfile' en 'auto',
		// GenerateDockerfile no tiene plantilla y el contenido queda vacío.
		req.Dockerfile = DockerfileFor(fw, c.Project)
		req.DockerfileContent, _ = GenerateDockerfile(fw, c.Project)
	}

	out := b.runner.Run(bctx, req)
	if out.Err != nil {
		if req.IsCanceled != nil && req.IsCanceled() {
			return store.BuildResult{Status: domain.BuildCanceled, Framework: fw, LogsKey: out.LogsKey, ErrorMsg: "cancelada"}
		}
		return store.BuildResult{
			Status:    domain.BuildFailed,
			Framework: fw,
			LogsKey:   out.LogsKey,
			ErrorMsg:  truncate(out.Err.Error(), 1000),
		}
	}
	return store.BuildResult{
		Status:    domain.BuildSucceeded,
		ImageRef:  out.ImageRef,
		Framework: fw,
		LogsKey:   out.LogsKey,
	}
}

// imageDestination deriva la etiqueta destino en el Container Registry. Se
// etiqueta por commit corto (inmutable) bajo el repo del proyecto.
func (b *Builder) imageDestination(p domain.Project, bld domain.Build) string {
	tag := bld.CommitSHA
	if len(tag) > 12 {
		tag = tag[:12]
	}
	if tag == "" {
		tag = bld.ID.String()[:12]
	}
	return fmt.Sprintf("%s/%s:%s", strings.TrimRight(b.cfg.RegistryURL, "/"), p.Slug, tag)
}

// recordBuildUsage registra los minutos de build consumidos (facturación).
func (b *Builder) recordBuildUsage(ctx context.Context, c store.ClaimedBuild) {
	var minutes float64
	if c.Build.StartedAt != nil {
		minutes = time.Since(*c.Build.StartedAt).Minutes()
	}
	if minutes <= 0 {
		return
	}
	pid := c.Project.ID
	_ = b.svc.Store().RecordUsage(ctx, store.UsageInput{
		Tenant:     c.Build.Tenant,
		ProjectID:  &pid,
		Metric:     "build_minutes",
		Quantity:   minutes,
		Unit:       "minutes",
		PeriodFrom: c.Build.CreatedAt,
		PeriodTo:   time.Now().UTC(),
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
