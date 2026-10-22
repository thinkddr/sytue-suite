// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command analytics-ingest es el COLECTOR de Sytue Analytics (ADR 0026): sirve
// /_sytue/s.js y recibe /_sytue/e bajo el dominio del propio sitio del cliente.
//
// Es el endpoint público, sin autenticar y de mayor volumen de toda la
// plataforma, y por eso es un proceso aparte: un fallo en el parser del beacon
// no puede estar a un salto de las credenciales de Kubernetes ni de la
// facturación (que viven en services/cloud). Aquí dentro solo hay la sal del
// día, la tabla de sitios y una cola de escritura.
//
// Subcomandos:
//
//	analytics-ingest              arranca el colector (por defecto)
//	analytics-ingest healthcheck  sonda de liveness para Docker
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	"git.sytue.eu/sytue/one/services/analytics/internal/config"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
	analyticshttp "git.sytue.eu/sytue/one/services/analytics/internal/http"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(config.PortIngest))
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(config.ServiceNameIngest, config.PortIngest)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceNameIngest, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceNameIngest, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// El colector NO migra: las migraciones las aplica analytics-api. Este
	// proceso escala a más réplicas y no tiene por qué poder tocar el esquema.
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("KEK de la sal: %w", err)
	}
	if !cipher.Enabled() {
		logger.Warn("SIN KEK: la sal diaria se guarda EN CLARO. " +
			"Un volcado de PostgreSQL bastaría para recalcular los identificadores del día " +
			"(define SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY fuera de desarrollo)")
	}

	// País sin guardar IP. Sin fichero, todo el tráfico queda como "ZZ" y el
	// servicio arranca igual: la geolocalización es un desglose, no la medición.
	countries, err := domain.LoadCountries(cfg.GeoDBPath)
	if err != nil {
		logger.Warn("sin base de países: el desglose geográfico quedará en ZZ",
			"path", cfg.GeoDBPath, "error", err)
	}

	st := store.New(pool)
	writer := store.NewWriter(st, cfg.QueueSize, cfg.BatchSize, cfg.BatchInterval)

	router := analyticshttp.NewIngestRouter(analyticshttp.IngestDeps{
		Cfg:       cfg,
		Sites:     st,
		Salts:     store.NewSalts(pool, cipher, store.IngestSettingKey),
		Sink:      writer,
		Countries: countries,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
	})

	go func() {
		if err := writer.Run(ctx); err != nil {
			logger.Error("escritor de eventos detenido", "error", err)
		}
	}()

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	logger.Info("colector de Sytue Analytics arrancando", "addr", cfg.HTTPAddr, "env", string(cfg.Env))
	return srv.Run(ctx)
}

// healthcheck es la sonda usada por Docker (no requiere shell ni curl).
func healthcheck(defaultPort int) int {
	port := os.Getenv("PORT")
	if port == "" {
		port = fmt.Sprint(defaultPort)
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
