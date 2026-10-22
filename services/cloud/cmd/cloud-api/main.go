// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cloud-api es el binario de la API del control plane de Sytue Cloud
// (PaaS), puerto 8090: orgs/tenants, proyectos, builds, deployments, dominios,
// env/secretos, servicios gestionados, uso/facturación y webhooks git.
//
// Subcomandos:
//
//	cloud-api              arranca el servidor HTTP (por defecto)
//	cloud-api migrate      aplica las migraciones de base de datos y sale
//	cloud-api healthcheck  sonda de liveness para el healthcheck de Docker
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"

	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/forge"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
	cloudhttp "git.sytue.eu/sytue/one/services/cloud/internal/http"
	"git.sytue.eu/sytue/one/services/cloud/internal/identity"
	"git.sytue.eu/sytue/one/services/cloud/internal/runtime"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
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
		case "plan":
			if err := runPlan(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "plan:", err)
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

// conectarCache engancha la cache efímera al servicio si está configurada, y
// devuelve cómo cerrarla (nil si no hay nada que cerrar).
//
// DEGRADACIÓN: sin DRAGONFLY_URL, o si no se puede conectar, los tokens se
// resuelven contra PostgreSQL igual que antes. Nunca se depende de la cache para
// arrancar ni para decidir quién entra.
func conectarCache(ctx context.Context, cfg config.Config, svc *service.Service, logger *slog.Logger) func() {
	if cfg.DragonflyURL == "" {
		return nil
	}
	c, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
	if err != nil {
		logger.Warn("Dragonfly no disponible al arrancar; los tokens de API se resuelven en PostgreSQL hasta que reconecte", "error", err)
	}
	// La clave SELLA lo que se guarda, y sin ella no se guarda nada: cachear la
	// resolución de un token sin firmar es dejar que quien escriba en Dragonfly
	// dicte quién entra y como qué tenant. Ir siempre a PostgreSQL es más lento;
	// aceptar un valor falsificable no es una alternativa.
	if cfg.CacheSigningKey == "" {
		logger.Warn("CACHE_SIGNING_KEY sin definir: los tokens de API NO se cachean y se resuelven contra PostgreSQL")
	} else {
		c.UsarFirma(cache.ClaveDeFirma([]byte(cfg.CacheSigningKey), "cloud"))
	}
	svc.UsarCache(c)
	return func() { _ = c.Close() }
}

