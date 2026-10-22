// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command contacts es el binario de Sytue Contacts (servidor CardDAV RFC 6352 +
// API REST propia; estado en PostgreSQL, única fuente de verdad).
//
// Subcomandos:
//
//	contacts              arranca el servidor HTTP/CardDAV (por defecto)
//	contacts migrate      aplica las migraciones de base de datos y sale
//	contacts healthcheck  sonda de liveness para el healthcheck de Docker
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
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	contactsdb "git.sytue.eu/sytue/one/services/contacts/db"
	carddavsrv "git.sytue.eu/sytue/one/services/contacts/internal/carddav"
	"git.sytue.eu/sytue/one/services/contacts/internal/config"
	contactshttp "git.sytue.eu/sytue/one/services/contacts/internal/http"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(runHealthcheck())
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

// run arranca el servidor completo.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	// Telemetría (stub no-op por ahora).
	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// Migraciones al arrancar (paridad dev/prod; idempotente).
	if err := pg.MigrateUp(contactsdb.Migrations, contactsdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// PostgreSQL (único datastore).
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Verificador de JWT de Sytue ID (JWKS).
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	st := store.New(pool)
	// Cache efímera compartida: guarda la lista de libretas de cada usuario, que es
	// lo primero que pide CADA sincronización CardDAV. Ahorra la consulta y, sobre
	// todo, el UPSERT de auto-aprovisionamiento que corría en cada PROPFIND.
	//
	// DEGRADACIÓN: sin DRAGONFLY_URL, nil, y el backend consulta PostgreSQL.
	var davCache *cache.Client
	if cfg.DragonflyURL != "" {
		// El cliente se conserva aunque el Ping falle: la sonda reconecta sola.
		c, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
		if err != nil {
			logger.Warn("Dragonfly no disponible al arrancar; las libretas CardDAV se listan en PostgreSQL hasta que reconecte", "error", err)
		}
		davCache = c
		defer func() { _ = c.Close() }()
	}
	davHandler := carddavsrv.Handler(st, davCache)

	router := contactshttp.NewRouter(contactshttp.Deps{
		Cfg:      cfg,
		Store:    st,
		Verifier: verifier,
		CardDAV:  davHandler,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	// Sin el Secret sytue-contacts-internal (optional en el chart) la ruta
	// /internal/avatar-sync no se monta y la foto de Sytue ID no llega a las
	// fichas. Fuera de dev eso es un despliegue incompleto, y se dice.
	if cfg.Env != pcfg.EnvDev && cfg.InternalSecret == "" {
		logger.Warn("falta INTERNAL_SECRET (Secret sytue-contacts-internal): /internal/avatar-sync no se monta y la foto de Sytue ID no llega a las fichas")
	}
	logger.Info("Sytue Contacts arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL)
	return srv.Run(ctx)
}

// runMigrate aplica las migraciones y sale (uso operativo / Job de despliegue).
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return pg.MigrateUp(contactsdb.Migrations, contactsdb.MigrationsDir, cfg.DatabaseURL)
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/contacts","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
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
