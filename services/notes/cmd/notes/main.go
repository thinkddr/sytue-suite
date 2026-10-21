// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command notes es el binario del servicio Sytue Notepads (:8089): API REST de
// cuadernos, notas, etiquetas con estado, búsqueda y referencias cruzadas con el
// resto de la suite. Estado en PostgreSQL (única fuente de verdad); auth vía
// Sytue ID (JWT/JWKS).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/config"
	noteshttp "git.sytue.eu/sytue/one/services/notes/internal/http"
	"git.sytue.eu/sytue/one/services/notes/internal/reminders"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
	"git.sytue.eu/sytue/one/services/notes/internal/trashgc"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			// Sonda local del proceso usada por Docker/Compose.
			os.Exit(runHealthcheck())
		case "dbcheck":
			// Preflight del despliegue: verifica credenciales y red contra la RDB
			// sin cargar OIDC, migrar ni arrancar un servidor.
			if err := runDBCheck(); err != nil {
				fmt.Fprintf(os.Stderr, "dbcheck: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sytue-notes: error fatal: %v\n", err)
		os.Exit(1)
	}
}

func runDBCheck() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL está vacía")
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

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cargar configuracion: %w", err)
	}
	if version != "" {
		cfg.Version = version
	}

	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: config.ServiceName,
		Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	telemetry.SetDefault(tp)

	// --- PostgreSQL (única fuente de verdad) ---------------------------------
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pg.MigrateUp(notesdb.Migrations, notesdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// --- Verificador de tokens de Sytue ID (JWKS) ----------------------------
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("inicializar verificador OIDC: %w", err)
	}

	st := store.New(pool)

	router := noteshttp.NewRouter(noteshttp.Deps{
		Cfg:      cfg,
		Store:    st,
		Verifier: verifier,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
		},
	})

	// Auto-purga de la papelera: cada TrashGCInterval destruye lo que lleve más de
	// TrashRetention borrado, en todas las organizaciones. En segundo plano y con
	// el mismo contexto que el servidor, así que se apaga con él.
	//
	// El propio worker se planta si la retención está a cero, pero la condición
	// también va aquí para no dejar una gorutina viva que solo sabe salirse.
	if cfg.TrashRetention > 0 {
		gc := trashgc.NewWorker(st, cfg.TrashGCInterval, cfg.TrashRetention)
		go func() {
			if err := gc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("worker de auto-purga de papelera terminó con error", "error", err)
			}
		}()
	}

	// Recordatorios de notas: cada minuto avisa en el Hub de lo vencido, en todas las
	// organizaciones. Con el mismo contexto que el servidor, así que se apaga con él.
	avisos := reminders.NewWorker(st, time.Minute, hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret), cfg.PublicURL)
	go func() {
		if err := avisos.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("worker de recordatorios terminó con error", "error", err)
		}
	}()

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("sytue-notes iniciado",
		"addr", cfg.HTTPAddr, "public_url", cfg.PublicURL, "env", string(cfg.Env))
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-notes detenido limpiamente")
	return nil
}

// runHealthcheck implementa el subcomando "healthcheck": pide /healthz al puerto
// local y devuelve 0 si responde 200, 1 en caso contrario.
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
