// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command bases es el binario de Sytue Bases (:8102), las tablas relacionales
// colaborativas de la suite. Un binario, varios modos:
//
//	bases              la API (sin argumentos)
//	bases healthcheck  sonda local del proceso (Docker, compose)
//	bases dbcheck      preflight del despliegue contra la base
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	basesdb "git.sytue.eu/sytue/one/services/bases/db"
	"git.sytue.eu/sytue/one/services/bases/internal/config"
	baseshttp "git.sytue.eu/sytue/one/services/bases/internal/http"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

func main() {
	modo := ""
	if len(os.Args) > 1 {
		modo = os.Args[1]
	}
	var err error
	switch modo {
	case "healthcheck":
		os.Exit(runHealthcheck())
	case "dbcheck":
		err = runDBCheck()
	case "":
		err = runAPI()
	default:
		err = fmt.Errorf("subcomando desconocido %q (healthcheck, dbcheck)", modo)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sytue-bases %s: %v\n", modo, err)
		os.Exit(1)
	}
}

func runAPI() error {
	cfg, err := config.Cargar()
	if err != nil {
		return fmt.Errorf("cargar configuración: %w", err)
	}
	if version != "" {
		cfg.Version = version
	}
	logger := log.New(log.Options{Level: cfg.LogLevel, Format: cfg.LogFormat, Service: config.ServiceName, Version: cfg.Version})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)
	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	telemetry.SetDefault(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()
	if err := pg.MigrateUp(basesdb.Migrations, basesdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")
	st := store.New(pool)
	// Los recálculos de fórmulas que no caben en la petición que los provoca.
	go store.Bucle(ctx, 2*time.Second, st.RecalcularPendientes, func(err error) { logger.Error("recálculo de fórmulas", "err", err) })

	// Almacén de objetos de los adjuntos. Opcional: sin él, los adjuntos responden 503.
	var objetos *objectstore.Client
	if cfg.S3Configurado() {
		if objetos, err = objectstore.New(cfg.S3); err != nil {
			return fmt.Errorf("almacén de objetos: %w", err)
		}
		if cfg.Env != "prod" {
			if err := objetos.EnsureBucket(ctx, cfg.S3.Region); err != nil {
				logger.Warn("no se pudo asegurar el bucket S3", "error", err)
			}
		}
		go store.Bucle(ctx, 30*time.Second, func(ctx context.Context) (int, error) {
			return st.PurgarAdjuntos(ctx, baseshttp.BorrarObjeto(objetos))
		}, func(err error) { logger.Error("purga de adjuntos", "err", err) })
	} else {
		logger.Warn("sin almacén de objetos (S3_*): los adjuntos no están disponibles")
	}

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("inicializar verificador OIDC: %w", err)
	}

	altchaKey := hmac.New(sha256.New, []byte(cfg.AltchaSecret))
	altchaKey.Write([]byte("sytue-bases-altcha"))
	verificador := altcha.New(altchaKey.Sum(nil), altcha.ConRepeticiones(st.RegistrarAltcha))

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr: cfg.HTTPAddr,
		Handler: baseshttp.NewRouter(baseshttp.Deps{
			Cfg: cfg, Store: st, Verifier: verifier, Altcha: verificador,
			Hub:           hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret),
			Objetos:       baseshttp.AlmacenDe(objetos),
			ReadyCheckers: []httpx.Checker{httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz}},
		}),
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	logger.Info("sytue-bases iniciado", "addr", cfg.HTTPAddr, "env", string(cfg.Env))
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-bases detenido limpiamente")
	return nil
}

func runDBCheck() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL está vacía")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	pool.Close()
	return nil
}

// runHealthcheck pide /healthz al puerto local: 0 si responde 200.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: estado %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
