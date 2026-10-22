// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cloud-builder es el worker de build de Sytue Cloud (puerto 8091 para
// salud/observabilidad): drena la cola de builds en PostgreSQL, clona el repo,
// detecta el framework y construye la imagen OCI con Kaniko (Job Kapsule
// rootless), la empuja al Container Registry de Scaleway y cachea capas en S3.
//
// En dev usa un Runner stub (no hay clúster local); en prod se cablea el
// KubeRunner contra Kapsule.
//
// Subcomandos:
//
//	cloud-builder              arranca el worker + endpoint de salud (por defecto)
//	cloud-builder healthcheck  sonda de liveness para Docker
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	"git.sytue.eu/sytue/one/services/cloud/internal/builder"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(config.DefaultBuilderPort)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName + "-builder", Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName + "-builder", Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	sealer, err := crypto.NewSealer(cfg.SecretEncKey)
	if err != nil {
		return fmt.Errorf("sellador de secretos: %w", err)
	}

	st := store.New(pool)
	// blobs nil: el builder no sirve logs por HTTP (los sube al Object Storage
	// vía su propio S3LogSink); solo cloud-api los lee.
	svc := service.New(cfg, st, sealer, service.NewPoolPublisher(pool), nil)

	// GitHub App (opcional): habilita el clone de repos privados con un
	// installation token efímero. Clave inválida ⇒ no-op (deploy-key / público).
	if gh, gerr := github.NewClient(cfg.GitHubAppID, cfg.GitHubAppPrivateKey); gerr != nil {
		logger.Warn("GitHub App deshabilitada: configuración inválida", "error", gerr)
	} else if gh != nil {
		svc.SetGitHub(gh)
		logger.Info("GitHub App habilitada para clone de repos privados", "app_id", cfg.GitHubAppID)
	}

	// Object Storage para cache de builds y logs (best-effort en dev).
	var logSink builder.LogSink
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket,
	})
	if err != nil {
		logger.Warn("Object Storage no disponible; logs de build deshabilitados", "error", err)
	} else {
		if !cfg.Env.IsProd() {
			if berr := blobs.EnsureBucket(ctx, cfg.S3Region); berr != nil {
				logger.Warn("no se pudo asegurar el bucket build-cache", "error", berr)
			}
		}
		logSink = builder.NewS3LogSink(blobs)
	}

	// Runner: en prod, KubeRunner sobre Kapsule (client-go); en dev, StubRunner.
	// La selección es puramente de configuración (paridad dev/prod, §15).
	var runner builder.Runner
	if cfg.UseKubeRunner() {
		jobClient, jerr := builder.NewKubeJobClient(builder.KubeJobConfig{
			KubeconfigPath: cfg.KubeconfigPath,
			BuildkitImage:  cfg.BuildkitImage,
			CraneImage:     cfg.CraneImage,
		})
		if jerr != nil {
			return fmt.Errorf("cliente de Jobs de Kapsule: %w", jerr)
		}
		runner = builder.NewKubeRunner(cfg, jobClient, logSink)
		logger.Info("builder en modo Kapsule (client-go)", "build_namespace", cfg.BuildNamespace)
	} else {
		runner = builder.NewStubRunner(cfg)
		logger.Info("builder en modo stub (dev, sin clúster)")
	}

	bld := builder.New(cfg, svc, runner)

	// Endpoint de salud (8091) para Kubernetes/observabilidad.
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 5 * time.Second})
	httpx.MountHealth(r, cfg.Version, 3*time.Second, httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz})
	srv := httpx.NewServer(httpx.ServerConfig{Addr: cfg.HTTPAddr, Handler: r, ShutdownTimeout: cfg.ShutdownTimeout})

	logger.Info("Sytue Cloud builder arrancando", "addr", cfg.HTTPAddr, "env", string(cfg.Env))

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return bld.Run(gctx) })
	g.Go(func() error { return srv.Run(gctx) })
	return g.Wait()
}

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultBuilderPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
