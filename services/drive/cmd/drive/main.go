// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command drive es el binario de Sytue Drive (servidor de archivos WebDAV +
// API REST propia; metadatos en PostgreSQL, blobs en Object Storage S3).
//
// Subcomandos:
//
//	drive              arranca el servidor HTTP/WebDAV (por defecto)
//	drive migrate      aplica las migraciones de base de datos y sale
//	drive healthcheck  sonda de liveness para el healthcheck de Docker
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
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/docssync"
	drivehttp "git.sytue.eu/sytue/one/services/drive/internal/http"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/trashgc"
	"git.sytue.eu/sytue/one/services/drive/internal/webdav"
)

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
	if err := pg.MigrateUp(drivedb.Migrations, drivedb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// PostgreSQL (único datastore).
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Object Storage S3 (blobs).
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint:     cfg.S3Endpoint,
		Region:       cfg.S3Region,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		UseSSL:       cfg.S3UseSSL,
		Bucket:       cfg.S3Bucket,
		Remote:       cfg.S3Remote,
		LocalTenants: cfg.S3LocalTenants,
	})
	if err != nil {
		return fmt.Errorf("cliente S3: %w", err)
	}
	if !cfg.Env.IsProd() {
		// En dev creamos el bucket si no existe (en prod lo provisiona Terraform).
		if err := blobs.EnsureBucket(ctx, cfg.S3Region); err != nil {
			logger.Warn("no se pudo asegurar el bucket S3", "error", err)
		}
	}

	// Verificador de JWT de Sytue ID (JWKS).
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// Capa de servicio + WebDAV. La verja de los adjuntos de canal de Sytue Rooms
	// pregunta el roster a services/rooms (secreto compartido): sin configurar, esa
	// superficie queda cerrada y el resto de Drive funciona igual.
	st := service.NewStore(pool)
	// Interfaz declarada aparte a propósito: un *ChatClient nil metido en una
	// interfaz NO es nil (y el fail-closed del servicio compara contra nil).
	var chatGate service.ChannelAuthorizer
	if c := service.NewChatClient(cfg.RoomsURL, cfg.RoomsInternalSecret); c != nil {
		chatGate = c
	} else {
		logger.Info("adjuntos de canal de Rooms deshabilitados (sin CHAT_INTERNAL_SECRET)")
	}
	svc := service.New(cfg, st, blobs, service.NewPoolPublisher(pool), chatGate)
	davHandler := webdav.Handler("/dav", svc)

	router := drivehttp.NewRouter(drivehttp.Deps{
		Cfg:      cfg,
		Service:  svc,
		Verifier: verifier,
		WebDAV:   davHandler,
		// Basic en /dav: el gestor de ficheros y «Archivos» de Android (authn.AppPasswords).
		AppPasswords: authn.NewAppPasswords(cfg.IDBaseURL, cfg.IDInternalSecret),
		// Compliance (legal hold + eDiscovery): el store real y el objectstore (para
		// el export). Acceso admin gateado por rol + auditado. Ver handlers_compliance.go.
		Compliance: st,
		// Bloqueos WOPI: el mismo store. Sin esto el host WOPI no se monta.
		Locks:    st,
		Editors:  st,
		External: st,
		// Cuota compartida con Sytue Cast (mig 0026): el mismo store guarda lo que
		// Cast declara por la puerta interna.
		ExternalUsage: st,
		Blobs:         blobs,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
			httpx.CheckerFunc{Label: "objectstore", Fn: blobs.Healthz},
		},
	})

	// Worker de auto-purga de papelera: cada TrashGCInterval borra definitivamente
	// lo caducado (retención TrashRetention), cross-tenant y cross-réplica seguro.
	// Corre en segundo plano; termina al cancelarse el contexto (apagado ordenado).
	if cfg.TrashGCInterval > 0 {
		gcw := trashgc.NewWorker(svc, cfg.TrashGCInterval)
		go func() {
			if err := gcw.Run(ctx); err != nil {
				logger.Error("worker de auto-purga de papelera terminó con error", "error", err)
			}
		}()
	}

	// La papelera de Drive llega a Docs (mig 0027): tirar, restaurar o purgar el
	// fichero de un documento hace lo mismo con el documento.
	if dw := docssync.Nuevo(st, cfg.DocsURL, cfg.DocsInternalSecret, 0); dw != nil {
		go func() {
			if err := dw.Run(ctx); err != nil {
				logger.Error("worker de la papelera hacia Docs terminó con error", "error", err)
			}
		}()
	}

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
		// Subidas/descargas grandes y WebDAV: sin límite de lectura ni de escritura.
		// SinTimeout y no 0, que es «el valor por defecto» (15 s / 30 s).
		WriteTimeout: httpx.SinTimeout,
		ReadTimeout:  httpx.SinTimeout,
	})

	logger.Info("Sytue Drive arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL)
	return srv.Run(ctx)
}

// runMigrate aplica las migraciones y sale (uso operativo / Job de despliegue).
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return pg.MigrateUp(drivedb.Migrations, drivedb.MigrationsDir, cfg.DatabaseURL)
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/drive","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
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
