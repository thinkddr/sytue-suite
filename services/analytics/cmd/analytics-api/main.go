// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command analytics-api es la API de lectura de Sytue Analytics (ADR 0026) y el
// hogar de sus crons: rollup de agregados, creación y caducidad de particiones y
// rotación de la sal diaria.
//
// Todas sus consultas van contra los AGREGADOS. El crudo solo vive 48 h y es
// donde está el hash seudónimo: el panel no lo toca nunca.
//
// Subcomandos:
//
//	analytics-api              arranca la API + los crons (por defecto)
//	analytics-api migrate      aplica las migraciones y sale
//	analytics-api healthcheck  sonda de liveness para Docker
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	analyticsdb "git.sytue.eu/sytue/one/services/analytics/db"
	"git.sytue.eu/sytue/one/services/analytics/internal/config"
	analyticshttp "git.sytue.eu/sytue/one/services/analytics/internal/http"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck(config.PortAPI))
		case "migrate":
			if err := runMigrate(); err != nil {
				fmt.Fprintln(os.Stderr, "migrate:", err)
				os.Exit(1)
			}
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(config.ServiceNameAPI, config.PortAPI)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceNameAPI, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceNameAPI, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	if err := pg.MigrateUp(analyticsdb.Migrations, analyticsdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("KEK de la sal: %w", err)
	}
	st := store.New(pool)

	router := analyticshttp.NewAPIRouter(analyticshttp.APIDeps{
		Cfg:      cfg,
		Verifier: verifier,
		Store:    st,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
	})

	salts := store.NewSalts(pool, cipher, pg.MaintenanceSettingKey)
	go runMaintenance(ctx, st, salts, cfg)

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	logger.Info("API de Sytue Analytics arrancando", "addr", cfg.HTTPAddr, "env", string(cfg.Env))
	return srv.Run(ctx)
}

// runMaintenance es el cron único del servicio. Tres tareas, en este orden y por
// una razón cada una:
//
//  1. la SAL del día — pedirla rota la de ayer y borra su fila, que es lo que
//     hace imposible correlacionar visitantes entre días;
//  2. las PARTICIONES — crear las que faltan por delante y retirar con DROP las
//     que caducan (retención del crudo: 48 h);
//  3. el ROLLUP — recalcular los agregados, que son lo único que lee el panel.
//
// El rollup va DESPUÉS de crear particiones y ANTES de que caduquen las viejas
// dentro del mismo ciclo, para no perder un día sin agregar.
func runMaintenance(ctx context.Context, st *store.Store, salts *store.Salts, cfg config.Config) {
	tick := func() {
		if _, err := salts.Today(ctx); err != nil {
			log.From(ctx).Error("rotación de la sal diaria falló", "error", err)
		}
		if err := st.EnsurePartitions(ctx, cfg.PartitionsAhead); err != nil {
			log.From(ctx).Error("crear particiones falló", "error", err)
		}
		if err := st.Rollup(ctx, time.Now(), cfg.BreakdownTopN); err != nil {
			log.From(ctx).Error("rollup falló", "error", err)
		}
		if n, err := st.ExpirePartitions(ctx, cfg.RawRetentionDays); err != nil {
			log.From(ctx).Error("purga de particiones falló", "error", err)
		} else if n > 0 {
			log.From(ctx).Info("particiones de crudo retiradas", "count", n)
		}
	}

	tick()
	ticker := time.NewTicker(cfg.RollupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

func runMigrate() error {
	cfg, err := config.Load(config.ServiceNameAPI, config.PortAPI)
	if err != nil {
		return err
	}
	return pg.MigrateUp(analyticsdb.Migrations, analyticsdb.MigrationsDir, cfg.DatabaseURL)
}

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