// arrancarWorkers lanza los procesos de fondo del control-plane. Todos corren en
// TODAS las réplicas a propósito: la exclusión mutua la da la base de datos
// (SKIP LOCKED al reclamar, o la clave primaria al reservar un aviso), así que no
// hace falta elegir líder y la caída de una réplica no deja el trabajo sin hacer.
//
// Un fallo se REGISTRA y no tumba el proceso: que el emisor de facturas muera no
// puede dejar la API sin servir.
func arrancarWorkers(ctx context.Context, svc *service.Service, logger *slog.Logger) {
	// Worker de notificaciones de deploy: Dispatcher del outbox (canal "cloud")
	// que envía un webhook saliente al pasar un deployment a ready/error.
	// Idempotente por (deployment_id, status). Corre en cada réplica (SKIP LOCKED
	// reparte el trabajo entre ellas sin duplicar).
	go func() {
		if err := svc.RunDeployNotifier(ctx); err != nil {
			logger.Error("worker de notificaciones de deploy terminó con error", "error", err)
		}
	}()

	// Prober de uptime: cada tick RECLAMA los checks vencidos (FOR UPDATE SKIP
	// LOCKED reparte entre réplicas), los sondea por HTTP con guard anti-SSRF y,
	// en la transición up↔down, emite un evento al bus (que enruta a alert_policies).
	go func() {
		if err := svc.RunUptimeProber(ctx); err != nil {
			logger.Error("prober de uptime terminó con error", "error", err)
		}
	}()

	// Recolector de logs de runtime: cada tick reclama deployments vivos (FOR
	// UPDATE SKIP LOCKED reparte entre réplicas y los arrienda), lee del pod lo
	// escrito desde el último corte y lo guarda para que sea BUSCABLE. Sin esto
	// los logs solo existen mientras alguien mira el stream en vivo.
	go func() {
		if err := svc.RunLogCollector(ctx); err != nil {
			logger.Error("recolector de logs terminó con error", "error", err)
		}
	}()

	// Evaluador de alertas de coste: compara el gasto del mes con el límite que el
	// cliente configuró y avisa al cruzar un umbral. Corre en TODAS las réplicas a
	// propósito — la exclusión la da la clave primaria de billing_spend_alerts al
	// reservar el envío, así que no hace falta elegir líder y la caída de una
	// réplica no deja a nadie sin aviso.
	go func() {
		if err := svc.RunSpendAlerter(ctx); err != nil {
			logger.Error("evaluador de alertas de coste terminó con error", "error", err)
		}
	}()

	// Evaluador proactivo de tamaño: convierte la recomendación de rightsizing —que
	// solo se veía entrando al proyecto— en un aviso. Sin esto, el ahorro dependía
	// de que alguien abriera el panel, y el «se te está quedando corto» llegaba
	// después de la caída. Misma exclusión sin líder: la clave primaria de
	// sizing_alerts.
	go func() {
		if err := svc.RunSizingAlerter(ctx); err != nil {
			logger.Error("evaluador proactivo de tamaño terminó con error", "error", err)
		}
	}()

	// Emisor mensual de facturas: al cerrar el mes natural emite la factura de
	// consumo a quien le falte. Sin esto el motor estaba entregado y no lo
	// disparaba NADIE —no existía el CronJob que su propio comentario mencionaba,
	// y no podía existir: POST /billing/invoices:generate factura un solo tenant y
	// exige rol admin/owner—, así que la pestaña de facturación prometía una
	// factura mensual que no llegaba nunca. Corre en todas las réplicas: la
	// exclusión la da el UNIQUE(tenant, periodo) de invoices.
	go func() {
		if err := svc.RunInvoicer(ctx); err != nil {
			logger.Error("emisor de facturas terminó con error", "error", err)
		}
	}()
}

