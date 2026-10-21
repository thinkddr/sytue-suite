// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command hub es el binario de Sytue Hub (backend de integración seamless de la
// suite, "shell backend"): búsqueda federada, notificaciones unificadas,
// presencia e intents cross-app. Estado durable en PostgreSQL; tiempo real por
// LISTEN/NOTIFY; presencia efímera en Dragonfly (claves con TTL) con
// degradación a PostgreSQL. Auth: JWT de Sytue ID (client sytue-hub).
//
// Subcomandos:
//
//	hub              arranca el servidor HTTP + relay SSE (por defecto)
//	hub migrate      aplica las migraciones de base de datos y sale
//	hub healthcheck  sonda de liveness para el healthcheck de Docker
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
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/config"
	"git.sytue.eu/sytue/one/services/hub/internal/connectors"
	hubhttp "git.sytue.eu/sytue/one/services/hub/internal/http"
	"git.sytue.eu/sytue/one/services/hub/internal/instaladores"
	"git.sytue.eu/sytue/one/services/hub/internal/intents"
	"git.sytue.eu/sytue/one/services/hub/internal/interpretar"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/presence"
	"git.sytue.eu/sytue/one/services/hub/internal/search"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
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

// run arranca el servidor completo: HTTP + relay de notificaciones + purga.
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
	if err := pg.MigrateUp(hubdb.Migrations, hubdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// PostgreSQL (único datastore: notificaciones, presencia).
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Verificador de JWT de Sytue ID (JWKS), client sytue-hub.
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	st := store.New(pool)

	// Dragonfly (cache efímera): la presencia vive como claves con TTL en vez de
	// UPSERTs por heartbeat en PostgreSQL. DEGRADACIÓN: sin DRAGONFLY_URL o sin
	// conexión al arrancar => nil y la presencia sigue en PostgreSQL (el
	// comportamiento previo); el hub nunca depende de Dragonfly para arrancar.
	var presenceCache *cache.Client
	if cfg.DragonflyURL != "" {
		// EL CLIENTE SE CONSERVA AUNQUE EL PING FALLE. Antes se descartaba, y eso
		// convertía cualquier caída momentánea de Dragonfly en presencia degradada
		// para siempre: no había reconexión y solo un reinicio del pod la devolvía.
		c, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
		if err != nil {
			logger.Warn("Dragonfly no disponible al arrancar; la presencia sigue en PostgreSQL hasta que reconecte", "error", err)
		}
		presenceCache = c
		defer func() { _ = c.Close() }()
	}

	// Conectores a los servicios de la suite (búsqueda federada en nombre del usuario).
	correo := connectors.NewMailConnector(cfg.MailBaseURL, cfg.MailPublicURL, cfg.UpstreamTimeout)
	registry := connectors.NewRegistry(
		connectors.NewDriveConnector(cfg.DriveBaseURL, cfg.DrivePublicURL, cfg.UpstreamTimeout),
		correo,
		connectors.NewCalendarConnector(cfg.CalendarBaseURL, cfg.CalendarPublicURL, cfg.UpstreamTimeout),
		connectors.NewContactsConnector(cfg.ContactsBaseURL, cfg.ContactsPublicURL, cfg.UpstreamTimeout),
		connectors.NewRoomsConnector(cfg.RoomsBaseURL, cfg.RoomsPublicURL, cfg.UpstreamTimeout),
		connectors.NewNotesConnector(cfg.NotesBaseURL, cfg.NotesPublicURL, cfg.UpstreamTimeout),
		connectors.NewERPConnector(cfg.ERPBaseURL, cfg.ERPPublicURL, cfg.UpstreamTimeout),
		connectors.NewPagesConnector(cfg.PagesBaseURL, cfg.PagesPublicURL, cfg.UpstreamTimeout),
		connectors.NewBasesConnector(cfg.BasesBaseURL, cfg.BasesPublicURL, cfg.UpstreamTimeout),
		// SaaS conectados (Microsoft 365, Google Workspace…) vía el Integration Gateway.
		connectors.NewIntegrationsConnector(cfg.IntegrationsBaseURL, cfg.UpstreamTimeout),
	)

	searchSvc := search.New(registry, cfg.SearchTimeout, cfg.SearchLimit)
	presenceSvc := presence.New(st, presenceCache, cfg.PresenceTTL)

	// Notificaciones: Hub local de SSE + servicio (crea + publica NOTIFY) + relay.
	sseHub := notify.NewHub(64)
	notifySvc := notify.NewService(st, sseHub)
	relay := notify.NewRelay(pool, sseHub)

	intentResolver := intents.NewResolver(intents.Targets{
		Drive:    cfg.DrivePublicURL,
		Mail:     cfg.MailPublicURL,
		Calendar: cfg.CalendarPublicURL,
		Contacts: cfg.ContactsPublicURL,
		Chat:     cfg.RoomsPublicURL,
	})

	// El catálogo de instaladores para el actualizador de las apps de escritorio. Sin
	// bucket el Hub arranca igual y esas dos rutas responden 503.
	var catalogo *instaladores.Catalogo
	if d := cfg.Descargas; d.Configurada() {
		catalogo, err = instaladores.Nuevo(instaladores.Config{
			Endpoint: d.Endpoint, EndpointPublico: d.EndpointPublico, Region: d.Region,
			Bucket: d.Bucket, Prefijo: d.Prefijo, AccessKey: d.AccessKey, SecretKey: d.SecretKey,
		})
		if err != nil {
			return fmt.Errorf("instaladores: %w", err)
		}
	}

	router := hubhttp.NewRouter(hubhttp.Deps{
		Cfg:      cfg,
		Verifier: verifier,
		Search:   searchSvc,
		Notify:   notifySvc,
		Hub:      sseHub,
		Store:    st,
		Presence: presenceSvc,
		Intents:  intentResolver,
		// El plazo es la mitad del de un conector: la interpretación va ANTES del
		// fan-out, así que su espera se suma a la de la búsqueda entera.
		Interprete:   interpretar.NewCliente(cfg.AIBaseURL, cfg.SearchTimeout/2),
		Instaladores: catalogo,
		Correo:       correo,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
		// El SSE NO se arregla aquí: un 0 es «el de por defecto», 30 s, y así estuvo
		// muriendo el flujo de avisos. El flujo quita su plazo (httpx.AbrirSSE) y el
		// resto de rutas conserva el suyo.
	})

	// Relay LISTEN/NOTIFY + purga periódica en goroutines ligadas al contexto.
	go func() {
		if err := relay.Run(ctx); err != nil {
			logger.Error("relay de notificaciones detenido con error", "error", err)
		}
	}()
	go runMaintenance(ctx, st, cfg)

	logger.Info("Sytue Hub arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL)
	return srv.Run(ctx)
}

// runMaintenance purga periódicamente notificaciones leídas antiguas y presencia
// obsoleta (retención/limpieza). Se ejecuta hasta que se cancela el contexto.
func runMaintenance(ctx context.Context, st *store.Store, cfg config.Config) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := st.PurgeReadBefore(ctx, time.Now().Add(-cfg.NotifyRetention)); err != nil {
				log.From(ctx).Warn("purga de notificaciones falló", "error", err)
			} else if n > 0 {
				log.From(ctx).Info("notificaciones purgadas", "count", n)
			}
			// Presencia: borra filas sin heartbeat en mucho tiempo (24x la TTL).
			if _, err := st.PurgeStalePresence(ctx, time.Now().Add(-24*cfg.PresenceTTL)); err != nil {
				log.From(ctx).Warn("purga de presencia falló", "error", err)
			}
		}
	}
}

// runMigrate aplica las migraciones y sale (uso operativo / Job de despliegue).
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return pg.MigrateUp(hubdb.Migrations, hubdb.MigrationsDir, cfg.DatabaseURL)
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/hub","healthcheck"]).
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8092"
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
