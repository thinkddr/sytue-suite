// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command syphotos arranca el servicio de Sytue Photos: el índice de la galería y
// las recetas de edición. Los originales viven en Sytue Drive y las derivadas en
// el almacén de objetos; este proceso no custodia ningún fichero.
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

	"git.sytue.eu/sytue/one/services/syphotos/internal/trashgc"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	photosdb "git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	photoshttp "git.sytue.eu/sytue/one/services/syphotos/internal/http"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			// Sonda local del proceso usada por Docker/Compose.
			os.Exit(runHealthcheck())
		case "vectores-sacar":
			// Saca a un tar las miniaturas que faltan por vectorizar. La GPU
			// efímera recibe ESO y nada más: ni base de datos ni credenciales.
			if err := runVectoresSacar(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "vectores-sacar: %v\n", err)
				os.Exit(1)
			}
			return
		case "vectores-serverless":
			// VECTORIZACIÓN SIN GPU: describir con la visión de gemma-4
			// (serverless) y embeber la descripción con qwen3-embedding-8b
			// (serverless). Retira deploy/fotos-gpu por completo.
			if err := runVectoresServerless(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "vectores-serverless: %v\n", err)
				os.Exit(1)
			}
			return
		case "vectores-meter":
			// Guarda los vectores que devolvió la GPU.
			if err := runVectoresMeter(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "vectores-meter: %v\n", err)
				os.Exit(1)
			}
			return
		case "dbcheck":
			// Preflight del despliegue: verifica credenciales y red contra la RDB
			// sin cargar OIDC, migrar ni arrancar un servidor.
			if err := runDBCheck(); err != nil {
				fmt.Fprintf(os.Stderr, "dbcheck: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			// UN SUBCOMANDO DESCONOCIDO NO ARRANCA EL SERVIDOR. Antes caía al
			// `run()` de abajo, así que un `syphotos vectores-sacar` contra un
			// binario que no lo conocía levantaba la suite entera dentro de un
			// `kubectl exec` y dejaba el lote esperando a un servidor que nadie
			// iba a parar. Costó un diagnóstico.
			fmt.Fprintf(os.Stderr, "sytue-syphotos: subcomando desconocido %q\n", os.Args[1])
			os.Exit(2)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sytue-syphotos: error fatal: %v\n", err)
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

	// --- PostgreSQL (el ÍNDICE; los bytes no pasan por aquí) -----------------
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pg.MigrateUp(photosdb.Migrations, photosdb.MigrationsDir, cfg.DatabaseURL); err != nil {
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

	// --- Almacén de las DERIVADAS -------------------------------------------
	//
	// Bucket propio, separado del de Drive: aquí solo hay caché regenerable, y
	// su ciclo de vida no es el de un fichero de alguien.
	objetos, err := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket,
		Remote: cfg.S3Remote, LocalTenants: cfg.S3LocalTenants,
	})
	if err != nil {
		return fmt.Errorf("almacén de objetos: %w", err)
	}
	if err := objetos.EnsureBucket(ctx, cfg.S3Region); err != nil {
		return fmt.Errorf("asegurar el bucket de derivadas: %w", err)
	}

	dr := drive.Nuevo(cfg.DriveBaseURL, cfg.DriveTimeout)
	ingesta := service.NuevaIngesta(st, dr, objetos, cfg.BytesDeCabecera, cfg.LadosDeDerivada)
	limpieza := service.NuevaLimpieza(objetos)

	// Auto-purga de la papelera. La condición también va aquí, además de dentro
	// del worker, para no dejar una gorutina viva que solo sabe salirse.
	if cfg.TrashRetention > 0 {
		gc := trashgc.NewWorker(st, limpieza, cfg.TrashGCInterval, cfg.TrashRetention)
		go func() {
			if err := gc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("worker de auto-purga de la papelera terminó con error", "error", err)
			}
		}()
	}

	router := photoshttp.NewRouter(photoshttp.Deps{
		Cfg:      cfg,
		Store:    st,
		Objetos:  objetos,
		Ingesta:  ingesta,
		Drive:    dr,
		Limpieza: limpieza,
		Verifier: verifier,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
			// El almacén TAMBIÉN en la sonda: sin él la galería se ve pero no
			// enseña una sola miniatura, y un pod «listo» que no puede servir
			// imágenes recibe tráfico para no responderlo.
			httpx.CheckerFunc{Label: "objectstore", Fn: objetos.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("sytue-syphotos iniciado",
		"addr", cfg.HTTPAddr, "public_url", cfg.PublicURL,
		"drive", cfg.DriveBaseURL, "env", string(cfg.Env))
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-syphotos detenido limpiamente")
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
		fmt.Fprintf(os.Stderr, "healthcheck: HTTP %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
