// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command calendar es el binario del servicio Sytue Calendar (:8087): API REST
// de calendarios/eventos/invitaciones/free-busy + servidor CalDAV (RFC 4791).
// Estado en PostgreSQL (unica fuente de verdad); auth via Sytue ID (JWT/JWKS).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// tzdata: empotra la base de datos de zonas horarias IANA en el binario. La
	// imagen de runtime es distroless/static (sin /usr/share/zoneinfo), así que
	// time.LoadLocation y el parseo de DTSTART;TZID de go-ical dependen de esta
	// copia embebida para resolver zonas y DST en producción. Es stdlib.
	_ "time/tzdata"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	caldb "git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	calendarhttp "git.sytue.eu/sytue/one/services/calendar/internal/http"
	"git.sytue.eu/sytue/one/services/calendar/internal/reminders"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
	"git.sytue.eu/sytue/one/services/calendar/internal/subscriptions"
)

// version se inyecta en build via -ldflags "-X main.version=...". Si esta
// presente, prevalece sobre la variable de entorno VERSION.
var version string

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "avisos-historicos":
			if err := runAvisosHistoricos(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "avisos-historicos:", err)
				os.Exit(1)
			}
			return
		case "healthcheck":
			// Sonda local del proceso usada por Docker/Compose.
			os.Exit(runHealthcheck())
		case "dbcheck":
			// Preflight del despliegue: verifica credenciales y red contra la
			// RDB sin cargar OIDC, migrar ni arrancar un servidor.
			if err := runDBCheck(); err != nil {
				fmt.Fprintf(os.Stderr, "dbcheck: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sytue-calendar: error fatal: %v\n", err)
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

	// Telemetría. Este servicio no la tenía: sin ella no hay forma de saber
	// cuántas peticiones atiende ni con qué latencia, que es lo que tiene que
	// decidir cuándo escalar.
	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Provider de la instrumentación transversal: el middleware HTTP común mide
	// con él sin que el servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// --- PostgreSQL (unica fuente de verdad) ---------------------------------
	pool, err := pg.Connect(ctx, pg.Config{
		DSN:      cfg.DatabaseURL,
		MaxConns: cfg.DBMaxConns,
	})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Migraciones embebidas (golang-migrate sobre el FS embebido en package db).
	if err := pg.MigrateUp(caldb.Migrations, caldb.MigrationsDir, cfg.DatabaseURL); err != nil {
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

	// --- Capas de aplicacion -------------------------------------------------
	st := store.New(pool)
	publisher := &outboxPublisher{pool: pool}
	svc := service.New(cfg, st, publisher)
	// El nombre visible de la organización que convoca, en vez de su identificador.
	if n := personas.NuevosNombresOrgs(cfg.IDBaseURL, cfg.IDNombresOrgsSecret); n != nil {
		svc.SetNombresOrgs(n)
	}

	caldavBackend := caldav.NewBackend(st, cfg.CalDAVPrefix)
	// Avisos al Hub: los recordatorios (el worker de abajo) y las invitaciones, que la
	// web guarda por CalDAV.
	var notifier *hubnotify.Client
	if cfg.HubBaseURL != "" && cfg.HubInternalSecret != "" {
		notifier = hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	}
	svc.SetHubNotifier(notifier)
	// Invitaciones por correo (iMIP) a quien no es de la suite, por el relé de Sytue
	// Mail. Sin buzón configurado no sale ninguna y se dice una vez.
	enviar := service.CorreoDesdeConfig(correo.Config{
		Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, User: cfg.SMTPUser,
		Password: cfg.SMTPPassword, TLSServerName: cfg.SMTPTLSServerName,
	})
	if enviar == nil {
		logger.Info("sin SMTP_ADDR/SMTP_FROM: las invitaciones a direcciones de fuera de la suite no salen por correo")
	}
	svc.SetCorreo(enviar)
	// «Pedir acceso de edición» de un calendario compartido: aviso del Hub y correo al
	// dueño, con el mismo buzón que las invitaciones (calendario@).
	svc.SetAvisador(peticiones.Avisador{Hub: notifier, IDBaseURL: cfg.IDBaseURL, Correo: correo.Config{
		Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, User: cfg.SMTPUser,
		Password: cfg.SMTPPassword, TLSServerName: cfg.SMTPTLSServerName,
	}})
	if enviar != nil && cfg.MailInternalURL != "" {
		go svc.VigilarDominios(ctx, cfg.MailInternalURL, cfg.HubInternalSecret, 10*time.Minute)
	}
	caldavBackend.AlGuardar = svc.InvitarNuevos

	// Cache efímera compartida: guarda la lista de colecciones de cada usuario, que
	// es lo primero que pide CADA sincronización CalDAV. Ahorra la consulta y, sobre
	// todo, el UPSERT de auto-aprovisionamiento que se ejecutaba en cada PROPFIND.
	//
	// DEGRADACIÓN: sin DRAGONFLY_URL se consulta PostgreSQL como antes.
	if cfg.DragonflyURL != "" {
		// El cliente se conserva aunque el Ping falle: la sonda reconecta sola.
		c, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
		if err != nil {
			logger.Warn("Dragonfly no disponible al arrancar; las colecciones CalDAV se listan en PostgreSQL hasta que reconecte", "error", err)
		}
		caldavBackend.UsarCache(c)
		defer func() { _ = c.Close() }()
	}

	router := calendarhttp.NewRouter(calendarhttp.Deps{
		Cfg:      cfg,
		Service:  svc,
		Verifier: verifier,
		CalDAV:   caldavBackend.Handler(),
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
		},
	})

	// Worker de avisos de recordatorios (VALARM): drena la cola de disparos
	// vencidos y publica `calendar.reminder.due` en el bus. Corre en segundo plano;
	// termina al cancelarse el contexto (apagado ordenado).
	if cfg.ReminderScanInterval > 0 {
		w := reminders.NewWorker(st, cfg.ReminderScanInterval, notifier, cfg.PublicURL)
		go func() {
			if err := w.Run(ctx); err != nil {
				logger.Error("worker de recordatorios terminó con error", "error", err)
			}
		}()
	}

	// Worker de suscripciones ICS externas: reclama (cross-réplica, con lease) y
	// refresca los feeds vencidos SSRF-safe, cacheando sus eventos. Termina al
	// cancelarse el contexto.
	if cfg.SubscriptionScanInterval > 0 {
		sw := subscriptions.NewWorker(svc, cfg.SubscriptionScanInterval)
		go func() {
			if err := sw.Run(ctx); err != nil {
				logger.Error("worker de suscripciones ICS terminó con error", "error", err)
			}
		}()
	}

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info(
		"sytue-calendar iniciado",
		"addr", cfg.HTTPAddr,
		"public_url", cfg.PublicURL,
		"caldav_prefix", cfg.CalDAVPrefix,
		"env", string(cfg.Env),
	)
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-calendar detenido limpiamente")
	return nil
}

// outboxPublisher adapta events.PublishTx a la interfaz service.EventPublisher:
// publica cada evento en su propia transaccion sobre el outbox (PostgreSQL).
type outboxPublisher struct {
	pool *pg.Pool
}

func (p *outboxPublisher) Publish(ctx context.Context, ev events.Event) error {
	return events.PublishTx(ctx, p.pool, ev)
}

// runHealthcheck implementa el subcomando "healthcheck": pide /healthz al puerto
// local y devuelve 0 si responde 200, 1 en caso contrario.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}
	url := "http://127.0.0.1:" + port + "/healthz"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
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
