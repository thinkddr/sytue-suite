// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cloud-controller es el reconciliador de Sytue Cloud: despliega las
// apps de clientes en Kapsule (namespaces tenant o Serverless Containers),
// gestiona dominios + TLS (cert-manager), previews por PR y ResourceQuota/
// NetworkPolicy, y aprovisiona/inyecta los servicios gestionados. Reconcilia en
// bucle (cada RECONCILE_INTERVAL) y reacciona a los eventos del bus (LISTEN/
// NOTIFY sobre PostgreSQL; sin Redis).
//
// En dev usa una Platform/Provisioner stub (no hay clúster local); en prod se
// cablean las implementaciones con client-go contra Kapsule y los servicios
// gestionados de Scaleway.
//
// Subcomandos:
//
//	cloud-controller              arranca el reconciliador + salud (por defecto)
//	cloud-controller healthcheck  sonda de liveness para Docker
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/controller"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
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

//nolint:gocognit // arranque del proceso: cablea config, stores, watchers y apagado ordenado en secuencia lineal.
func run() error {
	cfg, err := config.Load(config.DefaultBuilderPort)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// El controller escucha salud en un puerto propio (8093 por convención local)
	// para no chocar con el builder cuando comparten host en dev.
	if os.Getenv("PORT") == "" {
		cfg.HTTPAddr = ":8093"
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName + "-controller", Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName + "-controller", Version: cfg.Version})
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
	svc := service.New(cfg, st, sealer, service.NewPoolPublisher(pool), nil) // sin blobs: no sirve logs

	// Platform y Provisioner: stub en dev (sin clúster); en prod, client-go sobre
	// Kapsule. La selección es puramente de configuración (paridad dev/prod, §15).
	var platform controller.Platform
	var provisioner controller.ServiceProvisioner
	if cfg.UseKubeRunner() {
		kp, perr := controller.NewKubePlatform(cfg)
		if perr != nil {
			return fmt.Errorf("plataforma Kapsule: %w", perr)
		}
		prov, verr := controller.NewKubeProvisioner(cfg, controller.NewS3BucketProvisioner(cfg),
			controller.NewServiceCredsResolver(st, sealer))
		if verr != nil {
			return fmt.Errorf("provisioner Kapsule: %w", verr)
		}
		platform, provisioner = kp, prov
		logger.Info("controller en modo Kapsule (client-go)")
	} else {
		platform = controller.NewStubPlatform()
		provisioner = controller.NewStubProvisioner(cfg)
		logger.Info("controller en modo stub (dev, sin clúster)")
	}
	ctrl := controller.New(cfg, svc, platform, provisioner)

	// Suscriptor de eventos: despierta una reconciliación inmediata al recibir un
	// NOTIFY del canal "cloud" (build encolada, deploy solicitado, rollback, etc.).
	// Wake coalesce las señales (canal buffer 1 + debounce), así una ráfaga no
	// dispara una tormenta de reconciles; el poll periódico sigue de red de seguridad.
	sub := events.NewSubscriber(pool, func(nctx context.Context, ch events.Channel, n events.Notification) {
		log.From(nctx).Debug("evento recibido; reconciliación reactiva", "type", n.Type)
		manejarEvento(nctx, pool, st, platform, provisioner, n)
		ctrl.Wake()
	}, service.EventChannel)

	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 5 * time.Second})
	httpx.MountHealth(r, cfg.Version, 3*time.Second, httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz})
	srv := httpx.NewServer(httpx.ServerConfig{Addr: cfg.HTTPAddr, Handler: r, ShutdownTimeout: cfg.ShutdownTimeout})

	logger.Info("Sytue Cloud controller arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "interval", cfg.ReconcileInterval.String())

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return ctrl.Run(gctx) })
	g.Go(func() error { return sub.Run(gctx) })
	g.Go(func() error { return srv.Run(gctx) })
	return g.Wait()
}

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8093"
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

// manejarEvento ejecuta el efecto lateral que pide un evento del bus antes de
// despertar al reconciliador.
//
// Vive fuera de run() porque cada evento nuevo le añadía una rama y la función de
// arranque acabó midiendo más complejidad que ningún otro sitio del servicio: lo
// que tiene que leerse ahí es el orden en que se levantan las piezas, no el
// despiece de cada payload.
//
// Un fallo aquí se REGISTRA y no se propaga: el suscriptor es best-effort —el
// barrido periódico del reconciliador vuelve a pasar—, y devolver error solo
// serviría para tirar la suscripción entera por un evento suelto.
func manejarEvento(
	ctx context.Context,
	pool *pg.Pool,
	st *store.Store,
	platform controller.Platform,
	provisioner controller.ServiceProvisioner,
	n events.Notification,
) {
	switch n.Type {
	case service.EventDomainRemoved:
		if ev, ok, err := events.GetByID(ctx, pool, n.ID); ok && err == nil {
			var payload struct {
				Hostname string `json:"hostname"`
			}
			_ = json.Unmarshal(ev.Payload, &payload)
			if payload.Hostname != "" {
				if err := platform.RemoveDomainTLS(ctx, ev.Tenant, payload.Hostname); err != nil {
					log.From(ctx).Warn("eliminar TLS de dominio", "error", err, "hostname", payload.Hostname)
				}
			}
		}
	case service.EventServiceBackup:
		// Copia a demanda: mismo camino que el restore, con el servicio resuelto
		// bajo el tenant del evento (RLS).
		if ev, ok, err := events.GetByID(ctx, pool, n.ID); ok && err == nil {
			var payload struct {
				ServiceID uuid.UUID `json:"service_id"`
			}
			_ = json.Unmarshal(ev.Payload, &payload)
			if err := controller.DispatchServiceBackup(ctx, st, provisioner, ev.Tenant, payload.ServiceID); err != nil {
				log.From(ctx).Warn("copia de servicio", "error", err, "service_id", payload.ServiceID)
			}
		}
	case service.EventServiceRun:
		// Ejecutar una tarea a mano: clona la plantilla del cron (o la spec del job
		// puntual) en un Job nuevo, sin tocar lo que el usuario tenga configurado.
		if ev, ok, err := events.GetByID(ctx, pool, n.ID); ok && err == nil {
			var payload struct {
				ServiceID uuid.UUID `json:"service_id"`
			}
			_ = json.Unmarshal(ev.Payload, &payload)
			if err := controller.DispatchServiceRun(ctx, st, provisioner, ev.Tenant, payload.ServiceID); err != nil {
				log.From(ctx).Warn("ejecución manual de tarea", "error", err, "service_id", payload.ServiceID)
			}
		}
	case service.EventServiceRestore:
		// Restore self-service (Wave 15): el servicio emite el evento con el id del
		// servicio + nombre de backup ya validado; aquí se resuelve bajo el tenant
		// del evento y se dispara el Job de restore en su namespace.
		if ev, ok, err := events.GetByID(ctx, pool, n.ID); ok && err == nil {
			var payload struct {
				ServiceID uuid.UUID `json:"service_id"`
				Backup    string    `json:"backup"`
			}
			_ = json.Unmarshal(ev.Payload, &payload)
			if err := controller.DispatchServiceRestore(ctx, st, provisioner, ev.Tenant, payload.ServiceID, payload.Backup); err != nil {
				log.From(ctx).Warn("restore de servicio", "error", err, "service_id", payload.ServiceID)
			}
		}
	}
}