func run() error {
	cfg, err := config.Load(config.DefaultAPIPort)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Solo la API sirve el endpoint de webhooks, así que solo ella exige el HMAC.
	if err := cfg.RequireWebhookSecret(); err != nil {
		return err
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// Migraciones al arrancar (paridad dev/prod; idempotente).
	if err := pg.MigrateUp(clouddb.Migrations, clouddb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	sealer, err := crypto.NewSealer(cfg.SecretEncKey)
	if err != nil {
		return fmt.Errorf("sellador de secretos: %w", err)
	}

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:   cfg.OIDCIssuer,
		JWKSURL:  cfg.OIDCJWKSURL,
		Audience: cfg.OIDCAudience,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// Object Storage: sirve los logs de build (los sube el builder a build-logs/
	// <id>.log). Best-effort: si no hay S3, GET /builds/{id}/logs degrada a 404.
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket,
	})
	if err != nil {
		logger.Warn("Object Storage no disponible; logs de build deshabilitados", "error", err)
		blobs = nil
	}

	st := store.New(pool)
	svc := service.New(cfg, st, sealer, service.NewPoolPublisher(pool), blobs)

	// Cache efímera compartida: guarda la resolución de los tokens de API, que
	// ocurre en CADA petición autenticada por token. Solo se cablea aquí —el
	// builder y el controller no atienden peticiones de clientes—.
	//
	if cerrarCache := conectarCache(ctx, cfg, svc, logger); cerrarCache != nil {
		defer cerrarCache()
	}
	svc.SetIdentity(identity.NewClient(cfg.IdentityAPIURL))

	// GitHub App (opcional): habilita el comentario de preview en el PR. Clave
	// inválida ⇒ integración deshabilitada (no-op), no aborta el arranque.
	if gh, gerr := github.NewClient(cfg.GitHubAppID, cfg.GitHubAppPrivateKey); gerr != nil {
		logger.Warn("GitHub App deshabilitada: configuración inválida", "error", gerr)
	} else if gh != nil {
		svc.SetGitHub(gh)
		logger.Info("GitHub App habilitada", "app_id", cfg.GitHubAppID)
	}

	// Sytue Git (opcional): habilita crear el repositorio del usuario con el
	// esqueleto de su stack. Sin credenciales OAuth, esa vía del alta no se ofrece
	// —y el panel lo DICE, porque un botón que desaparece sin explicación es
	// indistinguible de una función que nunca existió—.
	if f := forge.New(cfg.ForgeURL, cfg.ForgeClientID, cfg.ForgeClientSecret,
		strings.TrimSuffix(cfg.PublicURL, "/")+"/api/forge/callback"); f != nil {
		svc.SetForge(f)
		logger.Info("Alta con repositorio nuevo habilitada", "forja", cfg.ForgeURL)
	} else {
		logger.Info("Alta con repositorio nuevo deshabilitada: falta FORGE_URL/FORGE_CLIENT_ID/FORGE_CLIENT_SECRET")
	}

	// El derecho de acceso y el cobro recurrente viven en Sytue ID + Mollie.
	// Cloud conserva aquí solo su contabilidad local de consumo y límites.
	logger.Info("Facturación Cloud local; suscripción de suite verificada por Sytue ID")

	logStreamer, _ := runtime.NewLogStreamer(cfg)
	svc.SetLogStreamer(logStreamer)
	execSession, _ := runtime.NewExecSession(cfg)

	// Cron de purga/rollup de retención (builds, webhooks, outbox, previews,
	// uso). Corre en cada réplica: es idempotente y los DELETE se serializan
	// por bloqueo de fila, así que la concurrencia entre réplicas es inocua.
	go purgeLoop(ctx, st, logger)

	// Re-sellado de secretos con la KEK primaria: lo que convierte «hay una clave
	// nueva en SECRET_ENC_KEY» en «ya no queda nada cifrado con la vieja», sin
	// parada y sin tocar a los clientes.
	go resealLoop(ctx, st, sealer, logger)

	// Los seis trabajadores de fondo, cada uno con su porqué, en su función:
	// aquí solo se arrancan. `run` estaba en el techo de complejidad y cada worker
	// nuevo la subía, cuando lo que hace es siempre lo mismo.
	arrancarWorkers(ctx, svc, logger)

	router := cloudhttp.NewRouter(cloudhttp.Deps{
		Cfg:         cfg,
		Service:     svc,
		Verifier:    verifier,
		ExecSession: execSession,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("Sytue Cloud API arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL)
	return srv.Run(ctx)
}

// purgeLoop ejecuta periódicamente la purga/rollup de retención sobre una
// transacción de mantenimiento (cross-tenant acotado, sin BYPASSRLS), como el
// cron de limpieza de Sytue ID. Loguea cuántas filas purga (métrica del cron).
func purgeLoop(ctx context.Context, st *store.Store, logger *slog.Logger) {
	// ponytail: intervalo fijo de 1h (patrón hub); variable de entorno solo si
	// operación lo pide.
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := st.PurgeExpired(ctx, time.Now(), store.KeepBuildsPerProject)
			if err != nil {
				logger.Warn("purga/rollup de retención falló", "error", err)
				continue
			}
			if stats.Total() > 0 || stats.UsageRollups > 0 {
				logger.Info("purga/rollup de retención completada",
					"builds", stats.Builds,
					"webhooks", stats.Webhooks,
					"outbox", stats.OutboxEvents,
					"previews", stats.Previews,
					"usage_detail", stats.UsageDetail,
					"usage_rollups", stats.UsageRollups,
					"alert_deliveries", stats.AlertDeliveries)
			}
		}
	}
}

// resealLoop pasa a la KEK PRIMARIA los secretos que sigan cifrados con una versión
// anterior del anillo (SECRET_ENC_KEY admite "1:…,2:…"). Rotar la clave es entonces:
// desplegar con la nueva AÑADIDA al anillo —las lecturas no se enteran, la vieja
// sigue descifrando—, esperar a que este bucle deje el censo con una sola versión, y
// desplegar quitando la vieja. Sin este bucle, «rotar» significaría reescribir a mano
// cada secreto o quedarse con la clave original para siempre.
//
// Arranca de inmediato y repite cada hora: con la clave nueva desplegada, la rotación
// empieza sola. Corre en TODAS las réplicas a propósito: el lote se reclama con FOR
// UPDATE SKIP LOCKED, así que se reparten el trabajo sin duplicarlo ni bloquearse.
func resealLoop(ctx context.Context, st *store.Store, sealer *crypto.Sealer, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		stats, err := st.RotateSecretKeys(ctx, sealer.Version(), sealer.Reseal)
		switch {
		case err != nil:
			logger.Warn("re-sellado de secretos falló", "error", err, "resellados", stats.Total())
		case stats.Total() > 0:
			// El censo va DESPUÉS de la pasada: es cuando el recuento de lo que
			// queda en claves viejas es de fiar, y es el dato que decide si ya se
			// puede retirar la anterior de SECRET_ENC_KEY.
			logger.Info("secretos re-sellados con la KEK primaria",
				"version", sealer.Version(),
				"env_vars", stats.EnvVars, "servicios", stats.Services, "proyectos", stats.Projects,
				"pendientes", pendingOldVersions(ctx, st, sealer.Version(), logger))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pendingOldVersions cuenta los secretos que siguen cifrados con una KEK anterior.
// Devuelve -1 si el censo falla: mejor un valor imposible en el log que un 0 que
// invitaría a retirar una clave que aún hace falta.
func pendingOldVersions(ctx context.Context, st *store.Store, primary int, logger *slog.Logger) int64 {
	census, err := st.SecretKeyVersions(ctx)
	if err != nil {
		logger.Warn("censo de versiones de clave falló", "error", err)
		return -1
	}
	var pending int64
	for version, n := range census {
		if version != primary {
			pending += n
		}
	}
	return pending
}

func runMigrate() error {
	cfg, err := config.Load(config.DefaultAPIPort)
	if err != nil {
		return err
	}
	return pg.MigrateUp(clouddb.Migrations, clouddb.MigrationsDir, cfg.DatabaseURL)
}

// runPlan es la vía del operador para cambiar el plan de una organización:
// `cloud-api plan <tenant> <free|pro|team>`. La API pública ya no lo permite
// (el plan cambia tras un pago confirmado o por el operador) y queda auditado.
func runPlan(args []string) error {
	if len(args) != 2 {
		return errors.New("uso: cloud-api plan <tenant> <plan>")
	}
	cfg, err := config.Load(config.DefaultAPIPort)
	if err != nil {
		return err
	}
	t, err := tenancy.New(args[0])
	if err != nil {
		return err
	}
	ctx := tenancy.Into(context.Background(), t)
	ctx = authn.Into(ctx, authn.Claims{Subject: "operador", Tenant: args[0]})
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: 2})
	if err != nil {
		return err
	}
	defer pool.Close()
	svc := service.New(cfg, store.New(pool), nil, nil, nil)
	org, err := svc.SetPlanByOperator(ctx, domain.Plan(args[1]))
	if err != nil {
		return err
	}
	fmt.Printf("%s: plan %s\n", t.Slug, org.Plan)
	return nil
}

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultAPIPort
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
